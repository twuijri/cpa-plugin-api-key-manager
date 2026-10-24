package desktop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Explicit opt-in only: sends a short diagnostic query, never logs account
// profile, token values, full server frames, or conversation text.
func TestSelectedNativeModelsLive(t *testing.T) {
	path := os.Getenv("GEMINI_DESKTOP_GENERATION_TOKEN_FILE")
	if path == "" {
		t.Skip("live native generation is opt-in")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read local credential")
	}
	var stored struct {
		AccessToken  string  `json:"access_token"`
		RefreshToken string  `json:"refresh_token"`
		ObtainedAt   float64 `json:"obtained_at"`
		ExpiresIn    int64   `json:"expires_in"`
	}
	if json.Unmarshal(raw, &stored) != nil || stored.AccessToken == "" {
		t.Fatal("invalid local credential")
	}
	token := stored.AccessToken
	if stored.ObtainedAt > 0 && time.Now().Unix() >= int64(stored.ObtainedAt)+stored.ExpiresIn-60 {
		client := fakeHTTP{func(req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
			httpReq, err := http.NewRequestWithContext(context.Background(), req.Method, req.URL, strings.NewReader(string(req.Body)))
			if err != nil {
				return pluginapi.HTTPResponse{}, err
			}
			httpReq.Header = req.Headers
			response, err := (&http.Client{Timeout: 30 * time.Second}).Do(httpReq)
			if err != nil {
				return pluginapi.HTTPResponse{}, err
			}
			defer response.Body.Close()
			body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			return pluginapi.HTTPResponse{StatusCode: response.StatusCode, Body: body}, err
		}}
		credential, err := Refresh(context.Background(), client, Credential{Type: Provider, RefreshToken: stored.RefreshToken})
		if err != nil {
			t.Fatal("local test credential refresh failed")
		}
		token = credential.AccessToken
		// Preserve a rotated refresh grant in the existing private input file.
		var object map[string]any
		if json.Unmarshal(raw, &object) != nil {
			t.Fatal("invalid local credential object")
		}
		object["access_token"], object["refresh_token"] = credential.AccessToken, credential.RefreshToken
		object["obtained_at"], object["expires_in"] = time.Now().Unix(), int64(time.Until(credential.ExpiresAt).Seconds())
		updated, _ := json.Marshal(object)
		if os.WriteFile(path, updated, 0600) != nil {
			t.Fatal("cannot persist refreshed local test credential")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	models, err := DiscoverModels(ctx, token, "")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) == 0 {
		t.Fatal("account returned no native modes")
	}
	for _, selected := range models {
		t.Run(selected.Title, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
			defer cancel()
			conn, err := nativeConnection("")
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
			stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ClientStreams: true, ServerStreams: true}, rpcMethod)
			if err != nil {
				t.Fatal(rpcError(err))
			}
			for _, frame := range requestFramesSelected("Reply with exactly GEMINI_SELECTED_OK", selected) {
				if err := stream.SendMsg(frame); err != nil {
					t.Fatal(rpcError(err))
				}
			}
			if err := stream.CloseSend(); err != nil {
				t.Fatal(rpcError(err))
			}
			marker, resolved, serverID := false, uint64(0), ""
			for {
				var frame []byte
				err := stream.RecvMsg(&frame)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(rpcError(err))
				}
				if text, found, _ := responseText(frame); found && strings.Contains(text, "GEMINI_SELECTED_OK") {
					marker = true
				}
				responses, _ := nested(frame, 1003, 1)
				for _, response := range responses {
					fields, _ := parseWire(response)
					for _, f := range fields {
						if f.Number == 41 && f.Kind == 0 {
							resolved = f.Integer
						}
						if f.Number == 25 && f.Kind == 2 {
							serverID = string(f.Bytes)
						}
						if f.Number == 31 && f.Kind == 0 && f.Integer != 0 {
							t.Fatalf("server downgraded requested native mode (reason %d)", f.Integer)
						}
					}
				}
			}
			if !marker {
				t.Fatal("native generation did not return diagnostic marker")
			}
			if resolved != 0 && resolved != uint64(selected.Category) {
				t.Fatalf("server resolved category %d instead of %d", resolved, selected.Category)
			}
			if serverID != "" && selected.ConfigID != "" && serverID != selected.ConfigID {
				t.Fatal("server returned a different native model config ID")
			}
			t.Logf("native mode %q succeeded; requested category=%d resolved category=%d matching server config=%t", selected.Title, selected.Category, resolved, serverID != "" && serverID == selected.ConfigID)
		})
	}
}
