package main

import (
	"errors"
	"os"
	"path/filepath"
)

// Default matches CPA's standard auth directory, including the user's Docker volume.
// A non-JSON suffix prevents the host from treating this state as provider credentials.
// Custom CPA auth-dir installations must use the explicit override.
func statePath() (string, error) {
	if p := os.Getenv("MIFTAH_STATE_PATH"); p != "" {
		return p, nil
	}
	if _, err := os.Stat("data/miftah/state.json"); err == nil {
		return "", errors.New("legacy Miftah state found: stop CPA and migrate it to ~/.cli-proxy-api/miftah/state.db, or explicitly set MIFTAH_STATE_PATH to the old path")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cli-proxy-api", "miftah", "state.db"), nil
}
