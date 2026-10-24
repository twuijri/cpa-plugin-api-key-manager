# Local hardened build 0.3.0-local.1

Upstream: https://github.com/hex-ci/cpa-plugin-model-playground at commit 72469d5fd0d01cff6682de092cd76f1252f34898.
Original MIT license remains in LICENSE.

This variant replaces the panel with an Arabic chat testing interface. Enter a client API key manually, load models, select a model, and chat with streaming replies. Keys and conversation history remain only in page memory and are lost on reload. Requests use same-origin /v1/models and /v1/chat/completions, with redirects refused. No browser storage, management credentials, API-key lists, URL key parameters, or arbitrary-origin proxy requests are accessed. Output uses textContent only. No Markdown/HTML rendering or remote assets are used.

Removed upstream server /keys helper because it forwarded management authorization to an arbitrary origin query parameter, allowing authenticated SSRF and credential disclosure. Replaced upstream regex-based Markdown sanitizer because raw model output could bypass it with entity-encoded URL schemes or other active markup.

Tradeoffs: plain-text output; no persistent sessions, parameter controls, usage stats, or Markdown. Manual client key entry is required. Chat displays reasoning text when the model provides it. Aborted/failed replies are not committed to conversation history. Changing models keeps conversation history until New conversation is pressed.

Build targets Linux amd64 with glibc 2.17 baseline using Zig, SDK imports updated to CLIProxyAPI/v8 pinned remote module v8.0.41, ABI 1/schema 6. Go unit tests validate resource serving and absence of /keys helper. Native macOS testing requires the older installed MacOSX15.4 SDK because the system linker cannot read MacOSX27 SDK stubs. No deployment or real model invocation performed by this agent.
