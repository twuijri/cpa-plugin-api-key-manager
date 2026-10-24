package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

type fakeHTTP struct {
	do func(pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error)
}

func (f fakeHTTP) Do(_ context.Context, r pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	return f.do(r)
}
func (f fakeHTTP) DoStream(context.Context, pluginapi.HTTPRequest) (pluginapi.HTTPStreamResponse, error) {
	return pluginapi.HTTPStreamResponse{}, fmt.Errorf("not used")
}

func TestNativeOAuthPKCEAndRefresh(t *testing.T) {
	l, err := Start("http://localhost:51123/auth")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(l.URL)
	q := u.Query()
	h := sha256.Sum256([]byte(l.Verifier))
	if q.Get("client_id") != nativeClientID || q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(h[:]) || q.Get("scope") != flexibleScope {
		t.Fatal("native OAuth configuration or PKCE mismatch")
	}
	var calls int
	client := fakeHTTP{func(r pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		calls++
		if r.URL != tokenEndpoint || r.Method != "POST" {
			t.Fatal("unexpected OAuth destination")
		}
		v, _ := url.ParseQuery(string(r.Body))
		if v.Get("client_id") != nativeClientID || v.Get("client_secret") == "" {
			t.Fatal("missing native client configuration")
		}
		if calls == 1 {
			if v.Get("code_verifier") != l.Verifier || v.Get("grant_type") != "authorization_code" {
				t.Fatal("invalid code exchange")
			}
			return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"access_token":"first","refresh_token":"refresh-test","expires_in":3600}`)}, nil
		}
		if v.Get("refresh_token") != "refresh-test" || v.Get("scope") != tokenScopes {
			t.Fatal("refresh did not use native service scopes")
		}
		return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"access_token":"scoped","expires_in":3600}`)}, nil
	}}
	if _, err = Exchange(context.Background(), client, l, "code", "wrong"); err == nil || calls != 0 {
		t.Fatal("invalid state reached token service")
	}
	c, err := Exchange(context.Background(), client, l, "code", l.State)
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "scoped" || c.RefreshToken != "refresh-test" || calls != 2 {
		t.Fatal("refresh token was lost or native refresh failed")
	}
	if time.Until(c.ExpiresAt) < 59*time.Minute {
		t.Fatal("invalid token expiry")
	}
	for _, redirect := range []string{"https://example.com/auth", "http://localhost/auth", "http://user@localhost:51123/auth", "http://localhost:51123/auth?x=1"} {
		if _, err := Start(redirect); err == nil {
			t.Fatalf("accepted invalid redirect %q", redirect)
		}
	}
}

func TestTokenErrorsDoNotLeakResponse(t *testing.T) {
	client := fakeHTTP{func(pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		return pluginapi.HTTPResponse{StatusCode: 400, Body: []byte(`{"error":"invalid_grant","error_description":"secret-test-token account@example.com"}`)}, nil
	}}
	_, err := Refresh(context.Background(), client, Credential{RefreshToken: "test"})
	if err == nil || strings.Contains(err.Error(), "secret-test") || strings.Contains(err.Error(), "account@") {
		t.Fatal("token response leaked")
	}
	if err.(*StatusError).StatusCode() != 401 {
		t.Fatal("revoked credentials must be classified as authentication failure")
	}
}

func newTestPlugin(t *testing.T) *Plugin {
	t.Helper()
	built, err := Build(nil)
	if err != nil {
		t.Fatal(err)
	}
	p := built.Capabilities.AuthProvider.(*Plugin)
	t.Cleanup(p.Close)
	return p
}

func TestOAuthCallbackSingleUseAndIsolation(t *testing.T) {
	p := newTestPlugin(t)
	ctx := context.Background()
	start, err := p.StartLogin(ctx, pluginapi.AuthLoginStartRequest{})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	payload, _ := json.Marshal(map[string]string{"state": start.State, "code": "test-code"})
	path := filepath.Join(dir, ".oauth-"+Provider+"-"+start.State+".oauth")
	if err = os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	client := fakeHTTP{func(pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
		calls.Add(1)
		return pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"access_token":"token","refresh_token":"refresh-test","expires_in":3600}`)}, nil
	}}
	req := pluginapi.AuthLoginPollRequest{State: start.State, Host: pluginapi.HostConfigSummary{AuthDir: dir}, HTTPClient: client}
	resp, err := p.PollLogin(ctx, req)
	if err != nil || resp.Status != pluginapi.AuthLoginStatusSuccess {
		t.Fatalf("login failed: %v", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("callback was not consumed")
	}
	resp, err = p.PollLogin(ctx, req)
	if err != nil || resp.Status != pluginapi.AuthLoginStatusError || calls.Load() != 2 {
		t.Fatal("OAuth callback replay reached token service")
	}
	if _, err = p.PollLogin(ctx, pluginapi.AuthLoginPollRequest{State: "../../outside"}); err == nil {
		t.Fatal("accepted path traversal")
	}
	parsed, err := p.ParseAuth(ctx, pluginapi.AuthParseRequest{RawJSON: []byte(`{"type":"antigravity","refresh_token":"other"}`)})
	if err != nil || parsed.Handled {
		t.Fatal("Gemini parser intercepted an Antigravity credential")
	}
}

func textFrame(text string) []byte {
	return message(1003, message(1, message(1, message(2, message(1, []byte(text))))))
}

func TestNativeGRPCSequenceAndResponse(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.ForceServerCodec(rawCodec{}), grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		method, _ := grpc.MethodFromServerStream(stream)
		if method != rpcMethod {
			t.Error("wrong native RPC")
		}
		md, _ := metadata.FromIncomingContext(stream.Context())
		if len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer test-token" {
			t.Error("missing OAuth bearer")
		}
		for i, want := range requestFrames("hello") {
			var got []byte
			if err := stream.RecvMsg(&got); err != nil {
				return err
			}
			if string(got) != string(want) {
				t.Errorf("request frame %d mismatch", i)
			}
		}
		for _, s := range []string{"مر", "مرحبا", "مرحبا"} {
			if err := stream.SendMsg(textFrame(s)); err != nil {
				return err
			}
		}
		return nil
	}))
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///local", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.ForceCodec(rawCodec{})), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var snapshots []string
	err = generateOnConnection(context.Background(), conn, "test-token", "hello", func(s string) error { snapshots = append(snapshots, s); return nil })
	if err != nil || strings.Join(snapshots, "|") != "مر|مرحبا|مرحبا" {
		t.Fatalf("native stream failed: %v, %v", err, snapshots)
	}
}

func credentialRequest() pluginapi.ExecutorRequest {
	raw, _ := json.Marshal(Credential{Type: Provider, AccessToken: "test-token", RefreshToken: "test-refresh", ExpiresAt: time.Now().Add(time.Hour)})
	return pluginapi.ExecutorRequest{Model: Model, Format: "openai", StorageJSON: raw, Payload: []byte(`{"messages":[{"role":"user","content":"hello"}]}`)}
}

func TestStreamCumulativeSnapshotsDoNotDuplicate(t *testing.T) {
	p := newTestPlugin(t)
	p.generate = func(_ context.Context, _, _, _ string, emit func(string) error) error {
		for _, s := range []string{"مر", "مرحبا", "مرحبا"} {
			if err := emit(s); err != nil {
				return err
			}
		}
		return nil
	}
	r, err := p.ExecuteStream(context.Background(), credentialRequest())
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	done := 0
	for chunk := range r.Chunks {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		s := string(chunk.Payload)
		if s == "data: [DONE]\n\n" {
			done++
			continue
		}
		var e struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err = json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(s, "data: "))), &e); err != nil {
			t.Fatal(err)
		}
		for _, c := range e.Choices {
			output.WriteString(c.Delta.Content)
		}
	}
	if output.String() != "مرحبا" || done != 1 {
		t.Fatalf("bad streamed output: %q (%d DONE)", output.String(), done)
	}
}

func TestStreamStopsOnCancellation(t *testing.T) {
	p := newTestPlugin(t)
	finished := make(chan struct{})
	p.generate = func(ctx context.Context, _, _, _ string, emit func(string) error) error {
		defer close(finished)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r, err := p.ExecuteStream(ctx, credentialRequest())
	if err != nil {
		t.Fatal(err)
	}
	<-r.Chunks
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("native stream leaked after cancellation")
	}
}

func TestUnsupportedToolsAndImagesAreRejected(t *testing.T) {
	for _, payload := range []string{`{"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function"}]}`, `{"messages":[{"role":"user","content":[{"type":"image_url"}]}]}`, `{"messages":[{"role":"tool","content":"result"}]}`} {
		if _, err := conversation([]byte(payload)); err == nil {
			t.Fatal("unsupported request was silently accepted")
		}
	}
}

func FuzzWireParser(f *testing.F) {
	f.Add(textFrame("hello"))
	f.Add([]byte{0xff})
	f.Add([]byte{0x0a, 0xff, 0x01})
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parseWire(b); _, _, _ = responseText(b) })
}
