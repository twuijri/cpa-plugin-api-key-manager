package main

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"miftah.local/plugin/internal/bridge"
	"miftah.local/plugin/internal/core"
)

func testRuntime(t *testing.T, path string) *pluginRuntime {
	t.Helper()
	r := &pluginRuntime{path: func() (string, error) { return path, nil }}
	t.Cleanup(func() { _, _ = r.dispatch("plugin.shutdown", nil) })
	return r
}

func TestRuntimeHandoverAndRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	old, next := testRuntime(t, path), testRuntime(t, path)
	if _, err := old.dispatch("plugin.register", nil); err != nil {
		t.Fatal(err)
	}
	if err := old.app.Store.PutRoute(core.Route{Alias: "model", Targets: []string{"upstream"}, MaxOutput: 64}, old.app.Store.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	_, secret, err := old.app.Store.Create(core.Key{Name: "original", Models: []string{"model"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := next.dispatch("plugin.register", nil); err == nil {
		t.Fatal("second writer admitted")
	}
	if _, err := old.dispatch("plugin.quiesce", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := old.dispatch("management.handle", nil); err == nil {
		t.Fatal("quiesced RPC admitted")
	}
	if _, err := next.dispatch("plugin.register", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := next.app.Store.Authenticate(secret); err != nil {
		t.Fatal(err)
	}
	k := next.app.Store.Snapshot().Keys[0]
	k.Name = "changed by new version"
	if err := next.app.Store.Update(k, next.app.Store.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	// Delayed old-library shutdown must not disturb the replacement lock.
	_, _ = old.dispatch("plugin.shutdown", nil)
	if s, err := core.Open(path); err == nil {
		s.Close()
		t.Fatal("replacement lock lost")
	}
	_, _ = next.dispatch("plugin.quiesce", nil)
	if _, err := old.dispatch("plugin.reconfigure", nil); err != nil {
		t.Fatal(err)
	}
	if old.app.Store.Snapshot().Keys[0].Name != k.Name {
		t.Fatal("rollback resumed stale state")
	}
}

func TestRuntimeQuiesceWaitsForExecution(t *testing.T) {
	r := testRuntime(t, filepath.Join(t.TempDir(), "state.json"))
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	r.host = func(string, any, any) error {
		close(entered)
		<-release
		return &bridge.RPCError{Code: "test", Message: "test", HTTPStatus: 401}
	}
	if _, err := r.dispatch("plugin.register", nil); err != nil {
		t.Fatal(err)
	}
	s := r.app.Store
	if err := s.PutRoute(core.Route{Alias: "model", Targets: []string{"upstream"}, MaxOutput: 64}, s.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	_, key, err := s.Create(core.Key{Name: "test", Models: []string{"model"}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(bridge.Request{Model: "model", Headers: map[string][]string{"Authorization": {"Bearer " + key}}, Payload: []byte(`{"model":"model","messages":[],"max_tokens":8}`)})
	done := make(chan struct{})
	go func() { defer close(done); _, _ = r.dispatch("executor.execute", raw) }()
	<-entered
	quiesced := make(chan struct{})
	go func() { _, _ = r.dispatch("plugin.quiesce", nil); close(quiesced) }()
	select {
	case <-quiesced:
		t.Fatal("closed store before active execution finished")
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
	<-done
	select {
	case <-quiesced:
	case <-time.After(2 * time.Second):
		t.Fatal("quiesce stuck")
	}
}
