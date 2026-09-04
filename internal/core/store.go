package core

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const Prefix = "mf_"

type Limits struct {
	Total      int64 `json:"total"`
	Daily      int64 `json:"daily"`
	Weekly     int64 `json:"weekly"`
	Monthly    int64 `json:"monthly"`
	RPM        int   `json:"rpm"`
	Concurrent int   `json:"concurrent"`
}
type Key struct {
	Fallbacks []KeyFallback `json:"fallbacks,omitempty"`
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Owner     string        `json:"owner"`
	Hash      string        `json:"hash,omitempty"`
	Preview   string        `json:"preview"`
	Enabled   bool          `json:"enabled"`
	ExpiresAt string        `json:"expires_at"`
	Models    []string      `json:"models"`
	Limits    Limits        `json:"limits"`
	CreatedAt time.Time     `json:"created_at"`
}
type KeyFallback struct {
	Primary       string   `json:"primary"`
	Fallbacks     []string `json:"fallbacks"`
	RetryStatuses []int    `json:"retry_statuses"`
	RetryUnknown  bool     `json:"retry_unknown"`
}
type Route struct {
	Kind          string   `json:"kind,omitempty"` // empty is a legacy named route; direct uses the actual model ID
	Alias         string   `json:"alias"`
	Targets       []string `json:"targets"`
	RetryStatuses []int    `json:"retry_statuses"`
	RetryUnknown  bool     `json:"retry_unknown"`
	InputPrice    int64    `json:"input_price"`
	OutputPrice   int64    `json:"output_price"`
	MaxOutput     int64    `json:"max_output"`
}
type Entry struct {
	ID       string    `json:"id"`
	KeyID    string    `json:"key_id"`
	Alias    string    `json:"alias"`
	Model    string    `json:"model"`
	At       time.Time `json:"at"`
	Cost     int64     `json:"cost"`
	Input    int64     `json:"input"`
	Output   int64     `json:"output"`
	Status   string    `json:"status"`
	Attempts int       `json:"attempts"`
}
type Audit struct {
	At      time.Time `json:"at"`
	Action  string    `json:"action"`
	Subject string    `json:"subject"`
}
type State struct {
	Version  int     `json:"version"`
	Revision int64   `json:"revision"`
	Keys     []Key   `json:"keys"`
	Routes   []Route `json:"routes"`
	Entries  []Entry `json:"entries"`
	Audit    []Audit `json:"audit"`
}
type Store struct {
	mu     sync.Mutex
	path   string
	state  State
	now    func() time.Time
	closed bool
	lock   *os.File
}

var ErrConflict = errors.New("configuration changed; refresh and retry")

func Open(path string) (*Store, error) {
	s := &Store{path: path, now: func() time.Time { return time.Now().UTC() }, state: State{Version: 1, Keys: []Key{}, Routes: []Route{}, Entries: []Entry{}, Audit: []Audit{}}}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("state is already open by another process")
	}
	s.lock = lock
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	b, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(b, &s.state); err != nil {
			return nil, fmt.Errorf("invalid state: %w", err)
		}
		if s.state.Version != 1 {
			return nil, errors.New("unsupported state version")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Outstanding reservations survive restarts and continue to count toward spend.
	// They are not silently released after a crash: upstream may already have charged.
	for i := range s.state.Entries {
		if s.state.Entries[i].Status == "held" {
			s.state.Entries[i].Status = "uncertain"
		}
	}
	if err = s.save(s.state); err != nil {
		return nil, err
	}
	ok = true
	return s, nil
}

func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.lock != nil {
		_ = syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
		_ = s.lock.Close()
		s.lock = nil
	}
}
func (s *Store) save(st State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".miftah-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, s.path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		s.closed = true
		return err
	}
	err = dir.Sync()
	_ = dir.Close()
	if err != nil {
		s.closed = true
		return err
	}
	return nil
}
func clone(st State) State {
	b, _ := json.Marshal(st)
	var out State
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *Store) change(fn func(*State) error) error {
	if s.closed {
		return errors.New("store unavailable")
	}
	st := clone(s.state)
	if err := fn(&st); err != nil {
		return err
	}
	st.Revision++
	if err := s.save(st); err != nil {
		return err
	}
	s.state = st
	return nil
}
func (s *Store) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := clone(s.state)
	for i := range st.Keys {
		st.Keys[i].Hash = ""
	}
	return st
}
func random(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
func digest(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func validKey(k Key) error {
	if strings.TrimSpace(k.Name) == "" || len(k.Name) > 100 || len(k.Owner) > 100 {
		return errors.New("name required; names limited to 100 characters")
	}
	if len(k.Models) == 0 {
		return errors.New("select at least one model or route")
	}
	if k.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, k.ExpiresAt); err != nil {
			return errors.New("invalid expiry")
		}
	}
	l := k.Limits
	if l.Total < 0 || l.Daily < 0 || l.Weekly < 0 || l.Monthly < 0 || l.RPM < 0 || l.Concurrent < 0 || l.Total > 1e15 || l.Daily > 1e15 || l.Weekly > 1e15 || l.Monthly > 1e15 {
		return errors.New("invalid limits")
	}
	return nil
}
func logAudit(st *State, at time.Time, action, id string) {
	st.Audit = append(st.Audit, Audit{at, action, id})
}
func (s *Store) Create(k Key) (Key, string, error) {
	return s.CreateWithPolicies(k, nil)
}
func (s *Store) CreateWithPolicies(k Key, policies []Route) (Key, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validKey(k); err != nil {
		return Key{}, "", err
	}
	id, err := random(8)
	if err != nil {
		return Key{}, "", err
	}
	secret, err := random(32)
	if err != nil {
		return Key{}, "", err
	}
	raw := Prefix + id + "_" + secret
	k.ID = id
	k.Hash = digest(raw)
	k.Preview = Prefix + id + "_…"
	k.CreatedAt = s.now()
	k.Enabled = true
	err = s.change(func(st *State) error {
		if err := addDirectPolicies(st, k, policies, s.now()); err != nil {
			return err
		}
		if err := validateKeyFallbacks(*st, k); err != nil {
			return err
		}
		for _, m := range k.Models {
			if _, ok := route(*st, m); !ok {
				return errors.New("unknown route")
			}
		}
		st.Keys = append(st.Keys, k)
		logAudit(st, s.now(), "key.created", id)
		return nil
	})
	k.Hash = ""
	if err != nil {
		return Key{}, "", err
	}
	return k, raw, nil
}
func (s *Store) Update(k Key, revision int64) error {
	return s.UpdateWithPolicies(k, revision, nil)
}
func (s *Store) UpdateWithPolicies(k Key, revision int64, policies []Route) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validKey(k); err != nil {
		return err
	}
	return s.change(func(st *State) error {
		if revision != st.Revision {
			return ErrConflict
		}
		if err := addDirectPolicies(st, k, policies, s.now()); err != nil {
			return err
		}
		if err := validateKeyFallbacks(*st, k); err != nil {
			return err
		}
		for _, m := range k.Models {
			if _, ok := route(*st, m); !ok {
				return errors.New("unknown route")
			}
		}
		for i, old := range st.Keys {
			if old.ID == k.ID {
				k.Hash = old.Hash
				k.Preview = old.Preview
				k.CreatedAt = old.CreatedAt
				st.Keys[i] = k
				logAudit(st, s.now(), "key.updated", k.ID)
				return nil
			}
		}
		return errors.New("key not found")
	})
}
func (s *Store) Rotate(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secret, err := random(32)
	if err != nil {
		return "", err
	}
	raw := Prefix + id + "_" + secret
	err = s.change(func(st *State) error {
		for i := range st.Keys {
			if st.Keys[i].ID == id {
				st.Keys[i].Hash = digest(raw)
				logAudit(st, s.now(), "key.rotated", id)
				return nil
			}
		}
		return errors.New("key not found")
	})
	if err != nil {
		return "", err
	}
	return raw, nil
}
func (s *Store) PutRoute(r Route, rev int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validRoute(r); err != nil {
		return err
	}
	return s.change(func(st *State) error {
		if rev != st.Revision {
			return ErrConflict
		}
		for i := range st.Routes {
			if st.Routes[i].Alias == r.Alias {
				if (st.Routes[i].Kind == "direct") != (r.Kind == "direct") {
					return errors.New("name already belongs to a different model/route type")
				}
				st.Routes[i] = r
				logAudit(st, s.now(), "route.updated", r.Alias)
				return nil
			}
		}
		st.Routes = append(st.Routes, r)
		logAudit(st, s.now(), "route.created", r.Alias)
		return nil
	})
}
func validRoute(r Route) error {
	if r.Kind != "" && r.Kind != "route" && r.Kind != "direct" {
		return errors.New("invalid policy kind")
	}
	if strings.TrimSpace(r.Alias) != r.Alias || r.Alias == "" || len(r.Alias) > 200 || len(r.Targets) == 0 || len(r.Targets) > 5 || r.InputPrice <= 0 || r.OutputPrice <= 0 || r.InputPrice > 1e9 || r.OutputPrice > 1e9 || r.MaxOutput < 1 || r.MaxOutput > 131072 {
		return errors.New("route needs 1–5 targets, positive prices and output cap")
	}
	if r.Kind == "direct" && r.Targets[0] != r.Alias {
		return errors.New("direct policy must start with requested model")
	}
	seen := map[string]bool{}
	for _, v := range r.Targets {
		if v == "" || strings.TrimSpace(v) != v || len(v) > 200 || seen[v] {
			return errors.New("invalid or duplicate target")
		}
		seen[v] = true
	}
	for _, n := range r.RetryStatuses {
		if n != 429 && n != 502 && n != 503 && n != 504 {
			return errors.New("retry only 429/502/503/504")
		}
	}
	return nil
}

// New policies and the key are committed atomically. This never overwrites shared pricing/fallback.
func addDirectPolicies(st *State, k Key, policies []Route, now time.Time) error {
	if len(policies) > 200 {
		return errors.New("too many new models")
	}
	for _, p := range policies {
		if p.Kind != "direct" {
			return errors.New("only direct models can be added with a key")
		}
		if err := validRoute(p); err != nil {
			return err
		}
		selected := false
		for _, name := range k.Models {
			if name == p.Alias {
				selected = true
			}
		}
		for _, f := range k.Fallbacks {
			for _, name := range f.Fallbacks {
				if name == p.Alias {
					selected = true
				}
			}
		}
		if !selected {
			return errors.New("new policy must be selected by key")
		}
		if _, exists := route(*st, p.Alias); exists {
			return ErrConflict
		}
		st.Routes = append(st.Routes, p)
		logAudit(st, now, "model.created", p.Alias)
	}
	return nil
}
func route(st State, alias string) (Route, bool) {
	for _, r := range st.Routes {
		if r.Alias == alias {
			return r, true
		}
	}
	return Route{}, false
}
func (s *Store) Route(alias string) (Route, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return route(clone(s.state), alias)
}
func find(st State, raw string, now time.Time) (Key, error) {
	if !strings.HasPrefix(raw, Prefix) {
		return Key{}, errors.New("invalid virtual key")
	}
	h := digest(raw)
	for _, k := range st.Keys {
		if subtle.ConstantTimeCompare([]byte(h), []byte(k.Hash)) == 1 {
			if !k.Enabled {
				return Key{}, errors.New("key disabled")
			}
			if k.ExpiresAt != "" {
				t, _ := time.Parse(time.RFC3339, k.ExpiresAt)
				if !now.Before(t) {
					return Key{}, errors.New("key expired")
				}
			}
			return k, nil
		}
	}
	return Key{}, errors.New("invalid virtual key")
}
func (s *Store) Authenticate(raw string) (Key, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Key{}, errors.New("store unavailable")
	}
	return find(s.state, raw, s.now())
}
func cost(in, out int64, r Route) int64 {
	return (in*r.InputPrice + out*r.OutputPrice + 999999) / 1000000
}
func (s *Store) Reserve(raw, alias string, bytes int, maxOutput int64) (Entry, Route, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	k, err := find(s.state, raw, now)
	if err != nil {
		return Entry{}, Route{}, err
	}
	allowed := false
	for _, m := range k.Models {
		if m == alias {
			allowed = true
		}
	}
	if !allowed {
		return Entry{}, Route{}, errors.New("model not allowed")
	}
	r, ok := route(s.state, alias)
	if !ok {
		return Entry{}, Route{}, errors.New("unknown route")
	}
	for _, f := range k.Fallbacks {
		if f.Primary != alias {
			continue
		}
		r.Targets = append([]string{alias}, f.Fallbacks...)
		r.RetryStatuses = f.RetryStatuses
		r.RetryUnknown = f.RetryUnknown
		for _, target := range f.Fallbacks {
			p, exists := route(s.state, target)
			if !exists || p.Kind != "direct" {
				return Entry{}, Route{}, errors.New("fallback pricing unavailable")
			}
			if p.InputPrice > r.InputPrice {
				r.InputPrice = p.InputPrice
			}
			if p.OutputPrice > r.OutputPrice {
				r.OutputPrice = p.OutputPrice
			}
		}
		break
	}
	if bytes < 1 || bytes > 4<<20 {
		return Entry{}, Route{}, errors.New("request too large")
	}
	if maxOutput <= 0 || maxOutput > r.MaxOutput {
		return Entry{}, Route{}, errors.New("explicit output cap required within route limit")
	}
	held := cost(int64(bytes), maxOutput, r)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	week := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	var total, daily, weekly, monthly int64
	rpm, active := 0, 0
	for _, e := range s.state.Entries {
		if e.KeyID != k.ID {
			continue
		}
		total += e.Cost
		if !e.At.Before(day) {
			daily += e.Cost
		}
		if !e.At.Before(week) {
			weekly += e.Cost
		}
		if !e.At.Before(month) {
			monthly += e.Cost
		}
		if e.At.After(now.Add(-time.Minute)) {
			rpm++
		}
		if e.Status == "held" {
			active++
		}
	}
	for _, p := range [][2]int64{{k.Limits.Total, total}, {k.Limits.Daily, daily}, {k.Limits.Weekly, weekly}, {k.Limits.Monthly, monthly}} {
		if p[0] > 0 && held > p[0]-p[1] {
			return Entry{}, Route{}, errors.New("budget exceeded")
		}
	}
	if k.Limits.RPM > 0 && rpm >= k.Limits.RPM {
		return Entry{}, Route{}, errors.New("rate limit exceeded")
	}
	if k.Limits.Concurrent > 0 && active >= k.Limits.Concurrent {
		return Entry{}, Route{}, errors.New("concurrency limit exceeded")
	}
	id, err := random(12)
	if err != nil {
		return Entry{}, Route{}, err
	}
	e := Entry{ID: id, KeyID: k.ID, Alias: alias, At: now, Cost: held, Status: "held"}
	err = s.change(func(st *State) error { st.Entries = append(st.Entries, e); return nil })
	return e, r, err
}
func (s *Store) Finish(id, model string, in, out int64, known, success bool, attempts int, r Route) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.change(func(st *State) error {
		for i := range st.Entries {
			e := &st.Entries[i]
			if e.ID != id {
				continue
			}
			if e.Status != "held" {
				return nil
			}
			e.Model = model
			e.Attempts = attempts
			if known && in >= 0 && out >= 0 && in <= 1e9 && out <= 1e9 {
				e.Input = in
				e.Output = out
				e.Cost = cost(in, out, r)
				e.Status = "settled"
			} else {
				e.Status = "uncertain"
			}
			if !success {
				e.Status = "failed_" + e.Status
			}
			return nil
		}
		return errors.New("reservation not found")
	})
}
