package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestSelectedModeEncodesNativeTemplateCategoryAndVariant(t *testing.T) {
	selected := NativeModel{Title: "4 Argon", Category: 8, VariantID: "native-variant"}
	frames := requestFramesSelected("hello", selected)
	templates, err := nested(frames[0], 1000, 1001, 1)
	if err != nil || len(templates) != 1 {
		t.Fatalf("missing native generate template: %v", err)
	}
	fields, err := parseWire(templates[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields[0].Number != 47 || fields[0].Integer != 8 || fields[1].Number != 66 || string(fields[1].Bytes) != "native-variant" {
		t.Fatalf("selected mode was not encoded using native selection fields: %+v", fields)
	}
	auto, err := nested(requestFrames("hello")[0], 1000, 1001, 1)
	if err != nil || len(auto) != 1 || len(auto[0]) != 0 {
		t.Fatal("legacy auto selection changed")
	}
}

func requestWithNativeModels(t *testing.T, models ...NativeModel) pluginapi.ExecutorRequest {
	t.Helper()
	r := credentialRequest()
	var c Credential
	if err := json.Unmarshal(r.StorageJSON, &c); err != nil {
		t.Fatal(err)
	}
	c.Models = models
	r.StorageJSON, _ = json.Marshal(c)
	return r
}

func TestSelectedModelExecutionAndMetadataUseAccountNames(t *testing.T) {
	p := newTestPlugin(t)
	selected := NativeModel{ConfigID: "native-id", Title: "3.8 Flash", Category: 1}
	r := requestWithNativeModels(t, selected)
	r.Model = selected.Alias()
	p.generate = func(context.Context, string, string, string, func(string) error) error {
		t.Fatal("selected model silently fell back to auto")
		return nil
	}
	selections := 0
	p.generateSelected = func(_ context.Context, _, _, _ string, got NativeModel, emit func(string) error) error {
		if got != selected {
			t.Fatalf("wrong native model passed to transport: %+v", got)
		}
		selections++
		return emit("مرحبا")
	}
	metadata, err := p.ModelsForAuth(context.Background(), pluginapi.AuthModelRequest{StorageJSON: r.StorageJSON})
	if err != nil || len(metadata.Models) != 2 || metadata.Models[0].DisplayName != selected.Title || metadata.Models[0].ID != r.Model || metadata.Models[1].ID != Model {
		t.Fatalf("real model name missing from account model registration: %v", err)
	}
	response, err := p.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(response.Payload, &body) != nil || body.Model != r.Model {
		t.Fatal("completion did not retain selected model ID")
	}
	stream, err := p.ExecuteStream(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	for chunk := range stream.Chunks {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		if json.Unmarshal(chunk.Payload, &body) != nil || body.Model != r.Model {
			t.Fatal("stream did not retain selected model ID")
		}
	}
	if selections != 2 {
		t.Fatalf("native selection dispatch count = %d", selections)
	}
	r.Model = "gemini-desktop-unknown"
	if _, err := p.Execute(context.Background(), r); err == nil {
		t.Fatal("unavailable account model accepted")
	}
	r = credentialRequest()
	r.Model = selected.Alias()
	if _, err := p.Execute(context.Background(), r); err == nil || !strings.Contains(err.Error(), "refresh") {
		t.Fatal("missing account model list did not request refresh")
	}
}

func TestModelDiscoveryRefreshPersistsTokensAndCachedModesOnFailure(t *testing.T) {
	p := newTestPlugin(t)
	selected := NativeModel{Title: "3.1 Pro", Category: 3}
	r := requestWithNativeModels(t, selected)
	p.discover = func(_ context.Context, token, proxy string) ([]NativeModel, error) {
		if token != "renewed-access" || proxy != "socks5://127.0.0.1:1080" {
			t.Fatal("discovery did not use refreshed access token and host proxy")
		}
		return nil, errors.New("temporary native status outage")
	}
	client := fakeHTTP{func(req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		if req.URL == userinfoEndpoint {
			return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"email":"person@example.com","verified_email":true}`)}, nil
		}
		return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"access_token":"renewed-access","refresh_token":"rotated-refresh","expires_in":3600}`)}, nil
	}}
	result, err := p.RefreshAuth(context.Background(), pluginapi.AuthRefreshRequest{AuthID: "existing.json", StorageJSON: r.StorageJSON, HTTPClient: client, Host: pluginapi.HostConfigSummary{ProxyURL: "socks5://127.0.0.1:1080"}})
	if err != nil {
		t.Fatal(err)
	}
	var saved Credential
	if err := json.Unmarshal(result.Auth.StorageJSON, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.RefreshToken != "rotated-refresh" || len(saved.Models) != 1 || saved.Models[0] != selected || result.Auth.ID != "existing.json" {
		t.Fatal("model discovery failure discarded rotated credential or cached models")
	}
	p.discover = func(context.Context, string, string) ([]NativeModel, error) {
		return []NativeModel{{Title: "4 Argon", Category: 8}}, nil
	}
	p.discoverCredentialModels(context.Background(), &saved)
	if len(saved.Models) != 1 || saved.Models[0].Category != 8 {
		t.Fatal("successful account discovery did not replace cached modes")
	}
}
