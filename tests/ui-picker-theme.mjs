import {chromium} from 'playwright';
import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
const browser=await chromium.launch({executablePath:'/usr/bin/google-chrome',headless:true});
const page=await browser.newPage({viewport:{width:1440,height:1000},colorScheme:'dark'});
const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.route('**/v0/management/auth-files',r=>r.fulfill({json:{files:[{name:'mock'}]}}));
await page.route('**/v0/management/auth-files/models?*',r=>r.fulfill({json:{models:['picker/a','picker/b','picker/c'].map(id=>({id}))}}));
try{
 await page.goto('http://127.0.0.1:8741');
 assert.equal(await page.locator('html').getAttribute('data-theme'),'dark');
 await page.emulateMedia({colorScheme:'light'});
 await page.waitForFunction(()=>document.documentElement.dataset.theme==='light');
 await page.locator('#adminToken').fill('miftah-local-preview-only');await page.locator('#loginForm button').click();
 await page.locator('#login').waitFor({state:'hidden'});await page.locator('#newKey').click();
 for(const id of ['Total','Daily','Weekly','Monthly','RPM','Concurrent'])assert.equal(await page.locator('#limit'+id).inputValue(),'0');
 await page.locator('#keyModels option[value="picker/a"]').waitFor({state:'attached'});
 await page.locator('#keyName').fill('Picker theme test');
 await page.locator('#pickerToggle').click();await page.locator('#pickerSearch').fill('picker/a');
 assert.equal(await page.locator('.picker-option').count(),1);
 await page.locator('.picker-option input').check();assert.equal(await page.locator('#selectedChips .model-chip').count(),1);
 await page.locator('#pickerSearch').press('Escape');
 await page.locator('#addFallbackRow').click();await page.locator('.fallback-primary').fill('picker/a');
 for(const name of ['picker/b','picker/c']){
  await page.locator('.fallback-draft').fill(name);await page.locator('.fallback-draft').press('Enter');
 }
 await page.locator('.fallback-chain .model-chip').nth(1).dragTo(page.locator('.fallback-chain .model-chip').first());
 assert((await page.locator('.fallback-chain bdi').first().innerText()).includes('picker/c'));
 await page.locator('#directInput').fill('1');await page.locator('#directOutput').fill('2');
 await page.locator('#limitMonthly').fill('5');
 await mkdir('artifacts',{recursive:true});
 await page.screenshot({path:'artifacts/picker-light.png',fullPage:true});
 await page.emulateMedia({colorScheme:'dark'});await page.waitForFunction(()=>document.documentElement.dataset.theme==='dark');
 await page.screenshot({path:'artifacts/picker-dark.png',fullPage:true});
 await page.locator('#keyForm button[type=submit]').click();await page.locator('#secretDialog').waitFor();
 await page.locator('[data-close=secretDialog]').click();
 await page.locator('nav [data-page=keys]').click();
 await page.locator('#keyTable tr').filter({hasText:'Picker theme test'}).locator('[data-edit]').click();
 assert.equal(await page.locator('#limitMonthly').inputValue(),'5');
 assert.equal(await page.locator('#selectedChips .model-chip').count(),1);
 assert.deepEqual(await page.locator('.fallback-chain bdi').allTextContents(),['1. picker/c','2. picker/b']);
 await page.locator('[data-close=keyDialog]').click();
 // CPA embeds same-origin resources and sets data-theme on its root.
 await page.route('**/theme-host',r=>r.fulfill({contentType:'text/html',body:'<!doctype html><html data-theme="white"><iframe src="/v0/resource/plugins/miftah/console"></iframe></html>'}));
 await page.goto('http://127.0.0.1:8741/theme-host');
 const frame=page.frameLocator('iframe');
 await frame.locator('#adminToken').waitFor({state:'attached'});
 assert.equal(await frame.locator('html').getAttribute('data-theme'),'light','host white must override dark OS');
 await page.evaluate(()=>document.documentElement.dataset.theme='dark');
 await page.waitForFunction(()=>document.querySelector('iframe').contentDocument.documentElement.dataset.theme==='dark');
 await page.evaluate(()=>document.documentElement.removeAttribute('data-theme'));
 await page.waitForFunction(()=>document.querySelector('iframe').contentDocument.documentElement.dataset.theme==='light');
 assert.deepEqual(errors,[]);
 console.log('PASS searchable chips, drag ordered per-key fallbacks, persisted edit, unlimited defaults, standalone system + live host dark/white/light');
}finally{await browser.close()}
