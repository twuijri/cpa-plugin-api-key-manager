// Package bridge implements the CPA JSON contract without importing provider code.
package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"miftah.local/plugin/internal/core"
	"miftah.local/plugin/internal/ui"
)

type Call func(string, any, any) error
type App struct {
	Store     *core.Store
	Host      Call
	streams   sync.WaitGroup
	lifeMu    sync.Mutex
	closing   bool
	calls     sync.WaitGroup
	streamIDs map[string]string
}

// Quiesce stops admission, closes active stream bridges, then waits before unload.
func (a *App) Quiesce() {
	a.lifeMu.Lock()
	a.closing = true
	ids := map[string]string{}
	for k, v := range a.streamIDs {
		ids[k] = v
	}
	a.lifeMu.Unlock()
	if a.Host != nil {
		for downstream, upstream := range ids {
			_ = a.Host("host.model.stream_close", map[string]string{"stream_id": upstream}, nil)
			_ = a.Host("host.stream.close", map[string]string{"stream_id": downstream, "error": "plugin shutting down"}, nil)
		}
	}
	a.calls.Wait()
	a.streams.Wait()
}

type Request struct {
	Method, Path, RequestedModel, Model, SourceFormat, Format, Alt string
	Headers                                                        http.Header
	Query                                                          map[string][]string
	Body, Payload, OriginalRequest                                 []byte
	Metadata                                                       map[string]any
	Stream                                                         bool
	StreamID                                                       string `json:"stream_id"`
	Callback                                                       string `json:"host_callback_id"`
}
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}
type RPCError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"http_status"`
}

func (e *RPCError) Error() string { return e.Message }
func (a *App) Resume()            { a.lifeMu.Lock(); a.closing = false; a.lifeMu.Unlock() }
func token(h http.Header) string {
	for k, v := range h {
		if strings.EqualFold(k, "Authorization") && len(v) > 0 {
			p := strings.SplitN(v[0], " ", 2)
			if len(p) == 2 && strings.EqualFold(p[0], "Bearer") {
				return strings.TrimSpace(p[1])
			}
		}
	}
	return ""
}
func (a *App) Handle(method string, raw []byte) (any, error) {
	var r Request
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, errors.New("invalid request")
		}
	}
	switch method {
	case "frontend_auth.identifier", "executor.identifier":
		return map[string]string{"identifier": "miftah"}, nil
	case "frontend_auth.authenticate":
		secret := token(r.Headers)
		if !strings.HasPrefix(secret, core.Prefix) {
			return map[string]bool{"Authenticated": false}, nil
		}
		k, err := a.Store.Authenticate(secret)
		if err != nil {
			return map[string]bool{"Authenticated": false}, nil
		}
		// Allow authenticated discovery on the official host's global catalog.
		// Execution still enforces the per-key allowlist; discovery grants no access.
		discovery := r.Method == "GET" && r.Path == "/v1/models"
		if !discovery && (r.Method != "POST" || (r.Path != "/v1/chat/completions" && r.Path != "/v1/responses" && r.Path != "/v1/messages")) {
			return map[string]bool{"Authenticated": false}, nil
		}
		return map[string]any{"Authenticated": true, "Principal": "miftah:" + k.ID, "Metadata": map[string]string{"virtual_key_id": k.ID}}, nil
	case "model.route":
		return map[string]any{"Handled": strings.HasPrefix(token(r.Headers), core.Prefix), "TargetKind": "self"}, nil
	case "executor.execute", "executor.execute_stream":
		a.lifeMu.Lock()
		if a.closing {
			a.lifeMu.Unlock()
			return nil, &RPCError{"unavailable", "plugin shutting down", 503}
		}
		a.calls.Add(1)
		a.lifeMu.Unlock()
		defer a.calls.Done()
		return a.execute(r, method == "executor.execute_stream")
	case "executor.count_tokens", "executor.http_request":
		return nil, &RPCError{"unsupported", "endpoint not supported for virtual keys", 403}
	case "management.register":
		routes := []map[string]string{}
		for _, x := range [][2]string{{"GET", "state"}, {"GET", "prices"}, {"PUT", "catalog-prices"}, {"POST", "keys"}, {"PUT", "keys"}, {"POST", "rotate"}, {"PUT", "routes"}} {
			routes = append(routes, map[string]string{"Method": x[0], "Path": "miftah/" + x[1]})
		}
		resources := []map[string]string{{"Path": "/models"}, {"Path": "/console", "Menu": "API Key Manager — مفتاح"}, {"Path": "/app.js"}, {"Path": "/direct.js"}, {"Path": "/pricing.js"}, {"Path": "/picker.js"}, {"Path": "/i18n.js"}, {"Path": "/theme.js"}, {"Path": "/style.css"}, {"Path": "/picker.css"}, {"Path": "/theme.css"}}
		return map[string]any{"Routes": routes, "Resources": resources}, nil
	case "management.handle":
		return a.Manage(r), nil
	default:
		return nil, errors.New("unsupported method")
	}
}

// CPA requires a nonempty repository field even for private local builds.
// Replace via -ldflags when the owner chooses an actual publication URL.
var Repository = "https://github.com/twuijri/cpa-plugin-api-key-manager"
var Version = "0.1.4"

func Registration() any {
	return map[string]any{"schema_version": 4, "metadata": map[string]any{"Name": "API Key Manager — مفتاح", "Version": Version, "Author": "Abdulaziz", "GitHubRepository": Repository}, "capabilities": map[string]any{"frontend_auth_provider": true, "frontend_auth_provider_exclusive": false, "model_router": true, "executor": true, "executor_model_scope": "both", "executor_input_formats": []string{"openai", "chat-completions", "openai-response", "responses", "claude"}, "executor_output_formats": []string{"openai", "chat-completions", "openai-response", "responses", "claude"}, "management_api": true}}
}
func reply(status int, v any) Response {
	b, _ := json.Marshal(v)
	return Response{status, http.Header{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}, "X-Content-Type-Options": {"nosniff"}}, b}
}
func decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("only one JSON object is allowed")
	}
	return nil
}
func (a *App) Manage(r Request) Response {
	if r.Path == ModelsResourcePath {
		return a.models(r)
	}
	if strings.HasPrefix(r.Path, "/v0/resource/plugins/miftah/") {
		name := strings.TrimPrefix(r.Path, "/v0/resource/plugins/miftah/")
		b, ct, ok := ui.Asset(name)
		if !ok {
			return reply(404, map[string]string{"error": "not found"})
		}
		return Response{200, http.Header{"Content-Type": {ct}, "Cache-Control": {"no-store"}, "X-Content-Type-Options": {"nosniff"}, "Content-Security-Policy": {"default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'"}, "Referrer-Policy": {"no-referrer"}}, b}
	}
	path := strings.TrimPrefix(r.Path, "/v0/management/miftah/")
	var err error
	switch {
	case r.Method == "GET" && path == "prices":
		return referencePrices()
	case r.Method == "PUT" && path == "catalog-prices":
		var v struct {
			Models   []string              `json:"models"`
			Prices   map[string]core.Price `json:"prices"`
			Revision int64                 `json:"revision"`
		}
		if err = decode(r.Body, &v); err == nil {
			err = a.Store.SyncDirectModels(v.Models, v.Prices, v.Revision)
		}
	case r.Method == "GET" && path == "state":
		return reply(200, a.Store.Snapshot())
	case r.Method == "POST" && path == "keys":
		var v struct {
			core.Key
			DirectPolicies []core.Route `json:"direct_policies"`
		}
		if err = decode(r.Body, &v); err == nil {
			var secret string
			var k core.Key
			k, secret, err = a.Store.CreateWithPolicies(v.Key, v.DirectPolicies)
			if err == nil {
				return reply(201, map[string]any{"key": k, "secret": secret})
			}
		}
	case r.Method == "PUT" && path == "keys":
		var v struct {
			Key            core.Key     `json:"key"`
			Revision       int64        `json:"revision"`
			DirectPolicies []core.Route `json:"direct_policies"`
		}
		if err = decode(r.Body, &v); err == nil {
			err = a.Store.UpdateWithPolicies(v.Key, v.Revision, v.DirectPolicies)
		}
	case r.Method == "POST" && path == "rotate":
		var v struct {
			ID string `json:"id"`
		}
		if err = decode(r.Body, &v); err == nil {
			var secret string
			secret, err = a.Store.Rotate(v.ID)
			if err == nil {
				return reply(200, map[string]string{"secret": secret})
			}
		}
	case r.Method == "PUT" && path == "routes":
		var v struct {
			Route    core.Route `json:"route"`
			Revision int64      `json:"revision"`
		}
		if err = decode(r.Body, &v); err == nil {
			err = a.Store.PutRoute(v.Route, v.Revision)
		}
	default:
		return reply(404, map[string]string{"error": "not found"})
	}
	if err != nil {
		status := 400
		if errors.Is(err, core.ErrConflict) {
			status = 409
		}
		return reply(status, map[string]string{"error": err.Error()})
	}
	return reply(200, map[string]bool{"ok": true})
}

type hostResponse struct {
	Status   int         `json:"status_code"`
	Headers  http.Header `json:"headers"`
	Body     []byte      `json:"body"`
	StreamID string      `json:"stream_id"`
}

func usage(b []byte) (int64, int64, bool) {
	var v map[string]json.RawMessage
	if json.Unmarshal(b, &v) != nil {
		return 0, 0, false
	}
	if resp, ok := v["response"]; ok {
		return usage(resp)
	}
	var u map[string]json.RawMessage
	if json.Unmarshal(v["usage"], &u) != nil || u == nil {
		return 0, 0, false
	}
	read := func(primary, alternate string) (int64, bool) {
		raw, ok := u[primary]
		if !ok {
			raw, ok = u[alternate]
		}
		var value int64
		if !ok || string(raw) == "null" || json.Unmarshal(raw, &value) != nil || value < 0 {
			return 0, false
		}
		return value, true
	}
	in, ok := read("input_tokens", "prompt_tokens")
	out, ok2 := read("output_tokens", "completion_tokens")
	return in, out, ok && ok2
}
func (a *App) execute(r Request, stream bool) (any, error) {
	if a.Host == nil {
		return nil, &RPCError{"unavailable", "host executor unavailable", 503}
	}
	body := r.OriginalRequest
	if len(body) == 0 {
		body = r.Payload
	}
	var data map[string]json.RawMessage
	if len(body) > 4<<20 || json.Unmarshal(body, &data) != nil {
		return nil, &RPCError{"invalid", "invalid request body", 400}
	}
	var cap int64
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if b, ok := data[key]; ok {
			var n int64
			if json.Unmarshal(b, &n) != nil || n <= 0 {
				return nil, &RPCError{"invalid", "invalid output cap", 400}
			}
			if n > cap {
				cap = n
			}
		}
	}
	// Multimodal payloads require a provider-aware estimator, not a text byte estimate.
	for _, marker := range []string{"\"image_url\"", "\"input_image\"", "\"input_audio\"", "\"input_file\"", "\"document\""} {
		if bytes.Contains(body, []byte(marker)) {
			return nil, &RPCError{"unsupported", "multimodal requests not supported in this version", 400}
		}
	}
	reservation, route, err := a.Store.Reserve(token(r.Headers), r.Model, len(body), cap)
	if err != nil {
		return nil, &RPCError{"policy_denied", err.Error(), 403}
	}
	attempts := 0
	var result hostResponse
	var selected string
	for _, target := range route.Targets {
		attempts++
		selected = target
		data["model"], _ = json.Marshal(target)
		data["stream"], _ = json.Marshal(stream)
		format := r.SourceFormat
		if format == "" {
			format = r.Format
		}
		if stream && (format == "openai" || format == "chat-completions" || format == "") {
			// Request authoritative usage even when the client omitted it.
			options := map[string]json.RawMessage{}
			_ = json.Unmarshal(data["stream_options"], &options)
			if options == nil {
				options = map[string]json.RawMessage{}
			}
			options["include_usage"] = json.RawMessage("true")
			data["stream_options"], _ = json.Marshal(options)
		}
		out, _ := json.Marshal(data)
		method := "host.model.execute"
		if stream {
			method = "host.model.execute_stream"
		}
		req := map[string]any{"host_callback_id": r.Callback, "entry_protocol": format, "exit_protocol": format, "model": target, "stream": stream, "body": out, "headers": http.Header{}, "query": map[string][]string{}, "alt": r.Alt}
		result = hostResponse{}
		err = a.Host(method, req, &result)
		status := result.Status
		if e, ok := err.(*RPCError); ok {
			status = e.HTTPStatus
			if status == 0 {
				var v struct {
					Error struct {
						Code int `json:"code"`
					} `json:"error"`
				}
				if json.Unmarshal([]byte(e.Message), &v) == nil {
					status = v.Error.Code
				}
			}
		}
		if err == nil && status >= 200 && status < 300 {
			break
		}
		retry := status == 0 && err != nil && route.RetryUnknown
		for _, n := range route.RetryStatuses {
			if n == status {
				retry = true
			}
		}
		if !retry {
			break
		}
	}
	if err != nil || result.Status < 200 || result.Status >= 300 {
		_ = a.Store.Finish(reservation.ID, selected, 0, 0, false, false, attempts, route)
		return nil, &RPCError{"upstream_failed", "upstream failed; reservation retained for reconciliation", 502}
	}
	if !stream {
		in, out, known := usage(result.Body)
		if err = a.Store.Finish(reservation.ID, selected, in, out, known, true, attempts, route); err != nil {
			return nil, &RPCError{"ledger_failed", "cannot persist settlement", 503}
		}
		return map[string]any{"Payload": result.Body, "Headers": http.Header{}}, nil
	}
	if r.StreamID == "" || result.StreamID == "" {
		_ = a.Store.Finish(reservation.ID, selected, 0, 0, false, false, attempts, route)
		return nil, &RPCError{"stream_failed", "stream bridge unavailable", 502}
	}
	a.lifeMu.Lock()
	if a.closing {
		a.lifeMu.Unlock()
		_ = a.Host("host.model.stream_close", map[string]string{"stream_id": result.StreamID}, nil)
		_ = a.Store.Finish(reservation.ID, selected, 0, 0, false, false, attempts, route)
		return nil, &RPCError{"unavailable", "plugin shutting down", 503}
	}
	if a.streamIDs == nil {
		a.streamIDs = map[string]string{}
	}
	a.streamIDs[r.StreamID] = result.StreamID
	a.streams.Add(1)
	a.lifeMu.Unlock()
	go func() {
		defer a.streams.Done()
		defer func() { a.lifeMu.Lock(); delete(a.streamIDs, r.StreamID); a.lifeMu.Unlock() }()
		var in, out int64
		known, success := false, true
		pending := []byte{}
		defer func() {
			_ = a.Store.Finish(reservation.ID, selected, in, out, known, success, attempts, route)
			_ = a.Host("host.model.stream_close", map[string]string{"stream_id": result.StreamID}, nil)
			_ = a.Host("host.stream.close", map[string]string{"stream_id": r.StreamID}, nil)
		}()
		for {
			var c struct {
				Payload []byte `json:"payload"`
				Done    bool   `json:"done"`
				Error   string `json:"error"`
			}
			err := a.Host("host.model.stream_read", map[string]string{"stream_id": result.StreamID}, &c)
			if err != nil || c.Error != "" {
				success = false
				_ = a.Host("host.stream.emit", map[string]string{"stream_id": r.StreamID, "error": "upstream stream interrupted"}, nil)
				return
			}
			// Host callbacks may return a JSON event without SSE framing/newlines.
			frame := bytes.TrimSpace(bytes.TrimPrefix(bytes.TrimSpace(c.Payload), []byte("data:")))
			if len(pending) == 0 && json.Valid(frame) {
				if x, y, ok := usage(frame); ok {
					in, out, known = x, y, true
				}
			} else {
				pending = append(pending, c.Payload...)
			}
			if len(pending) > 2<<20 {
				pending = nil
			}
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				line := bytes.TrimSpace(pending[:i])
				pending = pending[i+1:]
				line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
				if x, y, ok := usage(line); ok {
					in, out, known = x, y, true
				}
			}
			if len(c.Payload) > 0 {
				if a.Host("host.stream.emit", map[string]any{"stream_id": r.StreamID, "payload": c.Payload}, nil) != nil {
					success = false
					return
				}
			}
			if c.Done {
				return
			}
		}
	}()
	return map[string]any{"headers": http.Header{}}, nil
}
