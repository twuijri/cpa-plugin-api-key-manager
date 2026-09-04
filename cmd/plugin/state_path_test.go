package main

import (
	"miftah.local/plugin/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentStatePath(t *testing.T) {
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MIFTAH_STATE_PATH", "")
	p, err := statePath()
	if err != nil || p != filepath.Join(home, ".cli-proxy-api/miftah/state.db") {
		t.Fatalf("%q %v", p, err)
	}
	s, err := core.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = core.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("state permissions")
	}
	if err := os.MkdirAll("data/miftah", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("data/miftah/state.json", []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := statePath(); err == nil {
		t.Fatal("legacy state silently ignored")
	}
	t.Setenv("MIFTAH_STATE_PATH", "data/miftah/state.json")
	if got, err := statePath(); err != nil || got != "data/miftah/state.json" {
		t.Fatal("override broken")
	}
}
