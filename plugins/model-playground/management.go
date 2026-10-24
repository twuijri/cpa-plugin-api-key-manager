package main

import (
	"encoding/json"
	"net/http"
	"strings"

	_ "embed"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

//go:embed panel.html
var panelHTML []byte

// handleManagement dispatches the plugin's Management API calls: the panel
// resource (unauthenticated) and the /keys helper (management-authenticated).
func handleManagement(reqBytes []byte) ([]byte, error) {
	var req struct {
		Method  string
		Path    string
		Query   map[string][]string
		Headers map[string][]string
	}
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		return errorEnvelope("bad_request", "malformed management request"), nil
	}
	method := strings.ToUpper(req.Method)
	path := strings.TrimRight(req.Path, "/")

	// Browser UI resource routes (unauthenticated).
	resPrefix := "/v0/resource/plugins/" + providerName
	if method == http.MethodGet && strings.HasPrefix(path, resPrefix) {
		sub := strings.TrimPrefix(path, resPrefix)
		switch sub {
		case "", "/", "/panel", "/panel.html":
			return okEnvelope(mgmtHTMLResponse(panelHTML))
		default:
			return okEnvelope(mgmtJSONResponse(http.StatusNotFound, map[string]any{"error": "not found"}))
		}
	}

	return okEnvelope(mgmtJSONResponse(http.StatusNotFound, map[string]any{"error": "unknown path"}))
}

// mgmtJSONResponse builds a ManagementResponse carrying a JSON-encoded body.
func mgmtJSONResponse(status int, v any) pluginapi.ManagementResponse {
	body, _ := json.Marshal(v)
	return mgmtResponse(status, "application/json", body)
}

func mgmtHTMLResponse(body []byte) pluginapi.ManagementResponse {
	return mgmtResponse(http.StatusOK, "text/html; charset=utf-8", body)
}

func mgmtJSResponse(body []byte) pluginapi.ManagementResponse {
	return mgmtResponse(http.StatusOK, "application/javascript; charset=utf-8", body)
}

// mgmtResponse builds the wire ManagementResponse the host converts back into
// an HTTP reply for the management/resource route.
func mgmtResponse(status int, ct string, body []byte) pluginapi.ManagementResponse {
	h := http.Header{}
	h.Set("Content-Type", ct)
	return pluginapi.ManagementResponse{StatusCode: status, Headers: h, Body: body}
}
