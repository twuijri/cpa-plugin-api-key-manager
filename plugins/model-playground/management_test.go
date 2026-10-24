package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// wireResponse mirrors what okEnvelope serializes: pluginapi.ManagementResponse
// has no field tags, so the wire uses PascalCase keys (StatusCode/Headers/Body).
type wireResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

func decodeWire(t *testing.T, raw []byte) wireResponse {
	t.Helper()
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("envelope decode: %v", err)
	}
	if !env.OK {
		t.Fatalf("envelope not ok: %s", raw)
	}
	var w wireResponse
	if err := json.Unmarshal(env.Result, &w); err != nil {
		t.Fatalf("wire decode: %v", err)
	}
	return w
}

func TestHandleManagement_Panel(t *testing.T) {
	req, _ := json.Marshal(map[string]any{
		"method": "GET",
		"path":   "/v0/resource/plugins/model-playground/panel",
	})
	raw, err := handleManagement(req)
	if err != nil {
		t.Fatalf("handleManagement: %v", err)
	}
	w := decodeWire(t, raw)
	if w.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.StatusCode)
	}
	if !strings.Contains(string(w.Body), "Model Playground") {
		t.Fatalf("panel missing Model Playground title")
	}
}

func TestHandleManagement_UnknownRoute(t *testing.T) {
	req, _ := json.Marshal(map[string]any{
		"method": "GET",
		"path":   "/v0/management/plugins/model-playground/nope",
	})
	raw, err := handleManagement(req)
	if err != nil {
		t.Fatalf("handleManagement: %v", err)
	}
	w := decodeWire(t, raw)
	if w.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.StatusCode)
	}
}

func TestKeysHelperRemoved(t *testing.T) {
	req, _ := json.Marshal(map[string]any{"method": "GET", "path": "/v0/management/plugins/model-playground/keys", "query": map[string][]string{"origin": {"https://example.invalid"}}})
	raw, _ := handleManagement(req)
	if got := decodeWire(t, raw).StatusCode; got != http.StatusNotFound {
		t.Fatalf("status %d", got)
	}
}
