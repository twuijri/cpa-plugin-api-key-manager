// Local hardened variant of hex-ci/cpa-plugin-model-playground.
// Serves a memory-only chat UI calling the same-origin /v1 API.
// No management-key discovery, storage access, or upstream proxy helper.
package main

/*
#include <stdlib.h>
#include <stdint.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef void* cliproxy_host_ctx;
typedef int (*cliproxy_host_call_fn)(cliproxy_host_ctx, const char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	cliproxy_host_ctx host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

static int mc_call_host(cliproxy_host_api* api, const char* method, uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	return api->call(api->host_ctx, method, request, request_len, response);
}
static void mc_free_host_buffer(cliproxy_host_api* api, void* ptr, size_t len) {
	api->free_buffer(ptr, len);
}

// Declarations for the exported entry points, referenced from Go via
// C.cliproxyPluginCall etc. cgo re-declares them at the end of the generated
// header wrapped in extern "C"; matching the linkage here (under C++) keeps
// the header warning-free for C++ consumers.
#ifdef __cplusplus
extern "C" {
#endif
extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
#ifdef __cplusplus
}
#endif
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const providerName = "model-playground"

// version is injected at build time via -ldflags "-X main.version=...".
// The default matches the VERSION file for builds without ldflags.
var version = "0.3.0-local.2"

var hostAPI *C.cliproxy_host_api

func main() {}

// -----------------------------------------------------------------------------
// C ABI exports
// -----------------------------------------------------------------------------

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	hostAPI = host
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	// Intentionally a no-op: the host dlclose()s this library right after
	// calling shutdown, touching Go runtime state here risks a cgo SIGSEGV.
}

// -----------------------------------------------------------------------------
// Host calls
// -----------------------------------------------------------------------------

func hostCall(method string, request []byte) ([]byte, error) {
	if hostAPI == nil || hostAPI.call == nil {
		return nil, fmt.Errorf("host API unavailable")
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var cReq unsafe.Pointer
	var reqLen C.size_t
	if len(request) > 0 {
		cReq = C.CBytes(request)
		defer C.free(cReq)
		reqLen = C.size_t(len(request))
	}
	var resp C.cliproxy_buffer
	rc := C.mc_call_host(hostAPI, cMethod, (*C.uint8_t)(cReq), reqLen, &resp)
	var out []byte
	if resp.ptr != nil && resp.len > 0 {
		out = C.GoBytes(resp.ptr, C.int(resp.len))
	}
	if resp.ptr != nil && hostAPI.free_buffer != nil {
		C.mc_free_host_buffer(hostAPI, resp.ptr, resp.len)
	}
	if rc != 0 {
		return out, fmt.Errorf("host call %s returned %d", method, int(rc))
	}
	return out, nil
}

// -----------------------------------------------------------------------------

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func okEnvelope(v any) ([]byte, error) {
	result, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: result})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return okEnvelope(mcRegistration())
	case pluginabi.MethodManagementRegister:
		return handleManagementRegister(request)
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	default:
		return nil, fmt.Errorf("unsupported method %s", method)
	}
}

// registration mirrors the plugin ABI wire shape: schema version + metadata +
// capability flags. model-playground is Management-API-only (panel + gateway proxy),
// so every provider/executor/auth capability stays false.
type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	ModelProvider         bool                         `json:"model_provider"`
	AuthProvider          bool                         `json:"auth_provider"`
	FrontendAuthProvider  bool                         `json:"frontend_auth_provider"`
	Executor              bool                         `json:"executor"`
	ExecutorModelScope    pluginapi.ExecutorModelScope `json:"executor_model_scope"`
	ExecutorInputFormats  []string                     `json:"executor_input_formats,omitempty"`
	ExecutorOutputFormats []string                     `json:"executor_output_formats,omitempty"`
	Scheduler             bool                         `json:"scheduler"`
	ManagementAPI         bool                         `json:"management_api"`
	UsagePlugin           bool                         `json:"usage_plugin"`
}

// mcRegistration declares the plugin's metadata and capabilities. model-playground
// proxies chat to the CPA gateway and serves an embedded panel; it registers
// neither a provider nor an executor.
func mcRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             providerName,
			Version:          version,
			Author:           "model-playground",
			GitHubRepository: "https://github.com/twuijri/cpa-plugin-api-key-manager",
			Logo:             "",
			// No configuration: the panel calls the same-origin /v1 API.
		},
		Capabilities: registrationCapability{
			ManagementAPI: true,
		},
	}
}

func handleManagementRegister(request []byte) ([]byte, error) {
	var req pluginapi.ManagementRegistrationRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return nil, err
	}
	// Routes are registered relative to the management prefix; the host mounts
	// them under /v0/management/plugins/<id>/
	// req.BasePath (e.g. /v0/management/plugins/model-playground) is not re-appended —
	// doing so yields /v0/management/plugins/model-playground/v0/... → 404.
	_ = req.BasePath
	return okEnvelope(pluginapi.ManagementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{},
		Resources: []pluginapi.ResourceRoute{
			{Path: "/panel", Menu: "Model Playground", Description: "Model playground: try any model exposed by the CPA gateway."},
		},
	})
}
