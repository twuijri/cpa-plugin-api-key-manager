# متجر بلقنات CLIProxyAPI — Abdulaziz

مصدر واحد لإضافات CLIProxyAPI الخاصة بهذا المستودع. أضف الرابط التالي مرة واحدة في **Config Panel → Advanced & Experimental → Plugins → Third-party Plugin Sources**، ثم ثبّت الإضافات من **Plugin Store**:

```text
https://raw.githubusercontent.com/twuijri/cpa-plugin-api-key-manager/main/registry.json
```

| الإضافة | الإصدار | الوظيفة | المنصة | الدليل |
| --- | --- | --- | --- | --- |
| API Key Manager — مفتاح | 0.1.5 | إدارة مفاتيح العملاء والميزانيات والمسارات | Linux amd64، glibc ≥ 2.34 | [مفتاح](MIFTAH.md) · [التثبيت](INSTALL.md) |
| Gemini Desktop | 0.1.1-preview | ربط حساب Gemini Desktop بمحادثات نصية وبث الرد | Linux amd64، glibc ≥ 2.17، CPA 8.0.41 | [الدليل](plugins/gemini-desktop/README_AR.md) |
| Model Playground — شات التجربة | 0.3.0-local.1 | تجربة موديلات البروكسي بمحادثات نصية وبث | Linux amd64، CPA 8.0.41 | [الدليل](plugins/model-playground/README.md) |

Gemini Desktop إصدار تجريبي، وموديله `gemini-desktop-auto` يستخدم الموديل الافتراضي الذي تختاره Google للحساب. لا يدعم الصور أو استدعاء الأدوات في هذا الإصدار. اجتاز اختبارات محلية للمحادثات والبث؛ التحقق على السيرفر موضح عند إتمامه، وليس شرطًا مفترضًا لمجرد النشر.

## تنظيم المستودع

- `registry.json`: فهرس كل الإضافات، بمعرّف وإصدار وتحميل وبصمة SHA-256 مستقل لكل إضافة.
- `plugins/gemini-desktop/`: مصدر Gemini Desktop وترخيصه واختباراته وتعليماته.
- `downloads/<plugin-id>/<version>/`: حزم تثبيت محفوظة بأرقام إصدارات محددة.
- كود مفتاح الحالي في جذر المستودع للحفاظ على أدوات بنائه وروابطه الحالية. توثيقه الكامل في `MIFTAH.md`.
- [إضافة بلقن جديد أو تحديثه](docs/PUBLISHING_AR.md).

مفتاح يحتفظ بإشعار ملكيته في `OWNERSHIP.md`. Gemini Desktop مرخّص حسب `plugins/gemini-desktop/LICENSE` وإشعارات الطرف الثالث المصاحبة له. لا ينطبق ترخيص إضافة على بقية الإضافات تلقائيًا.

الجلسات وملفات الحسابات ومفاتيح الإدارة تبقى خارج Git؛ لا يتضمن المتجر بيانات دخول المستخدم.
