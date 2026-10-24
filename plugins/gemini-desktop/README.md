# Gemini Desktop provider for CLIProxyAPI

Experimental native provider **0.1.1-preview** for **CLIProxyAPI v8.0.41**. It signs in through the Google client used by Gemini Desktop and calls Gemini's native `RobinService.ProcessQuery` service. It does not use Antigravity credentials, Gemini CLI credentials, or browser cookies.

The provider ID is `gemini-desktop`. The model alias is **`gemini-desktop-auto`**. Google selects the account's default model; this alias does not pin Argon, Pro, Flash, or a particular model version.

[العربية](README_AR.md)

## Current scope

- Google browser sign-in with PKCE, followed by automatic refresh-token renewal.
- Text conversations through OpenAI Chat Completions, including streamed SSE output.
- Full conversation history supplied in `messages`, folded into a single text transcript for each independent native request.
- Separate credentials and routing from Antigravity and Gemini CLI.

This preview does **not** implement API tool calling, images, attachments, structured JSON/schema output, exact token accounting, or native conversation resumption. Unsupported tools and attachments return an explicit error. Token usage is omitted rather than estimated. Sampling settings and token-output limits are not mapped to native controls in this preview.

The implementation uses reverse-engineered native request fields observed in Gemini macOS **1.135.1.1291**. Google can change those fields or account requirements. The plugin is an independent integration; successful requests do not establish Google endorsement or guarantee compatibility with provider terms.

## Validation boundary

A local development probe completed native Desktop OAuth, refreshed the session, and received known diagnostic text from the native Gemini service. Unit and race tests cover the OAuth flow, callback replay protection, credential isolation, protobuf parsing, stream text de-duplication, and cancellation.

Loading the final library into a production CLIProxyAPI instance and testing from that instance are separate checks. Do not treat the local probe or a successful build as proof that a particular server deployment has passed them.

## Build

Requirements:

- Go **1.26.10** or a compatible newer toolchain.
- A C compiler, because CLIProxyAPI loads a native shared library.
- A normal CLIProxyAPI build with shared-library plugin support. A `no-plugin` build cannot load this provider.

On a Linux build machine, from the repository directory:

```sh
go test -race ./...
./scripts/build-linux.sh amd64
```

For ARM64 Linux, use `./scripts/build-linux.sh arm64` on an appropriate build machine. A matching cross compiler can also be supplied through `CC`.

To cross-compile on macOS with Zig:

```sh
GO_BIN=/absolute/path/to/go \
ZIG_BIN=/absolute/path/to/zig \
./scripts/build-linux.sh amd64
```

The Zig target uses GNU libc 2.17 as the compatibility baseline. The script writes the shared library, a single-library ZIP, and checksums under `build/linux_amd64/` or `build/linux_arm64/`. The 0.1.1 archive contains `gemini-desktop-v0.1.1-preview.so` at its root; the standalone `gemini-desktop.so` is also kept for manual installation. The host installer accepts this versioned name and installs a new version path rather than overwriting a library that is already loaded. A host restart may still be needed to activate the update. Build caches stay under `build/cache/` unless overridden. These are build targets, not claims that both platforms have been tested in a running host.

For a local macOS development build:

```sh
go build -buildmode=c-shared -o build/gemini-desktop.dylib ./cmd/plugin
```

Use a consistent macOS SDK and compiler if the installed toolchain reports SDK linker errors. There is no need to modify the system SDK.

## Install and configure

### Personal plugin store

The release is distributed through the same personal registry as **API Key Manager — Miftah**:

```text
https://raw.githubusercontent.com/twuijri/cpa-plugin-api-key-manager/main/registry.json
```

Add that registry URL to the host's plugin-store sources if it is not already configured. After the Gemini Desktop release and registry entry are published, open **Plugin Store → Refresh**, search for **Gemini Desktop Preview**, and install its matching platform package. The plugin store page on the current server is [proxy.i3u.us/management.html#/plugin-store](https://proxy.i3u.us/management.html#/plugin-store). This personal registry is separate from the official router-for-me store.

The repository URL alone is not the store source: use the complete `registry.json` URL above. The published binary for this preview targets Linux AMD64 with GNU libc; it does not target Alpine/musl. Keep the other installed plugins in place.

### Manual installation

1. Back up the existing CLIProxyAPI configuration. Keep the current working provider credentials intact.
2. Copy the library for the server's operating system and architecture into its **configured plugin directory**. On Linux the filename must be `gemini-desktop.so`. With Docker, place it in the persistent volume mounted at that directory, so recreating the container does not remove it.
3. Merge the following provider entry into the existing configuration. Preserve the existing plugin directory and all other plugin entries.

```yaml
plugins:
  enabled: true
  configs:
    gemini-desktop:
      enabled: true
      callback_port: 51123
      proxy_url: ""
```

4. Restart the CLIProxyAPI service when its plugin loader requires it. Confirm that **Gemini Desktop Preview** is registered before adding a credential.

`proxy_url` controls the provider's native gRPC connection. It accepts the host SDK's HTTP CONNECT, HTTPS CONNECT, or SOCKS proxy formats. When empty it inherits the top-level `proxy-url`; an account's saved `proxy_url` takes precedence. OAuth token exchanges use the host HTTP client. HTTP/2 TLS access to `robinfrontend-pa.googleapis.com:443` must be available from the server.

The native C ABI is **1**, and the RPC schema is **6**. The host bridge is adapted from the [Gemini CLI plugin's C ABI bridge](https://github.com/router-for-me/cpa-plugin-gemini-cli); the provider authentication and execution path here are for Gemini Desktop. See [CLIProxyAPI's plugin examples](https://github.com/router-for-me/CLIProxyAPI/tree/v8.0.41/examples/plugin) for the host contract.

## Sign in without copying cookies

Select `gemini-desktop` in the host OAuth panel and open the generated Google sign-in URL. The Google page identifies the client as **Gemini**. Sign in with the account that works in Gemini Desktop.

The current preview generates a loopback callback such as `http://localhost:51123/auth`. It does **not** start a callback listener on your computer or the server. When Google redirects to that address, the browser may show an unreachable local page. Copy the **complete final callback URL** into the host panel's manual OAuth callback field for this login. This is a one-time authorization result, not a browser cookie. Do not paste that URL into chat or publish it.

The pending login expires after ten minutes and is consumed once. Complete it before restarting or reconfiguring the plugin. After a successful login the host stores the provider credential and renews the session automatically. If Google revokes the grant, sign in again.

A credential exported by a local helper must use `"type": "gemini-desktop"` and be imported through the host's credential controls. Account tokens must be kept outside the source tree and release archives. The embedded native application client configuration is shared application configuration; it is not a user's account token.

### Account labels and upgrading from 0.1.0-preview

Version **0.1.1-preview** retrieves the verified account email from Google's user-info service. New logins show that email as the credential label and use a filename such as `gemini-desktop-person@example.com.json`.

Existing credential files keep their original filename and host ID. A legacy credential without an email acquires its account label on a subsequent refresh when Google user-info is available; no manual rename or fresh login is required for that upgrade. If the email lookup temporarily fails after a successful token refresh, the refreshed credential is still persisted, including any rotated refresh token, and the email lookup can be retried on a later refresh. A new login must identify a verified account before it is accepted.

## Test

Use the same proxy base URL and client key already used for your CLIProxyAPI instance. First confirm that `/v1/models` includes `gemini-desktop-auto` after a valid Desktop credential has been loaded.

```sh
curl "$PROXY_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $PROXY_CLIENT_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gemini-desktop-auto","messages":[{"role":"user","content":"Reply with exactly GEMINI_DESKTOP_OK"}]}'
```

Repeat with `"stream": true` to check incremental output. A successful streaming response ends with `data: [DONE]` and must not repeat cumulative snapshots. Do not send coding-agent requests with `tools` to this text-only preview.

Typical errors:

| Status | Meaning |
| --- | --- |
| 400 | Unsupported input or native request format rejected. |
| 401 | Expired, missing, or revoked authentication; refresh or sign in again. |
| 403 | Google denied the request for the account. |
| 429 | Account quota or rate limit reached. |
| 501 | An unimplemented API operation, such as exact token counting. |
| 502 | Native service/transport failure, unsupported response framing, or no text response. |

## Rollback

Disable only `plugins.configs.gemini-desktop.enabled`, then restart the service if needed. To remove the preview, move its library out of the configured plugin directory and restart. Preserve its credential file until you decide whether to retain or revoke the Google grant. Other providers and their credentials do not need to be changed.

## Source and licensing

MIT. See [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.txt](THIRD_PARTY_NOTICES.txt). Source and release assets are hosted alongside Miftah in [twuijri/cpa-plugin-api-key-manager](https://github.com/twuijri/cpa-plugin-api-key-manager). Distribution through that personal registry is not an official router-for-me plugin-store listing.
