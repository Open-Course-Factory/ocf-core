// tests/terminalTrainer/terminalKeyBudgetSync_test.go
//
// The tt-backend per-key budget used to be computed once, at key creation. A
// later plan change never reached the key, so the backstop could be looser or
// tighter than the plan until the key was recreated. SyncUserKeyBudget
// recomputes the user's ceiling and pushes it to the key that already exists.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	terminalHooks "soli/formations/src/terminalTrainer/hooks"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// keyAdminTTServer fakes tt-backend's admin api-keys surface. It answers the
// key lookup with the stored shape and records every update body it receives.
type keyAdminTTServer struct {
	*httptest.Server
	mu      sync.Mutex
	updates []map[string]any
	creates int
}

func newKeyAdminTTServer(t *testing.T, keyID int64, limitsDisabled bool) *keyAdminTTServer {
	t.Helper()
	s := &keyAdminTTServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/admin/api-keys"):
			s.mu.Lock()
			s.creates++
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"id": keyID, "key_value": "tt-key-value", "name": "learner", "is_active": true},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/admin/api-keys/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id": keyID, "name": "learner", "is_admin": false, "is_active": true,
					"limits_disabled": limitsDisabled, "max_cpu_total": 1, "max_memory_mb_total": 512,
				},
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/admin/api-keys/"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			body["_path"] = r.URL.Path
			s.mu.Lock()
			s.updates = append(s.updates, body)
			s.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *keyAdminTTServer) recordedUpdates() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.updates...)
}

func seedPersonalPlan(t *testing.T, db *gorm.DB, userID string, maxCPU, maxMemMB int) {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "key-sync-plan-" + uuid.New().String(),
		MaxCPU:      maxCPU,
		MaxMemoryMB: maxMemMB,
		IsActive:    true,
	}
	require.NoError(t, db.Create(plan).Error)
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		SubscriptionType:   "personal",
		Status:             "active",
	}).Error)
}

func seedProvisionedKey(t *testing.T, db *gorm.DB, userID string, ttKeyID int64) {
	t.Helper()
	require.NoError(t, db.Create(&models.UserTerminalKey{
		UserID:               userID,
		APIKey:               "key-" + userID,
		KeyName:              "learner",
		IsActive:             true,
		TerminalTrainerKeyID: ttKeyID,
	}).Error)
}

func TestSyncUserKeyBudget_PushesRecomputedCeilingToExistingKey(t *testing.T) {
	srv := newKeyAdminTTServer(t, 42, true)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-sync-" + uuid.New().String()
	seedProvisionedKey(t, db, userID, 42)
	// 2500 mCPU rounds up to 3 whole CPUs on the tt-backend side.
	seedPersonalPlan(t, db, userID, 2500, 4096)

	require.NoError(t, services.NewTerminalTrainerService(db).SyncUserKeyBudget(userID))

	updates := srv.recordedUpdates()
	require.Len(t, updates, 1, "exactly one update reaches tt-backend")
	assert.True(t, strings.HasSuffix(updates[0]["_path"].(string), "/admin/api-keys/42"), "the update addresses the key by its tt-backend id")
	assert.EqualValues(t, 3, updates[0]["max_cpu_total"])
	assert.EqualValues(t, 4096, updates[0]["max_memory_mb_total"])
	assert.Equal(t, "learner", updates[0]["name"], "fields tt-backend requires on update are carried over unchanged")
	assert.Equal(t, true, updates[0]["is_active"])
	assert.Equal(t, true, updates[0]["limits_disabled"], "an operator's limits_disabled must survive a budget sync")
}

func TestSyncUserKeyBudget_KeyWithoutTTBackendID_ReportsAndSendsNothing(t *testing.T) {
	srv := newKeyAdminTTServer(t, 42, false)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-sync-legacy-" + uuid.New().String()
	seedProvisionedKey(t, db, userID, 0) // provisioned before the id was stored
	seedPersonalPlan(t, db, userID, 2000, 2048)

	err := services.NewTerminalTrainerService(db).SyncUserKeyBudget(userID)

	require.Error(t, err, "a key that cannot be addressed must say so rather than silently keep its stale budget")
	assert.Empty(t, srv.recordedUpdates())
}

func TestSyncUserKeyBudget_NoKey_IsANoOp(t *testing.T) {
	srv := newKeyAdminTTServer(t, 42, false)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-sync-no-key-" + uuid.New().String()
	seedPersonalPlan(t, db, userID, 2000, 2048)

	require.NoError(t, services.NewTerminalTrainerService(db).SyncUserKeyBudget(userID),
		"a user without a key has nothing to re-provision; the key gets its budget at creation")
	assert.Empty(t, srv.recordedUpdates())
}

func TestSyncUserKeyBudget_NoBudgetOnEitherAxis_LeavesKeyUntouched(t *testing.T) {
	srv := newKeyAdminTTServer(t, 42, false)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-sync-no-plan-" + uuid.New().String()
	seedProvisionedKey(t, db, userID, 42)
	// No subscription at all: the ceiling is zero on both axes, and
	// tt-backend cannot express "clear the budget" (omitted keeps, 0 is
	// rejected), so there is nothing valid to send.

	require.NoError(t, services.NewTerminalTrainerService(db).SyncUserKeyBudget(userID))
	assert.Empty(t, srv.recordedUpdates())
}

func TestCreateUserKey_StoresTTBackendKeyID(t *testing.T) {
	srv := newKeyAdminTTServer(t, 77, false)
	configureTTServer(t, srv.URL)
	db := freshTestDB(t)
	userID := "key-create-" + uuid.New().String()
	seedPersonalPlan(t, db, userID, 2000, 2048)

	require.NoError(t, services.NewTerminalTrainerService(db).CreateUserKey(userID, "learner"))

	var key models.UserTerminalKey
	require.NoError(t, db.Where("user_id = ?", userID).First(&key).Error)
	assert.EqualValues(t, 77, key.TerminalTrainerKeyID, "the id tt-backend assigned is what a later re-provision addresses")
	assert.Equal(t, "tt-key-value", key.APIKey)
}

// ---------------------------------------------------------------------------
// Hook: which users get re-provisioned after which entity change.
// ---------------------------------------------------------------------------

type recordingKeyBudgetSyncer struct {
	mu    sync.Mutex
	users []string
}

func (r *recordingKeyBudgetSyncer) SyncUserKeyBudget(userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users = append(r.users, userID)
	return nil
}

func execAfter(hook hooks.Hook, entityName string, hookType hooks.HookType, entity any) error {
	return hook.Execute(&hooks.HookContext{EntityName: entityName, HookType: hookType, NewEntity: entity})
}

func TestTerminalKeyBudgetSyncHook_UserSubscription_SyncsThatUser(t *testing.T) {
	db := freshTestDB(t)
	syncer := &recordingKeyBudgetSyncer{}
	hook := terminalHooks.NewTerminalKeyBudgetSyncHook(db, syncer, "UserSubscription")

	require.NoError(t, execAfter(hook, "UserSubscription", hooks.AfterCreate, &paymentModels.UserSubscription{UserID: "buyer-1"}))
	require.NoError(t, execAfter(hook, "UserSubscription", hooks.AfterDelete, &paymentModels.UserSubscription{UserID: "buyer-2"}))

	assert.Equal(t, []string{"buyer-1", "buyer-2"}, syncer.users)
}

func TestTerminalKeyBudgetSyncHook_OrganizationMember_SyncsThatMember(t *testing.T) {
	db := freshTestDB(t)
	syncer := &recordingKeyBudgetSyncer{}
	hook := terminalHooks.NewTerminalKeyBudgetSyncHook(db, syncer, "OrganizationMember")

	require.NoError(t, execAfter(hook, "OrganizationMember", hooks.AfterUpdate, &orgModels.OrganizationMember{UserID: "learner-9"}))

	assert.Equal(t, []string{"learner-9"}, syncer.users)
}

func TestTerminalKeyBudgetSyncHook_OrganizationRolePlan_SyncsEveryActiveMember(t *testing.T) {
	db := freshTestDB(t)
	orgID := uuid.New()
	for _, m := range []struct {
		user   string
		active bool
	}{{"m-active-1", true}, {"m-active-2", true}, {"m-left", false}} {
		require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
			BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: orgID,
			UserID:         m.user,
			Role:           "member",
			IsActive:       m.active,
		}).Error)
		if !m.active {
			// GORM's default:true rewrites an explicit false on create.
			require.NoError(t, db.Model(&orgModels.OrganizationMember{}).Where("user_id = ?", m.user).Update("is_active", false).Error)
		}
	}
	syncer := &recordingKeyBudgetSyncer{}
	hook := terminalHooks.NewTerminalKeyBudgetSyncHook(db, syncer, "OrganizationRolePlan")

	require.NoError(t, execAfter(hook, "OrganizationRolePlan", hooks.AfterUpdate, &paymentModels.OrganizationRolePlan{OrganizationID: orgID, Role: "member"}))

	assert.ElementsMatch(t, []string{"m-active-1", "m-active-2"}, syncer.users, "a role plan moves every active member's ceiling; a departed member has no ceiling to move")
}
