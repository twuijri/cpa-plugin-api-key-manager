import {chromium} from 'playwright';
import assert from 'node:assert/strict';
import {mkdir} from 'node:fs/promises';
const browser=await chromium.launch({executablePath:process.env.CHROME_BINARY||'/usr/bin/google-chrome',headless:true});
const page=await browser.newPage({viewport:{width:1440,height:1050}});
async function chooseModels(values){
 if(!Array.isArray(values))values=[values];
 while(await page.locator('#selectedChips button').count())await page.locator('#selectedChips button').first().click();
 await page.locator('#pickerToggle').click();
 for(const value of values){await page.locator('#pickerSearch').fill(value);await page.locator('.picker-option').filter({hasText:value}).locator('input').check()}
 await page.locator('#pickerSearch').press('Escape');
}
const errors=[];page.on('pageerror',e=>errors.push(e.message));
const external=[];page.on('request',r=>{if(!r.url().startsWith('http://127.0.0.1:8741'))external.push(r.url())});
try{
 await page.goto('http://127.0.0.1:8741');await page.locator('#adminToken').fill('miftah-local-preview-only');await page.locator('#loginForm button').click();await page.locator('#login').waitFor({state:'hidden'});const initialCount=Number(await page.locator('#activeCount').textContent());
 await page.locator('nav [data-page=routes]').click();await page.locator('#newRoute').click();await page.locator('#routeAlias').fill('design-assistant');await page.locator('#targetList input').first().fill('primary-model');await page.locator('#addTarget').click();await page.locator('#targetList input').nth(1).fill('backup-model');
 await page.locator('#targetList [data-up]').nth(1).click();assert.equal(await page.locator('#targetList input').first().inputValue(),'backup-model');
 await page.locator('#targetList [data-down]').first().click();assert.equal(await page.locator('#targetList input').first().inputValue(),'primary-model');
 await page.locator('#targetList .target').nth(1).dragTo(page.locator('#targetList .target').first(),{targetPosition:{x:10,y:5}});assert.equal(await page.locator('#targetList input').first().inputValue(),'backup-model');
 await page.locator('#routeForm button[type=submit]').click();await page.locator('#routeDialog').waitFor({state:'hidden'});
 await page.locator('#newKey').click();await page.locator('#keyName').fill('مساعد التصميم');await page.locator('#keyOwner').fill('اختبار محلي');await chooseModels('design-assistant');await page.locator('#keyForm button[type=submit]').click();await page.locator('#secretDialog').waitFor();const secret=await page.locator('#secret').inputValue();assert(secret.startsWith('mf_'));await page.locator('[data-close=secretDialog]').click();assert.equal(await page.locator('#secret').inputValue(),'');
 await page.locator('nav [data-page=overview]').click();await page.locator('#activeCount').filter({hasText:String(initialCount+1)}).waitFor();
 assert.deepEqual(await page.evaluate(()=>({local:Object.keys(localStorage),session:Object.keys(sessionStorage)})),{local:[],session:[]});assert.equal(external.length,0);assert.deepEqual(errors,[]);
 await mkdir('artifacts',{recursive:true});await page.screenshot({path:'artifacts/dashboard.png',fullPage:true});
 await page.setViewportSize({width:390,height:844});await page.screenshot({path:'artifacts/mobile.png',fullPage:true});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'mobile horizontal overflow');
 console.log('PASS UI login, route ordering, create key, secret clearing, no secret storage, no external requests, desktop/mobile');
}finally{await browser.close()}
