// tests/terminalTrainer/startSessionExpiry_test.go
//
// Bug pinned here:
//
// When a user's plan caps MaxSessionDurationMinutes (e.g. 1 min for the
// Member Pro plan in a smoke test), POST /api/v1/terminals/:id/start
// (Resume) currently posts a NIL body to tt-backend. tt-backend then falls
// back to the instance config default (1 hour), and the resumed session
// lives way past the plan cap. The initial create path (StartComposedSession)
// is OK — it reads the plan and clamps Expiry — but Resume bypasses the
// plan entirely.
//
// SSOT rule pinned: the plan's MaxSessionDurationMinutes is the canonical
// session-duration cap. Every code path that starts (or restarts) an
// instance must derive expiry from it.
//
// Tests assert on the OUTBOUND HTTP request body to tt-backend (the
// user-observable contract), NOT on mock invocations.
package terminalTrainer_tests

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

// startSessionRecorder captures the JSON body posted to
// POST /1.0/sessions/{id}/start so tests can assert on the expiry field.
type startSessionRecorder struct {
	gotBody    map[string]any
	bodyParsed bool
	rawBody    string
	calls      int
}

// startSessionTTServer spins a fake tt-backend that responds to the start
// endpoint and captures the request body for assertion.
func startSessionTTServer(t *testing.T) (*httptest.Server, *startSessionRecorder) {
	t.Helper()
	rec := &startSessionRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
			rec.calls++
			body, _ := io.ReadAll(r.Body)
			rec.rawBody = string(body)
			parsed := map[string]any{}
			if len(body) > 0 {
				if err := json.Unmarshal(body, &parsed); err == nil {
					rec.gotBody = parsed
					rec.bodyParsed = true
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"state":           "running",
				"last_started_at": time.Now().Unix(),
				"expires_at":      time.Now().Add(5 * time.Minute).Unix(),
			})
			return
		}
		http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}))
	return srv, rec
}

// makePlanWithDuration produces a SubscriptionPlan whose
// MaxSessionDurationMinutes is the only field we exercise here. Other fields
// stay zero/default because they are not load-bearing for the resume path.
func makePlanWithDuration(minutes int) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "duration-test-plan",
		IsActive:                  true,
		MaxSessionDurationMinutes: minutes,
	}
}

// TestStartSession_SendsPlanDerivedExpiryToTTBackend is the RED test for the
// bug: the resume path must forward the plan's MaxSessionDurationMinutes
// (converted to seconds) as `expiry` in the body posted to tt-backend.
//
// Pre-fix, the body is empty, so tt-backend falls back to its instance
// config default (~1h) instead of honoring the plan cap. This test will
// fail until ocf-core's StartSession looks up the plan and forwards expiry.
func TestStartSession_SendsPlanDerivedExpiryToTTBackend(t *testing.T) {
	srv, rec := startSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "expiry-user-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Seed a plan with MaxSessionDurationMinutes=5 (300 seconds).
	plan := makePlanWithDuration(5)
	require.NoError(t, db.Create(plan).Error)

	// Seed a stopped terminal referencing that plan.
	sessionID := "sess-" + uuid.New().String()
	planID := plan.ID
	terminal := &models.Terminal{
		SessionID:          sessionID,
		UserID:             userID,
		Name:               "exp-test",
		State:              "stopped",
		PersistenceMode:    "persistent",
		ExpiresAt:          time.Now().Add(-time.Minute), // already expired
		InstanceType:       "test",
		MachineSize:        "S",
		SubscriptionPlanID: &planID,
		UserTerminalKeyID:  userKey.ID,
	}
	require.NoError(t, db.Create(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	err = svc.StartSession(sessionID)
	require.NoError(t, err, "StartSession must succeed against the fake backend")

	// User-observable contract: tt-backend received a body carrying the
	// plan's MaxSessionDurationMinutes converted to seconds.
	require.Equal(t, 1, rec.calls, "tt-backend /start must be called once")
	require.True(t, rec.bodyParsed, "tt-backend must receive a JSON body; got raw=%q", rec.rawBody)
	require.Contains(t, rec.gotBody, "expiry",
		"body must include 'expiry' so tt-backend can honor the plan cap; got body=%v", rec.gotBody)
	// JSON numbers come back as float64 from a generic decode.
	assert.Equal(t, float64(300), rec.gotBody["expiry"],
		"plan.MaxSessionDurationMinutes (5) * 60 must arrive as expiry=300 on the wire; got %v", rec.gotBody["expiry"])
}

// TestStartSession_NoPlan_DoesNotSendExpiry pins the fallback: when a legacy
// terminal row has no SubscriptionPlanID, the resume path must NOT inject an
// expiry — tt-backend uses its instance config default. This avoids breaking
// sessions created before the plan-tracking field existed.
func TestStartSession_NoPlan_DoesNotSendExpiry(t *testing.T) {
	srv, rec := startSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "expiry-noplan-user-" + uuid.New().String()
	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	// Seed a stopped terminal with NO plan reference (legacy row).
	sessionID := "sess-legacy-" + uuid.New().String()
	terminal := &models.Terminal{
		SessionID:         sessionID,
		UserID:            userID,
		Name:              "legacy-test",
		State:             "stopped",
		PersistenceMode:   "persistent",
		ExpiresAt:         time.Now().Add(-time.Minute),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		// SubscriptionPlanID intentionally nil
	}
	require.NoError(t, db.Create(terminal).Error)

	svc := services.NewTerminalTrainerService(db)
	err = svc.StartSession(sessionID)
	require.NoError(t, err, "StartSession must succeed even without a plan")

	require.Equal(t, 1, rec.calls, "tt-backend /start must be called once")
	// Either no body, or a body without an expiry key. Both keep tt-backend
	// on its instance config default.
	if rec.bodyParsed {
		_, present := rec.gotBody["expiry"]
		assert.False(t, present,
			"body must NOT carry 'expiry' for legacy terminals with no plan; got body=%v", rec.gotBody)
	}
}
