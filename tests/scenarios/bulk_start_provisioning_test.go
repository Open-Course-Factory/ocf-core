package scenarios_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// bulkStartFixture builds the smallest world a bulk start needs: a team org, a
// scenario inside it, a plan every member resolves to, and a class whose roles
// the caller chooses.
//
// membersByRole is role -> user IDs, so a test can state "one manager and two
// members" and then assert on who was actually started.
func bulkStartFixture(t *testing.T, name string, membersByRole map[string][]string) (*services.TeacherDashboardService, *capturingTTService, models.Scenario, uuid.UUID) {
	t.Helper()
	db := setupTestDB(t)

	orgID := uuid.New()
	ownerID := name + "-owner"
	require.NoError(t, db.Omit("Metadata").Create(&orgModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: orgID},
		Name:             name + "-org",
		DisplayName:      name + " Org",
		OwnerUserID:      ownerID,
		IsActive:         true,
		OrganizationType: orgModels.OrgTypeTeam,
	}).Error)

	scenario := models.Scenario{
		Name:           name,
		Title:          name,
		InstanceType:   "xs",
		CreatedByID:    ownerID,
		OrganizationID: &orgID,
	}
	require.NoError(t, db.Create(&scenario).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&models.ScenarioStep{
			ScenarioID: scenario.ID, Order: i, Title: "Step", TextContent: "content",
		}).Error)
	}

	plan := paymentModels.SubscriptionPlan{
		Name:                        name + "-plan",
		IsActive:                    true,
		MaxSessionDurationMinutes:   240,
		CommandHistoryRetentionDays: 30,
	}
	require.NoError(t, db.Create(&plan).Error)

	ttMock := newCapturingTTService()
	groupID := uuid.New()
	for role, users := range membersByRole {
		for _, uid := range users {
			require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
				GroupID: groupID, UserID: uid, Role: groupModels.GroupMemberRole(role),
				JoinedAt: time.Now(), IsActive: true,
			}).Error)
			require.NoError(t, db.Omit("Metadata").Create(&orgModels.OrganizationMember{
				BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
				OrganizationID: orgID,
				UserID:         uid,
				Role:           "member",
				JoinedAt:       time.Now(),
				IsActive:       true,
			}).Error)
			require.NoError(t, db.Create(&paymentModels.UserSubscription{
				UserID:             uid,
				SubscriptionPlanID: plan.ID,
				Status:             "active",
				CurrentPeriodStart: time.Now().Add(-24 * time.Hour),
				CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
			}).Error)
			ttMock.addKey(uid)
		}
	}

	sessionSvc := services.NewScenarioSessionService(db, &mockFlagService{}, &mockVerificationService{})
	dashSvc := services.NewTeacherDashboardService(db, ttMock, sessionSvc)
	return dashSvc, ttMock, scenario, groupID
}

// TestBulkStartScenario_BuildsTheContainerTheScenarioAsked verifies that a class
// launch asks for the size and the features the scenario declares — including
// the build-time ones.
//
// This is the defect that broke a live class: bulk start hardcoded size "S" and
// sent no features at all, so a scenario declaring build_features ["network"]
// got a container with no interface. Its setup script then could not resolve
// deb.debian.org, apt exited 100, and every learner landed in setup_failed
// while the same scenario launched one-at-a-time worked perfectly.
func TestBulkStartScenario_BuildsTheContainerTheScenarioAsked(t *testing.T) {
	dashSvc, ttMock, scenario, groupID := bulkStartFixture(t, "provisioning", map[string][]string{
		"member": {"provisioning-learner"},
	})

	provisioning := services.ScenarioProvisioning{
		Backend:       "some-backend",
		Distribution:  "Debian",
		Size:          "xs",
		Features:      map[string]bool{"effects": true},
		BuildFeatures: map[string]bool{"network": true},
	}
	result, err := dashSvc.BulkStartScenario(groupID, scenario.ID, provisioning, 0, "trainer")
	require.NoError(t, err)
	require.Empty(t, result.Errors)
	require.Len(t, ttMock.capturedInputs, 1)

	got := ttMock.capturedInputs[0]
	assert.Equal(t, "xs", got.Size,
		"the scenario's own size must reach the container, not a hardcoded default")
	assert.Equal(t, "Debian", got.Distribution)
	assert.Equal(t, "some-backend", got.Backend)
	assert.Equal(t, map[string]bool{"network": true}, got.BuildFeatures,
		"a scenario whose setup installs packages needs its build-time network")
	assert.Equal(t, map[string]bool{"effects": true}, got.Features)
}

// TestBulkStartScenario_ClosesTheProvisioningWindow verifies that a class launch
// gives the build-time features back once setup is done.
//
// Half of the same defect, and the more dangerous half: the teacher controller
// built a session service without wiring the build-complete callback, and
// finishBuild returns silently when it is nil. Handing out build features
// without this would leave every learner's machine holding a NIC nobody
// intended, for the whole session.
func TestBulkStartScenario_ClosesTheProvisioningWindow(t *testing.T) {
	dashSvc, ttMock, scenario, groupID := bulkStartFixture(t, "buildwindow", map[string][]string{
		"member": {"buildwindow-learner"},
	})

	_, err := dashSvc.BulkStartScenario(groupID, scenario.ID, services.ScenarioProvisioning{
		Distribution:  "Debian",
		Size:          "xs",
		BuildFeatures: map[string]bool{"network": true},
	}, 0, "trainer")
	require.NoError(t, err)

	assert.Equal(t, []string{"terminal-buildwindow-learner"}, ttMock.buildCompleted,
		"the provisioning window must be closed for every container the class launch built")
}
