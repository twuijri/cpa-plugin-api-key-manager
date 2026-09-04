package core

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Store, string) {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "state.json"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	r := Route{Alias: "work", Targets: []string{"a", "b"}, RetryStatuses: []int{429, 503}, InputPrice: 1000000, OutputPrice: 1000000, MaxOutput: 1000}
	if e = s.PutRoute(r, s.Snapshot().Revision); e != nil {
		t.Fatal(e)
	}
	_, raw, e := s.Create(Key{Name: "test", Models: []string{"work"}, Limits: Limits{Daily: 100000, Concurrent: 2}})
	if e != nil {
		t.Fatal(e)
	}
	return s, raw
}
func TestSecretsNotPersisted(t *testing.T) {
	s, raw := fixture(t)
	b, _ := os.ReadFile(s.path)
	if strings.Contains(string(b), raw) {
		t.Fatal("plaintext persisted")
	}
	if s.Snapshot().Keys[0].Hash != "" {
		t.Fatal("hash exposed")
	}
	st, _ := os.Stat(s.path)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
}
func TestRotationAndDisable(t *testing.T) {
	s, raw := fixture(t)
	k := s.Snapshot().Keys[0]
	next, e := s.Rotate(k.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(raw); e == nil {
		t.Fatal("old key accepted")
	}
	if _, e = s.Authenticate(next); e != nil {
		t.Fatal(e)
	}
	k.Enabled = false
	if e = s.Update(k, s.Snapshot().Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(next); e == nil {
		t.Fatal("disabled key accepted")
	}
}
func TestExpiryAndModelIsolation(t *testing.T) {
	s, raw := fixture(t)
	if _, _, e := s.Reserve(raw, "other", 100, 10); e == nil {
		t.Fatal("wrong model allowed")
	}
	k := s.Snapshot().Keys[0]
	k.ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if e := s.Update(k, s.Snapshot().Revision); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Authenticate(raw); e == nil {
		t.Fatal("expired accepted")
	}
}
func TestConcurrentReservationLimit(t *testing.T) {
	s, raw := fixture(t)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, e := s.Reserve(raw, "work", 100, 10); e == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 2 {
		t.Fatalf("accepted %d", accepted.Load())
	}
}
func TestBudgetAndSettlementIdempotency(t *testing.T) {
	s, raw := fixture(t)
	k := s.Snapshot().Keys[0]
	k.Limits.Daily = 150
	if e := s.Update(k, s.Snapshot().Revision); e != nil {
		t.Fatal(e)
	}
	e, r, err := s.Reserve(raw, "work", 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Reserve(raw, "work", 100, 10); err == nil {
		t.Fatal("budget exceeded")
	}
	if err = s.Finish(e.ID, "a", 10, 10, true, true, 1, r); err != nil {
		t.Fatal(err)
	}
	_ = s.Finish(e.ID, "a", 999, 999, true, true, 1, r)
	if s.Snapshot().Entries[0].Cost != 20 {
		t.Fatal("settled twice")
	}
}
func TestCrashRetainsReservation(t *testing.T) {
	s, raw := fixture(t)
	_, _, e := s.Reserve(raw, "work", 100, 10)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	reopened, e := Open(s.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	entry := reopened.Snapshot().Entries[0]
	if entry.Status != "uncertain" || entry.Cost != 110 {
		t.Fatal(entry)
	}
}
func TestExclusiveWriter(t *testing.T) {
	s, _ := fixture(t)
	other, e := Open(s.path)
	if e == nil {
		other.Close()
		t.Fatal("second writer admitted")
	}
}
func TestConflictAndMissingState(t *testing.T) {
	s, _ := fixture(t)
	r := s.Snapshot().Routes[0]
	if e := s.PutRoute(r, -1); e != ErrConflict {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "broken.json")
	os.WriteFile(p, []byte("{bad"), 0600)
	if _, e := Open(p); e == nil {
		t.Fatal("corruption ignored")
	}
}
func TestExplicitOutputLimit(t *testing.T) {
	s, raw := fixture(t)
	for _, cap := range []int64{-1, 0, 1001} {
		if _, _, e := s.Reserve(raw, "work", 100, cap); e == nil {
			t.Fatalf("cap %d accepted", cap)
		}
	}
}
func TestUnknownUsageRetainsHold(t *testing.T) {
	s, raw := fixture(t)
	e, r, _ := s.Reserve(raw, "work", 100, 10)
	_ = s.Finish(e.ID, "a", 0, 0, false, true, 1, r)
	got := s.Snapshot().Entries[0]
	if got.Cost != 110 || got.Status != "uncertain" {
		t.Fatal(got)
	}
}
func TestPeriodRollOver(t *testing.T) {
	s, raw := fixture(t)
	now := time.Date(2026, 9, 4, 23, 59, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	k := s.Snapshot().Keys[0]
	k.Limits.Daily = 110
	_ = s.Update(k, s.Snapshot().Revision)
	e, r, err := s.Reserve(raw, "work", 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Finish(e.ID, "a", 100, 10, true, true, 1, r)
	now = now.Add(2 * time.Minute)
	if _, _, err = s.Reserve(raw, "work", 100, 10); err != nil {
		t.Fatal(err)
	}
}
