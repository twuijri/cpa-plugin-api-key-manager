'use strict';
let keyPrices = {}, keyPriceMode = 'models', priceDraft = {}, priceSession = 0;
const pricingButton = document.createElement('button');
pricingButton.id = 'customizePrices'; pricingButton.type = 'button'; pricingButton.className = 'quiet';
pricingButton.textContent = 'تخصيص الأسعار لهذا المفتاح';
$('keyForm').querySelector('button[type="submit"]').before(pricingButton);
const priceDialog = document.createElement('dialog'); priceDialog.id = 'priceDialog';
priceDialog.innerHTML = '<form id="priceForm"><div class="dialog-head"><h2>أسعار هذا المفتاح فقط</h2><button type="button" id="cancelPrices" class="quiet">إلغاء</button></div><p>دولار لكل مليون توكن. التخصيص لا يغيّر أسعار المفاتيح الأخرى. الصفر مجاني داخل مفتاح فقط. يتم احتساب الموديل الذي نفّذ الطلب، بما فيه البديل.</p><label>طريقة الاحتساب<select id="keyPriceMode"><option value="models">حسب سعر كل موديل فعلي</option><option value="">السلوك القديم: سعر المسار / أغلى بديل</option></select></label><div class="dialog-actions"><button type="button" id="importPrices" class="quiet">جلب الأسعار المرجعية</button><button type="button" id="resetAllPrices" class="quiet">الرجوع للأسعار الأساسية</button></div><p id="priceStatus" role="status">الأسعار المرجعية من LiteLLM وليست فاتورة أو اتصالًا مباشرًا بالشركات. الاستيراد يطابق الاسم حرفيًا ولا يحفظ حتى توافق.</p><label>زيادة أو خصم على الأسعار المعروضة (%)<input id="pricePercent" type="number" min="-100" max="10000" step="0.01" value="0"></label><button id="applyPercent" type="button" class="quiet">تطبيق النسبة</button><div id="priceRows"></div><p>القيم المخصصة تثبت لهذا المفتاح؛ الأسعار الأساسية تتبع إعدادات الموديل. التغييرات تخص الطلبات الجديدة فقط. أسعار الكاش والشرائح والصور غير محسوبة منفصلًا في هذه النسخة.</p><button type="submit" class="primary">اعتماد الأسعار للمفتاح</button></form>';
document.body.append(priceDialog);
function initKeyPricing(k) { keyPrices = structuredClone(k?.prices || {}); keyPriceMode = k ? (k.pricing_mode || '') : 'models'; priceSession++; }
function keyPricingPayload() { return {prices:structuredClone(keyPrices), pricing_mode:keyPriceMode}; }
function pricingTargets() {
 const names = new Map();
 for (const id of [...$('keyModels').selectedOptions].map(o=>o.value)) {
  const policy = knownPolicy(id);
  const fallback = keyFallbackDraft.find(f=>f.primary===id);
  const targets = fallback ? [id,...fallback.fallbacks] : (policy?.targets || [id]);
  for (const target of targets) if (!names.has(target)) names.set(target,policy);
 }
 return names;
}
function basePrice(id, parent) {
 const policy = knownPolicy(id)?.kind==='direct' ? knownPolicy(id) : parent;
 return {input_price:policy?.input_price ?? Math.round(Number($('directInput').value)*1e6), output_price:policy?.output_price ?? Math.round(Number($('directOutput').value)*1e6)};
}
function renderPriceRows() {
 $('priceRows').replaceChildren();
 for (const [id,parent] of pricingTargets()) {
  const custom = Object.hasOwn(priceDraft,id), p = custom ? priceDraft[id] : basePrice(id,parent);
  const row = document.createElement('fieldset'); row.dataset.priceModel=id;
  row.innerHTML='<legend><code>'+esc(id)+'</code></legend><small>'+esc(custom ? (p.source || 'سعر مخصص') : 'السعر الأساسي')+'</small><div class="form-grid"><label>إدخال ($ / مليون)<input data-price="input_price" type="number" min="0" max="1000" step="0.000001" required></label><label>إخراج ($ / مليون)<input data-price="output_price" type="number" min="0" max="1000" step="0.000001" required></label></div><button type="button" class="quiet">استعادة الأساسي</button>';
  for (const input of row.querySelectorAll('input')) {
   input.value=p[input.dataset.price]/1e6;
   input.oninput=()=>{priceDraft={...priceDraft,[id]:{input_price:Math.round(Number(row.querySelector('[data-price=input_price]').value)*1e6),output_price:Math.round(Number(row.querySelector('[data-price=output_price]').value)*1e6)}};row.querySelector('small').textContent='سعر مخصص';};
  }
  row.querySelector('button').onclick=()=>{delete priceDraft[id];renderPriceRows();};
  $('priceRows').append(row);
 }
 if (!$('priceRows').children.length) $('priceRows').textContent='اختر الموديلات أولًا.';
}
pricingButton.onclick=()=>{priceDraft=structuredClone(keyPrices);$('keyPriceMode').value=keyPriceMode;$('pricePercent').value='0';$('priceStatus').textContent='مصدر الاستيراد: LiteLLM. تطابق حرفي فقط؛ الأسعار الأساسية للنص دون الكاش والشرائح. راجع قبل الاعتماد.';priceSession++;renderPriceRows();priceDialog.showModal();};
$('cancelPrices').onclick=()=>priceDialog.close();
priceDialog.addEventListener('close',()=>priceSession++);
$('resetAllPrices').onclick=()=>{priceDraft={};renderPriceRows();};
$('applyPercent').onclick=()=>{
 if (!$('priceForm').reportValidity()) return;
 const factor=1+Number($('pricePercent').value)/100;
 let next=structuredClone(priceDraft);
 for(const [id,parent] of pricingTargets()) {
  const p=Object.hasOwn(next,id)?next[id]:basePrice(id,parent);
  const input_price=Math.round(p.input_price*factor),output_price=Math.round(p.output_price*factor);
  if(input_price>1e9||output_price>1e9) return toast('السعر يتجاوز الحد: 1000 دولار لكل مليون');
  next={...next,[id]:{input_price,output_price}};
 }
 priceDraft=next;renderPriceRows();
};
$('importPrices').onclick=()=>guard(async()=>{
 const session=priceSession, sessionToken=token;
 $('importPrices').disabled=true;
 try {
  const result=await api('prices');
  if(session!==priceSession||token!==sessionToken||!priceDialog.open) return;
  let matched=0; const missing=[];
  for(const id of pricingTargets().keys()) {
   if(Object.hasOwn(result.prices,id)) {priceDraft={...priceDraft,[id]:{...result.prices[id],source:'LiteLLM · '+result.fetched_at}};matched++;}
   else missing.push(id);
  }
  $('priceStatus').textContent='تمت مطابقة '+matched+' موديلات. غير المطابقة بقيت أسعارها كما هي: '+(missing.join('، ')||'لا يوجد')+'. راجع الأسعار قبل الاعتماد.';
  renderPriceRows();
 } finally { $('importPrices').disabled=false; }
});
$('priceForm').onsubmit=e=>{e.preventDefault();keyPrices=structuredClone(priceDraft);keyPriceMode=$('keyPriceMode').value;priceDialog.close();toast('تم اعتماد المسودة؛ اضغط حفظ المفتاح لتطبيقها');};
