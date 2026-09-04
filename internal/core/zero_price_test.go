package core

import (
	"path/filepath"
	"testing"
)

func TestZeroPricingKeepsUsageAndLimits(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := directPolicy("free")
	p.InputPrice = 0
	p.OutputPrice = 0
	_, raw, err := s.CreateWithPolicies(Key{Name: "personal", Models: []string{"free"}, Limits: Limits{Concurrent: 1, RPM: 2}}, []Route{p})
	if err != nil {
		t.Fatal(err)
	}
	e, r, err := s.Reserve(raw, "free", 10, 10)
	if err != nil || e.Cost != 0 {
		t.Fatal("zero reserve", err, e)
	}
	if _, _, err = s.Reserve(raw, "free", 10, 10); err == nil {
		t.Fatal("concurrency bypass")
	}
	if err = s.Finish(e.ID, "free", 100, 200, true, true, 1, r); err != nil {
		t.Fatal(err)
	}
	e = s.Snapshot().Entries[0]
	if e.Input != 100 || e.Output != 200 || e.Cost != 0 || e.Status != "settled" {
		t.Fatal("zero pricing lost usage", e)
	}
	e, r, err = s.Reserve(raw, "free", 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(e.ID, "free", 1, 1, true, true, 1, r); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Reserve(raw, "free", 10, 10); err == nil {
		t.Fatal("RPM bypass")
	}
	for _, prices := range [][2]int64{{0, 1000000}, {1000000, 0}, {0, 0}} {
		p.InputPrice = prices[0]
		p.OutputPrice = prices[1]
		if err = validRoute(p); err != nil {
			t.Fatal(err)
		}
	}
	p.InputPrice = -1
	if validRoute(p) == nil {
		t.Fatal("negative input accepted")
	}
	p.InputPrice = 0
	p.OutputPrice = -1
	if validRoute(p) == nil {
		t.Fatal("negative output accepted")
	}
}
