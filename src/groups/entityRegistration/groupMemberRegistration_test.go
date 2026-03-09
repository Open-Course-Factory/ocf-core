package groupRegistration

import (
	"errors"
	"soli/formations/src/groups/dto"
	"sync/atomic"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// newTestCache creates a casdoorUserCache with injectable behaviour.
// bulkCallCount is incremented every time fetchAllUsers is invoked so
// tests can assert how many bulk fetches happened.
func newTestCache(users []*casdoorsdk.User, bulkErr error, bulkCallCount *int64) *casdoorUserCache {
	return &casdoorUserCache{
		fetchAllUsers: func() ([]*casdoorsdk.User, error) {
			atomic.AddInt64(bulkCallCount, 1)
			if bulkErr != nil {
				return nil, bulkErr
			}
			return users, nil
		},
		fetchUserByID: func(id string) (*casdoorsdk.User, error) {
			for _, u := range users {
				if u.Id == id {
					return u, nil
				}
			}
			return nil, nil
		},
	}
}

func TestUserCache_BulkLoadsOnFirstCall(t *testing.T) {
	alice := &casdoorsdk.User{Id: "alice-id", Name: "alice", DisplayName: "Alice", Email: "alice@example.com"}
	bob := &casdoorsdk.User{Id: "bob-id", Name: "bob", DisplayName: "Bob", Email: "bob@example.com"}

	var calls int64
	cache := newTestCache([]*casdoorsdk.User{alice, bob}, nil, &calls)

	user, err := cache.get("alice-id")
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "alice", user.Name)
	assert.Equal(t, int64(1), atomic.LoadInt64(&calls), "first call should trigger bulk fetch")
}

func TestUserCache_ReturnsCachedOnSubsequentCalls(t *testing.T) {
	alice := &casdoorsdk.User{Id: "alice-id", Name: "alice"}

	var calls int64
	cache := newTestCache([]*casdoorsdk.User{alice}, nil, &calls)

	// First call — populates cache
	_, _ = cache.get("alice-id")
	assert.Equal(t, int64(1), atomic.LoadInt64(&calls))

	// Second call within TTL — should use cache, no new bulk fetch
	user, err := cache.get("alice-id")
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "alice", user.Name)
	assert.Equal(t, int64(1), atomic.LoadInt64(&calls), "second call within TTL should not trigger another bulk fetch")
}

func TestUserCache_RefreshesAfterTTL(t *testing.T) {
	alice := &casdoorsdk.User{Id: "alice-id", Name: "alice"}

	var calls int64
	cache := newTestCache([]*casdoorsdk.User{alice}, nil, &calls)

	// First call — populates cache
	_, _ = cache.get("alice-id")
	assert.Equal(t, int64(1), atomic.LoadInt64(&calls))

	// Simulate TTL expiry by back-dating fetchedAt
	cache.mu.Lock()
	cache.fetchedAt = time.Now().Add(-casdoorUserCacheTTL - time.Second)
	cache.mu.Unlock()

	// Next call should trigger a fresh bulk fetch
	user, err := cache.get("alice-id")
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, int64(2), atomic.LoadInt64(&calls), "call after TTL should trigger another bulk fetch")
}

func TestUserCache_FallsBackToSingleFetch(t *testing.T) {
	alice := &casdoorsdk.User{Id: "alice-id", Name: "alice"}

	var calls int64
	cache := newTestCache([]*casdoorsdk.User{alice}, errors.New("bulk fetch failed"), &calls)

	// Bulk fetch will fail, fallback to fetchUserByID
	user, err := cache.get("alice-id")
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "alice", user.Name)
	assert.Equal(t, int64(1), atomic.LoadInt64(&calls), "bulk fetch should have been attempted")

	// Cache should NOT be populated after a failed bulk fetch —
	// the next call should try bulk fetch again
	assert.Nil(t, cache.users, "cache should not be populated when bulk fetch fails")
}

func TestUserCache_ReturnsNilForUnknownUser(t *testing.T) {
	alice := &casdoorsdk.User{Id: "alice-id", Name: "alice"}

	var calls int64
	cache := newTestCache([]*casdoorsdk.User{alice}, nil, &calls)

	user, err := cache.get("unknown-id")
	assert.NoError(t, err)
	assert.Nil(t, user, "unknown user ID should return nil without error")
}

func TestEnrichGroupMember_EmptyUserID_ReturnsUnchanged(t *testing.T) {
	output := &dto.GroupMemberOutput{
		ID:      uuid.New(),
		GroupID: uuid.New(),
		UserID:  "",
	}

	result := enrichGroupMemberWithUser(output)
	assert.Nil(t, result.User, "empty UserID should leave User field nil")
	assert.Equal(t, output.ID, result.ID, "output should be returned unchanged")
}

func TestEnrichGroupMember_UserFound_PopulatesFields(t *testing.T) {
	alice := &casdoorsdk.User{
		Id:          "alice-id",
		Name:        "alice",
		DisplayName: "Alice Wonderland",
		Email:       "alice@example.com",
	}

	// Replace the global cache temporarily
	origCache := userCache
	var calls int64
	userCache = newTestCache([]*casdoorsdk.User{alice}, nil, &calls)
	defer func() { userCache = origCache }()

	output := &dto.GroupMemberOutput{
		ID:      uuid.New(),
		GroupID: uuid.New(),
		UserID:  "alice-id",
	}

	result := enrichGroupMemberWithUser(output)
	assert.NotNil(t, result.User)
	assert.Equal(t, "alice-id", result.User.ID)
	assert.Equal(t, "alice", result.User.Name)
	assert.Equal(t, "Alice Wonderland", result.User.DisplayName)
	assert.Equal(t, "alice@example.com", result.User.Email)
	assert.Equal(t, "alice", result.User.Username)
}

func TestEnrichGroupMember_DisplayNameFallback(t *testing.T) {
	// User has no DisplayName — enrichment should fall back to Name
	bob := &casdoorsdk.User{
		Id:          "bob-id",
		Name:        "bob",
		DisplayName: "", // empty
		Email:       "bob@example.com",
	}

	origCache := userCache
	var calls int64
	userCache = newTestCache([]*casdoorsdk.User{bob}, nil, &calls)
	defer func() { userCache = origCache }()

	output := &dto.GroupMemberOutput{
		ID:      uuid.New(),
		GroupID: uuid.New(),
		UserID:  "bob-id",
	}

	result := enrichGroupMemberWithUser(output)
	assert.NotNil(t, result.User)
	assert.Equal(t, "bob", result.User.DisplayName, "should fall back to Name when DisplayName is empty")
}
