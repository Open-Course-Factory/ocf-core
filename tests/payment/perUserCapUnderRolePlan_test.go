// tests/payment/perUserCapUnderRolePlan_test.go
//
// A plan reached through an organization is the member's own cap. The quota
// engine used to count every member's sessions against it as one shared pool:
// with a seat plan mapped to the member role, the first student's XL blocked
// the rest of the class. Each student now gets the seat the plan describes.
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBudget_RoleMappedSeatCapsEachStudentAlone(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	orgPlan := planWithBudget(t, db, "Formateur classe", 40, 6000, 6144)
	seat := planWithBudget(t, db, "Siege eleve XL", 10, 4000, 4096)
	org := orgSubscriptionOn(t, db, "teacher-1", orgPlan)
	addMemberWithRole(t, db, org.ID, "student-a", "member")
	addMemberWithRole(t, db, org.ID, "student-b", "member")
	rolePlanOn(t, db, org.ID, "member", seat.ID)

	plans := services.NewEffectivePlanService(db)
	quota := services.NewQuotaService(db, plans)

	// Student A holds an XL.
	require.NoError(t, db.Create(&terminalModels.Terminal{
		BaseModel:     entityManagementModels.BaseModel{ID: uuid.New()},
		SessionID:     "sess-a",
		UserID:        "student-a",
		State:         terminalModels.StateRunning,
		SizeCPU:       4000,
		SizeMemoryMB:  4096,
		LastStartedAt: time.Now(),
		ExpiresAt:     time.Now().Add(time.Hour),
	}).Error)

	resultB, err := plans.GetUserEffectivePlan("student-b", &org.ID)
	require.NoError(t, err)
	assert.Equal(t, seat.ID, resultB.Plan.ID, "the member role maps to the seat plan")

	check, err := quota.CheckBudget("student-b", nil, resultB.Plan, 4000, 4096)
	require.NoError(t, err)
	assert.True(t, check.Allowed, "student B's seat is their own: student A's XL does not consume it (reason=%s)", check.Reason)

	// Student A, already holding an XL, is at their own cap.
	resultA, err := plans.GetUserEffectivePlan("student-a", &org.ID)
	require.NoError(t, err)
	check, err = quota.CheckBudget("student-a", nil, resultA.Plan, 4000, 4096)
	require.NoError(t, err)
	assert.False(t, check.Allowed, "a second XL exceeds student A's own seat")
}
