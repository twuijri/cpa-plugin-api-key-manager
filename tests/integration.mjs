// Exercises the real .so through a locally built CPA, with a local mock upstream.
// No user credentials, remote services, or production configuration are touched.
import assert from 'node:assert/strict';
import {mkdtemp,writeFile,mkdir} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {resolve,join} from 'node:path';
import {spawn} from 'node:child_process';
import http from 'node:http';
const binary=process.env.CPA_TEST_BINARY;
if(!binary)throw Error('Set CPA_TEST_BINARY to the CPA v7.2.146 binary');
const work=await mkdtemp(join(tmpdir(),'miftah-integration-'));
let calls=[];let gateway;let gatewayBase;
const upstream=http.createServer(async(req,res)=>{let body='';for await(const c of req)body+=c;let v=JSON.parse(body||'{}');calls.push(v.model);assert(!String(req.headers.authorization).includes('mf_'));
 if((v.model==='primary'||v.model==='direct-primary'||v.model==='key-primary')){res.writeHead(503,{'Content-Type':'application/json'});res.end('{"error":{"message":"test unavailable"}}');return}
 if(v.stream){res.writeHead(200,{'Content-Type':'text/event-stream'});res.write('data: '+JSON.stringify({id:'test',object:'chat.completion.chunk',model:v.model,choices:[{index:0,delta:{content:'hello'},finish_reason:null}]})+'\n\n');res.write('data: '+JSON.stringify({id:'test',object:'chat.completion.chunk',model:v.model,choices:[],usage:{prompt_tokens:3,completion_tokens:4,total_tokens:7}})+'\n\n');res.end('data: [DONE]\n\n');return}
 res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify({id:'test',object:'chat.completion',model:v.model,choices:[{index:0,message:{role:'assistant',content:'hello'},finish_reason:'stop'}],usage:{prompt_tokens:3,completion_tokens:4,total_tokens:7}}));});
await new Promise(r=>upstream.listen(0,'127.0.0.1',r));
const probe=http.createServer();await new Promise(r=>probe.listen(0,'127.0.0.1',r));const port=probe.address().port;await new Promise(r=>probe.close(r));
const config=`host: "127.0.0.1"
port: ${port}
auth-dir: "${work}/auth"
remote-management:
  allow-remote: false
  secret-key: "integration-admin-not-production"
  disable-control-panel: true
api-keys: ["native-test-master"]
plugins:
  enabled: true
  dir: "${resolve('dist')}"
  configs:
    miftah:
      enabled: true
request-retry: 0
max-retry-interval: 0
openai-compatibility:
  - name: mock
    base-url: "http://127.0.0.1:${upstream.address().port}/v1"
    api-key-entries:
      - api-key: "mock-upstream-secret"
    models:
      - name: primary
      - name: backup
      - name: key-primary
      - name: backup-two
      - name: direct-primary
`;
await mkdir(join(work,'auth'));await writeFile(join(work,'config.yaml'),config);
const child=spawn(binary,['-config',join(work,'config.yaml')],{cwd:work,env:{...process.env,MIFTAH_STATE_PATH:join(work,'state.json')}});let logs='';child.stdout.on('data',d=>logs+=d);child.stderr.on('data',d=>logs+=d);
const base=`http://127.0.0.1:${port}`;
async function admin(path,method='GET',body){const res=await fetch(base+'/v0/management/miftah/'+path,{method,headers:{Authorization:'Bearer integration-admin-not-production','Content-Type':'application/json'},body:body?JSON.stringify(body):undefined});const raw=await res.text();assert.equal(res.status,method==='POST'&&path==='keys'?201:200,raw);return JSON.parse(raw)}
async function completion(key,model='work',stream=false){return fetch(base+'/v1/chat/completions',{method:'POST',headers:{Authorization:'Bearer '+key,'Content-Type':'application/json'},body:JSON.stringify({model,messages:[{role:'user',content:'hi'}],max_tokens:10,stream}),signal:AbortSignal.timeout(15000)})}
try{
 let ready=false;for(let i=0;i<80;i++){try{const r=await fetch(base+'/v0/management/miftah/state',{headers:{Authorization:'Bearer integration-admin-not-production'}});if(r.ok){ready=true;break}}catch{}await new Promise(r=>setTimeout(r,150))};assert(ready,'CPA did not load plugin:\n'+logs);
 assert.equal((await fetch(base+'/v0/management/miftah/state')).status,401,'management route exposed');
 const metadataHeaders={Authorization:'Bearer integration-admin-not-production'};
 const files=await (await fetch(base+'/v0/management/auth-files',{headers:metadataHeaders})).json();
 const catalog=new Set();for(const f of files.files||[]){const v=await (await fetch(base+'/v0/management/auth-files/models?name='+encodeURIComponent(f.name),{headers:metadataHeaders})).json();for(const m of v.models||[])catalog.add(m.id)}
 assert(Array.isArray(files.files),'real host metadata shape changed');console.log('PASS management model metadata endpoint (API-key-only fixture may have no auth files)');
 let s=await admin('state');await admin('routes','PUT',{revision:s.revision,route:{alias:'work',targets:['primary','backup'],retry_statuses:[503],retry_unknown:true,input_price:1000000,output_price:1000000,max_output:1000}});
 const created=await admin('keys','POST',{name:'Integration key',owner:'test',models:['work'],limits:{total:1000000,rpm:30,concurrent:2}});

 const modelList=async key=>fetch(base+'/v0/resource/plugins/miftah/models',{headers:{Authorization:'Bearer '+key}});
 let list=await modelList(created.secret);assert.equal(list.status,200);assert.deepEqual((await list.json()).data.map(m=>m.id),['work']);assert.equal(list.headers.get('cache-control'),'no-store');
 for(const key of ['','mf_invalid','native-test-master'])assert.equal((await modelList(key)).status,401);
 if(process.env.CADDY_TEST_BINARY){
  const listener=http.createServer();await new Promise(r=>listener.listen(0,'127.0.0.1',r));const edgePort=listener.address().port;await new Promise(r=>listener.close(r));
  const caddyfile=join(work,'Caddyfile');
  await writeFile(caddyfile,`{
 admin off
 auto_https off
}
http://127.0.0.1:${edgePort} {
 @miftah_models {
  method GET
  path /v1/models
  header_regexp Authorization ^(?i:Bearer)[[:space:]]+mf_
 }
 rewrite @miftah_models /v0/resource/plugins/miftah/models
 reverse_proxy 127.0.0.1:${port}
}
`);
  gateway=spawn(process.env.CADDY_TEST_BINARY,['run','--config',caddyfile,'--adapter','caddyfile'],{stdio:'ignore'});
  gatewayBase='http://127.0.0.1:'+edgePort;
  let edgeReady=false;for(let i=0;i<60;i++){try{await fetch(gatewayBase+'/healthz');edgeReady=true;break}catch{}await new Promise(r=>setTimeout(r,100))}assert(edgeReady,'Caddy not ready');
  const edgeList=key=>fetch(gatewayBase+'/v1/models',{headers:{Authorization:'Bearer '+key}});
  let lr=await edgeList(created.secret);assert.equal(lr.status,200);assert.deepEqual((await lr.json()).data.map(m=>m.id),['work']);
  lr=await edgeList('native-test-master');assert.equal(lr.status,200);assert((await lr.json()).data.some(m=>m.id==='backup'));
  assert.equal((await edgeList('mf_invalid')).status,401);
  assert.equal((await fetch(gatewayBase+'/v1/models')).status,401);
  console.log('PASS real Caddy rewrite: virtual allowlist only, native listing preserved, invalid/missing keys denied');
 }
 console.log('PASS authenticated plugin discovery resource');
 let res=await completion('native-test-master','backup');assert.equal(res.status,200,await res.text());console.log('PASS native key continues working');
 res=await completion(created.secret);let text=await res.text();assert.equal(res.status,200,text);assert(text.includes('hello'));assert(calls.includes('primary')&&calls.includes('backup'));console.log('PASS virtual key, real host callback and fallback');
 res=await completion(created.secret,'work',true);text=await res.text();assert.equal(res.status,200,text);assert(text.includes('hello')&&text.includes('[DONE]'));console.log('PASS streaming callback and completion');
 const before=calls.length;res=await completion(created.secret,'backup');assert(res.status>=400);await res.text();assert.equal(calls.length,before);console.log('PASS model allowlist denies direct upstream model');
 s=await admin('state');let k=s.keys[0];k.enabled=false;await admin('keys','PUT',{key:k,revision:s.revision});res=await completion(created.secret);assert(res.status>=400);await res.text();assert.equal(calls.length,before);console.log('PASS disabled virtual key denied');assert.equal((await modelList(created.secret)).status,401);if(gatewayBase)assert.equal((await fetch(gatewayBase+'/v1/models',{headers:{Authorization:'Bearer '+created.secret}})).status,401);
 res=await completion('native-test-master','backup');assert.equal(res.status,200,await res.text());console.log('PASS native key unaffected after virtual disable');
 const direct=await admin('keys','POST',{name:'Direct model key',models:['direct-primary'],direct_policies:[{kind:'direct',alias:'direct-primary',targets:['direct-primary','backup'],retry_statuses:[503],retry_unknown:true,input_price:1000000,output_price:1000000,max_output:1000}],limits:{total:1000000}});
 const start=calls.length;res=await completion(direct.secret,'direct-primary');text=await res.text();assert.equal(res.status,200,text);assert.deepEqual(calls.slice(start),['direct-primary','backup']);console.log('PASS real model name with ordered fallback, no alias');
 const deniedStart=calls.length;res=await completion(direct.secret,'backup');await res.text();assert(res.status>=400);assert.equal(calls.length,deniedStart);console.log('PASS fallback does not grant direct access to backup');
 res=await completion(direct.secret,'direct-primary',true);text=await res.text();assert.equal(res.status,200,text);assert(text.includes('[DONE]'));console.log('PASS direct model streaming fallback');
 const single=await admin('keys','POST',{name:'No fallback',models:['backup'],direct_policies:[{kind:'direct',alias:'backup',targets:['backup'],retry_statuses:[],input_price:1000000,output_price:1000000,max_output:1000}],limits:{total:1000000}});
 const one=calls.length;res=await completion(single.secret,'backup');assert.equal(res.status,200,await res.text());assert.deepEqual(calls.slice(one),['backup']);console.log('PASS direct model without fallback');
 s=await admin('state');const d=s.keys.find(k=>k.id===direct.key.id);d.enabled=false;await admin('keys','PUT',{key:d,revision:s.revision});res=await completion(direct.secret,'direct-primary');await res.text();assert(res.status>=400);res=await completion('native-test-master','backup');assert.equal(res.status,200,await res.text());console.log('PASS direct key disable preserves native access');
 const policy=alias=>({kind:'direct',alias,targets:[alias],retry_statuses:[],input_price:1000000,output_price:1000000,max_output:1000});
 const ka=await admin('keys','POST',{name:'Per-key A',models:['key-primary'],fallbacks:[{primary:'key-primary',fallbacks:['backup'],retry_statuses:[503],retry_unknown:true}],direct_policies:[policy('key-primary')]});
 const kb=await admin('keys','POST',{name:'Per-key B',models:['key-primary'],fallbacks:[{primary:'key-primary',fallbacks:['backup-two'],retry_statuses:[503],retry_unknown:true}],direct_policies:[policy('backup-two')]});
 const aStart=calls.length;res=await completion(ka.secret,'key-primary');text=await res.text();assert.equal(res.status,200,text);assert(calls.slice(aStart).includes('backup'));assert(!calls.slice(aStart).includes('backup-two'));
 const bStart=calls.length;res=await completion(kb.secret,'key-primary',true);text=await res.text();assert.equal(res.status,200,text);assert(text.includes('[DONE]'));assert(calls.slice(bStart).includes('backup-two'));assert(!calls.slice(bStart).includes('backup'));
 const deny=calls.length;res=await completion(kb.secret,'backup-two');await res.text();assert(res.status>=400);assert.equal(calls.length,deny);
 s=await admin('state');assert.deepEqual(s.routes.find(r=>r.alias==='key-primary').targets,['key-primary']);
 console.log('PASS independent per-key fallback, streaming, backup allowlist isolation and unchanged shared policy');
 console.log('Integration passed. State retained for inspection at '+work);
}catch(e){console.error(logs);throw e}finally{gateway?.kill('SIGTERM');child.kill('SIGTERM');upstream.close();}
