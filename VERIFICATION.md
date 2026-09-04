# Verification

## 0.1.0-alpha.6

- New resource GET /v0/resource/plugins/miftah/models authenticates virtual keys and returns an OpenAI model list containing only their explicit allowed names, including named routes. It does not expand fallbacks or expose state.
- Unit/race tests: valid list, invalid/native/missing/disabled/expired keys denied, no secret or backup leakage, no-store headers, no accounting entries, non-GET denied.
- Real CPA v7.2.146 and Caddy v2.11.4 mock-upstream integration: rewritten GET /v1/models returns virtual allowlist; native list unchanged; missing/invalid/disabled virtual keys denied; prior chat/streaming/fallback tests pass.
- Requires a Caddy rewrite for standard client discovery. No host-native per-key list hook exists in the inspected SDK. No live deployment or Hermes end-to-end test performed.
- Hot-reload lock behavior was inspected, not changed. CPA restart is still needed for this plugin's updates.

## 0.1.0-alpha.5

- Zero input/output prices accepted independently, negative prices rejected. Defaults are zero for new direct models and named routes; saved prices are retained.
- Core race tests and vet pass. New test verifies zero reservation/settlement cost with usage retained and RPM/concurrency limits still enforced.
- Browser test passes creating a key/model and named route at zero, editing to nonzero and preserving it, and resetting new forms to zero.
- Prior real CPA mock-upstream integration passes. No production server changes; provider charges are unaffected by these bookkeeping prices.

## 0.1.0-alpha.4

- Searchable multi-select chips and ordered, draggable per-key fallback rows. New keys default to unlimited monetary/RPM/concurrency limits; saved limits are retained.
- Core race tests and vet pass: per-key isolation, backup authorization denial, highest backup price reservation, empty override persistence, invalid policy atomic rollback, plus prior tests.
- Real CPA v7.2.146 integration with mock upstream passes: independent per-key fallback including streaming, shared policy unchanged, native access preserved, plus prior routing/disable tests.
- Playwright passes: search/checkbox/chips, actual backup drag/drop, create/edit persistence, unlimited defaults, old direct and named-route workflows, desktop/mobile.
- Theme test reproduces CPA's same-origin iframe and data-theme contract: live dark/white/light switching overrides OS theme. Standalone system theme switching also passes. Screenshots inspected in light and dark modes.
- Theme code only observes the parent's theme attribute; no host credentials or storage are read. Cross-origin embedding falls back to OS theme.
- No production server changes or real provider tests. Per-request output caps remain required; periodic token quotas do not exist yet.

## 0.1.0-alpha.3

- Direct model selection with optional shared per-model fallback and pricing; named routes remain backward compatible.
- Core tests: direct resolution, fixed primary, fallback does not grant direct access, mixed allowlists, persistence/reopen, atomic policy-plus-key changes, stale revision rejection, type collision and shared policy overwrite rejection.
- Real CPA v7.2.146 with mock upstream: model metadata listing, direct model requests without aliases, ordered fallback, streaming, no-fallback requests, key disable, and all prior native-key and route tests.
- Isolated Playwright: direct selection on empty state, mock model catalog, explicit pricing, fixed primary/backup ordering, optional aliases, mixed editing, manual IDs, mobile layout, and no local/session credential storage or external requests. Prior route drag/drop UI test also passes.
- No live user deployment or real provider billing validation. Alpha limitations below remain in effect.

## 0.1.0-alpha.2

- Default state moved to the standard home auth volume: `~/.cli-proxy-api/miftah/state.db` (JSON format, not a provider auth JSON file).
- New path tests pass: home resolution, persistent reopen, 0600 state permissions, explicit override, and fail-closed legacy-state detection.
- `go test -race ./...` and `go vet ./...` passed; Linux amd64 shared library rebuilt with Go 1.27.1.
- This change does not dynamically discover a custom CPA auth-dir. Existing overrides remain authoritative. See migration instructions before upgrading an installation with existing state.
- Live user server was not modified or tested. Full host integration below was performed on alpha.1, not rerun for alpha.2.

## 0.1.0-alpha.1

Executed locally on 2026-09-04. No production server or user credentials used.

## Passed

- `go test -race ./...`: 17 unit tests, no race reports.
- `go vet ./...`: passed.
- Core statement coverage: 81.7%; bridge statement coverage: 36.4% (separate process integration is not included).
- Built a native `miftah.so` and zip, with SHA-256 checksum.
- Real CPA v7.2.146 (official source commit `d31b15916d15b550bbf388fd6da4a47d4d864109`) loads/registers the plugin.
- Local mock upstream through the real CPA: native key still works, virtual key works, fallback executes, streaming delivers content and completes.
- Direct access to a disallowed model and disabled virtual keys rejected without calling mock upstream.
- Management API rejects missing management credentials.
- Playwright isolated browser: login, route creation, keyboard reorder, actual drag/drop, key creation, one-time secret clearing, desktop/mobile layouts.
- Browser test confirmed no localStorage/sessionStorage entries and no external requests from the UI.

## Compatibility

- Built with Go 1.27.1 / GCC on Linux amd64.
- This local binary requires glibc 2.34 or newer. Rebuild for the deployment container when necessary; do not use this glibc binary on Alpine/musl.
- No runtime Go dependencies outside the standard library; Playwright is development-only.

## Not yet verified / release blockers

- Real provider accounts; Responses/Claude protocol end-to-end coverage.
- High load, fault injection, long stream cancellation/hot reload combinations.
- Cost correctness against real invoices, cache/reasoning/image pricing, multi-node storage.
- The complete requested product scope: user portal, token quotas, reconciliation, encrypted backups, English UI, production hardening.

This is a tested alpha foundation, not a production-security certification or completed billing platform.
