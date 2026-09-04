package bridge

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestModelDiscoveryRestricted(t *testing.T) {
	a, raw := setup(t)
	req := Request{Method: "GET", Path: ModelsResourcePath, Headers: http.Header{"Authorization": {"Bearer " + raw}}}
	before := len(a.Store.Snapshot().Entries)
	res := a.Manage(req)
	var got struct {
		Object string
		Data   []struct{ ID string }
	}
	if res.StatusCode != 200 || json.Unmarshal(res.Body, &got) != nil || got.Object != "list" || len(got.Data) != 1 || got.Data[0].ID != "work" {
		t.Fatal(res, string(res.Body))
	}
	if strings.Contains(string(res.Body), raw) || strings.Contains(string(res.Body), "backup") || len(a.Store.Snapshot().Entries) != before {
		t.Fatal("leak or accounting mutation")
	}
	if res.Headers.Get("Cache-Control") != "no-store" {
		t.Fatal("cache must be disabled")
	}
	for _, secret := range []string{"", "native-master", "mf_invalid"} {
		req.Headers.Set("Authorization", "Bearer "+secret)
		if a.Manage(req).StatusCode != 401 {
			t.Fatal("invalid accepted")
		}
	}
	req.Headers.Set("Authorization", "Bearer "+raw)
	req.Method = "POST"
	if a.Manage(req).StatusCode != 405 {
		t.Fatal("method")
	}
	req.Method = "GET"
	k := a.Store.Snapshot().Keys[0]
	k.Enabled = false
	if err := a.Store.Update(k, a.Store.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	if a.Manage(req).StatusCode != 401 {
		t.Fatal("disabled accepted")
	}
	k.Enabled = true
	k.ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if err := a.Store.Update(k, a.Store.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	if a.Manage(req).StatusCode != 401 {
		t.Fatal("expired accepted")
	}
}
