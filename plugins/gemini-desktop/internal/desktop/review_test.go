package desktop

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// CLIProxyAPI passes the per-instance configuration body at plugin.register,
// not the complete host configuration document.
func TestReviewFlatRuntimeConfiguration(t *testing.T) {
	built, err := Build([]byte("enabled: true\npriority: 0\ncallback_port: 57321\nproxy_url: socks5://127.0.0.1:1080\n"))
	if err != nil {
		t.Fatal(err)
	}
	p := built.Capabilities.AuthProvider.(*Plugin)
	defer p.Close()
	if p.config.CallbackPort != 57321 || p.config.ProxyURL != "socks5://127.0.0.1:1080" {
		t.Fatalf("host-scoped plugin settings were ignored: callback_port=%d proxy_url=%q", p.config.CallbackPort, p.config.ProxyURL)
	}
	if _, err := Build([]byte("callback_port: -1\n")); err == nil {
		t.Fatal("invalid per-instance callback_port was silently accepted")
	}
}

func TestReviewRevisedStreamDoesNotClaimSuccessfulCompletion(t *testing.T) {
	p := newTestPlugin(t)
	p.generate = func(_ context.Context, _, _, _ string, emit func(string) error) error {
		if err := emit("old text"); err != nil {
			return err
		}
		return emit("replacement text")
	}
	r, err := p.ExecuteStream(context.Background(), credentialRequest())
	if err != nil {
		t.Fatal(err)
	}
	sawFailure, sawDone := false, false
	for chunk := range r.Chunks {
		if chunk.Err != nil {
			sawFailure = true
		}
		if strings.Contains(string(chunk.Payload), "[DONE]") || strings.Contains(string(chunk.Payload), `"finish_reason":"stop"`) {
			sawDone = true
		}
	}
	if !sawFailure || sawDone {
		t.Fatalf("stream revision must be a failure, not a successful completion: failure=%v done=%v", sawFailure, sawDone)
	}
}

func TestReviewRejectedRequestDoesNotContactUpstream(t *testing.T) {
	p := newTestPlugin(t)
	called := false
	p.generate = func(context.Context, string, string, string, func(string) error) error {
		called = true
		return nil
	}
	r := credentialRequest()
	r.Payload = []byte(`{"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function"}]}`)
	if _, err := p.Execute(context.Background(), r); err == nil {
		t.Fatal("unsupported tools did not fail before upstream execution")
	}
	if called {
		t.Fatal("unsupported tools request contacted upstream")
	}
}

func TestReviewExpiredExecutorDoesNotRotateUnpersistedCredential(t *testing.T) {
	p := newTestPlugin(t)
	called := false
	r := credentialRequest()
	raw, _ := json.Marshal(Credential{Type: Provider, AccessToken: "expired", RefreshToken: "refresh-test", ExpiresAt: time.Now().Add(-time.Hour)})
	r.StorageJSON = raw
	r.HTTPClient = fakeHTTP{func(pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		called = true
		return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"access_token":"new-access","refresh_token":"rotated-refresh","expires_in":3600}`)}, nil
	}}
	_, err := p.Execute(context.Background(), r)
	if err == nil {
		t.Fatal("expired credential must wait for host-owned refresh")
	}
	if called {
		t.Fatal("executor rotated a credential without persisting it through host RefreshAuth")
	}
	if status, ok := err.(interface{ StatusCode() int }); !ok || status.StatusCode() != 401 {
		t.Fatalf("expired credential must be an authentication failure: %v", err)
	}
}

func TestReviewNativeTransportInheritsHostProxyAndAccountOverride(t *testing.T) {
	p := newTestPlugin(t)
	_, err := p.StaticModels(context.Background(), pluginapi.StaticModelRequest{Host: pluginapi.HostConfigSummary{ProxyURL: "http://127.0.0.1:7890"}})
	if err != nil {
		t.Fatal(err)
	}
	r := credentialRequest()
	_, _, proxyURL, err := p.prepare(context.Background(), r)
	if err != nil || proxyURL != "http://127.0.0.1:7890" {
		t.Fatalf("native gRPC ignored the host proxy: %q, %v", proxyURL, err)
	}
	var c Credential
	if err := json.Unmarshal(r.StorageJSON, &c); err != nil {
		t.Fatal(err)
	}
	c.ProxyURL = "socks5://127.0.0.1:1080"
	r.StorageJSON, _ = json.Marshal(c)
	_, _, proxyURL, err = p.prepare(context.Background(), r)
	if err != nil || proxyURL != c.ProxyURL {
		t.Fatalf("account-specific proxy did not override host proxy: %q, %v", proxyURL, err)
	}
}

func TestReviewNativeUnsuffixedContentTypeAndCompressedResponse(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	want := strings.Repeat("مرحبا من Gemini. ", 32)
	server := grpc.NewServer(grpc.ForceServerCodec(rawCodec{}), grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		md, _ := metadata.FromIncomingContext(stream.Context())
		if values := md.Get("content-type"); len(values) != 1 || values[0] != "application/grpc" {
			t.Errorf("native frontend requires unsuffixed Content-Type application/grpc, got %v", values)
		}
		// Do not import the gzip package here: its registration must come from
		// production code, otherwise this test could conceal a missing decoder.
		if err := grpc.SetSendCompressor(stream.Context(), "gzip"); err != nil {
			return err
		}
		for range requestFrames("hello") {
			var frame []byte
			if err := stream.RecvMsg(&frame); err != nil {
				return err
			}
		}
		return stream.SendMsg(textFrame(want))
	}))
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///local", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.ForceCodec(rawCodec{})), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var got string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = generateOnConnection(ctx, conn, "test-token", "hello", func(text string) error { got = text; return nil })
	if err != nil || got != want {
		t.Fatalf("compressed native response failed: %v (received %d bytes, expected %d)", err, len(got), len(want))
	}
}
