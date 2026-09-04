package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef struct {void *ptr; size_t len;} cliproxy_buffer;
typedef int (*host_call)(void*,const char*,const uint8_t*,size_t,cliproxy_buffer*);
typedef void (*buffer_free)(void*,size_t);
typedef struct {uint32_t abi_version; void *host_ctx;host_call call;buffer_free free_buffer;} cliproxy_host_api;
typedef int (*plugin_call)(char*,uint8_t*,size_t,cliproxy_buffer*);
typedef struct {uint32_t abi_version;plugin_call call;buffer_free free_buffer;void (*shutdown)(void);} cliproxy_plugin_api;
extern int miftahCall(char*,uint8_t*,size_t,cliproxy_buffer*);
extern void miftahFree(void*,size_t);
extern void miftahShutdown(void);
static int invoke(cliproxy_host_api *h,char *m,void *p,size_t n,cliproxy_buffer *r){return h->call(h->host_ctx,m,p,n,r);}
static void release(cliproxy_host_api *h,cliproxy_buffer *r){if(r->ptr && h->free_buffer)h->free_buffer(r->ptr,r->len);}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"miftah.local/plugin/internal/bridge"
	"unsafe"
)

var host C.cliproxy_host_api
var runtime = pluginRuntime{path: statePath, host: callback}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(h *C.cliproxy_host_api, p *C.cliproxy_plugin_api) C.int {
	if h == nil || p == nil || h.abi_version != 1 {
		return 1
	}
	host = *h
	p.abi_version = 1
	p.call = C.plugin_call(C.miftahCall)
	p.free_buffer = C.buffer_free(C.miftahFree)
	p.shutdown = (*[0]byte)(C.miftahShutdown)
	return 0
}
func callback(method string, req, out any) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	m := C.CString(method)
	defer C.free(unsafe.Pointer(m))
	p := C.CBytes(b)
	defer C.free(p)
	var result C.cliproxy_buffer
	status := C.invoke(&host, m, p, C.size_t(len(b)), &result)
	defer C.release(&host, &result)
	if result.len > 64<<20 {
		return errors.New("host response exceeds limit")
	}
	raw := C.GoBytes(result.ptr, C.int(result.len))
	var env struct {
		OK     bool             `json:"ok"`
		Result json.RawMessage  `json:"result"`
		Error  *bridge.RPCError `json:"error"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return errors.New("invalid host envelope")
	}
	if !env.OK || status != 0 {
		if env.Error != nil {
			return env.Error
		}
		return errors.New("host call failed")
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

//export miftahCall
func miftahCall(method *C.char, data *C.uint8_t, n C.size_t, out *C.cliproxy_buffer) (rc C.int) {
	if out == nil {
		return 1
	}
	out.ptr = nil
	out.len = 0
	defer func() {
		if recover() != nil {
			b := []byte(`{"ok":false,"error":{"code":"panic","message":"plugin failed closed","http_status":503}}`)
			out.ptr = C.CBytes(b)
			out.len = C.size_t(len(b))
			rc = 1
		}
	}()
	if method == nil || n > 8<<20 {
		return 1
	}
	m := C.GoString(method)
	raw := C.GoBytes(unsafe.Pointer(data), C.int(n))
	value, err := runtime.dispatch(m, raw)
	env := map[string]any{"ok": err == nil}
	if err == nil {
		env["result"] = value
	} else {
		e, ok := err.(*bridge.RPCError)
		if !ok {
			e = &bridge.RPCError{Code: "plugin_error", Message: err.Error(), HTTPStatus: 503}
		}
		env["error"] = e
	}
	b, e := json.Marshal(env)
	if e != nil {
		return 1
	}
	out.ptr = C.CBytes(b)
	out.len = C.size_t(len(b))
	return 0
}

//export miftahFree
func miftahFree(p unsafe.Pointer, n C.size_t) { C.free(p) }

//export miftahShutdown
func miftahShutdown() {
	_, _ = runtime.dispatch("plugin.shutdown", nil)
}
