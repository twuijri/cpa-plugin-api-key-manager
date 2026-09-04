# Verification

## 0.1.4

- Added an atomic catalog sync that creates every model discovered from enabled proxy auth entries, applies exact-name public reference prices, preserves existing fallback/output configuration, and leaves unmatched new models at zero rather than guessing.
- Core tests cover exact prices, preservation, unmatched models, conflict handling, duplicate rejection, and atomic rollback.
- Playwright covers full catalog import/pricing, Arabic/English persistence and RTL/LTR direction, and live host light/dark theme inheritance with neutral colors.
- `go test -race ./...`, `go vet ./...`, package build, and the real CPA v7.2.146 mock-provider integration passed, including native keys, virtual keys, streaming, fallback, isolation, and actual-model per-key charging.
- Release artifact SHA-256: `813b0e3f164f3088879eda73d620d76edbb64fd51ca32fb10e0d00f4fc71c6ee`.

## 0.1.3

- Added an opt-in tab-session admin token using `sessionStorage` (checked by default), with explicit private-device guidance.
- Browser test verifies reload restoration, logout clearing and unchecked opt-out. A 401 also clears the saved token before reopening login.
- Existing pricing/zero-price browser tests, Go race tests/vet/build and real CPA v7.2.146 integration pass.

## 0.1.0

- Removed Alpha branding and published a non-prerelease version at the owner's request; no new claim of certification or additional platform support.
- Plugin ID and stored state are unchanged. Compatibility and accounting limits remain documented.
- Full Go race tests/vet, shared-library build and real CPA v7.2.146 integration (including actual fallback billing in chat and stream) passed.

## 0.1.0-alpha.9

- Full Go race tests and vet pass. Built linux/amd64 shared library. Real unmodified CPA v7.2.146 integration verifies custom fallback charges for both chat and streaming, unchanged shared prices, and all existing access/fallback tests.
- Fixed usage parsing for nested token details and unframed JSON host stream events; previously these could retain an uncertain estimate despite available usage. Missing usage still retains an uncertain reservation rather than inventing token counts.
- Live public reference fetch returned 2586 supported text-price rows, including an exact gpt-5.6-sol match; this is a point-in-time availability check, not a guarantee of current manufacturer billing.

- Added per-key model prices with explicit zero, bounded validation, legacy compatibility and per-request price snapshots. Core tests cover isolation, persisted free overrides, fallback settlement, mid-request edits and budget enforcement.
- Reference catalog uses a fixed public LiteLLM HTTPS URL, bounded response/time, no redirects and no user credentials or selections. Parser tests cover unit conversion, missing rates, negatives and non-text modes. Only exact IDs are imported; advanced billing tiers/cache/media are explicitly excluded.
- Browser test: reference import, 50% markup, cancel, save/reopen and isolation from another key; zero-price regression also passes. Screenshot inspected.

## 0.1.0-alpha.8

- Full Go race tests and vet passed. Runtime gate now covers all RPCs, including management/auth, while quiesce drains background stream workers before releasing the store lock.
- Unit tests: second writer blocked, quiesced calls denied, delayed old-library shutdown cannot release new writer lock, rollback reloads newer persisted state, active execution completes before quiesce releases ownership.
- Real unmodified CPA v7.2.146 process loaded alpha.8 and replaced it with an unpublished alpha.9-test fixture, preserving PID and exact idle state. Existing keys, discovery, chat and streaming worked afterward.
- Invalid 0.2.0-test shared-library replacement exercised host rollback. The prior library resumed and served the same key without a process restart.
- Run: `CPA_TEST_BINARY=... MIFTAH_RELOAD_TEST_BINARY=... node tests/integration.mjs`. Build the replacement fixture with `-ldflags '-X miftah.local/plugin/internal/bridge.Version=0.1.0-alpha.9-test'` outside dist.
- This is not a zero-downtime guarantee: quiesce can end active streams; hosts with incompatible reload behavior may differ. User VPS not changed/tested by this release.
- Bootstrap from alpha.7 or earlier still needs one restart because those old libraries retain the lock until shutdown.

## 0.1.0-alpha.7

- `go test -race ./...` and `go vet ./...` passed.
- Real shared library loaded by unmodified local CPA v7.2.146: direct GET /v1/models with a valid virtual key returned 200 and matched the native-key catalog.
- Missing/invalid keys rejected. Unit tests also reject disabled/expired keys and unrelated GET/POST endpoints.
- Discovering an upstream model did not grant execution access; unauthorized execution made no upstream call.
- Existing native-key, streaming, direct-model and per-key fallback integration tests passed.
- Not yet verified on the user's VPS. Does not claim to fix the separately reported Hermes "Missing Authentication header" error or hot-reload locking.

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
