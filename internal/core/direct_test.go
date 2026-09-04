package core

import (
	"path/filepath"
	"testing"
)

func directPolicy(name string) Route {
	return Route{Kind: "direct", Alias: name, Targets: []string{name}, InputPrice: 1000000, OutputPrice: 1000000, MaxOutput: 1000}
}
func TestDirectModelsAndLegacyRoutes(t *testing.T) {
	s, legacy := fixture(t)
	p := directPolicy("provider/real-model")
	k, raw, err := s.CreateWithPolicies(Key{Name: "direct", Models: []string{p.Alias}}, []Route{p})
	if err != nil {
		t.Fatal(err)
	}
	entry, resolved, err := s.Reserve(raw, p.Alias, 10, 10)
	if err != nil || resolved.Targets[0] != p.Alias {
		t.Fatalf("direct: %+v %v", resolved, err)
	}
	if err = s.Finish(entry.ID, p.Alias, 1, 1, true, true, 1, resolved); err != nil {
		t.Fatal(err)
	}
	p.Targets = append(p.Targets, "backup")
	if err = s.PutRoute(p, s.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	_, resolved, err = s.Reserve(raw, p.Alias, 10, 10)
	if err != nil || len(resolved.Targets) != 2 || resolved.Targets[0] != p.Alias {
		t.Fatal("fallback policy lost")
	}
	if _, _, err = s.Reserve(raw, "backup", 10, 10); err == nil {
		t.Fatal("backup became directly authorized")
	}
	if _, _, err = s.Reserve(legacy, "work", 10, 10); err != nil {
		t.Fatal("legacy route broken", err)
	}
	if _, _, err = s.Reserve(legacy, p.Alias, 10, 10); err == nil {
		t.Fatal("legacy key gained direct access")
	}
	k.Models = []string{"work", p.Alias}
	if err = s.UpdateWithPolicies(k, s.Snapshot().Revision, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Reserve(raw, "work", 10, 10); err != nil {
		t.Fatal("mixed selection failed")
	}
	path := s.path
	s.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, r, err := reopened.Reserve(raw, p.Alias, 10, 10); err != nil || r.Kind != "direct" {
		t.Fatal("restart lost direct policy", err)
	}
}
func TestDirectPoliciesAtomicAndTypeSafe(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := directPolicy("real")
	_, _, err = s.CreateWithPolicies(Key{Name: "invalid", Models: []string{"real", "missing"}}, []Route{p})
	if err == nil || len(s.Snapshot().Routes) != 0 || len(s.Snapshot().Keys) != 0 {
		t.Fatal("partial transaction")
	}
	p.Targets = []string{"different"}
	if err = s.PutRoute(p, s.Snapshot().Revision); err == nil {
		t.Fatal("primary mismatch accepted")
	}
	p = directPolicy("real")
	k, _, err := s.CreateWithPolicies(Key{Name: "ok", Models: []string{"real"}}, []Route{p})
	if err != nil {
		t.Fatal(err)
	}
	p.Kind = "route"
	if err = s.PutRoute(p, s.Snapshot().Revision); err == nil {
		t.Fatal("policy kind changed")
	}
	p = directPolicy("real")
	p.InputPrice = 999
	if _, _, err = s.CreateWithPolicies(Key{Name: "overwrite", Models: []string{"real"}}, []Route{p}); err == nil {
		t.Fatal("shared policy overwritten")
	}
	q := directPolicy("new")
	k.Models = append(k.Models, "new")
	if err = s.UpdateWithPolicies(k, -1, []Route{q}); err == nil {
		t.Fatal("stale revision")
	}
	if _, ok := s.Route("new"); ok {
		t.Fatal("stale update leaked new policy")
	}
	if err = s.UpdateWithPolicies(k, s.Snapshot().Revision, []Route{q}); err != nil {
		t.Fatal(err)
	}
}
