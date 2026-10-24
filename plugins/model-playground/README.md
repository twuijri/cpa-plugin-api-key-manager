> This local **0.3.0-local.3** variant uses an Arabic panel with manual client API key entry. See [LOCAL-CHANGES.md](LOCAL-CHANGES.md) for its behavior and differences from upstream. Dedicated `gemini-desktop-` model IDs are grouped as Gemini Desktop; other sources use an exact server-specific map and unknown IDs remain unclassified.

# CPA Model Playground Plugin

A CLIProxyAPI (CPA) plugin that gives you a model playground for **every model your CPA gateway exposes** — no matter which provider it comes from (WorkBuddy, BaiLian, Nvidia, Claude-compatible, Codex, …).

It adds an Apple-style web panel to CPA:

- **Sessions** — create, rename, delete and switch conversations; stored in `localStorage` (survives reloads), auto-named from the first line.
- **Model list** — pulled live from the gateway's `/v1/models`, grouped by provider, searchable; remembers your last picked model.
- **Chat** — talk to any model from the list; multi-turn, **true SSE streaming**, full **markdown rendering** (GFM via bundled marked; reasoning shown as a collapsible-feel muted block above the answer), usage stats.
- **Layout** — full-width, side panel + flexible content (CPA admin style).

The plugin owns no upstream credentials and registers no provider. The panel talks to the gateway's own OpenAI-compatible `/v1` API **same-origin** (no CORS, no proxy hop), authenticated with the gateway's `api-keys`. The key is picked up automatically — from the CPA main panel login state (`cli-proxy-auth`), from a `?key=` URL parameter, or by querying the gateway's full api-keys list. `api-keys` supports multiple entries; on 401 the panel walks to the next candidate key.

## Install

Build:

```bash
make build
```

Place `model-playground.so` into your CPA plugins directory (e.g. `plugins/linux/amd64/`), then add to `config.yaml`:

```yaml
plugins:
  configs:
    model-playground:
      enabled: true
```

Open the panel: `http://<cpa-host>/v0/resource/plugins/model-playground/panel`

## API

| Route | Method | Description |
|---|---|---|
| `/v0/resource/plugins/model-playground/panel` | GET | The panel UI (unauthenticated resource route) |
| `/v0/resource/plugins/model-playground/marked.js` | GET | Bundled marked markdown renderer |
| `/v0/management/plugins/model-playground/keys` | GET | List the gateway's `api-keys` (management-authenticated); the plugin queries it in-process so it works even with remote management restricted |

## Security

- The plugin **never stores or logs credentials** — keys live in the browser's `localStorage`/`sessionStorage` on the same origin, or in the gateway's `config.yaml`.
- The `panel` resource route is intentionally unauthenticated (it is served by the gateway itself); the `/keys` management route requires the management key.
- Rendered markdown is sanitized: `script`/`iframe`/`object`/`embed`/`form`/`style` tags, `on*` attributes and `javascript:` URLs are stripped.
- Remote requests to the management endpoints follow the gateway's `remote-management.allow-remote` setting; the `/keys` helper queries the management API **in-process**, so it works even when remote management is restricted.

## Development

```bash
make build   # CGO_ENABLED=1 go build -buildmode=c-shared -o model-playground.so .
make test    # go test -race -count=1 ./...
make lint    # gofmt -l (must be empty) + go vet
```

- `panel.html` is embedded with `go:embed` — editing it requires a rebuild.
- CI runs `gofmt`, `go vet`, `go test -race` and `make build` on every push/PR.

## Credits

- [marked](https://github.com/markedjs/marked) v15.0.6 (MIT, © Christopher Jeffrey and contributors) is bundled at `marked.min.js` for offline markdown rendering.
- Built for [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI).

## License

[MIT](LICENSE)
