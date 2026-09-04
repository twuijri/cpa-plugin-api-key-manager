'use strict';
// Original implementation of the user's reference interaction: searchable chips and ordered fallback rows.
const nativeModels=$('keyModels');nativeModels.hidden=true;nativeModels.required=false;
const picker=document.createElement('div');picker.className='model-picker';
picker.innerHTML='<div id="selectedChips" class="model-chips"></div><button type="button" id="pickerToggle" class="quiet" aria-expanded="false">اختيار الموديلات ▾</button><div id="pickerPopup" class="picker-popup" hidden><input id="pickerSearch" type="search" placeholder="ابحث عن موديل…" aria-label="بحث الموديلات"><div id="pickerOptions"></div></div>';
nativeModels.after(picker);
function renderModelPicker() {
 $('selectedChips').replaceChildren();
 for(const option of nativeModels.selectedOptions) {
  const chip=document.createElement('span');chip.className='model-chip';
  const label=document.createElement('bdi');label.textContent=option.value;chip.append(label);
  const remove=document.createElement('button');remove.type='button';remove.textContent='×';remove.setAttribute('aria-label','إزالة '+option.value);
  remove.onclick=()=>{option.selected=false;nativeModels.dispatchEvent(new Event('change'));renderModelPicker()};chip.append(remove);$('selectedChips').append(chip);
 }
 const filter=$('pickerSearch').value.toLowerCase();$('pickerOptions').replaceChildren();
 for(const group of nativeModels.querySelectorAll('optgroup')) {
  const options=[...group.children].filter(o=>o.value.toLowerCase().includes(filter));if(!options.length)continue;
  const title=document.createElement('strong');title.textContent=group.label;$('pickerOptions').append(title);
  for(const option of options) {
   const label=document.createElement('label');label.className='picker-option';
   const box=document.createElement('input');box.type='checkbox';box.checked=option.selected;
   box.onchange=()=>{option.selected=box.checked;nativeModels.dispatchEvent(new Event('change'));renderModelPicker()};
   const text=document.createElement('bdi');text.textContent=option.textContent;label.append(box,text);$('pickerOptions').append(label);
  }
 }
 if(!$('pickerOptions').children.length)$('pickerOptions').textContent='لا توجد نتائج؛ يمكنك إضافة الاسم يدويًا أدناه.';
}
$('pickerToggle').onclick=()=>{const open=$('pickerPopup').hidden;$('pickerPopup').hidden=!open;$('pickerToggle').setAttribute('aria-expanded',String(open));if(open){renderModelPicker();$('pickerSearch').focus()}};
$('pickerSearch').oninput=renderModelPicker;
picker.addEventListener('keydown',e=>{if(e.key==='Escape'){e.preventDefault();e.stopPropagation();$('pickerPopup').hidden=true;$('pickerToggle').setAttribute('aria-expanded','false');$('pickerToggle').focus()}});
document.addEventListener('click',e=>{if(!picker.contains(e.target)){$('pickerPopup').hidden=true;$('pickerToggle').setAttribute('aria-expanded','false')}});
new MutationObserver(renderModelPicker).observe(nativeModels,{childList:true,subtree:true});
nativeModels.addEventListener('change',renderModelPicker);
const fallbackBox=document.createElement('fieldset');fallbackBox.id='keyFallbackBox';
fallbackBox.innerHTML='<legend>الفيل باك لهذا المفتاح</legend><p>كل صف: موديل مباشر مسموح ← بدائل مرتبة. الصف يستبدل بدائل الموديل لهذا المفتاح فقط. بدون صف تُستخدم إعدادات الموديل الحالية. صف بلا بدائل يلغي الفيل باك لهذا الموديل.</p><button type="button" class="quiet" id="addFallbackRow">＋ إضافة صف</button><div id="keyFallbackRows"></div>';
$('newModelPrices').after(fallbackBox);
function moveItem(items,from,to){if(from===to||from<0||to<0)return;items.splice(to,0,items.splice(from,1)[0])}
function reorderable(node,items,index,rerender,group) {
 node.draggable=true;
 node.ondragstart=e=>{if(e.target.closest('input')){e.preventDefault();return}e.stopPropagation();e.dataTransfer.setData('text/plain',group+':'+index)};
 node.ondragover=e=>{e.preventDefault();e.stopPropagation()};
 node.ondrop=e=>{e.preventDefault();e.stopPropagation();const v=e.dataTransfer.getData('text/plain');if(!v.startsWith(group+':'))return;const from=Number(v.slice(group.length+1));if(Number.isInteger(from)&&from>=0&&from<items.length){moveItem(items,from,index);rerender()}};
}
function smallButton(label,fn){const b=document.createElement('button');b.type='button';b.className='quiet';b.textContent=label;b.onclick=fn;return b}
function renderFallbackEditor(){
 $('keyFallbackRows').replaceChildren();
 if(!keyFallbackDraft.length){const empty=document.createElement('p');empty.className='empty';empty.textContent='لا توجد بدائل مخصصة لهذا المفتاح.';$('keyFallbackRows').append(empty)}
 keyFallbackDraft.forEach((f,index)=>{
  const row=document.createElement('article');row.className='fallback-row';reorderable(row,keyFallbackDraft,index,renderFallbackEditor,'rows');
  const bar=document.createElement('div');bar.className='dialog-actions';const title=document.createElement('strong');title.textContent='⠿ '+(index+1);bar.append(title,
   smallButton('↑',()=>{if(index){moveItem(keyFallbackDraft,index,index-1);renderFallbackEditor()}}),
   smallButton('↓',()=>{if(index+1<keyFallbackDraft.length){moveItem(keyFallbackDraft,index,index+1);renderFallbackEditor()}}),
   smallButton('حذف الصف',()=>{keyFallbackDraft.splice(index,1);renderFallbackEditor();priceVisibility()}));row.append(bar);
  const primaryLabel=document.createElement('label');primaryLabel.textContent='الموديل الأساسي';
  const primary=document.createElement('input');primary.className='fallback-primary';primary.dir='ltr';primary.required=true;primary.maxLength=200;primary.value=f.primary;primary.setAttribute('list','modelSuggestions');
  primary.oninput=()=>{f.primary=primary.value.trim()};primaryLabel.append(primary);row.append(primaryLabel);
  const chain=document.createElement('div');chain.className='model-chips fallback-chain';
  f.fallbacks.forEach((name,i)=>{
   const chip=document.createElement('span');chip.className='model-chip';reorderable(chip,f.fallbacks,i,renderFallbackEditor,'chain'+index);
   const label=document.createElement('bdi');label.textContent=(i+1)+'. '+name;chip.append(label,
    smallButton('↑',()=>{if(i){moveItem(f.fallbacks,i,i-1);renderFallbackEditor()}}),
    smallButton('↓',()=>{if(i+1<f.fallbacks.length){moveItem(f.fallbacks,i,i+1);renderFallbackEditor()}}),
    smallButton('×',()=>{f.fallbacks.splice(i,1);renderFallbackEditor();priceVisibility()}));chain.append(chip);
  });row.append(chain);
  const add=document.createElement('div');add.className='dialog-actions';const input=document.createElement('input');input.className='fallback-draft';input.dir='ltr';input.maxLength=200;input.placeholder='أضف موديلًا بديلًا';input.setAttribute('list','modelSuggestions');
  const addModel=()=>{const n=input.value.trim();if(!n)return;if(n===f.primary||f.fallbacks.includes(n))return toast('الموديل مكرر');if(f.fallbacks.length>=4)return toast('الحد أربعة بدائل');if(knownPolicy(n)&&knownPolicy(n).kind!=='direct')return toast('اختر موديلًا مباشرًا، وليس مسارًا');f.fallbacks.push(n);renderFallbackEditor();priceVisibility()};
  input.onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();addModel()}};add.append(input,smallButton('إضافة بديل',addModel));row.append(add);
  const settings=document.createElement('div');settings.className='retry-options';
  for(const status of [429,502,503,504]){const label=document.createElement('label');label.className='check';const check=document.createElement('input');check.type='checkbox';check.checked=(f.retry_statuses||[]).includes(status);check.onchange=()=>{f.retry_statuses=check.checked?[...(f.retry_statuses||[]),status]:(f.retry_statuses||[]).filter(s=>s!==status)};label.append(check,document.createTextNode(String(status)));settings.append(label)}
  const unknown=document.createElement('label');unknown.className='check';const check=document.createElement('input');check.type='checkbox';check.checked=!!f.retry_unknown;check.onchange=()=>{f.retry_unknown=check.checked};unknown.append(check,document.createTextNode('أخطاء غير مصنفة (قد تكرر تكلفة الطلب)'));settings.append(unknown);row.append(settings);$('keyFallbackRows').append(row);
 });
}
$('addFallbackRow').onclick=()=>{keyFallbackDraft.push({primary:'',fallbacks:[],retry_statuses:[429,503],retry_unknown:false});renderFallbackEditor()};
const submitWithModels=$('keyForm').onsubmit;
$('keyForm').onsubmit=e=>{
 if(!nativeModels.selectedOptions.length){e.preventDefault();toast('اختر موديلًا واحدًا على الأقل؛ الاختيار الفارغ لا يفتح الوصول للجميع');return}
 const selected=[...nativeModels.selectedOptions].map(o=>o.value),seen=new Set();
 for(const f of keyFallbackDraft){if(!selected.includes(f.primary)||seen.has(f.primary)||knownPolicy(f.primary)?.kind==='route'||(knownPolicy(f.primary)&&knownPolicy(f.primary).kind!=='direct')){e.preventDefault();toast('كل موديل أساسي يجب أن يكون مباشرًا ومختارًا مرة واحدة');return}seen.add(f.primary)}
 submitWithModels(e);
};
