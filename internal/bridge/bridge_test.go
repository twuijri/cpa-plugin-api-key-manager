package bridge

import (
	"encoding/json"
	"miftah.local/plugin/internal/core"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) (*App, string) {
	t.Helper()
	s, e := core.Open(filepath.Join(t.TempDir(), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	_ = s.PutRoute(core.Route{Alias: "work", Targets: []string{"primary", "backup"}, RetryStatuses: []int{429, 503}, InputPrice: 1000000, OutputPrice: 1000000, MaxOutput: 1000}, s.Snapshot().Revision)
	_, raw, e := s.Create(core.Key{Name: "test", Models: []string{"work"}})
	if e != nil {
		t.Fatal(e)
	}
	return &App{Store: s}, raw
}
func request(raw string) Request {
	return Request{Model: "work", SourceFormat: "openai", Headers: http.Header{"Authorization": {"Bearer " + raw}}, Payload: []byte(`{"model":"work","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`)}
}
func TestNativeKeysUntouched(t *testing.T) {
	a, _ := setup(t)
	raw, _ := json.Marshal(Request{Headers: http.Header{"Authorization": {"Bearer native-master"}}})
	v, e := a.Handle("frontend_auth.authenticate", raw)
	if e != nil || v.(map[string]bool)["Authenticated"] {
		t.Fatal(v, e)
	}
	v, e = a.Handle("model.route", raw)
	if e != nil || v.(map[string]any)["Handled"].(bool) {
		t.Fatal(v, e)
	}
}
func TestFallback(t *testing.T) {
	a, key := setup(t)
	calls := 0
	a.Host = func(method string, r, out any) error {
		calls++
		req := r.(map[string]any)
		if token(req["headers"].(http.Header)) != "" {
			t.Fatal("secret forwarded")
		}
		if calls == 1 {
			return &RPCError{HTTPStatus: 503, Message: "unavailable"}
		}
		if req["model"] != "backup" {
			t.Fatal(req)
		}
		v := out.(*hostResponse)
		v.Status = 200
		v.Body = []byte(`{"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
		return nil
	}
	if _, e := a.execute(request(key), false); e != nil {
		t.Fatal(e)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestNoRetryAuthenticationErrors(t *testing.T) {
	a, key := setup(t)
	calls := 0
	a.Host = func(string, any, any) error { calls++; return &RPCError{HTTPStatus: 401, Message: "denied"} }
	_, err := a.execute(request(key), false)
	if err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestDisabledKeyRejectedAtExecution(t *testing.T) {
	a, key := setup(t)
	k := a.Store.Snapshot().Keys[0]
	k.Enabled = false
	_ = a.Store.Update(k, a.Store.Snapshot().Revision)
	a.Host = func(string, any, any) error { t.Fatal("upstream called"); return nil }
	if _, e := a.execute(request(key), false); e == nil {
		t.Fatal("disabled key bypass")
	}
}
func TestManagementResourcesNeverContainState(t *testing.T) {
	a, key := setup(t)
	for _, p := range []string{"console", "app.js", "style.css"} {
		r := a.Manage(Request{Path: "/v0/resource/plugins/miftah/" + p})
		if r.StatusCode != 200 || strings.Contains(string(r.Body), key) {
			t.Fatal(p)
		}
		if !strings.Contains(r.Headers.Get("Content-Security-Policy"), "script-src 'self'") {
			t.Fatal("missing CSP")
		}
	}
	r := a.Manage(Request{Path: "/v0/resource/plugins/miftah/../../state.json"})
	if r.StatusCode != 404 {
		t.Fatal(r.StatusCode)
	}
}
func TestUsageParsing(t *testing.T) {
	for _, raw := range []string{`{"usage":{"input_tokens":10,"output_tokens":2}}`, `{"response":{"usage":{"input_tokens":10,"output_tokens":2}}}`, `{"usage":{"prompt_tokens":10,"completion_tokens":2}}`} {
		in, out, ok := usage([]byte(raw))
		if !ok || in != 10 || out != 2 {
			t.Fatal(in, out, ok)
		}
	}
}
