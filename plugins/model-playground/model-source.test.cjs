const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
const panel = fs.readFileSync(__dirname + '/panel.html', 'utf8');
const sources = panel.match(/^const modelSources=.*;$/m)?.[0];
const classify = panel.match(/^function modelSource\(id\).*$/m)?.[0];
assert.ok(sources && classify, 'panel classification code must be present');
const context = vm.createContext({});
vm.runInContext(sources + '\n' + classify, context);
const cases = [
  ['gemini-desktop-auto', 'Gemini Desktop'],
  ['gemini-desktop-pro', 'Gemini Desktop'],
  ['gemini-desktop-future-model-123', 'Gemini Desktop'],
  ['gemini-3-flash', 'Antigravity'],
  ['gpt-6-astra', 'Codex'],
  ['gemini-future-model', 'غير محدد'],
  ['gemini-desktopish-pro', 'غير محدد'],
  ['gemini-desktop', 'غير محدد'],
  ['other-gemini-desktop-pro', 'غير محدد'],
  ['Gemini-Desktop-pro', 'غير محدد'],
  ['toString', 'غير محدد'],
  ['__proto__', 'غير محدد'],
];
for (const [id, expected] of cases) assert.equal(context.modelSource(id), expected, id);
console.log('Model source classification passed: ' + cases.length + ' cases');
