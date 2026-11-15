package identity_test

// The shared, cached learner identity lookup (#586). Every teacher surface that
// shows learner names — live progress, participant controls, the live wall —
// resolves them through identity.LookupUsers, so a class is looked up once and
// a 47-learner class no longer costs 47 sequential Casdoor calls per request.
//
// Contract (package soli/formations/src/auth/identity):
//
//	type Identity struct{ Name, Email string }
//	var FetchUser func(id string) (*casdoorsdk.User, error) // = casdoorsdk.GetUserByUserId
//	var TTL time.Duration                                   // = 5 * time.Minute
//	var Now func() time.Time                                // = time.Now
//	const MaxConcurrentFetches = 8
//	func LookupUsers(ids []string) map[string]Identity
//	func Reset()                                            // drops every cached entry
//
//   - a fresh entry is served from the cache, no Casdoor call;
//   - misses are fetched concurrently, never more than MaxConcurrentFetches at once;
//   - an expired entry is served immediately (stale) and refreshed in the
//     background, once, however many lookups ask for it meanwhile;
//   - a failed fetch (error, or no user) is not cached: the id is absent from
//     the result and fetched again by the next lookup; a failed refresh keeps
//     the stale identity;
//   - Name is the Casdoor DisplayName, falling back to the login Name;
//   - empty ids are never fetched, duplicate ids are fetched once.

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/identity"
)

// fakeCasdoor replaces identity.FetchUser. reply decides each answer; it runs
// outside the lock, so it may block.
type fakeCasdoor struct {
	mu          sync.Mutex
	calls       map[string]int
	inFlight    atomic.Int32
	maxInFlight atomic.Int32
	reply       func(id string) (*casdoorsdk.User, error)
}

func (f *fakeCasdoor) fetch(id string) (*casdoorsdk.User, error) {
	now := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		seen := f.maxInFlight.Load()
		if now <= seen || f.maxInFlight.CompareAndSwap(seen, now) {
			break
		}
	}
	f.mu.Lock()
	f.calls[id]++
	reply := f.reply
	f.mu.Unlock()
	return reply(id)
}

func (f *fakeCasdoor) setReply(reply func(id string) (*casdoorsdk.User, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reply = reply
}

func (f *fakeCasdoor) callsFor(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[id]
}

func (f *fakeCasdoor) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for _, n := range f.calls {
		total += n
	}
	return total
}

func named(id string) (*casdoorsdk.User, error) {
	return &casdoorsdk.User{Id: id, DisplayName: "Name of " + id, Email: id + "@example.com"}, nil
}

// fakeClock replaces identity.Now so expiry is reached without sleeping.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// setup empties the cache and swaps the Casdoor call and the clock for fakes,
// restoring the originals afterwards.
func setup(t *testing.T) (*fakeCasdoor, *fakeClock) {
	t.Helper()
	fake := &fakeCasdoor{calls: map[string]int{}, reply: named}
	clock := &fakeClock{now: time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)}
	origFetch, origNow, origTTL := identity.FetchUser, identity.Now, identity.TTL
	identity.Reset()
	identity.FetchUser = fake.fetch
	identity.Now = clock.Now
	identity.TTL = 5 * time.Minute
	t.Cleanup(func() {
		identity.FetchUser, identity.Now, identity.TTL = origFetch, origNow, origTTL
		identity.Reset()
	})
	return fake, clock
}

func ids(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%02d", prefix, i)
	}
	return out
}

// TestLookupUsers_ColdClass_FetchesConcurrentlyWithinPool is the production
// slowness: a cold class of 40 learners at 50 ms per Casdoor call took 2 s
// sequentially. Concurrent fetches bring it under 10 calls' worth, and the pool
// never sends Casdoor more than MaxConcurrentFetches requests at once.
func TestLookupUsers_ColdClass_FetchesConcurrentlyWithinPool(t *testing.T) {
	fake, _ := setup(t)
	fake.setReply(func(id string) (*casdoorsdk.User, error) {
		time.Sleep(50 * time.Millisecond)
		return named(id)
	})
	class := ids("cold", 40)

	start := time.Now()
	got := identity.LookupUsers(class)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 10*50*time.Millisecond, "40 lookups at 50 ms each must run concurrently")
	assert.LessOrEqual(t, fake.MaxInFlightValue(), identity.MaxConcurrentFetches, "the pool bounds concurrent Casdoor calls")
	assert.Greater(t, fake.MaxInFlightValue(), 1, "misses must be fetched concurrently, not one by one")
	require.Len(t, got, 40)
	for _, id := range class {
		assert.Equal(t, identity.Identity{Name: "Name of " + id, Email: id + "@example.com"}, got[id])
		assert.Equal(t, 1, fake.callsFor(id), "each learner is fetched exactly once")
	}
}

// MaxInFlightValue reads the highest concurrency the fake saw.
func (f *fakeCasdoor) MaxInFlightValue() int { return int(f.maxInFlight.Load()) }

// TestLookupUsers_WithinTTL_MakesNoCasdoorCall: the wall polls every 30 s, so a
// second lookup of the same class inside the TTL must not reach Casdoor.
func TestLookupUsers_WithinTTL_MakesNoCasdoorCall(t *testing.T) {
	fake, clock := setup(t)
	class := ids("warm", 10)
	first := identity.LookupUsers(class)
	require.Equal(t, 10, fake.totalCalls())

	clock.advance(4 * time.Minute)
	second := identity.LookupUsers(class)

	assert.Equal(t, 10, fake.totalCalls(), "a fresh entry is served from the cache")
	assert.Equal(t, first, second)
}

// TestLookupUsers_ExpiredEntry_ServedStaleAndRefreshedOnceInBackground: when a
// class's entries expire together, the next request must not wait on Casdoor
// again. The stale identity is returned at once and refreshed behind the scenes
// — once, even when many requests ask for the same learner meanwhile.
func TestLookupUsers_ExpiredEntry_ServedStaleAndRefreshedOnceInBackground(t *testing.T) {
	fake, clock := setup(t)
	fake.setReply(func(id string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: id, DisplayName: "Old name", Email: "old@example.com"}, nil
	})
	require.Equal(t, "Old name", identity.LookupUsers([]string{"stale"})["stale"].Name)
	require.Equal(t, 1, fake.callsFor("stale"))

	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	fake.setReply(func(id string) (*casdoorsdk.User, error) {
		<-release
		return &casdoorsdk.User{Id: id, DisplayName: "New name", Email: "new@example.com"}, nil
	})
	clock.advance(6 * time.Minute)

	var wg sync.WaitGroup
	results := make([]identity.Identity, 20)
	durations := make([]time.Duration, 20)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := time.Now()
			results[i] = identity.LookupUsers([]string{"stale"})["stale"]
			durations[i] = time.Since(start)
		}(i)
	}
	wg.Wait() // would hang (test timeout) if a lookup waited on the blocked refresh
	for i := range results {
		assert.Equal(t, identity.Identity{Name: "Old name", Email: "old@example.com"}, results[i], "an expired entry is served stale")
		assert.Less(t, durations[i], 200*time.Millisecond, "a stale lookup never waits on Casdoor")
	}

	once.Do(func() { close(release) })
	require.Eventually(t, func() bool {
		return identity.LookupUsers([]string{"stale"})["stale"].Name == "New name"
	}, 2*time.Second, 10*time.Millisecond, "the background refresh replaces the stale identity")
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 2, fake.callsFor("stale"), "one initial fetch + exactly one background refresh")
}

// TestLookupUsers_FailedFetch_NotCachedAndRetried: a Casdoor hiccup must not pin
// a learner to a blank name for five minutes, nor blank the rest of the class.
func TestLookupUsers_FailedFetch_NotCachedAndRetried(t *testing.T) {
	fake, _ := setup(t)
	var failing atomic.Bool
	failing.Store(true)
	fake.setReply(func(id string) (*casdoorsdk.User, error) {
		if failing.Load() {
			switch id {
			case "flaky":
				return nil, errors.New("casdoor unavailable")
			case "ghost":
				return nil, nil
			}
		}
		return named(id)
	})

	got := identity.LookupUsers([]string{"ok", "flaky", "ghost"})
	assert.Equal(t, "Name of ok", got["ok"].Name, "one failure does not fail the whole lookup")
	assert.NotContains(t, got, "flaky", "a failed fetch yields no identity")
	assert.NotContains(t, got, "ghost", "no user yields no identity")

	failing.Store(false)
	again := identity.LookupUsers([]string{"ok", "flaky", "ghost"})
	assert.Equal(t, "Name of flaky", again["flaky"].Name, "a failed fetch is retried by the next lookup")
	assert.Equal(t, "Name of ghost", again["ghost"].Name)
	assert.Equal(t, 1, fake.callsFor("ok"), "the successful entry stayed cached")
	assert.Equal(t, 2, fake.callsFor("flaky"))
	assert.Equal(t, 2, fake.callsFor("ghost"))
}

// TestLookupUsers_FailedRefresh_KeepsStaleIdentity: a refresh that fails must
// not erase a name the trainer was already seeing.
func TestLookupUsers_FailedRefresh_KeepsStaleIdentity(t *testing.T) {
	fake, clock := setup(t)
	require.Equal(t, "Name of keep", identity.LookupUsers([]string{"keep"})["keep"].Name)

	fake.setReply(func(string) (*casdoorsdk.User, error) { return nil, errors.New("casdoor unavailable") })
	clock.advance(6 * time.Minute)
	assert.Equal(t, "Name of keep", identity.LookupUsers([]string{"keep"})["keep"].Name, "served stale")

	require.Eventually(t, func() bool { return fake.callsFor("keep") >= 2 },
		2*time.Second, 10*time.Millisecond, "the expired entry is refreshed")
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, identity.Identity{Name: "Name of keep", Email: "keep@example.com"},
		identity.LookupUsers([]string{"keep"})["keep"], "a failed refresh keeps the stale identity")
}

// TestLookupUsers_RefreshFindsNoUser_EvictsEntry: Casdoor answering "no such
// user" (nil, nil) on a refresh means the account is gone, unlike a transport
// error (FailedRefresh_KeepsStaleIdentity). The entry is evicted: later
// lookups never return the deleted user's stale name.
func TestLookupUsers_RefreshFindsNoUser_EvictsEntry(t *testing.T) {
	fake, clock := setup(t)
	require.Equal(t, "Name of gone", identity.LookupUsers([]string{"gone", "kept"})["gone"].Name)

	fake.setReply(func(id string) (*casdoorsdk.User, error) {
		if id == "gone" {
			return nil, nil
		}
		return named(id)
	})
	clock.advance(6 * time.Minute)
	identity.LookupUsers([]string{"gone", "kept"}) // stale answer allowed; triggers the refresh

	require.Eventually(t, func() bool {
		_, present := identity.LookupUsers([]string{"gone"})["gone"]
		return !present
	}, 2*time.Second, 10*time.Millisecond, "a user Casdoor no longer knows is evicted, not served stale")
	time.Sleep(50 * time.Millisecond)

	got := identity.LookupUsers([]string{"gone", "kept"})
	assert.NotContains(t, got, "gone", "the deleted user stays out of later lookups")
	assert.Equal(t, "Name of kept", got["kept"].Name, "other entries are untouched")
}

// TestLookupUsers_EmptyDisplayName_FallsBackToLoginName pins the name rule both
// former implementations are unified on (live progress already had it).
func TestLookupUsers_EmptyDisplayName_FallsBackToLoginName(t *testing.T) {
	fake, _ := setup(t)
	fake.setReply(func(id string) (*casdoorsdk.User, error) {
		return &casdoorsdk.User{Id: id, DisplayName: "", Name: "jdoe", Email: "jdoe@example.com"}, nil
	})
	assert.Equal(t, identity.Identity{Name: "jdoe", Email: "jdoe@example.com"},
		identity.LookupUsers([]string{"no-display"})["no-display"])
}

// TestLookupUsers_EmptyAndDuplicateIDs: an unowned row (empty id) is a data
// fault, never a Casdoor call; a learner listed twice is fetched once.
func TestLookupUsers_EmptyAndDuplicateIDs(t *testing.T) {
	fake, _ := setup(t)
	got := identity.LookupUsers([]string{"dup", "", "dup"})
	assert.Equal(t, 1, fake.callsFor("dup"))
	assert.Equal(t, 0, fake.callsFor(""))
	assert.NotContains(t, got, "")
	assert.Equal(t, "Name of dup", got["dup"].Name)
}
