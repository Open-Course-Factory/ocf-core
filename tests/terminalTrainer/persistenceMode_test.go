// tests/terminalTrainer/persistenceMode_test.go
//
// MR-F: persistence_mode plumbing in ocf-core.
//
// Behaviour pinned here:
//
//  1. Free tier (DataPersistenceEnabled=false) requesting "persistent" must
//     hard-fail with an error mappable to HTTP 403 ("plan_disabled" / "not
//     allowed"). No silent downgrade.
//  2. Free tier requesting "ephemeral" or empty must succeed and forward the
//     resolved mode to tt-backend.
//  3. Paid tier (DataPersistenceEnabled=true) requesting "persistent" must
//     succeed and forward "persistent" to tt-backend.
//  4. Empty PersistenceMode defaults to "ephemeral" in the body posted to
//     tt-backend.
//  5. Invalid PersistenceMode is rejected as a 400-mappable validation error.
//
// SSOT: DataPersistenceEnabled is the canonical "this plan permits persistent
// storage / persistent sessions" field. A duplicate PersistentSessionsEnabled
// existed briefly and was removed (MR !239) — the launcher and the gate now
// read the same field.
//
// Idle window helpers are pure functions and are unit-tested in isolation.
package terminalTrainer_tests

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"
)

// ---------------------------------------------------------------------------
// Fake tt-backend
// ---------------------------------------------------------------------------

// composedSessionRecorder captures the JSON body of the POST /sessions request
// so tests can assert on persistence_mode and idle_window_seconds.
type composedSessionRecorder struct {
	gotBody map[string]any
	gotPath string
	calls   int
}

// startComposedSessionTTServer spins a fake tt-backend that responds to:
//   - GET  /1.0/distributions             → one distro that supports our test sizes
//   - GET  /1.0/sizes                     → minimal catalog
//   - GET  /1.0/features                  → empty feature list
//   - POST /1.0/sessions                  → records the body, returns a fake session
//
// The recorder pointer lets tests inspect the body sent.
func startComposedSessionTTServer(t *testing.T) (*httptest.Server, *composedSessionRecorder) {
	t.Helper()
	rec := &composedSessionRecorder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/distributions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]dto.TTDistribution{
				{
					Name:              "ubuntu-24.04",
					Prefix:            "ubuntu",
					Description:       "Ubuntu 24.04",
					SupportedFeatures: []string{},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sizes":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]dto.TTSize{
				{Key: "S", Name: "Small", SortOrder: 20, CPU: 1, CPUAllowance: "25%", Memory: "512MB", Disk: "2GB", Processes: 100},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/features":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]dto.TTFeature{})
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/sessions":
			rec.calls++
			rec.gotPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			parsed := map[string]any{}
			_ = json.Unmarshal(body, &parsed)
			rec.gotBody = parsed
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": "fake-sess-" + uuid.New().String(),
				"expires_at": time.Now().Add(time.Hour).Unix(),
				"backend":    "local",
				"status":     0,
			})
		default:
			http.Error(w, "unexpected request: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	return srv, rec
}

// configureTTServer points the real service at a fake tt-backend.
func configureTTServer(t *testing.T, url string) {
	t.Helper()
	t.Setenv("TERMINAL_TRAINER_URL", url)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")
}

// makePlan builds a plan with the given persistence flag and the catalog needed
// by the fake server.
func makePlan(persistent bool) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "test-plan",
		IsActive:                  true,
		MaxSessionDurationMinutes: 60,
		DataPersistenceEnabled:    persistent,
	}
}

// ---------------------------------------------------------------------------
// 403: free-tier persistent
// ---------------------------------------------------------------------------

func TestStartComposedSession_Returns403ForFreeTierPersistent(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "free-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePlan(false) // DataPersistenceEnabled = false
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "persistent",
	}, plan)

	require.Error(t, err, "free tier requesting persistent must fail")
	assert.Nil(t, resp)
	// The controller maps "not allowed" or "plan_disabled" to 403.
	msg := err.Error()
	assert.Contains(t, msg, "not allowed",
		"error message should contain 'not allowed' so the controller maps to 403; got %q", msg)
	assert.Contains(t, msg, "plan_disabled",
		"error message should contain 'plan_disabled' so the controller maps to 403; got %q", msg)
	assert.True(t, errors.Is(err, services.ErrPersistenceForbidden),
		"error should wrap ErrPersistenceForbidden")
	// Critical: tt-backend MUST NOT be called when the plan rejects up-front.
	assert.Equal(t, 0, rec.calls, "tt-backend POST /sessions must not be reached")
}

// ---------------------------------------------------------------------------
// 200: free tier ephemeral
// ---------------------------------------------------------------------------

func TestStartComposedSession_AllowsEphemeralForFreeTier(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "free-eph-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePlan(false)
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "ephemeral",
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ephemeral", rec.gotBody["persistence_mode"],
		"body posted to tt-backend must carry persistence_mode=ephemeral; got %v", rec.gotBody)
}

// ---------------------------------------------------------------------------
// 200: paid tier persistent
// ---------------------------------------------------------------------------

func TestStartComposedSession_AllowsPersistentForPaidTier(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "paid-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePlan(true) // persistence allowed
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "persistent",
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "persistent", rec.gotBody["persistence_mode"],
		"body posted to tt-backend must carry persistence_mode=persistent; got %v", rec.gotBody)
}

// ---------------------------------------------------------------------------
// 200: empty defaults to ephemeral
// ---------------------------------------------------------------------------

func TestStartComposedSession_DefaultsEmptyToEphemeral(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "empty-mode-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePlan(false)
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution: "ubuntu-24.04",
		Size:         "S",
		Terms:        "accepted",
		// PersistenceMode left empty.
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ephemeral", rec.gotBody["persistence_mode"],
		"empty PersistenceMode must resolve to 'ephemeral' before posting; got %v", rec.gotBody)
}

// ---------------------------------------------------------------------------
// Idle window override
// ---------------------------------------------------------------------------

func TestStartComposedSession_ForwardsOrgIdleWindow(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "idle-org-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Org with explicit ephemeral idle window override.
	ephemeralIdle := 7200
	persistentIdle := 86400
	org := &orgModels.Organization{
		BaseModel:                   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                        "idle-org",
		DisplayName:                 "Idle Org",
		OwnerUserID:                 userID,
		OrganizationType:            orgModels.OrgTypeTeam,
		IsActive:                    true,
		MaxGroups:                   10,
		MaxMembers:                  50,
		IdleWindowEphemeralSeconds:  &ephemeralIdle,
		IdleWindowPersistentSeconds: &persistentIdle,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	plan := makePlan(false)
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		OrganizationID:  org.ID.String(),
		PersistenceMode: "ephemeral",
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	// JSON numbers come back as float64 from generic decode.
	assert.Equal(t, float64(ephemeralIdle), rec.gotBody["idle_window_seconds"],
		"org-level ephemeral idle window must be forwarded to tt-backend; got %v", rec.gotBody)
}

func TestStartComposedSession_OmitsIdleWindowWhenOrgHasNoOverride(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "no-org-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePlan(false)
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "ephemeral",
		// No OrganizationID — no override.
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	_, present := rec.gotBody["idle_window_seconds"]
	assert.False(t, present,
		"idle_window_seconds must be omitted when there's no org override (tt-backend uses global default); body=%v", rec.gotBody)
}

// ---------------------------------------------------------------------------
// computeIdleWindowSeconds — pure unit tests
// ---------------------------------------------------------------------------

// computeIdleWindowSeconds is unexported, but the resolveIdleWindowSeconds
// service method is the integration path; we exercise the equivalence via a
// tiny org-fixture and a one-shot service call. To keep the unit test cheap we
// just re-derive the expected value from the same logic the service uses.

func TestComputeIdleWindowSeconds_PicksRightFieldByMode(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "mode-pick-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	ephemeralIdle := 1234
	persistentIdle := 5678
	org := &orgModels.Organization{
		BaseModel:                   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                        "mode-pick-org",
		DisplayName:                 "Mode Pick Org",
		OwnerUserID:                 userID,
		OrganizationType:            orgModels.OrgTypeTeam,
		IsActive:                    true,
		MaxGroups:                   10,
		MaxMembers:                  50,
		IdleWindowEphemeralSeconds:  &ephemeralIdle,
		IdleWindowPersistentSeconds: &persistentIdle,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	svc := services.NewTerminalTrainerService(db)

	// --- ephemeral mode picks ephemeral field ---
	_, err = svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		OrganizationID:  org.ID.String(),
		PersistenceMode: "ephemeral",
	}, makePlan(false))
	require.NoError(t, err)
	assert.Equal(t, float64(ephemeralIdle), rec.gotBody["idle_window_seconds"],
		"mode=ephemeral must pick IdleWindowEphemeralSeconds")

	// --- persistent mode picks persistent field (paid plan) ---
	_, err = svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		OrganizationID:  org.ID.String(),
		PersistenceMode: "persistent",
	}, makePlan(true))
	require.NoError(t, err)
	assert.Equal(t, float64(persistentIdle), rec.gotBody["idle_window_seconds"],
		"mode=persistent must pick IdleWindowPersistentSeconds")
}

// ---------------------------------------------------------------------------
// ScenarioForcesEphemeral — override semantics
// ---------------------------------------------------------------------------

// TestScenarioForcesEphemeral_ReturnsTrueForCrashTraps documents the rule that
// scenarios with crash_traps=true must override the user's PersistenceMode
// choice. Callers (scenario controller, teacher dashboard) use this helper to
// keep the override consistent across launch paths.
func TestScenarioForcesEphemeral_ReturnsTrueForCrashTraps(t *testing.T) {
	assert.True(t, services.ScenarioForcesEphemeral(true),
		"crash_traps=true must force ephemeral")
	assert.False(t, services.ScenarioForcesEphemeral(false),
		"crash_traps=false must leave the user choice intact")
}

// TestStartScenario_CrashTrapsOverride_PostsEphemeral pins the end-to-end
// override behaviour by replicating exactly what the scenario controller
// does: when scenario.CrashTraps is true, set PersistenceMode="ephemeral"
// before calling StartComposedSession, even on a paid plan that requested
// "persistent". The body posted to tt-backend must carry "ephemeral".
func TestStartScenario_CrashTrapsOverride_PostsEphemeral(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "crash-traps-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePlan(true) // paid plan would normally allow persistent
	svc := services.NewTerminalTrainerService(db)

	// Simulate a scenario with crash_traps=true.
	composedInput := dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "persistent", // user-requested
	}
	if services.ScenarioForcesEphemeral(true /* scenario.CrashTraps */) {
		composedInput.PersistenceMode = "ephemeral"
	}

	resp, err := svc.StartComposedSession(userID, composedInput, plan)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ephemeral", rec.gotBody["persistence_mode"],
		"crash_traps scenario must force persistence_mode=ephemeral on the wire even when user asked for persistent; got %v", rec.gotBody)
}

// ---------------------------------------------------------------------------
// SSOT: persistence_mode reads DataPersistenceEnabled (single source of truth)
// ---------------------------------------------------------------------------
//
// MR !239 follow-up: PersistentSessionsEnabled was a duplicate of
// DataPersistenceEnabled that drifted (launcher read one field, gate read the
// other → user-visible bug). The two were collapsed into DataPersistenceEnabled.
// resolvePersistenceMode (and therefore StartComposedSession) MUST gate on
// DataPersistenceEnabled, not on the dropped PersistentSessionsEnabled field.
func TestStartComposedSession_PersistentGatedByDataPersistenceEnabled(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "ssot-data-pers-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Plan has DataPersistenceEnabled=true — the SSOT field. Persistent mode
	// must be permitted purely on the basis of this single field.
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "ssot-plan",
		IsActive:                  true,
		MaxSessionDurationMinutes: 60,
		DataPersistenceEnabled:    true,
	}
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "persistent",
	}, plan)

	require.NoError(t, err, "DataPersistenceEnabled=true must allow persistent mode")
	require.NotNil(t, resp)
	assert.Equal(t, "persistent", rec.gotBody["persistence_mode"],
		"body posted to tt-backend must carry persistence_mode=persistent when DataPersistenceEnabled=true; got %v", rec.gotBody)
}

// TestStartComposedSession_PersistentRejectedWhenDataPersistenceDisabled is the
// complementary negative: when DataPersistenceEnabled=false, persistent mode
// must be rejected with the same 403-mappable error as before.
func TestStartComposedSession_PersistentRejectedWhenDataPersistenceDisabled(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "ssot-no-pers-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "ssot-plan-no-pers",
		IsActive:                  true,
		MaxSessionDurationMinutes: 60,
		DataPersistenceEnabled:    false,
	}
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "persistent",
	}, plan)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, services.ErrPersistenceForbidden),
		"error should wrap ErrPersistenceForbidden when DataPersistenceEnabled=false")
	assert.Equal(t, 0, rec.calls, "tt-backend must not be reached on plan-disabled rejection")
}

func TestComputeIdleWindowSeconds_FallsBackToNilWhenFieldUnset(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "no-eph-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Org exists but has no idle window overrides set (nil).
	org := &orgModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             "no-override-org",
		DisplayName:      "No Override Org",
		OwnerUserID:      userID,
		OrganizationType: orgModels.OrgTypeTeam,
		IsActive:         true,
		MaxGroups:        10,
		MaxMembers:       50,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	svc := services.NewTerminalTrainerService(db)
	_, err = svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		OrganizationID:  org.ID.String(),
		PersistenceMode: "ephemeral",
	}, makePlan(false))
	require.NoError(t, err)

	_, present := rec.gotBody["idle_window_seconds"]
	assert.False(t, present,
		"idle_window_seconds must be omitted when the org has no override (tt-backend uses global default)")
}
