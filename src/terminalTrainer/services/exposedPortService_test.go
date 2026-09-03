package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	paymentModels "soli/formations/src/payment/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupExposedPortTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.UserTerminalKey{},
		&models.Terminal{},
		&models.ExposedPort{},
		&paymentModels.SubscriptionPlan{},
	))
	return db
}

// newExposedPortTestService wires an exposedPortService whose proxy talks to
// the given tt-backend stub, mirroring newCommandHistoryTestService's pattern
// for the collaborator under test.
func newExposedPortTestService(baseURL string, db *gorm.DB) *exposedPortService {
	repo := repositories.NewTerminalRepository(db)
	proxy := newTerminalProxyClient(repo)
	proxy.baseURL = baseURL
	proxy.apiVersion = "1.0"
	proxy.terminalType = ""
	return newExposedPortService(proxy, repo, db)
}

// createTestPlan inserts a SubscriptionPlan with the given PortExposureEnabled
// value and returns its ID.
func createTestPlan(t *testing.T, db *gorm.DB, portExposureEnabled bool) uuid.UUID {
	plan := &paymentModels.SubscriptionPlan{
		Name:                "Test Plan",
		PortExposureEnabled: portExposureEnabled,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan.ID
}

// createTestTerminal creates a running terminal owned by the given plan.
func createTestTerminal(t *testing.T, db *gorm.DB, sessionID string, planID uuid.UUID) *models.Terminal {
	userKey := &models.UserTerminalKey{
		UserID:      "user1",
		APIKey:      "test-api-key",
		KeyName:     "test-key",
		IsActive:    true,
		MaxSessions: 5,
	}
	require.NoError(t, db.Create(userKey).Error)

	terminal := &models.Terminal{
		SessionID:          sessionID,
		UserID:             "user1",
		State:              models.StateRunning,
		ExpiresAt:          time.Now().Add(time.Hour),
		InstanceType:       "alp",
		MachineSize:        "S",
		UserTerminalKeyID:  userKey.ID,
		UserTerminalKey:    *userKey,
		SubscriptionPlanID: &planID,
	}
	require.NoError(t, db.Create(terminal).Error)
	return terminal
}

func infoStub(ip string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"stub","status":0,"ip":"` + ip + `"}`))
	}
}

func TestCreateExposedPort_RejectsWhenPlanDisallows(t *testing.T) {
	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, false)
	createTestTerminal(t, db, "sess-1", planID)

	svc := newExposedPortTestService("http://unused", db)
	_, err := svc.CreateExposedPort("sess-1", 8080)

	require.Error(t, err)
	var planErr *PlanDisabledError
	assert.ErrorAs(t, err, &planErr)
}

func TestCreateExposedPort_RejectsOutOfRangePort(t *testing.T) {
	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-1", planID)

	svc := newExposedPortTestService("http://unused", db)

	_, err := svc.CreateExposedPort("sess-1", 80)
	assert.Error(t, err)

	_, err = svc.CreateExposedPort("sess-1", 70000)
	assert.Error(t, err)
}

func TestCreateExposedPort_RejectsWhenSessionNotRunning(t *testing.T) {
	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	terminal := createTestTerminal(t, db, "sess-1", planID)
	terminal.State = models.StateStopped
	require.NoError(t, db.Save(terminal).Error)

	svc := newExposedPortTestService("http://unused", db)
	_, err := svc.CreateExposedPort("sess-1", 8080)
	assert.Error(t, err)
}

func TestCreateExposedPort_Success(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()

	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-1", planID)

	svc := newExposedPortTestService(server.URL, db)
	resp, err := svc.CreateExposedPort("sess-1", 8080)

	require.NoError(t, err)
	assert.Equal(t, 8080, resp.Port)
	assert.Len(t, resp.Slug, slugLength)
	assert.NotEmpty(t, resp.URL)

	stored, err := svc.repository.GetExposedPortsBySessionID("sess-1")
	require.NoError(t, err)
	require.Len(t, *stored, 1)
	assert.Equal(t, "10.0.0.5", (*stored)[0].ContainerIP)
}

func TestCreateExposedPort_EnforcesPerSessionCap(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()

	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-1", planID)

	svc := newExposedPortTestService(server.URL, db)
	for i := 0; i < maxExposedPortsPerSession; i++ {
		_, err := svc.CreateExposedPort("sess-1", 8080+i)
		require.NoError(t, err)
	}

	_, err := svc.CreateExposedPort("sess-1", 9999)
	assert.Error(t, err)
}

func TestDeleteExposedPort_ScopedToOwningSession(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()

	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-1", planID)
	createTestTerminal(t, db, "sess-2", planID)

	svc := newExposedPortTestService(server.URL, db)
	resp, err := svc.CreateExposedPort("sess-1", 8080)
	require.NoError(t, err)

	// Deleting via the WRONG session id must fail, even though the row exists.
	err = svc.DeleteExposedPort("sess-2", resp.ID)
	assert.Error(t, err)

	err = svc.DeleteExposedPort("sess-1", resp.ID)
	assert.NoError(t, err)
}

func TestGetActiveExposedPortsForTraefik_ExcludesStoppedSessions(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()

	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	running := createTestTerminal(t, db, "sess-running", planID)
	_ = running
	stopped := createTestTerminal(t, db, "sess-stopped", planID)

	svc := newExposedPortTestService(server.URL, db)
	_, err := svc.CreateExposedPort("sess-running", 8080)
	require.NoError(t, err)
	_, err = svc.CreateExposedPort("sess-stopped", 8081)
	require.NoError(t, err)

	stopped.State = models.StateStopped
	require.NoError(t, db.Save(stopped).Error)

	active, err := svc.GetActiveExposedPortsForTraefik()
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "sess-running", active[0].SessionID)
}
