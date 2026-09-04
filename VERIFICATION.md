# Verification — 0.1.0-alpha.1

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
