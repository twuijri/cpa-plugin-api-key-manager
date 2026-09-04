import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const browser=await chromium.launch({executablePath:'/usr/bin/google-chrome',headless:true});
const page=await browser.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
await page.route('**/v0/management/auth-files',r=>r.fulfill({json:{files:[]}}));
try{
 await page.goto('http://127.0.0.1:8741');await page.locator('#adminToken').fill('miftah-local-preview-only');await page.locator('#loginForm button').click();await page.locator('#login').waitFor({state:'hidden'});
 assert.equal(await page.evaluate(()=>sessionStorage.getItem('api-key-manager.admin-token.v1')),'miftah-local-preview-only');
 await page.reload();await page.locator('#login').waitFor({state:'hidden'});assert.equal(await page.locator('#connection').textContent(),'متصل · إدارة محلية');
 await page.locator('#logout').click();assert.equal(await page.evaluate(()=>sessionStorage.getItem('api-key-manager.admin-token.v1')),null);await page.reload();await page.locator('#login').waitFor({state:'visible'});
 await page.locator('#rememberSession').uncheck();await page.locator('#adminToken').fill('miftah-local-preview-only');await page.locator('#loginForm button').click();await page.locator('#login').waitFor({state:'hidden'});assert.equal(await page.evaluate(()=>sessionStorage.getItem('api-key-manager.admin-token.v1')),null);
 assert.deepEqual(errors,[]);console.log('PASS tab session restore, logout clearing and opt-out');
}finally{await browser.close()}
