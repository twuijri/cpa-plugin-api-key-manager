package desktop

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func modernModelFixture(category uint64, title, variant string, disabled bool) []byte {
	b := number(3, category)
	b = append(b, message(14, []byte(title))...)
	if variant != "" {
		b = append(b, message(5, []byte(variant))...)
	}
	if disabled {
		b = append(b, number(4, 1)...)
	}
	return message(2, message(1, b))
}
func legacyModelFixture(category uint64, id, title string) []byte {
	b := message(1, []byte(id))
	b = append(b, number(23, category)...)
	b = append(b, message(19, []byte(title))...)
	return message(17, b)
}
func TestNativeModelDiscoveryPolicyAndLegacyIDs(t *testing.T) {
	response := legacyModelFixture(6, "flash-lite-config", "Old label")
	response = append(response, legacyModelFixture(8, "argon-config", "4 Argon")...)
	policy := modernModelFixture(6, "3.5 Flash-Lite", "", false)
	policy = append(policy, modernModelFixture(8, "4 Argon", "", true)...)
	policy = append(policy, modernModelFixture(999, "Future Native Mode", "variant-z", false)...)
	response = append(response, message(34, policy)...)
	models, err := parseNativeModels(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models", len(models))
	}
	if models[0].ConfigID != "flash-lite-config" || models[0].Title != "3.5 Flash-Lite" || models[0].Alias() != "gemini-desktop-3.5-flash-lite" {
		t.Fatalf("unexpected model %#v", models[0])
	}
	if models[1].Category != 999 || models[1].VariantID != "variant-z" {
		t.Fatalf("future model lost: %#v", models[1])
	}
}
func TestNativeModelDiscoveryLegacyAndDisabledPolicy(t *testing.T) {
	old := legacyModelFixture(3, "pro", "3.1 Pro")
	models, err := parseNativeModels(old)
	if err != nil || len(models) != 1 {
		t.Fatalf("legacy: %#v %v", models, err)
	}
	response := append(old, message(34, modernModelFixture(3, "3.1 Pro", "", true))...)
	models, err = parseNativeModels(response)
	if err != nil || len(models) != 0 {
		t.Fatalf("disabled resurrected: %#v %v", models, err)
	}
}
func TestNativeModelDiscoveryRejectsMalformedMetadata(t *testing.T) {
	cases := map[string][]byte{
		"bad policy config kind": message(34, number(2, 1)),
		"bad policy mode kind":   message(34, message(2, number(1, 1))),
		"truncated":              {0x8a},
		"zero category":          message(34, modernModelFixture(0, "Mode", "", false)),
		"title long":             message(34, modernModelFixture(1, strings.Repeat("x", 129), "", false)),
		"title controls":         message(34, modernModelFixture(1, "Mode\nInjected", "", false)),
		"title bidi control":     message(34, modernModelFixture(1, "Mode\u202e", "", false)),
		"title invalid utf8":     message(34, modernModelFixture(1, string([]byte{0xff}), "", false)),
		"variant long":           message(34, modernModelFixture(1, "Mode", strings.Repeat("v", 257), false)),
		"category overflow":      message(34, modernModelFixture(1<<33, "Mode", "", false)),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseNativeModels(b); err == nil {
				t.Fatal("accepted invalid metadata")
			}
		})
	}
	var policy []byte
	for i := 1; i <= 65; i++ {
		policy = append(policy, modernModelFixture(uint64(i), "Mode", "", false)...)
	}
	if _, err := parseNativeModels(message(34, policy)); err == nil {
		t.Fatal("accepted more than 64 modes")
	}
}

type discoveryConnection struct {
	t        *testing.T
	response []byte
}

func (c discoveryConnection) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	if method != userStatusMethod {
		c.t.Fatalf("method %s", method)
	}
	if b, ok := args.([]byte); !ok || len(b) != 0 {
		c.t.Fatal("status request is not empty")
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer test-token" {
		c.t.Fatal("missing authorization")
	}
	*(reply.(*[]byte)) = c.response
	return nil
}
func (c discoveryConnection) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	panic("unexpected streaming request")
}
func TestDiscoverModelsUsesNativeUnaryStatus(t *testing.T) {
	models, err := discoverModelsOnConnection(context.Background(), discoveryConnection{t, message(34, modernModelFixture(1, "3.8 Flash", "", false))}, "test-token")
	if err != nil || len(models) != 1 || models[0].Alias() != "gemini-desktop-3.8-flash" {
		t.Fatalf("discovery %#v %v", models, err)
	}
}

func TestNativeModelAliasesRejectAmbiguousSelections(t *testing.T) {
	policy := modernModelFixture(1, "Same title", "", false)
	policy = append(policy, modernModelFixture(3, "Same title", "", false)...)
	if _, err := parseNativeModels(message(34, policy)); err == nil {
		t.Fatal("accepted ambiguous model alias")
	}
	policy = modernModelFixture(1, "Same title", "a", false)
	policy = append(policy, modernModelFixture(1, "Same title", "b", false)...)
	if _, err := parseNativeModels(message(34, policy)); err == nil {
		t.Fatal("accepted ambiguous variants")
	}
	policy = modernModelFixture(1, "3.8 Flash", "", false)
	policy = append(policy, modernModelFixture(1, "3.8 Flash", "", false)...)
	models, err := parseNativeModels(message(34, policy))
	if err != nil || len(models) != 1 {
		t.Fatalf("exact duplicates %#v %v", models, err)
	}
}

// Opt-in account metadata verification; no response fixture or credential is retained.
func TestDiscoverModelsLive(t *testing.T) {
	path := os.Getenv("GEMINI_DESKTOP_DISCOVERY_TOKEN_FILE")
	if path == "" {
		t.Skip("live discovery is opt-in")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read credential file")
	}
	var credential struct {
		AccessToken string `json:"access_token"`
	}
	if json.Unmarshal(raw, &credential) != nil || credential.AccessToken == "" {
		t.Fatal("invalid credential file")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	models, err := DiscoverModels(ctx, credential.AccessToken, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("discovered %d native modes", len(models))
	for _, model := range models {
		t.Logf("title=%q category=%d", model.Title, model.Category)
	}
}
