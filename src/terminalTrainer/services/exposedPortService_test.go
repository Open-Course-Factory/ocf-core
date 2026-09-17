package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	auditModels "soli/formations/src/audit/models"
	configModels "soli/formations/src/configuration/models"
	paymentModels "soli/formations/src/payment/models"
	scenarioModels "soli/formations/src/scenarios/models"
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
		&scenarioModels.Scenario{},
		&scenarioModels.ScenarioSession{},
		&configModels.Feature{},
	))
	// The platform flag fails closed: the row must exist, enabled, for any
	// exposure to work. Tests that exercise the switch flip it explicitly.
	setPortExposureFeature(t, db, true)
	return db
}

// setPortExposureFeature writes the platform flag row; absent, the
// repository defaults to enabled, which is what every other test relies on.
func setPortExposureFeature(t *testing.T, db *gorm.DB, enabled bool) {
	var existing configModels.Feature
	if err := db.Where("key = ?", PortExposureFeatureKey).First(&existing).Error; err == nil {
		require.NoError(t, db.Model(&existing).Update("enabled", enabled).Error)
		return
	}
	require.NoError(t, db.Create(&configModels.Feature{Key: PortExposureFeatureKey, Name: "x", Enabled: enabled}).Error)
}

// createOpenScenarioRun links an open scenario run to the given terminal
// session, on a scenario that does or does not allow port exposure.
func createOpenScenarioRun(t *testing.T, db *gorm.DB, sessionID string, portExposureAllowed bool) {
	scenario := &scenarioModels.Scenario{Name: "s", Title: "S", InstanceType: "alp", PortExposureAllowed: portExposureAllowed}
	require.NoError(t, db.Create(scenario).Error)
	run := &scenarioModels.ScenarioSession{
		ScenarioID:        scenario.ID,
		UserID:            "user1",
		TerminalSessionID: &sessionID,
		Status:            "active",
		StartedAt:         time.Now(),
	}
	require.NoError(t, db.Create(run).Error)
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

func TestCreateExposedPort_RejectsWhenScenarioDisallows(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()

	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-1", planID)
	createOpenScenarioRun(t, db, "sess-1", false)

	svc := newExposedPortTestService(server.URL, db)
	_, err := svc.CreateExposedPort("sess-1", 8080)

	var scenarioErr *ScenarioDisallowsError
	assert.ErrorAs(t, err, &scenarioErr)
}

func TestCreateExposedPort_AllowedWhenScenarioAllows(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()

	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-1", planID)
	createOpenScenarioRun(t, db, "sess-1", true)

	svc := newExposedPortTestService(server.URL, db)
	_, err := svc.CreateExposedPort("sess-1", 8080)
	assert.NoError(t, err)
}

func TestListExposedPorts_RunsTheSameGateAsCreate(t *testing.T) {
	db := setupExposedPortTestDB(t)
	deniedPlan := createTestPlan(t, db, false)
	createTestTerminal(t, db, "sess-denied", deniedPlan)
	allowedPlan := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-allowed", allowedPlan)

	svc := newExposedPortTestService("http://unused", db)

	_, err := svc.ListExposedPorts("sess-denied")
	var planErr *PlanDisabledError
	assert.ErrorAs(t, err, &planErr)

	list, err := svc.ListExposedPorts("sess-allowed")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestCreateExposedPort_RecordsBackend(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()

	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	terminal := createTestTerminal(t, db, "sess-1", planID)
	terminal.Backend = "hexceos"
	require.NoError(t, db.Save(terminal).Error)

	svc := newExposedPortTestService(server.URL, db)
	_, err := svc.CreateExposedPort("sess-1", 8080)
	require.NoError(t, err)

	stored, err := svc.repository.GetExposedPortsBySessionID("sess-1")
	require.NoError(t, err)
	assert.Equal(t, "hexceos", (*stored)[0].Backend)
}

func TestExposedPortURL_FollowsScheme(t *testing.T) {
	t.Setenv("EXPOSE_DOMAIN", "expose.example")
	t.Setenv("EXPOSE_SCHEME", "")
	assert.Equal(t, "http://abc.expose.example", ExposedPortURL("abc"))
	assert.False(t, ExposeTLSEnabled())

	t.Setenv("EXPOSE_SCHEME", "https")
	assert.Equal(t, "https://abc.expose.example", ExposedPortURL("abc"))
	assert.True(t, ExposeTLSEnabled())
}

func TestExposedPorts_FeatureFlagOffRefusesAndPublishesNothing(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()

	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-1", planID)
	svc := newExposedPortTestService(server.URL, db)

	_, err := svc.CreateExposedPort("sess-1", 8080)
	require.NoError(t, err, "flag row enabled")

	setPortExposureFeature(t, db, false)

	_, err = svc.CreateExposedPort("sess-1", 8081)
	var featureErr *FeatureDisabledError
	assert.ErrorAs(t, err, &featureErr)
	_, err = svc.ListExposedPorts("sess-1")
	assert.ErrorAs(t, err, &featureErr)

	active, err := svc.GetActiveExposedPortsForTraefik()
	require.NoError(t, err)
	assert.Empty(t, active, "the kill switch: an existing exposure is no longer published")
}

func TestCreateExposedPort_LifetimeIsThePlanTTLOrTheSessionEndWhicheverComesFirst(t *testing.T) {
	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	require.NoError(t, db.Model(&paymentModels.SubscriptionPlan{}).Where("id = ?", planID).Update("port_exposure_ttl_minutes", 30).Error)
	stub := httptest.NewServer(infoStub("10.0.0.5"))
	defer stub.Close()
	svc := newExposedPortTestService(stub.URL, db)

	// session ends in an hour: the 30 min TTL wins
	createTestTerminal(t, db, "sess-ttl", planID)
	resp, err := svc.CreateExposedPort("sess-ttl", 8000)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(30*time.Minute), resp.ExpiresAt, 5*time.Second)

	// session ends in ten minutes: the session end wins
	short := createTestTerminal(t, db, "sess-short", planID)
	short.ExpiresAt = time.Now().Add(10 * time.Minute)
	require.NoError(t, db.Save(short).Error)
	resp, err = svc.CreateExposedPort("sess-short", 8000)
	require.NoError(t, err)
	assert.WithinDuration(t, short.ExpiresAt, resp.ExpiresAt, time.Second)
}

func TestExposureExpiry_UnsetPlanTTLMeansAnHour(t *testing.T) {
	farSessionEnd := time.Now().Add(8 * time.Hour)
	assert.WithinDuration(t, time.Now().Add(time.Hour), exposureExpiry(farSessionEnd, 0), time.Second)
	assert.WithinDuration(t, time.Now().Add(2*time.Hour), exposureExpiry(farSessionEnd, 120), time.Second)
}

func TestExposedPorts_ExpiredOnesAreNeitherListedNorCounted(t *testing.T) {
	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	stub := httptest.NewServer(infoStub("10.0.0.5"))
	defer stub.Close()
	svc := newExposedPortTestService(stub.URL, db)
	terminal := createTestTerminal(t, db, "sess-exp", planID)

	// three expired rows the sweep has not deleted yet
	for i := 0; i < maxExposedPortsPerSession; i++ {
		require.NoError(t, db.Create(&models.ExposedPort{TerminalID: terminal.ID, SessionID: "sess-exp", UserID: "user1",
			ContainerPort: 9000 + i, Slug: fmt.Sprintf("old%d", i), ContainerIP: "10.0.0.5", ExpiresAt: time.Now().Add(-time.Minute)}).Error)
	}

	listed, err := svc.ListExposedPorts("sess-exp")
	require.NoError(t, err)
	assert.Empty(t, listed, "expired exposures are unreachable, the list must not show them")

	_, err = svc.CreateExposedPort("sess-exp", 8000)
	assert.NoError(t, err, "expired exposures must not count against the per-session cap")
}

func TestExposedPorts_ExposeAndWithdrawLeaveAnAuditTrail(t *testing.T) {
	db := setupExposedPortTestDB(t)
	require.NoError(t, db.AutoMigrate(&auditModels.AuditLog{}))
	planID := createTestPlan(t, db, true)
	stub := httptest.NewServer(infoStub("10.0.0.5"))
	defer stub.Close()
	svc := newExposedPortTestService(stub.URL, db)
	createTestTerminal(t, db, "sess-audit", planID)

	created, err := svc.CreateExposedPort("sess-audit", 8000)
	require.NoError(t, err)
	require.NoError(t, svc.DeleteExposedPort("sess-audit", created.ID))

	var trail []auditModels.AuditLog
	require.NoError(t, db.Order("created_at").Find(&trail).Error)
	require.Len(t, trail, 2)
	assert.Equal(t, auditModels.AuditEventPortExposed, trail[0].EventType)
	assert.Equal(t, auditModels.AuditEventPortUnexposed, trail[1].EventType)
	for _, e := range trail {
		assert.Equal(t, "exposed_port", e.TargetType)
		assert.Equal(t, created.Slug, e.TargetName)
		assert.Equal(t, "sess-audit", e.SessionID)
		assert.Contains(t, e.Metadata, `"container_ip":"10.0.0.5"`)
		assert.Contains(t, e.Metadata, `"port":8000`)
	}
}

func TestExposureAuditEntry_AdminKillNamesTheOwnerAsOnBehalfOf(t *testing.T) {
	owner, admin := uuid.New(), uuid.New()
	ep := &models.ExposedPort{UserID: owner.String(), SessionID: "s", Slug: "abc", ContainerPort: 8000}
	entry := ExposureAuditEntry(auditModels.AuditEventPortUnexposed, ep, admin.String())
	require.NotNil(t, entry.ActorID)
	assert.Equal(t, admin, *entry.ActorID)
	require.NotNil(t, entry.OnBehalfOfID, "an admin killing someone's exposure is recorded as acting on their behalf")
	assert.Equal(t, owner, *entry.OnBehalfOfID)

	own := ExposureAuditEntry(auditModels.AuditEventPortUnexposed, ep, owner.String())
	assert.Nil(t, own.OnBehalfOfID, "the owner acting on their own exposure has no on-behalf-of")
}

func TestExposedPorts_MissingFeatureRowFailsClosed(t *testing.T) {
	server := httptest.NewServer(infoStub("10.0.0.5"))
	defer server.Close()
	db := setupExposedPortTestDB(t)
	require.NoError(t, db.Where("key = ?", PortExposureFeatureKey).Delete(&configModels.Feature{}).Error)
	planID := createTestPlan(t, db, true)
	createTestTerminal(t, db, "sess-norow", planID)
	svc := newExposedPortTestService(server.URL, db)

	_, err := svc.CreateExposedPort("sess-norow", 8080)
	var featureErr *FeatureDisabledError
	require.ErrorAs(t, err, &featureErr, "no flag row must read as disabled, never as enabled")
	rows, err := svc.GetActiveExposedPortsForTraefik()
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestExposureExpiry_PlanTTLIsCapped(t *testing.T) {
	farSessionEnd := time.Now().Add(48 * time.Hour)
	assert.WithinDuration(t, time.Now().Add(maxExposeTTL), exposureExpiry(farSessionEnd, 24*60), time.Second)
}

// infoStubPerSession answers /info differently per session id: a map value
// of "" means 404, "stopped:<ip>" a present-but-stopped container.
func infoStubPerSession(answers map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		answer, ok := answers[r.URL.Query().Get("id")]
		if !ok || answer == "" {
			http.NotFound(w, r)
			return
		}
		running, ip := "true", answer
		if strings.HasPrefix(answer, "stopped:") {
			running, ip = "false", strings.TrimPrefix(answer, "stopped:")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"stub","status":0,"ip":"` + ip + `","instance_running":` + running + `}`))
	}
}

func TestReconcileExposedPorts_DropsGoneStoppedAndReaddressedContainers(t *testing.T) {
	db := setupExposedPortTestDB(t)
	require.NoError(t, db.AutoMigrate(&auditModels.AuditLog{}))
	planID := createTestPlan(t, db, true)
	create := httptest.NewServer(infoStub("10.0.0.5"))
	defer create.Close()
	svc := newExposedPortTestService(create.URL, db)
	for _, sess := range []string{"sess-ok", "sess-gone", "sess-stopped", "sess-moved", "sess-unknown"} {
		createTestTerminal(t, db, sess, planID)
		_, err := svc.CreateExposedPort(sess, 8000)
		require.NoError(t, err)
	}

	// what tt-backend says now; sess-unknown gets a broken backend
	reconcile := httptest.NewServer(infoStubPerSession(map[string]string{
		"sess-ok": "10.0.0.5", "sess-gone": "", "sess-stopped": "stopped:10.0.0.5", "sess-moved": "10.0.0.9",
		"sess-unknown": "10.0.0.5",
	}))
	defer reconcile.Close()
	newExposedPortTestService(reconcile.URL, db).ReconcileExposedPorts()

	var left []models.ExposedPort
	require.NoError(t, db.Find(&left).Error)
	sessions := []string{}
	for _, ep := range left {
		sessions = append(sessions, ep.SessionID)
	}
	assert.ElementsMatch(t, []string{"sess-ok", "sess-unknown"}, sessions, "only exposures whose container still matches survive")

	var trail []auditModels.AuditLog
	require.NoError(t, db.Where("event_type = ? AND error_message <> ''", auditModels.AuditEventPortUnexposed).Order("target_name").Find(&trail).Error)
	reasons := map[string]string{}
	for _, e := range trail {
		reasons[e.SessionID] = e.ErrorMessage
	}
	assert.Equal(t, map[string]string{"sess-gone": "container gone", "sess-stopped": "container stopped", "sess-moved": "container address changed"}, reasons)
}

func TestReconcileExposedPorts_UnreachableBackendChangesNothing(t *testing.T) {
	db := setupExposedPortTestDB(t)
	planID := createTestPlan(t, db, true)
	create := httptest.NewServer(infoStub("10.0.0.5"))
	svc := newExposedPortTestService(create.URL, db)
	createTestTerminal(t, db, "sess-1", planID)
	_, err := svc.CreateExposedPort("sess-1", 8000)
	require.NoError(t, err)
	create.Close() // tt-backend now refuses connections

	newExposedPortTestService(create.URL, db).ReconcileExposedPorts()
	var count int64
	require.NoError(t, db.Model(&models.ExposedPort{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "no answer is not evidence that the container is gone")
}
