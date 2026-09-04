'use strict';
// Direct model policies and optional aliases share the accounting engine, not the UI workflow.
let catalog = new Set(), catalogToken = '', policyKind = 'route';
titles.models = ['الموديلات، بأسمائها الأصلية.', 'اختر الموديل مباشرة، واضبط السعر والبدائل الاختيارية.'];
const modelNav = document.createElement('button');
modelNav.dataset.page = 'models'; modelNav.textContent = '◉ الموديلات المباشرة';
document.querySelector('nav [data-page="routes"]').before(modelNav);
const modelsPage = document.createElement('section');
modelsPage.id = 'models'; modelsPage.className = 'page hidden';
modelsPage.innerHTML = '<div class="panel"><div class="panel-head"><div><h2>موديلات مباشرة</h2><p>يستخدم العميل اسم الموديل الحقيقي. البدائل اختيارية ومشتركة بين المفاتيح التي تسمح بهذا الموديل.</p></div><button id="newModel" class="primary">＋ إعداد موديل</button></div><div id="directList" class="route-grid"></div></div>';
$('routes').before(modelsPage);
document.querySelector('#routes .panel-head p').textContent = 'اختياري: اسم مخصص يجمع موديلًا أساسيًا وبدائل مرتّبة. المسارات الحالية محفوظة.';
$('keyModels').parentElement.firstChild.textContent = 'الموديلات المسموحة والمسارات الاختيارية';
const pickerHelp = document.createElement('div');
pickerHelp.innerHTML = '<button id="loadModels" type="button" class="quiet">تحديث موديلات البروكسي</button><p id="catalogStatus" role="status"></p><label>إضافة اسم موديل غير ظاهر<div class="dialog-actions"><input id="manualModel" dir="ltr" maxlength="200" placeholder="اسم الموديل الحقيقي"><button id="addManualModel" type="button" class="quiet">إضافة</button></div></label>';
$('keyModels').parentElement.after(pickerHelp);
const newPrices = document.createElement('fieldset'); newPrices.id = 'newModelPrices'; newPrices.hidden = true;
newPrices.innerHTML = '<legend>تسعير الموديلات الجديدة المختارة</legend><p>تطبّق هذه الأسعار التقديرية على الموديلات التي لم تُضبط بعد فقط. لا تغيّر أسعار أو بدائل الموديلات الموجودة. يمكنك تعديل كل موديل لاحقًا.</p><div class="form-grid"><label>مليون توكن إدخال ($)<input id="directInput" type="number" min="0.000001" max="1000" step="0.000001"></label><label>مليون توكن إخراج ($)<input id="directOutput" type="number" min="0.000001" max="1000" step="0.000001"></label><label>أقصى إخراج للطلب<input id="directCap" type="number" min="1" max="131072" value="4096"></label></div>';
pickerHelp.after(newPrices);
const knownPolicy = name => state.routes.find(r => r.alias === name);
function priceVisibility() {
 const needed = [...$('keyModels').selectedOptions].some(o => !knownPolicy(o.value));
 newPrices.hidden = !needed;
 for (const id of ['directInput', 'directOutput', 'directCap']) $(id).required = needed;
}
function fillModelPicker(selected = [...$('keyModels').selectedOptions].map(o => o.value)) {
 const direct = new Set([...catalog, ...state.routes.filter(r => r.kind === 'direct').map(r => r.alias)]);
 const named = state.routes.filter(r => r.kind !== 'direct');
 for (const r of named) direct.delete(r.alias); // An existing alias keeps its meaning; no silent type conversion.
 const option = (name, label) => '<option value="' + esc(name) + '"' + (selected.includes(name) ? ' selected' : '') + '>' + esc(label) + '</option>';
 $('keyModels').innerHTML = '<optgroup label="موديلات مباشرة">' + [...direct].sort().map(n => option(n, n)).join('') + '</optgroup><optgroup label="مسارات اختيارية">' + named.map(r => option(r.alias, r.alias + ' · مسار')).join('') + '</optgroup>';
 priceVisibility();
}
async function managementRead(path, sessionToken) {
 const res = await fetch('/v0/management/' + path, {headers:{Authorization:'Bearer ' + sessionToken}, credentials:'omit', redirect:'error', cache:'no-store', signal:AbortSignal.timeout(10000)});
 if (!res.ok) throw Error('تعذر جلب قائمة الموديلات؛ يمكنك إدخال الاسم الحقيقي يدويًا.');
 return res.json();
}
async function loadCatalog() {
 const sessionToken = token;
 if (!sessionToken) return;
 $('catalogStatus').textContent = 'جاري جلب الموديلات من البروكسي…';
 try {
  // Metadata endpoints only. Never download credential JSON or provider configuration secrets.
  const files = (await managementRead('auth-files', sessionToken)).files || [];
  const names = [...new Set(files.filter(f => !f.disabled).map(f => f.name).filter(n => typeof n === 'string'))];
  const found = new Set(); let failed = 0;
  for (let i = 0; i < names.length; i += 4) {
   const results = await Promise.allSettled(names.slice(i, i + 4).map(n => managementRead('auth-files/models?name=' + encodeURIComponent(n), sessionToken)));
   if (token !== sessionToken) return;
   for (const r of results) {
    if (r.status !== 'fulfilled') { failed++; continue; }
    for (const m of r.value.models || []) if (typeof m.id === 'string' && m.id.length <= 200) found.add(m.id);
   }
  }
  if (token !== sessionToken) return;
  catalog = new Set([...catalog, ...found]); catalogToken = sessionToken;
  $('catalogStatus').textContent = found.size + ' موديلات من حسابات البروكسي' + (failed ? ' · بعض الحسابات تعذر قراءتها' : '') + '؛ إن لم يظهر موديلك أضف اسمه يدويًا.';
  if ($('keyDialog').open) fillModelPicker();
  updateDatalist();
 } catch (err) { if (token === sessionToken) $('catalogStatus').textContent = err.message; }
}
const suggestions = document.createElement('datalist'); suggestions.id = 'modelSuggestions'; document.body.append(suggestions);
function updateDatalist() { suggestions.innerHTML = [...catalog].sort().map(n => '<option value="' + esc(n) + '"></option>').join(''); }
const originalKeyForm = keyForm;
keyForm = function(k) {
 // Reuse field initialization, but do not require an alias or an existing pricing policy.
 originalKeyForm(k);
 fillModelPicker(k?.models || []);
 if (catalogToken !== token) void loadCatalog();
};
$('keyModels').onchange = priceVisibility;
$('loadModels').onclick = () => void loadCatalog();
$('addManualModel').onclick = () => {
 const name = $('manualModel').value.trim();
 if (!name || name.length > 200) return toast('أدخل اسم موديل صحيح');
 if (knownPolicy(name) && knownPolicy(name).kind !== 'direct') return toast('الاسم مستخدم لمسار؛ اختر اسم الموديل الفعلي لتجنب التعارض');
 const selected = [...$('keyModels').selectedOptions].map(o => o.value);
 catalog.add(name); fillModelPicker([...selected, name]); updateDatalist(); $('manualModel').value = '';
};
const originalRender = render;
render = function() {
 const all = state.routes;
 state.routes = all.filter(r => r.kind !== 'direct');
 try { originalRender(); } finally { state.routes = all; }
 $('directList').innerHTML = all.filter(r => r.kind === 'direct').map(r => '<article class="route-card"><h2><code>' + esc(r.alias) + '</code></h2><p>' + (r.targets.length - 1) + ' بدائل اختيارية · حد الإخراج ' + r.max_output + '</p><ol>' + r.targets.map(t => '<li>' + esc(t) + '</li>').join('') + '</ol><button class="quiet" data-direct="' + esc(r.alias) + '">السعر والبدائل ←</button></article>').join('') || '<div class="empty">اختر موديلات مباشرة عند إنشاء المفتاح، أو أضف إعداد موديل هنا. لا تحتاج إنشاء مسار.</div>';
 for (const th of document.querySelectorAll('#keyTable th, #recentKeys th')) if (th.textContent === 'المسارات') th.textContent = 'الموديلات / المسارات';
 for (const empty of document.querySelectorAll('#keyTable .empty, #recentKeys .empty')) empty.textContent = 'أنشئ مفتاحك الأول واختر الموديلات مباشرة.';
};
const originalRouteForm = routeForm;
routeForm = function(r) { policyKind = r?.kind === 'direct' ? 'direct' : 'route'; originalRouteForm(r); stylePolicyForm(); };
function stylePolicyForm() {
 const direct = policyKind === 'direct';
 $('routeForm').querySelector('h2').textContent = direct ? 'إعداد الموديل والبدائل' : 'تصميم مسار اختياري';
 $('routeAlias').parentElement.firstChild.textContent = direct ? 'اسم الموديل الحقيقي' : 'اسم المسار الذي يستخدمه العميل';
 $('routeAlias').setAttribute('list', 'modelSuggestions');
 $('routeAlias').maxLength = 200;
 const first = $('targetList').firstElementChild;
 if (direct && first) first.remove(); // Primary stays fixed to the requested ID; only backups are reorderable.
 $('routeForm').querySelector('button[type="submit"]').textContent = direct ? 'حفظ إعداد الموديل' : 'حفظ المسار';
}
$('newModel').onclick = () => { originalRouteForm(); policyKind = 'direct'; stylePolicyForm(); $('inputPrice').value=''; $('outputPrice').value=''; };
document.addEventListener('click', e => { const b = e.target.closest('[data-direct]'); if (b) routeForm(knownPolicy(b.dataset.direct)); });
$('routeForm').onsubmit = e => { e.preventDefault(); guard(async () => {
 const alias = $('routeAlias').value.trim();
 const inputs = [...$('targetList').querySelectorAll('input')].map(x => x.value.trim());
 const targets = policyKind === 'direct' ? [alias, ...inputs] : inputs;
 const route = {kind:policyKind, alias, targets, retry_statuses:[...document.querySelectorAll('[name=retry]:checked')].map(x=>Number(x.value)), retry_unknown:$('retryUnknown').checked, input_price:Math.round(Number($('inputPrice').value)*1e6), output_price:Math.round(Number($('outputPrice').value)*1e6), max_output:Number($('maxOutput').value)};
 await api('routes', 'PUT', {route, revision:formRevision}); $('routeDialog').close(); await refresh(); toast('تم حفظ إعداد التوجيه');
 }); };
$('keyForm').onsubmit = e => { e.preventDefault(); guard(async () => {
 const models = [...$('keyModels').selectedOptions].map(o => o.value);
 const direct_policies = models.filter(n => !knownPolicy(n)).map(alias => ({kind:'direct', alias, targets:[alias], retry_statuses:[], retry_unknown:false, input_price:Math.round(Number($('directInput').value)*1e6), output_price:Math.round(Number($('directOutput').value)*1e6), max_output:Number($('directCap').value)}));
 const limits = {};
 for (const n of ['Total','Daily','Weekly','Monthly']) limits[n.toLowerCase()] = Math.round(Number($('limit'+n).value)*1e6);
 limits.rpm = Number($('limitRPM').value); limits.concurrent = Number($('limitConcurrent').value);
 const k = {...(editing || {}), name:$('keyName').value, owner:$('keyOwner').value, enabled:$('keyEnabled').checked, models, limits, expires_at:$('keyExpiry').value ? new Date($('keyExpiry').value).toISOString() : ''};
 if (editing) await api('keys','PUT',{key:k,revision:formRevision,direct_policies});
 else { const result = await api('keys','POST',{...k,direct_policies}); showSecret(result.secret); }
 $('keyDialog').close(); await refresh(); toast('تم حفظ المفتاح');
 }); };
const originalLogout = $('logout').onclick;
$('addTarget').onclick = () => { if ($('targetList').children.length < (policyKind === 'direct' ? 4 : 5)) targetRow(); else toast('الحد خمسة موديلات بما فيها الأساسي'); };
$('logout').onclick = () => { catalog.clear(); catalogToken=''; suggestions.replaceChildren(); $('catalogStatus').textContent=''; originalLogout(); };
