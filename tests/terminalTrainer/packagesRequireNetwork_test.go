// Behavioural tests for the "custom startup packages require network"
// guard in StartComposedSession.
//
// Product rule: a composed session may only request custom startup
// `packages` when network egress is enabled FOR THAT SESSION. Custom
// packages are installed at container boot via cloud-init apt/apk
// install, which needs egress — so a plan or a session without network
// must reject a non-empty Packages list rather than silently boot a
// container whose package install will fail.
//
// "Network enabled for the session" means BOTH:
//   - the plan has NetworkAccessEnabled = true, AND
//   - the request enabled the network feature (Features["network"] = true)
//
// These tests drive the public TerminalTrainerService facade through the
// production controller → middleware → service → DB chain, against an
// httptest stub of tt-backend, exactly like composedSession_http_test.go
// and terminalComposer_test.go. We assert observable state (HTTP status +
// persisted Terminal rows), never mock invocations.
package terminalTrainer_tests

import (
	"bytes"
	"encoding/json"
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
	terminalServices "soli/formations/src/terminalTrainer/services"
)

// startNetworkComposedTTBackendStub is a network-aware variant of
// startComposedTTBackendStub: the ubuntu distro advertises "network" in
// its supported_features and the /features endpoint returns the network
// feature, so a request enabling network passes the catalog
// feature-validation step and reaches the packages guard / create flow.
func startNetworkComposedTTBackendStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/distributions"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"name":               "ubuntu-24.04",
					"prefix":             "ubuntu",
					"description":        "Ubuntu 24.04 LTS",
					"os_type":            "deb",
					"min_size_key":       "",
					"supported_features": []string{"network"},
					"default_size_key":   "S",
				},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sizes"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"key": "XS", "name": "Extra Small", "sort_order": 10, "cpu": 1, "memory": "256MB"},
				{"key": "S", "name": "Small", "sort_order": 20, "cpu": 1, "memory": "512MB"},
				{"key": "M", "name": "Medium", "sort_order": 30, "cpu": 2, "memory": "1GB"},
				{"key": "L", "name": "Large", "sort_order": 40, "cpu": 4, "memory": "2GB"},
				{"key": "XL", "name": "Extra Large", "sort_order": 50, "cpu": 4, "memory": "4GB"},
			})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/features"):
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"key": "network", "name": "Network Access", "sort_order": 10},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sessions"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id":    "stub-sess-" + uuid.New().String(),
				"status":        0,
				"expires_at":    time.Now().Add(time.Hour).Unix(),
				"backend":       "incus",
				"instance_name": "stub-instance",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// seedNetworkBudgetPlanForUser mirrors seedBudgetPlanForUser but lets the
// caller flip NetworkAccessEnabled so the network-enabled success path can
// be exercised. (seedBudgetPlanForUser leaves NetworkAccessEnabled at its
// zero value, false.)
func seedNetworkBudgetPlanForUser(t *testing.T, userID string, maxCPU, maxMemMB int, networkEnabled bool) uuid.UUID {
	t.Helper()
	plan := &paymentModels.SubscriptionPlan{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                 "BudgetHTTPNet",
		Priority:             5,
		IsActive:             true,
		BillingInterval:      "month",
		Currency:             "eur",
		MaxCPU:               maxCPU,
		MaxMemoryMB:          maxMemMB,
		MaxCourses:           10,
		NetworkAccessEnabled: networkEnabled,
	}
	require.NoError(t, sharedTestDB.Create(plan).Error)
	require.NoError(t, sharedTestDB.Create(&paymentModels.UserSubscription{
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		SubscriptionType:   "personal",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}).Error)
	return plan.ID
}

// postComposedSession is a small helper to POST a composed-session body
// and return the recorder, keeping the test bodies focused on the guard.
func postComposedSession(router http.Handler, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/terminals/start-composed-session", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func countTerminals(t *testing.T, userID string) int64 {
	t.Helper()
	var cnt int64
	require.NoError(t, sharedTestDB.Raw(
		`SELECT COUNT(*) FROM terminals WHERE user_id = ?`, userID,
	).Scan(&cnt).Error)
	return cnt
}

// TestStartComposedSession_PackagesRequireNetwork_RejectsWhenNetworkDisabled —
// a within-budget request that asks for custom startup Packages while
// network is NOT enabled for the session (plan NetworkAccessEnabled=false,
// no network feature requested) must be rejected AND persist no Terminal
// row. Without the guard this request currently SUCCEEDS (200, POSTs to
// tt-backend, persists a row) — that is the red the implementer must turn
// green.
func TestStartComposedSession_PackagesRequireNetwork_RejectsWhenNetworkDisabled(t *testing.T) {
	ttServer := startComposedTTBackendStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "pkg-net-reject-user"

	// Network disabled on the plan; plenty of budget so only the packages
	// guard can stop this request.
	seedBudgetPlanForUser(t, userID, 8000, 4096)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	router := setupBudgetHTTPRouter(t, userID, svc)

	w := postComposedSession(router, map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "M",
		"terms":        "accepted",
		"packages":     []string{"htop", "curl"},
	})

	assert.NotEqual(t, http.StatusOK, w.Code,
		"requesting custom packages without network must be rejected — got %d. Body: %s", w.Code, w.Body.String())
	assert.EqualValues(t, 0, countTerminals(t, userID),
		"a package-without-network request must not persist a Terminal row (and must not POST to tt-backend)")
}

// TestStartComposedSession_PackagesRequireNetwork_RejectsWhenNetworkFeatureOff —
// even on a network-capable plan, the request itself must enable the
// network feature for packages to be allowed. Here the plan has
// NetworkAccessEnabled=true but the request does NOT set
// features.network=true, so the session has no NIC and packages must be
// rejected.
func TestStartComposedSession_PackagesRequireNetwork_RejectsWhenNetworkFeatureOff(t *testing.T) {
	ttServer := startNetworkComposedTTBackendStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "pkg-net-featureoff-user"

	seedNetworkBudgetPlanForUser(t, userID, 8000, 4096, true)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	router := setupBudgetHTTPRouter(t, userID, svc)

	w := postComposedSession(router, map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "M",
		"terms":        "accepted",
		"features":     map[string]bool{"network": false},
		"packages":     []string{"htop"},
	})

	assert.NotEqual(t, http.StatusOK, w.Code,
		"packages must be rejected when the session does not enable the network feature — got %d. Body: %s", w.Code, w.Body.String())
	assert.EqualValues(t, 0, countTerminals(t, userID),
		"packages-without-network-feature must not persist a Terminal row")
}

// TestStartComposedSession_PackagesRequireNetwork_AllowsWhenNetworkEnabled —
// the positive case: plan NetworkAccessEnabled=true AND request enables the
// network feature → custom packages are allowed and the session is created
// (200 + exactly one persisted Terminal row). The guard must NOT fire here.
func TestStartComposedSession_PackagesRequireNetwork_AllowsWhenNetworkEnabled(t *testing.T) {
	ttServer := startNetworkComposedTTBackendStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "pkg-net-allow-user"

	seedNetworkBudgetPlanForUser(t, userID, 8000, 4096, true)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	router := setupBudgetHTTPRouter(t, userID, svc)

	w := postComposedSession(router, map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "M",
		"terms":        "accepted",
		"features":     map[string]bool{"network": true},
		"packages":     []string{"htop", "curl"},
	})

	require.Equal(t, http.StatusOK, w.Code,
		"packages with network enabled must succeed — got %d. Body: %s", w.Code, w.Body.String())
	assert.EqualValues(t, 1, countTerminals(t, userID),
		"a packages-with-network request must persist exactly one Terminal row")
}

// TestStartComposedSession_PackagesRequireNetwork_AllowsEmptyPackagesWithoutNetwork —
// the guard must only fire when packages are actually requested. An empty
// Packages list with network disabled must still succeed, so the guard does
// not regress the default (no-packages) composed-session flow.
func TestStartComposedSession_PackagesRequireNetwork_AllowsEmptyPackagesWithoutNetwork(t *testing.T) {
	ttServer := startComposedTTBackendStub(t)
	defer ttServer.Close()
	t.Setenv("TERMINAL_TRAINER_URL", ttServer.URL)
	t.Setenv("TERMINAL_TRAINER_ADMIN_KEY", "test-admin-key")
	t.Setenv("TERMINAL_TRAINER_API_VERSION", "1.0")

	freshTestDB(t)
	userID := "pkg-net-emptyok-user"

	seedBudgetPlanForUser(t, userID, 8000, 4096)
	_, err := createTestUserKey(sharedTestDB, userID)
	require.NoError(t, err)

	svc := terminalServices.NewTerminalTrainerService(sharedTestDB)
	router := setupBudgetHTTPRouter(t, userID, svc)

	w := postComposedSession(router, map[string]any{
		"distribution": "ubuntu-24.04",
		"size":         "M",
		"terms":        "accepted",
		// no "packages" key → empty Packages
	})

	require.Equal(t, http.StatusOK, w.Code,
		"no-packages request without network must still succeed — got %d. Body: %s", w.Code, w.Body.String())
	assert.EqualValues(t, 1, countTerminals(t, userID),
		"a no-packages composed request must persist exactly one Terminal row")
}
