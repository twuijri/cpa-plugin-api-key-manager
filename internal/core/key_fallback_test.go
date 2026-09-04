package core

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPerKeyFallbackIsolationAndPersistence(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	primary := directPolicy("main")
	primary.Targets = []string{"main", "shared"}
	backup := directPolicy("backup")
	backup.OutputPrice = 5000000
	k, a, err := s.CreateWithPolicies(Key{Name: "A", Models: []string{"main"}, Fallbacks: []KeyFallback{{Primary: "main", Fallbacks: []string{"backup"}, RetryStatuses: []int{503}}}}, []Route{primary, backup})
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := s.CreateWithPolicies(Key{Name: "B", Models: []string{"main"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, r, err := s.Reserve(a, "main", 10, 10)
	if err != nil || !reflect.DeepEqual(r.Targets, []string{"main", "backup"}) || r.OutputPrice != backup.OutputPrice {
		t.Fatalf("custom fallback/pricing: %+v %v", r, err)
	}
	_, r, err = s.Reserve(b, "main", 10, 10)
	if err != nil || !reflect.DeepEqual(r.Targets, []string{"main", "shared"}) {
		t.Fatalf("other key changed: %+v %v", r, err)
	}
	if _, _, err = s.Reserve(a, "backup", 10, 10); err == nil {
		t.Fatal("fallback granted direct access")
	}
	k.Fallbacks[0].Fallbacks = nil
	if err = s.UpdateWithPolicies(k, s.Snapshot().Revision, nil); err != nil {
		t.Fatal(err)
	}
	path := s.path
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, r, err = s.Reserve(a, "main", 10, 10)
	if err != nil || !reflect.DeepEqual(r.Targets, []string{"main"}) {
		t.Fatalf("empty override lost after restart: %+v %v", r, err)
	}
}

func TestPerKeyFallbackInvalidAtomic(t *testing.T) {
	for _, f := range []KeyFallback{
		{Primary: "not-allowed", Fallbacks: []string{"backup"}},
		{Primary: "main", Fallbacks: []string{"main"}},
		{Primary: "main", Fallbacks: []string{"backup", "backup"}},
		{Primary: "main", Fallbacks: []string{"missing"}},
		{Primary: "main", Fallbacks: []string{"backup"}, RetryStatuses: []int{401}},
	} {
		s, err := Open(filepath.Join(t.TempDir(), "state.db"))
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.CreateWithPolicies(Key{Name: "invalid", Models: []string{"main"}, Fallbacks: []KeyFallback{f}}, []Route{directPolicy("main"), directPolicy("backup")})
		if err == nil || len(s.Snapshot().Routes) != 0 || len(s.Snapshot().Keys) != 0 {
			t.Fatal("invalid policy was not rolled back", f)
		}
		s.Close()
	}
}
