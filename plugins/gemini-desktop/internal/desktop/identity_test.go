package desktop

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestAccountEmailPersistsAndNamesCredential(t *testing.T) {
	c := Credential{Type: Provider, AccessToken: "test", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour)}
	client := fakeHTTP{func(r pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"email":"person@example.com","verified_email":true}`)}, nil
	}}
	c, err := identify(context.Background(), client, c)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Auth(c)
	if err != nil {
		t.Fatal(err)
	}
	if a.FileName != "gemini-desktop-person@example.com.json" || a.Metadata["email"] != "person@example.com" || a.Label != "person@example.com" {
		t.Fatal("account identity missing")
	}
	var restored Credential
	if json.Unmarshal(a.StorageJSON, &restored) != nil || restored.Email != c.Email {
		t.Fatal("email not persisted")
	}
	// Existing files retain their host ID while gaining a meaningful account label.
	plugin, _ := Build(nil)
	p := plugin.Capabilities.AuthProvider
	parsed, err := p.ParseAuth(context.Background(), pluginapi.AuthParseRequest{RawJSON: a.StorageJSON, FileName: "gemini-desktop-old.json"})
	if err != nil || parsed.Auth.ID != "gemini-desktop-old.json" || parsed.Auth.Metadata["email"] != c.Email {
		t.Fatal("existing account migration changed its ID")
	}
}

func TestUnverifiedOrUnsafeEmailRejected(t *testing.T) {
	for _, body := range []string{`{"email":"person@example.com","verified_email":false}`, `{"email":"../../person@example.com","verified_email":true}`, `{"email":"not-an-email","verified_email":true}`} {
		_, err := identify(context.Background(), fakeHTTP{func(pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
			return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(body)}, nil
		}}, Credential{AccessToken: "test"})
		if err == nil {
			t.Fatal("invalid account profile accepted")
		}
	}
}

func TestLegacyRefreshPersistsRotatedTokenWhenIdentityLookupFails(t *testing.T) {
	p := newTestPlugin(t)
	raw, err := json.Marshal(Credential{Type: Provider, RefreshToken: "previous-refresh"})
	if err != nil {
		t.Fatal(err)
	}
	client := fakeHTTP{func(r pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		if r.URL == userinfoEndpoint {
			return pluginapi.HTTPResponse{StatusCode: 503, Body: []byte(`{"error":"temporarily_unavailable"}`)}, nil
		}
		values, errParse := url.ParseQuery(string(r.Body))
		if errParse != nil || r.URL != tokenEndpoint || values.Get("refresh_token") != "previous-refresh" {
			t.Fatal("unexpected refresh request")
		}
		return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"access_token":"renewed-access","refresh_token":"rotated-refresh","expires_in":3600}`)}, nil
	}}
	response, err := p.RefreshAuth(context.Background(), pluginapi.AuthRefreshRequest{AuthID: "existing-account.json", StorageJSON: raw, HTTPClient: client})
	if err != nil {
		t.Fatalf("identity lookup failure discarded a successful refresh: %v", err)
	}
	var stored Credential
	if err = json.Unmarshal(response.Auth.StorageJSON, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.RefreshToken != "rotated-refresh" || stored.AccessToken != "renewed-access" || stored.Email != "" {
		t.Fatal("host did not receive the rotated credential without an invented identity")
	}
	if response.Auth.ID != "existing-account.json" || response.Auth.FileName != "existing-account.json" {
		t.Fatal("legacy refresh renamed the existing credential")
	}
}

func TestNewLoginRequiresVerifiedIdentityAfterSuccessfulRefresh(t *testing.T) {
	login, err := Start("http://localhost:51123/auth")
	if err != nil {
		t.Fatal(err)
	}
	client := fakeHTTP{func(r pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		if r.URL == userinfoEndpoint {
			return pluginapi.HTTPResponse{StatusCode: 503}, nil
		}
		return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"access_token":"test-access","refresh_token":"test-refresh","expires_in":3600}`)}, nil
	}}
	_, err = Exchange(context.Background(), client, login, "authorization-code", login.State)
	if err == nil {
		t.Fatal("new login succeeded without a verified account identity")
	}
}
