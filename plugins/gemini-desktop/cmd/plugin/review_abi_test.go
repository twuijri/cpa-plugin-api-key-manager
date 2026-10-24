package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestReviewABIRegistrationAndErrorStatus(t *testing.T) {
	raw, err := handleABIMethod(context.Background(), pluginabi.MethodPluginRegister, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var envelope pluginabi.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.OK {
		t.Fatalf("invalid registration envelope: %v", err)
	}
	var registration abiRegistration
	if err := json.Unmarshal(envelope.Result, &registration); err != nil {
		t.Fatal(err)
	}
	if registration.SchemaVersion != pluginabi.SchemaVersion || !registration.Capabilities.AuthProvider || !registration.Capabilities.Executor || registration.Capabilities.RequestTranslator || registration.Capabilities.ThinkingApplier {
		t.Fatal("advertised capability contract does not match this preview")
	}
	raw, err = handleABIMethod(context.Background(), pluginabi.MethodExecutorCountTokens, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Error == nil || envelope.Error.HTTPStatus != 501 {
		t.Fatal("unsupported exact token counting must preserve HTTP 501 across the ABI")
	}
}

func TestReviewBufferedABIStreamPropagatesError(t *testing.T) {
	chunks := make(chan pluginapi.ExecutorStreamChunk, 1)
	want := errors.New("synthetic upstream failure")
	chunks <- pluginapi.ExecutorStreamChunk{Err: want}
	close(chunks)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := marshalABIStreamResponse(ctx, "", pluginapi.ExecutorStreamResponse{Chunks: chunks}, cancel)
	if !errors.Is(err, want) {
		t.Fatalf("buffered ABI stream swallowed the upstream error: %v", err)
	}
}
