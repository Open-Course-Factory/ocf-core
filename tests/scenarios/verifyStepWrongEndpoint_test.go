package scenarios_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/scenarios/models"
)

// /verify on a step answered through another endpoint is the caller's mistake,
// not a server failure: it must say so with a 400 and point to the right one.
func TestVerifyStep_StepWithItsOwnSubmission_Returns400WithTheEndpoint(t *testing.T) {
	cases := []struct{ stepType, endpoint string }{
		{"flag", "/submit-flag"},
		{"quiz", "/submit-quiz"},
	}
	for _, tc := range cases {
		t.Run(tc.stepType, func(t *testing.T) {
			db := freshTestDB(t)
			router := setupTestRouter(db)

			scenario := models.Scenario{
				Name: "verify-" + tc.stepType, Title: "Verify " + tc.stepType,
				InstanceType: "ubuntu:22.04", CreatedByID: "creator-1",
			}
			require.NoError(t, db.Create(&scenario).Error)
			require.NoError(t, db.Create(&models.ScenarioStep{
				ScenarioID: scenario.ID, Order: 0, Title: "s", StepType: tc.stepType,
			}).Error)
			terminalID := "terminal-verify-" + tc.stepType
			session := models.ScenarioSession{
				ScenarioID: scenario.ID, UserID: "test-user-123", CurrentStep: 0,
				Status: "active", StartedAt: time.Now(), TerminalSessionID: &terminalID,
			}
			require.NoError(t, db.Create(&session).Error)
			require.NoError(t, db.Create(&models.ScenarioStepProgress{
				SessionID: session.ID, StepOrder: 0, Status: "active",
			}).Error)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/api/v1/scenario-sessions/"+session.ID.String()+"/verify", nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
			assert.Contains(t, w.Body.String(), tc.endpoint)
		})
	}
}
