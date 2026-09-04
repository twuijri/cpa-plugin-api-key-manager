import {chromium} from 'playwright';
import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
const browser=await chromium.launch({executablePath:process.env.CHROME_BINARY||'/usr/bin/google-chrome',headless:true});
const page=await browser.newPage({viewport:{width:1440,height:1000}});
async function chooseModels(values){
 if(!Array.isArray(values))values=[values];
 while(await page.locator('#selectedChips button').count())await page.locator('#selectedChips button').first().click();
 await page.locator('#pickerToggle').click();
 for(const value of values){await page.locator('#pickerSearch').fill(value);await page.locator('.picker-option').filter({hasText:value}).locator('input').check()}
 await page.locator('#pickerSearch').press('Escape');
}
const errors=[],external=[];
page.on('pageerror',e=>errors.push(e.message));
page.on('request',r=>{if(!r.url().startsWith('http://127.0.0.1:8741'))external.push(r.url())});
await page.route('**/v0/management/auth-files',r=>r.fulfill({json:{files:[{name:'mock-account'}]}}));
await page.route('**/v0/management/auth-files/models?*',r=>r.fulfill({json:{models:[{id:'real/model-a'},{id:'real/model-b'}]}}));
try {
 await page.goto('http://127.0.0.1:8741');
 await page.locator('#adminToken').fill('miftah-local-preview-only');await page.locator('#loginForm button').click();await page.locator('#login').waitFor({state:'hidden'});
 await page.locator('#newKey').click();await page.locator('#keyDialog').waitFor();
 await page.locator('#keyModels option[value="real/model-a"]').waitFor({state:'attached'});
 await page.locator('#keyName').fill('Direct test');await chooseModels(['real/model-a','real/model-b']);
 assert(await page.locator('#newModelPrices').isVisible());
 await page.locator('#directInput').fill('1');await page.locator('#directOutput').fill('2');
 await page.locator('#keyForm button[type=submit]').click();await page.locator('#secretDialog').waitFor();
 assert((await page.locator('#secret').inputValue()).startsWith('mf_'));await page.locator('[data-close=secretDialog]').click();
 await page.locator('nav [data-page=models]').click();assert.equal(await page.locator('#directList .route-card').count(),2);
 await page.locator('[data-direct="real/model-a"]').click();assert.equal(await page.locator('#routeAlias').inputValue(),'real/model-a');assert(await page.locator('#routeAlias').evaluate(e=>e.readOnly));
 assert.equal(await page.locator('#targetList input').count(),0);
 await page.locator('#addTarget').click();await page.locator('#targetList input').first().fill('real/model-b');
 await page.locator('#addTarget').click();await page.locator('#targetList input').nth(1).fill('third-model');
 await page.locator('#targetList [data-up]').nth(1).click();assert.equal(await page.locator('#targetList input').first().inputValue(),'third-model');
 await page.locator('#routeForm button[type=submit]').click();await page.locator('#routeDialog').waitFor({state:'hidden'});
 assert((await page.locator('#directList .route-card').first().innerText()).includes('real/model-a'));
 await page.locator('nav [data-page=routes]').click();assert.equal(await page.locator('#routeList .route-card').count(),0);
 await page.locator('#newRoute').click();await page.locator('#routeAlias').fill('optional-alias');await page.locator('#targetList input').fill('real/model-a');await page.locator('#routeForm button[type=submit]').click();await page.locator('#routeDialog').waitFor({state:'hidden'});
 await page.locator('nav [data-page=keys]').click();await page.locator('#keyTable [data-edit]').first().click();
 assert.deepEqual(await page.locator('#keyModels').evaluate(e=>[...e.selectedOptions].map(o=>o.value)),['real/model-a','real/model-b']);
 assert(!(await page.locator('#newModelPrices').isVisible()));
 await chooseModels(['real/model-a','optional-alias']);await page.locator('#keyForm button[type=submit]').click();await page.locator('#keyDialog').waitFor({state:'hidden'});
 await page.locator('#newKey').click();await page.locator('#manualModel').fill('manual/model');await page.locator('#addManualModel').click();assert(await page.locator('#newModelPrices').isVisible());await page.locator('[data-close=keyDialog]').click();
 assert.deepEqual(errors,[]);assert.equal(external.length,0);
 assert.deepEqual(await page.evaluate(()=>[Object.keys(localStorage),Object.keys(sessionStorage)]),[[],[]]);
 await page.locator('nav [data-page=models]').click();await mkdir('artifacts',{recursive:true});await page.screenshot({path:'artifacts/direct-models.png',fullPage:true});
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:'artifacts/direct-mobile.png',fullPage:true});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
 console.log('PASS direct selection without alias, catalog, pricing, fixed primary, ordered optional fallback, named routes, mixed edit, manual entry, secret storage, mobile');
}finally{await browser.close()}
