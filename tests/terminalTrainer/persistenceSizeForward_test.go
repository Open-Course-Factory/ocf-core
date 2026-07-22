// tests/terminalTrainer/persistenceSizeForward_test.go
//
// RED tests for product decision 1a: ocf-core must forward the plan's storage
// quota (SubscriptionPlan.DataPersistenceGB) to tt-backend as persistence_size_gb
// on POST /1.0/sessions, so the persistent volume is actually sized to the plan.
//
// tt-backend (feat/persistent-volume-size) accepts an int persistence_size_gb and
// sizes the Incus volume "<N>GiB"; 0 or absent keeps today's unsized behaviour.
// ocf-core does NOT send the field today, so a paid plan with a 50 GB quota still
// provisions an unsized volume — the quota is not real.
//
// Contract pinned (all asserted on the JSON body POSTed to the fake tt-backend,
// following the composedSessionRecorder precedent in persistenceMode_test.go):
//   (a) persistent session + plan.DataPersistenceGB=N>0 → body carries
//       persistence_size_gb=N.
//   (b) persistent session + plan.DataPersistenceGB=0 → field OMITTED (matches the
//       idle_window_seconds convention: the composer only adds the key when it
//       carries a meaningful value, and tt-backend treats 0/absent as "unsized").
//   (c) ephemeral session → NO persistence_size_gb regardless of the plan's quota
//       (an ephemeral session gets no persistent volume, so its size is moot).
//   (d) org context still forwards the (effective) plan's quota — the org
//       backend/idle resolution path must not drop the size.
//
// These reuse startComposedSessionTTServer / configureTTServer / createTestUserKey
// / freshTestDB from persistenceMode_test.go (same package). They assert
// USER-OBSERVABLE wire state (the outgoing body), never a mock call, and add only
// the new persistence_size_gb key — no overlap with the existing persistence_mode
// / idle_window_seconds assertions.
package terminalTrainer_tests

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityManagementModels "soli/formations/src/entityManagement/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"
)

// makePersistencePlan builds a plan that permits persistent sessions and carries
// the given storage quota in GB.
func makePersistencePlan(gb int) *paymentModels.SubscriptionPlan {
	return &paymentModels.SubscriptionPlan{
		BaseModel:                 entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                      "persistence-plan",
		IsActive:                  true,
		MaxSessionDurationMinutes: 60,
		DataPersistenceEnabled:    true,
		DataPersistenceGB:         gb,
	}
}

// (a) persistent + quota>0 → forwarded ------------------------------------------

func TestStartComposedSession_ForwardsPersistenceSizeForPersistentSession(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "pers-size-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePersistencePlan(50)
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "persistent",
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, float64(50), rec.gotBody["persistence_size_gb"],
		"a persistent session must forward the plan's DataPersistenceGB as persistence_size_gb; got %v", rec.gotBody)
}

// (b) persistent + quota==0 → omitted -------------------------------------------

func TestStartComposedSession_OmitsPersistenceSizeWhenPlanQuotaZero(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "pers-size-zero-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePersistencePlan(0) // persistent allowed, but no storage quota
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "persistent",
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	_, present := rec.gotBody["persistence_size_gb"]
	assert.False(t, present,
		"persistence_size_gb must be omitted when the plan quota is 0 (tt-backend keeps its unsized default); body=%v", rec.gotBody)
}

// (c) ephemeral → never forwarded, even with a quota ----------------------------

func TestStartComposedSession_OmitsPersistenceSizeForEphemeralSession(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "eph-size-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	plan := makePersistencePlan(50) // quota set, but the session is ephemeral
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		PersistenceMode: "ephemeral",
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	_, present := rec.gotBody["persistence_size_gb"]
	assert.False(t, present,
		"an ephemeral session must NOT forward persistence_size_gb regardless of the plan quota; body=%v", rec.gotBody)
}

// (d) org context still forwards the effective plan's quota ---------------------

func TestStartComposedSession_ForwardsPersistenceSizeWithOrgContext(t *testing.T) {
	srv, rec := startComposedSessionTTServer(t)
	defer srv.Close()
	configureTTServer(t, srv.URL)

	db := freshTestDB(t)
	userID := "pers-size-org-user-" + uuid.New().String()
	_, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	org := &orgModels.Organization{
		BaseModel:        entityManagementModels.BaseModel{ID: uuid.New()},
		Name:             "pers-size-org",
		DisplayName:      "Persistence Size Org",
		OwnerUserID:      userID,
		OrganizationType: orgModels.OrgTypeTeam,
		IsActive:         true,
		MaxGroups:        10,
		MaxMembers:       50,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	// The effective plan (org- or role-resolved upstream) is what
	// StartComposedSession receives; its DataPersistenceGB must still reach the wire.
	plan := makePersistencePlan(100)
	svc := services.NewTerminalTrainerService(db)

	resp, err := svc.StartComposedSession(userID, dto.CreateComposedSessionInput{
		Distribution:    "ubuntu-24.04",
		Size:            "S",
		Terms:           "accepted",
		OrganizationID:  org.ID.String(),
		PersistenceMode: "persistent",
	}, plan)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, float64(100), rec.gotBody["persistence_size_gb"],
		"the org-context path must still forward the effective plan's DataPersistenceGB; got %v", rec.gotBody)
}
