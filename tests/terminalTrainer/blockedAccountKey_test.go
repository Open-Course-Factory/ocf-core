package terminalTrainer_tests

// Blocking an account disables its terminal key; CreateUserKey must not hand
// it a fresh one, whichever path asks (self-service, launch, bulk start).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authModels "soli/formations/src/auth/models"
	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// keyCountingTTServer answers metrics with plenty of RAM and key creation with
// a fresh key, counting creations; anything else fails.
func keyCountingTTServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var creates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/metrics"):
			_ = json.NewEncoder(w).Encode(dto.ServerMetricsResponse{RAMTotalGB: 100, RAMAvailableGB: 90, RAMPercent: 10, Timestamp: time.Now().Unix()})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/admin/api-keys"):
			n := creates.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"id": n, "key_value": "tt-key", "name": "learner", "is_active": true},
			})
		default:
			http.Error(w, "unexpected request", http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &creates
}

func TestCreateUserKey_BlockedAccount_RefusedWithUserBlocked(t *testing.T) {
	srv, creates := keyCountingTTServer(t)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "blocked-key-" + uuid.New().String()
	require.NoError(t, authModels.SetAccountBlocked(db, userID, true))

	err := services.NewTerminalTrainerService(db).CreateUserKey(userID, "auto-blocked")

	require.ErrorIs(t, err, authModels.ErrAccountBlocked)
	assert.Zero(t, creates.Load(), "tt-backend never asked for a key")
	var keys int64
	db.Model(&models.UserTerminalKey{}).Where("user_id = ?", userID).Count(&keys)
	assert.Zero(t, keys)

	require.NoError(t, authModels.SetAccountBlocked(db, userID, false))
	require.NoError(t, services.NewTerminalTrainerService(db).CreateUserKey(userID, "auto-unblocked"), "unblock restores key creation")
}

func TestBulkCreateTerminals_BlockedMember_ReportedUserBlockedOthersAttempted(t *testing.T) {
	srv, creates := keyCountingTTServer(t)
	configureTTServer(t, srv.URL)
	casdoorsdk.InitConfig("http://localhost:0", "dummy-endpoint", "dummy-client", "dummy-secret", "dummy-org", "dummy-app")
	db := freshTestDB(t)
	ownerID := "bulk-blocked-owner-" + uuid.New().String()
	group := createTestGroup(t, db, ownerID)
	blockedID, okID := "bulk-blocked-ko-"+uuid.New().String(), "bulk-blocked-ok-"+uuid.New().String()
	addActiveGroupMember(t, db, group.ID, blockedID, groupModels.GroupMemberRoleMember)
	addActiveGroupMember(t, db, group.ID, okID, groupModels.GroupMemberRoleMember)
	require.NoError(t, authModels.SetAccountBlocked(db, blockedID, true))
	plan := &paymentModels.SubscriptionPlan{Name: "BulkBlocked", IsActive: true, MaxCPU: 100000, MaxMemoryMB: 51200, MaxSessionDurationMinutes: 60}
	require.NoError(t, db.Create(plan).Error)

	resp, err := services.NewTerminalTrainerService(db).BulkCreateTerminalsForGroup(
		group.ID.String(), ownerID, []string{"member"},
		dto.BulkCreateTerminalsRequest{Terms: "accepted", InstanceType: "ubuntu-24.04"}, plan)
	require.NoError(t, err)

	results := map[string]dto.BulkTerminalCreationResult{}
	for _, r := range resp.Terminals {
		results[r.UserID] = r
	}
	require.Len(t, results, 2, "both members are reported")
	assert.False(t, results[blockedID].Success)
	assert.Equal(t, "user_blocked", results[blockedID].Error)
	assert.NotEqual(t, "user_blocked", results[okID].Error, "the other member is still attempted")
	assert.EqualValues(t, 1, creates.Load(), "a key is provisioned for the other member only")
}
