import {chromium} from 'playwright';
import assert from 'node:assert/strict';

const browser=await chromium.launch({executablePath:process.env.CHROME_BINARY||'/usr/bin/google-chrome',headless:true});
const page=await browser.newPage();
await page.route('**/v0/management/auth-files',r=>r.fulfill({json:{files:[{name:'pricing-account'}]}}));
await page.route('**/v0/management/auth-files/models?*',r=>r.fulfill({json:{models:[{id:'priced/model'},{id:'unmatched/model'}]}}));
await page.route('**/v0/management/miftah/prices',r=>r.fulfill({json:{prices:{'priced/model':{input_price:1250000,output_price:3500000,source:'reference'}},source:'test'}}));
try{
 await page.goto('http://127.0.0.1:8741');
 await page.locator('#adminToken').fill('miftah-local-preview-only');await page.locator('#loginForm button').click();await page.locator('#login').waitFor({state:'hidden'});
 await page.locator('nav [data-page=models]').click();await page.locator('#syncAllPrices').click();
 await page.locator('#syncPriceStatus').filter({hasText:'2 موديل'}).waitFor();
 assert.equal(await page.locator('[data-direct="priced/model"]').count(),1);assert.equal(await page.locator('[data-direct="unmatched/model"]').count(),1);
 await page.locator('[data-direct="priced/model"]').click();
 assert.equal(await page.locator('#inputPrice').inputValue(),'1.25');assert.equal(await page.locator('#outputPrice').inputValue(),'3.5');
 await page.locator('#routeDialog').evaluate(dialog=>dialog.close());await page.locator('[data-direct="unmatched/model"]').click();
 assert.equal(await page.locator('#inputPrice').inputValue(),'0');assert.equal(await page.locator('#outputPrice').inputValue(),'0');
 console.log('PASS full proxy catalog import, exact automatic pricing, safe zero for unmatched models');
}finally{await browser.close()}
