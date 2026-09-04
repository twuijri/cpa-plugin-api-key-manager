package core

import (
	"path/filepath"
	"testing"
)

func TestKeyPricesIsolationSnapshotAndFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	primary, backup := directPolicy("primary"), directPolicy("backup")
	primary.InputPrice, primary.OutputPrice = 1000000, 2000000
	backup.InputPrice, backup.OutputPrice = 3000000, 4000000
	a, rawA, err := s.CreateWithPolicies(Key{Name: "premium", Models: []string{"primary", "backup"}, PricingMode: "models", Prices: map[string]Price{"primary": {InputPrice: 5000000, OutputPrice: 6000000}, "backup": {InputPrice: 7000000, OutputPrice: 8000000}}, Fallbacks: []KeyFallback{{Primary: "primary", Fallbacks: []string{"backup"}, RetryStatuses: []int{503}}}}, []Route{primary, backup})
	if err != nil {
		t.Fatal(err)
	}
	_, rawB, err := s.Create(Key{Name: "standard", Models: []string{"primary"}, PricingMode: "models"})
	if err != nil {
		t.Fatal(err)
	}
	e, r, err := s.Reserve(rawA, "primary", 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if e.Cost != 1500 {
		t.Fatalf("reservation=%d", e.Cost)
	}
	a.Prices["backup"] = Price{}
	if err = s.Update(a, s.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(e.ID, "backup", 100, 200, true, true, 2, r); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Entries[0].Cost; got != 2300 {
		t.Fatalf("snapshot fallback cost=%d", got)
	}
	e, r, err = s.Reserve(rawB, "primary", 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(e.ID, "primary", 100, 200, true, true, 1, r); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().Entries[1].Cost; got != 500 {
		t.Fatalf("other key changed: %d", got)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	e, r, err = s.Reserve(rawA, "backup", 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if e.Cost != 0 {
		t.Fatal("explicit zero lost on reopen")
	}
	if err = s.Finish(e.ID, "backup", 100, 200, true, true, 1, r); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Entries[2].Cost != 0 {
		t.Fatal("zero settlement")
	}
}

func TestSyncDirectModelsPricesAtomically(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	existing := directPolicy("existing")
	existing.Targets = []string{"existing", "backup"}
	if err = s.PutRoute(existing, s.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	prices := map[string]Price{"existing": {InputPrice: 2, OutputPrice: 3}, "new": {InputPrice: 4, OutputPrice: 5}}
	if err = s.SyncDirectModels([]string{"existing", "new", "unknown"}, prices, s.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	if len(s.Snapshot().Routes) != 3 {
		t.Fatal(s.Snapshot().Routes)
	}
	r, _ := s.Route("existing")
	if r.InputPrice != 2 || len(r.Targets) != 2 {
		t.Fatal("existing fallback overwritten", r)
	}
	r, _ = s.Route("unknown")
	if r.InputPrice != 0 || r.OutputPrice != 0 {
		t.Fatal("unmatched price guessed", r)
	}
	revision := s.Snapshot().Revision
	if err = s.SyncDirectModels([]string{"bad", "bad"}, nil, revision); err == nil {
		t.Fatal("duplicate accepted")
	}
	if s.Snapshot().Revision != revision {
		t.Fatal("invalid sync mutated state")
	}
}

func TestKeyPricesBudgetAndValidation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, raw, err := s.CreateWithPolicies(Key{Name: "limited", Models: []string{"m"}, Limits: Limits{Total: 1}, Prices: map[string]Price{"m": {InputPrice: 1000000, OutputPrice: 1000000}}}, []Route{directPolicy("m")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Reserve(raw, "m", 100, 100); err == nil {
		t.Fatal("custom pricing bypassed budget")
	}
	before := s.Snapshot().Revision
	for _, p := range []Price{{InputPrice: -1}, {OutputPrice: 1000000001}} {
		if _, _, err = s.Create(Key{Name: "bad", Models: []string{"m"}, Prices: map[string]Price{"m": p}}); err == nil {
			t.Fatal("bad price accepted")
		}
	}
	if s.Snapshot().Revision != before {
		t.Fatal("invalid prices mutated state")
	}
}
