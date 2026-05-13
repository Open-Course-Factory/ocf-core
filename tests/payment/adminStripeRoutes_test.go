// tests/payment/adminStripeRoutes_test.go
//
// Failing tests for the admin Stripe pending-syncs endpoint (issue #327, MR-O).
//
// The implementer must add:
//   - src/payment/routes/adminStripeRoutes.go with:
//       NewAdminStripePendingSyncsHandler(queue paymentServices.StripeSyncQueue) gin.HandlerFunc
//
// Contract:
//   - GET /api/v1/admin/stripe/pending-syncs
//   - administrator role only (returns 403 for members)
//   - Response JSON: {"count": N, "items": [ {id, plan_id, operation, state,
//     attempts, created_at, last_error?, last_attempt_at?}, ... ]}
//   - The handler calls queue.ListPending(limit) — the limit value is an
//     implementation detail; tests assert that pending rows are surfaced.
package payment_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	entityManagementModels "soli/formations/src/entityManagement/models"
	paymentModels "soli/formations/src/payment/models"
	paymentRoutes "soli/formations/src/payment/routes"
	paymentServices "soli/formations/src/payment/services"
)

// setupAdminStripeRouter builds a minimal gin router that injects a userRoles
// context value (administrator / member) and wires the handler under test.
// Mirrors the auth-middleware stub used in tests/admin/users_test.go and
// tests/observability/metrics_endpoint_test.go.
func setupAdminStripeRouter(t *testing.T, db *gorm.DB, isAdmin bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if isAdmin {
			c.Set("userId", "test-admin")
			c.Set("userRoles", []string{"administrator"})
		} else {
			c.Set("userId", "test-member")
			c.Set("userRoles", []string{"member"})
		}
		c.Next()
	})
	queue := paymentServices.NewStripeSyncQueue(db)
	r.GET("/api/v1/admin/stripe/pending-syncs", paymentRoutes.NewAdminStripePendingSyncsHandler(queue))
	return r
}

// TestAdminStripePendingSyncs_RequiresAdmin asserts the Layer-2 AdminOnly check
// rejects non-admin callers and admits administrators.
func TestAdminStripePendingSyncs_RequiresAdmin(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	for _, tc := range []struct {
		name     string
		isAdmin  bool
		wantCode int
	}{
		{"member is rejected", false, http.StatusForbidden},
		{"administrator is allowed", true, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := setupAdminStripeRouter(t, db, tc.isAdmin)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/stripe/pending-syncs", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.wantCode {
				t.Errorf("expected %d, got %d (body: %s)", tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestAdminStripePendingSyncs_ReturnsPendingItems seeds two pending rows and
// asserts the handler surfaces them in the JSON response, with the contract
// fields populated.
func TestAdminStripePendingSyncs_ReturnsPendingItems(t *testing.T) {
	db := freshTestDB(t)
	if err := db.AutoMigrate(&paymentModels.StripeSync{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	queue := paymentServices.NewStripeSyncQueue(db)

	plan := &paymentModels.SubscriptionPlan{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "P",
		PriceAmount: 100,
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationCreate, plan); err != nil {
		t.Fatalf("Enqueue create failed: %v", err)
	}
	if err := queue.Enqueue(paymentModels.StripeSyncOperationUpdate, plan); err != nil {
		t.Fatalf("Enqueue update failed: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", "test-admin")
		c.Set("userRoles", []string{"administrator"})
		c.Next()
	})
	r.GET("/api/v1/admin/stripe/pending-syncs", paymentRoutes.NewAdminStripePendingSyncsHandler(queue))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/stripe/pending-syncs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v (raw: %s)", err, w.Body.String())
	}

	countRaw, ok := resp["count"].(float64)
	if !ok {
		t.Fatalf("count must be a number, got %T (value: %v)", resp["count"], resp["count"])
	}
	if int(countRaw) != 2 {
		t.Errorf("count: expected 2, got %v", countRaw)
	}

	items, ok := resp["items"].([]any)
	if !ok {
		t.Fatalf("items must be a JSON array, got %T (value: %v)", resp["items"], resp["items"])
	}
	if len(items) != 2 {
		t.Errorf("items: expected 2, got %d", len(items))
	}
	if len(items) >= 1 {
		first, ok := items[0].(map[string]any)
		if !ok {
			t.Fatalf("items[0] must be a JSON object, got %T", items[0])
		}
		for _, key := range []string{"id", "plan_id", "operation", "state", "attempts", "created_at"} {
			if _, present := first[key]; !present {
				t.Errorf("item missing key %q: %+v", key, first)
			}
		}
	}
}
