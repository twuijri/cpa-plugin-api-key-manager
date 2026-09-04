import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const browser=await chromium.launch({executablePath:'/usr/bin/google-chrome',headless:true});
const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.route('**/v0/management/auth-files',r=>r.fulfill({json:{files:[]}}));
await page.route('**/v0/management/miftah/prices',r=>r.fulfill({json:{prices:{'price-model':{input_price:2000000,output_price:8000000}},fetched_at:'2026-09-05T00:00:00Z'}}));
try {
 await page.goto('http://127.0.0.1:8741');await page.locator('#adminToken').fill('miftah-local-preview-only');await page.locator('#loginForm button').click();await page.locator('#login').waitFor({state:'hidden'});
 await page.locator('#newKey').click();await page.locator('#keyName').fill('Premium pricing test');await page.locator('#manualModel').fill('price-model');await page.locator('#addManualModel').click();
 await page.locator('#customizePrices').click();await page.locator('#importPrices').click();await page.getByText('تمت مطابقة 1 موديلات.',{exact:false}).waitFor();
 assert.equal(await page.locator('[data-price=input_price]').inputValue(),'2');
 await page.locator('#pricePercent').fill('50');await page.locator('#applyPercent').click();assert.equal(await page.locator('[data-price=input_price]').inputValue(),'3');
 await page.locator('#priceForm button[type=submit]').click();await page.locator('#customizePrices').click();assert.equal(await page.locator('[data-price=output_price]').inputValue(),'12');
 await page.locator('[data-price=input_price]').fill('99');await page.locator('#cancelPrices').click();await page.locator('#customizePrices').click();assert.equal(await page.locator('[data-price=input_price]').inputValue(),'3');await page.locator('#cancelPrices').click();
 const saved=page.waitForResponse(r=>r.url().endsWith('/miftah/keys')&&r.request().method()==='POST');
 await page.locator('#keyForm button[type=submit]').click();const result=await (await saved).json();assert.equal(result.key.prices['price-model'].input_price,3000000);assert.equal(result.key.pricing_mode,'models');
 await page.locator('#secretDialog').waitFor();await page.locator('[data-close=secretDialog]').click();
 await page.locator('#newKey').click();await page.locator('#manualModel').fill('price-model');await page.locator('#addManualModel').click();await page.locator('#customizePrices').click();assert.equal(await page.locator('[data-price=input_price]').inputValue(),'0');await page.locator('#cancelPrices').click();await page.locator('[data-close=keyDialog]').click();
 await page.locator('nav [data-page=keys]').click();await page.locator('#keyTable [data-edit="'+result.key.id+'"]').click();await page.locator('#customizePrices').click();assert.equal(await page.locator('[data-price=input_price]').inputValue(),'3');
 await page.screenshot({path:'/tmp/miftah-key-pricing.png',fullPage:true});assert.deepEqual(errors,[]);console.log('PASS per-key import, markup, cancel, persistence, isolation');
}finally{await browser.close()}
