// Package identity resolves learners' display identities (name, email) from
// Casdoor through one process-wide cache, shared by every trainer surface that
// shows learner names (live progress, participant controls, the live wall).
package identity

import (
	"sync"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// Identity is a learner's name and email as a trainer sees them.
type Identity struct{ Name, Email string }

// FetchUser is the Casdoor call behind a lookup; tests swap it.
var FetchUser func(id string) (*casdoorsdk.User, error) = casdoorsdk.GetUserByUserId

// TTL is how long a fetched identity is served without a refresh.
var TTL = 5 * time.Minute

// Now is the clock entries expire against; tests swap it.
var Now = time.Now

// MaxConcurrentFetches bounds the Casdoor calls in flight at once.
const MaxConcurrentFetches = 8

type entry struct {
	identity   Identity
	expiresAt  time.Time
	refreshing bool
}

var (
	mu    sync.Mutex
	cache = map[string]*entry{}
	slots = make(chan struct{}, MaxConcurrentFetches)
)

// LookupUsers returns the identity of each id it could resolve. Fresh entries
// come from the cache; misses are fetched concurrently and the call waits for
// them; an expired entry is returned as is and refreshed in the background,
// once. A failed fetch leaves the id out of the result and is not cached.
func LookupUsers(ids []string) map[string]Identity {
	out := make(map[string]Identity, len(ids))
	var misses []string
	now := Now()
	fetchUser := FetchUser // read once here: background refreshes never touch the global

	mu.Lock()
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, done := out[id]; done {
			continue
		}
		e, ok := cache[id]
		if !ok {
			out[id] = Identity{} // placeholder dedups misses; removed below if the fetch fails
			misses = append(misses, id)
			continue
		}
		out[id] = e.identity
		if !now.Before(e.expiresAt) && !e.refreshing {
			e.refreshing = true
			go refresh(fetchUser, id)
		}
	}
	mu.Unlock()

	fetched := make([]*Identity, len(misses))
	var wg sync.WaitGroup
	for i, id := range misses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fetched[i] = fetch(fetchUser, id)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	for i, id := range misses {
		if fetched[i] == nil {
			delete(out, id)
			continue
		}
		out[id] = *fetched[i]
		cache[id] = &entry{identity: *fetched[i], expiresAt: Now().Add(TTL)}
	}
	return out
}

// refresh re-fetches an expired entry; on failure the stale identity stays.
func refresh(fetchUser func(string) (*casdoorsdk.User, error), id string) {
	got := fetch(fetchUser, id)
	mu.Lock()
	defer mu.Unlock()
	e, ok := cache[id]
	if !ok {
		return // dropped by Reset meanwhile
	}
	e.refreshing = false
	if got != nil {
		e.identity = *got
		e.expiresAt = Now().Add(TTL)
	}
}

// fetch makes one bounded Casdoor call; nil means no identity.
func fetch(fetchUser func(string) (*casdoorsdk.User, error), id string) *Identity {
	slots <- struct{}{}
	defer func() { <-slots }()
	user, err := fetchUser(id)
	if err != nil || user == nil {
		return nil
	}
	name := user.DisplayName
	if name == "" {
		name = user.Name
	}
	return &Identity{Name: name, Email: user.Email}
}

// Reset drops every cached entry.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	cache = map[string]*entry{}
}
