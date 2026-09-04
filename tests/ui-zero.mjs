import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const browser=await chromium.launch({executablePath:'/usr/bin/google-chrome',headless:true});
const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.route('**/v0/management/auth-files',r=>r.fulfill({json:{files:[]}}));
try{
 await page.goto('http://127.0.0.1:8741');await page.locator('#adminToken').fill('miftah-local-preview-only');await page.locator('#loginForm button').click();await page.locator('#login').waitFor({state:'hidden'});
 await page.locator('#newKey').click();await page.locator('#keyName').fill('Zero price');
 await page.locator('#manualModel').fill('personal-zero');await page.locator('#addManualModel').click();
 for(const id of ['directInput','directOutput']){assert.equal(await page.locator('#'+id).inputValue(),'0');assert.equal(await page.locator('#'+id).getAttribute('min'),'0')}
 await page.locator('#keyForm button[type=submit]').click();await page.locator('#secretDialog').waitFor();await page.locator('[data-close=secretDialog]').click();
 await page.locator('nav [data-page=models]').click();await page.locator('[data-direct="personal-zero"]').click();
 for(const id of ['inputPrice','outputPrice'])assert.equal(await page.locator('#'+id).inputValue(),'0');
 await page.locator('#inputPrice').fill('2');await page.locator('#routeForm button[type=submit]').click();await page.locator('#routeDialog').waitFor({state:'hidden'});
 await page.locator('[data-direct="personal-zero"]').click();assert.equal(await page.locator('#inputPrice').inputValue(),'2');assert.equal(await page.locator('#outputPrice').inputValue(),'0');await page.locator('[data-close=routeDialog]').click();
 await page.locator('#newModel').click();assert.equal(await page.locator('#inputPrice').inputValue(),'0');assert.equal(await page.locator('#outputPrice').inputValue(),'0');await page.locator('[data-close=routeDialog]').click();
 await page.locator('nav [data-page=routes]').click();await page.locator('#newRoute').click();assert.equal(await page.locator('#inputPrice').inputValue(),'0');await page.locator('#routeAlias').fill('zero-route');await page.locator('#targetList input').fill('personal-zero');await page.locator('#routeForm button[type=submit]').click();await page.locator('#routeDialog').waitFor({state:'hidden'});
 assert.deepEqual(errors,[]);console.log('PASS zero default prices, create key/model/route, persisted nonzero edit');
}finally{await browser.close()}
