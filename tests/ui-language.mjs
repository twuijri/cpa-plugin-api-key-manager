import {chromium} from 'playwright';import assert from 'node:assert/strict';
const browser=await chromium.launch({executablePath:'/usr/bin/google-chrome',headless:true});const page=await browser.newPage();
await page.goto('http://127.0.0.1:8741');await page.locator('#loginLanguageToggle').click();
assert.equal(await page.locator('html').getAttribute('lang'),'en');assert.equal(await page.locator('html').getAttribute('dir'),'ltr');assert.equal(await page.locator('#login h2').textContent(),'Welcome to API Key Manager.');
await page.reload();assert.equal(await page.locator('html').getAttribute('lang'),'en');await page.locator('#loginLanguageToggle').click();assert.equal(await page.locator('html').getAttribute('dir'),'rtl');
await browser.close();console.log('PASS Arabic/English toggle, direction and persisted preference');
