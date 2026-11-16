package test_tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// FakeCasdoorUsers is an HTTP stand-in for Casdoor's GET /api/get-user, the
// endpoint behind casdoorsdk.GetUserByUserId. Pointing the SDK at it counts the
// user lookups a code path really makes, whatever seam it goes through, so a
// test can pin "how many Casdoor calls" without knowing the code's internals.
type FakeCasdoorUsers struct {
	mu          sync.Mutex
	users       map[string]casdoorsdk.User
	calls       map[string]int
	overlapWait time.Duration
	inFlight    atomic.Int32
	maxInFlight atomic.Int32
	firstCall   sync.Once
	deadline    time.Time
}

// StartFakeCasdoorUsers points the Casdoor SDK at a fresh fake. With a
// positive `overlapWait`, each user lookup is held until two lookups overlap,
// so concurrency shows whatever the runner's speed; a sequential caller never
// overlaps and is released once overlapWait has passed since the first lookup. Cleanup closes it and points the SDK back at
// the unreachable dummy endpoint the test packages use by default.
func StartFakeCasdoorUsers(t *testing.T, overlapWait time.Duration) *FakeCasdoorUsers {
	t.Helper()
	f := &FakeCasdoorUsers{users: map[string]casdoorsdk.User{}, calls: map[string]int{}, overlapWait: overlapWait}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	casdoorsdk.InitConfig(srv.URL, "dummy", "dummy", "dummy", "dummy", "dummy")
	t.Cleanup(func() {
		casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
		srv.Close()
	})
	return f
}

// AddUser registers a user the fake knows. An unknown id is answered with no
// user (data: null).
func (f *FakeCasdoorUsers) AddUser(id, displayName, name, email string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[id] = casdoorsdk.User{Id: id, DisplayName: displayName, Name: name, Email: email}
}

// Calls returns how many lookups were made for the given ids (all ids when
// none is given).
func (f *FakeCasdoorUsers) Calls(ids ...string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	if len(ids) == 0 {
		for _, n := range f.calls {
			total += n
		}
		return total
	}
	for _, id := range ids {
		total += f.calls[id]
	}
	return total
}

// MaxInFlight is the largest number of lookups the fake served at once.
func (f *FakeCasdoorUsers) MaxInFlight() int { return int(f.maxInFlight.Load()) }

func (f *FakeCasdoorUsers) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path != "/api/get-user" {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "msg": "unexpected " + r.URL.Path})
		return
	}
	now := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		seen := f.maxInFlight.Load()
		if now <= seen || f.maxInFlight.CompareAndSwap(seen, now) {
			break
		}
	}

	id := r.URL.Query().Get("userId")
	f.mu.Lock()
	f.calls[id]++
	user, known := f.users[id]
	f.mu.Unlock()

	f.firstCall.Do(func() { f.deadline = time.Now().Add(f.overlapWait) })
	for f.maxInFlight.Load() < 2 && time.Now().Before(f.deadline) {
		time.Sleep(time.Millisecond)
	}
	var data any
	if known {
		data = user
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": data})
}
