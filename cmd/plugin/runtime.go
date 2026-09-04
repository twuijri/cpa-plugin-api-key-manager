package main

import (
	"sync"

	"miftah.local/plugin/internal/bridge"
	"miftah.local/plugin/internal/core"
)

// Each loaded library owns its own runtime. The gate covers every RPC, including
// management/authentication, not just execution. Quiesce takes exclusive access,
// drains background streams, then releases the file lock before replacement opens.
type pluginRuntime struct {
	gate sync.RWMutex
	app  *bridge.App
	path func() (string, error)
	host bridge.Call
}

func (r *pluginRuntime) dispatch(method string, raw []byte) (any, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		r.gate.Lock()
		defer r.gate.Unlock()
		if r.app == nil {
			path, err := r.path()
			if err != nil {
				return nil, err
			}
			store, err := core.Open(path)
			if err != nil {
				return nil, err
			}
			// On rollback, reopen from disk. Never resume an old in-memory snapshot.
			r.app = &bridge.App{Store: store, Host: r.host}
		}
		return bridge.Registration(), nil
	case "plugin.quiesce", "plugin.shutdown":
		r.gate.Lock()
		defer r.gate.Unlock()
		if r.app != nil {
			r.app.Quiesce()
			r.app.Store.Close()
			r.app = nil
		}
		return map[string]bool{"ok": true}, nil
	default:
		r.gate.RLock()
		defer r.gate.RUnlock()
		if r.app == nil {
			return nil, &bridge.RPCError{Code: "unavailable", Message: "plugin suspended or not initialized", HTTPStatus: 503}
		}
		return r.app.Handle(method, raw)
	}
}
