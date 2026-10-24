package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOpenAIHostStreamContract(t *testing.T) {
	p := newTestPlugin(t)
	p.generate = func(_ context.Context, _, _, _ string, emit func(string) error) error {
		return emit("مرحبا")
	}
	response, err := p.ExecuteStream(context.Background(), credentialRequest())
	if err != nil {
		t.Fatal(err)
	}
	if response.Headers.Get("Content-Type") != "text/event-stream" || response.Headers.Get("Cache-Control") != "no-cache" {
		t.Fatal("stream response has incorrect HTTP headers")
	}
	var wire strings.Builder
	for chunk := range response.Chunks {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		// CLIProxyAPI v8's same-format executor path forwards these bytes
		// unchanged; its OpenAI handler then adds exactly this SSE framing.
		fmt.Fprintf(&wire, "data: %s\n\n", chunk.Payload)
	}
	wire.WriteString("data: [DONE]\n\n")
	scanner := bufio.NewScanner(strings.NewReader(wire.String()))
	text, done, finish := "", 0, 0
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			done++
			continue
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("Playground cannot decode host-framed stream data: %v", err)
		}
		for _, choice := range event.Choices {
			text += choice.Delta.Content
			if choice.FinishReason == "stop" {
				finish++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if text != "مرحبا" || done != 1 || finish != 1 {
		t.Fatalf("invalid browser stream result: text=%q done=%d finish=%d", text, done, finish)
	}
}
