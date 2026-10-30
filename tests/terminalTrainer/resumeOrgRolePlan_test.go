// tests/terminalTrainer/resumeOrgRolePlan_test.go
//
// A resume is charged to the plan the machine runs under: its owner's, in the
// organization the terminal is filed under. The front resumes with a bare
// POST /terminals/:id/start, so a plan resolved from the request falls back to
// the caller's global plan — which never sees an org role plan. In production
// (2026-10-30) a learner whose personal plan was Découverte (500 mCPU / 256
// MiB) could not resume the XL his class role plan had given him.
//
// Witness: the HTTP answer of the resume route and the row's state after it.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"
)

func TestStartSessionRoute_ResumeChargesTheTerminalOrgRolePlan(t *testing.T) {
	userID := "resume-role-plan-learner"
	db := freshTestDB(t)
	seedPersonalPlanFor(t, userID, 500, 256, 60, "Découverte")

	org := createOrgWithBackendConfig(t, db, "class-org", "teacher", organizationModels.OrgTypeTeam, "", nil)
	require.NoError(t, db.Omit("Metadata").Create(&organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           organizationModels.OrganizationMemberRole("member"),
		JoinedAt:       time.Now(),
		IsActive:       true,
	}).Error)
	seatXL := &paymentModels.SubscriptionPlan{Name: "Siège élève — XL", MaxCPU: 4000, MaxMemoryMB: 4096, MaxSessionDurationMinutes: 60, IsActive: true}
	require.NoError(t, db.Create(seatXL).Error)
	require.NoError(t, db.Create(&paymentModels.OrganizationRolePlan{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		Role:               "member",
		SubscriptionPlanID: seatXL.ID,
	}).Error)

	userKey, err := createTestUserKey(db, userID)
	require.NoError(t, err)
	sessionID := "resume-role-plan-xl"
	require.NoError(t, db.Create(&models.Terminal{
		SessionID:          sessionID,
		UserID:             userID,
		State:              models.StateStopped,
		PersistenceMode:    models.PersistenceModePersistent,
		ExpiresAt:          time.Now().Add(time.Hour),
		MachineSize:        "XL",
		SizeCPU:            4000,
		SizeMemoryMB:       4096,
		OrganizationID:     &org.ID,
		SubscriptionPlanID: &seatXL.ID,
		UserTerminalKeyID:  userKey.ID,
	}).Error)

	ttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/metrics":
			_ = json.NewEncoder(w).Encode(map[string]any{"ram_available_gb": 32.0, "ram_percent": 10.0})
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"sessions": []map[string]any{
				{"id": sessionID, "session_id": sessionID, "status": 1, "state": "stopped", "persistence_mode": "persistent", "expires_at": time.Now().Add(time.Hour).Unix()},
			}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start"):
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "running", "expires_at": time.Now().Add(time.Hour).Unix()})
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(ttSrv.Close)
	configureTTServer(t, ttSrv.URL)

	router := setupResumeRouterWithProdMiddleware(t, userID, services.NewTerminalTrainerService(sharedTestDB))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/terminals/"+sessionID+"/start", nil))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var row models.Terminal
	require.NoError(t, sharedTestDB.First(&row, "session_id = ?", sessionID).Error)
	require.Equal(t, models.StateRunning, row.State)
}
