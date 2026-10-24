package desktop

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/proxyutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const rpcEndpoint = "robinfrontend-pa.googleapis.com:443"
const rpcMethod = "/assistant.mobile.robin.RobinService/ProcessQuery"
const maxTextBytes = 4 << 20

type rawCodec struct{}

// Gemini's public native RPC frontend routes the unsuffixed application/grpc
// content type. ForceCodec supplies wire handling without a +proto subtype.
func (rawCodec) Name() string { return "" }
func (rawCodec) Marshal(value any) ([]byte, error) {
	data, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("expected protobuf bytes")
	}
	return data, nil
}
func (rawCodec) Unmarshal(data []byte, value any) error {
	target, ok := value.(*[]byte)
	if !ok {
		return fmt.Errorf("expected protobuf byte destination")
	}
	*target = append((*target)[:0], data...)
	return nil
}

func nativeConnection(proxyURL string) (*grpc.ClientConn, error) {
	dialer, _, err := proxyutil.BuildDialer(proxyURL)
	if err != nil {
		return nil, failure(400, "Invalid configured upstream proxy")
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(rawCodec{}), grpc.MaxCallRecvMsgSize(8<<20)),
	}
	if dialer != nil {
		contextDialer, ok := dialer.(interface {
			DialContext(context.Context, string, string) (net.Conn, error)
		})
		if !ok {
			return nil, failure(400, "Configured proxy does not support cancellable connections")
		}
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, address string) (net.Conn, error) {
			return contextDialer.DialContext(ctx, "tcp", address)
		}))
	}
	return grpc.NewClient(rpcEndpoint, opts...)
}

// Generate sends only the supplied conversation text to Gemini's native service.
// A caller must cancel ctx when its client disconnects; no generation deadline is imposed.
func Generate(ctx context.Context, token, proxyURL, prompt string, snapshot func(string) error) error {
	return GenerateSelected(ctx, token, proxyURL, prompt, NativeModel{}, snapshot)
}

func GenerateSelected(ctx context.Context, token, proxyURL, prompt string, selected NativeModel, snapshot func(string) error) error {
	conn, err := nativeConnection(proxyURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return generateSelectedOnConnection(ctx, conn, token, prompt, selected, snapshot)
}

func generateOnConnection(ctx context.Context, conn grpc.ClientConnInterface, token, prompt string, snapshot func(string) error) error {
	return generateSelectedOnConnection(ctx, conn, token, prompt, NativeModel{}, snapshot)
}

func generateSelectedOnConnection(ctx context.Context, conn grpc.ClientConnInterface, token, prompt string, selected NativeModel, snapshot func(string) error) error {
	if token == "" {
		return failure(401, "Gemini access token is missing")
	}
	if len(prompt) == 0 || len(prompt) > maxTextBytes {
		return failure(400, "Conversation text must be between 1 byte and 4 MiB")
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ClientStreams: true, ServerStreams: true}, rpcMethod)
	if err != nil {
		return rpcError(err)
	}
	for _, frame := range requestFramesSelected(prompt, selected) {
		if err = stream.SendMsg(frame); err != nil {
			return rpcError(err)
		}
	}
	if err = stream.CloseSend(); err != nil {
		return rpcError(err)
	}
	sawText := false
	for {
		var frame []byte
		err = stream.RecvMsg(&frame)
		if err == io.EOF {
			break
		}
		if err != nil {
			return rpcError(err)
		}
		text, found, errDecode := responseText(frame)
		if errDecode != nil {
			return failure(502, "Gemini returned an unsupported response frame")
		}
		if !found {
			continue
		}
		if !utf8.ValidString(text) || len(text) > maxTextBytes {
			return failure(502, "Gemini response text is invalid or exceeds the preview size limit")
		}
		if text != "" {
			sawText = true
		}
		if err = snapshot(text); err != nil {
			return err
		}
	}
	if !sawText {
		return failure(502, "Gemini completed without a text response; this preview supports text conversations only")
	}
	return nil
}

func rpcError(err error) error {
	if err == nil {
		return nil
	}
	if err == context.Canceled || status.Code(err) == codes.Canceled {
		return context.Canceled
	}
	code, httpStatus, message := status.Code(err), 502, "Gemini native service request failed"
	switch code {
	case codes.Unauthenticated:
		httpStatus, message = 401, "Gemini authentication expired or was revoked"
	case codes.PermissionDenied:
		httpStatus, message = 403, "Google denied this Gemini Desktop request for the account"
	case codes.ResourceExhausted:
		httpStatus, message = 429, "Gemini account quota or rate limit reached"
	case codes.InvalidArgument:
		httpStatus, message = 400, "Gemini rejected the native request format"
	case codes.DeadlineExceeded:
		httpStatus, message = 504, "Gemini request deadline exceeded"
	case codes.Unavailable:
		message = "Gemini native service is unavailable"
	}
	return &StatusError{Status: httpStatus, Message: message + " (" + code.String() + ")", Cause: err}
}

type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls json.RawMessage `json:"tool_calls"`
	} `json:"messages"`
	Tools          json.RawMessage `json:"tools"`
	Functions      json.RawMessage `json:"functions"`
	ResponseFormat json.RawMessage `json:"response_format"`
	N              *int            `json:"n"`
}

func nonemptyJSON(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null" && string(raw) != "[]"
}

func conversation(payload []byte) (string, error) {
	var r chatRequest
	if err := json.Unmarshal(payload, &r); err != nil {
		return "", failure(400, "Invalid Chat Completions request")
	}
	if len(r.Messages) == 0 {
		return "", failure(400, "messages is required")
	}
	if nonemptyJSON(r.Tools) || nonemptyJSON(r.Functions) {
		return "", failure(400, "Gemini Desktop preview does not support API tool calling")
	}
	if r.N != nil && *r.N != 1 {
		return "", failure(400, "Gemini Desktop preview supports n=1 only")
	}
	if nonemptyJSON(r.ResponseFormat) {
		var rf struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(r.ResponseFormat, &rf) != nil || rf.Type != "text" {
			return "", failure(400, "Gemini Desktop preview supports text output only")
		}
	}
	var text strings.Builder
	for _, m := range r.Messages {
		switch m.Role {
		case "system", "developer", "user", "assistant":
		default:
			return "", failure(400, "Unsupported conversation role")
		}
		if nonemptyJSON(m.ToolCalls) {
			return "", failure(400, "Tool history is not supported by the Gemini Desktop preview")
		}
		var content string
		if json.Unmarshal(m.Content, &content) != nil {
			var parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(m.Content, &parts) != nil || len(parts) == 0 {
				return "", failure(400, "Message content must contain text")
			}
			for _, part := range parts {
				if part.Type != "text" {
					return "", failure(400, "Images and other attachments are not supported by this preview")
				}
				content += part.Text
			}
		}
		if len(r.Messages) == 1 && m.Role == "user" {
			text.WriteString(content)
		} else {
			fmt.Fprintf(&text, "<%s>\n%s\n</%s>\n", m.Role, content, m.Role)
		}
		if text.Len() > maxTextBytes {
			return "", failure(400, "Conversation exceeds the preview size limit")
		}
	}
	if strings.TrimSpace(text.String()) == "" {
		return "", failure(400, "Conversation contains no text")
	}
	return text.String(), nil
}

func headers() http.Header { return http.Header{"Content-Type": {"application/json"}} }

func completion(id, model, text string, created int64) []byte {
	result, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": text}, "finish_reason": "stop"}}})
	return result
}

// The host's OpenAI handler owns SSE framing and the final [DONE] event.
// Same-format executor chunks reach that handler without a translator.
func streamChunk(id, model string, created int64, delta map[string]any, finish any) []byte {
	result, _ := json.Marshal(map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
	return result
}

var _ pluginapi.ProviderExecutor = (*Plugin)(nil)
