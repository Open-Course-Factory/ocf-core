// tests/payment/billingAddressValidationHTTP_test.go
//
// HTTP-contract tests for B2B billing-field validation (issue #383, review of
// !284). The service-layer tests in billingAddressB2BFields_test.go pin that the
// validation hook rejects a malformed siret; these pin the HTTP status the
// client sees. A rejected field is client error (400), not a server fault or a
// missing resource. Today the generic controller maps a BeforeCreate hook error
// to 500 (ENT007) and a BeforeUpdate hook error to a hard-coded 404 — both wrong.
// RED until validation-hook errors are mapped to 400.
package payment_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/casdoor"
	entityManagementModels "soli/formations/src/entityManagement/models"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/models"
)

// TestBillingAddress_InvalidSiret_CreateReturns400 drives POST through the real
// generic controller with a 13-digit siret and asserts 400 (today: 500 ENT007).
func TestBillingAddress_InvalidSiret_CreateReturns400(t *testing.T) {
	_ = freshTestDB(t)
	gin.SetMode(gin.TestMode)
	setupBillingAddressEntity(t)
	withPaymentHooksRegistered(t)

	gc := controller.NewGenericController(sharedTestDB, casdoor.Enforcer)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", "b2b-user"); c.Next() })
	router.POST("/api/v1/billing-addresses/", func(c *gin.Context) { gc.AddEntity(c) })

	body, _ := json.Marshal(map[string]any{
		"line1": "1 rue de Rivoli", "city": "Paris", "postal_code": "75001", "country": "FR",
		"siret": "1234567890123", // 13 digits — invalid
	})
	req := httptest.NewRequest("POST", "/api/v1/billing-addresses/", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"a rejected siret must be a 400 client error, not 500; got body: %s", w.Body.String())
}

// TestBillingAddress_InvalidSiret_UpdateReturns400 drives PATCH through the real
// generic controller with a 13-digit siret and asserts 400 (today: hard-coded 404).
func TestBillingAddress_InvalidSiret_UpdateReturns400(t *testing.T) {
	_ = freshTestDB(t)
	gin.SetMode(gin.TestMode)
	setupBillingAddressEntity(t)
	withPaymentHooksRegistered(t)

	// Seed a valid address owned by the patching user (direct GORM write bypasses
	// hooks) so ownership passes and the request reaches the validation hook.
	addr := &models.BillingAddress{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:     "b2b-user",
		Line1:      "1 rue de Rivoli",
		City:       "Paris",
		PostalCode: "75001",
		Country:    "FR",
	}
	require.NoError(t, sharedTestDB.Create(addr).Error)

	gc := controller.NewGenericController(sharedTestDB, casdoor.Enforcer)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userId", "b2b-user"); c.Next() })
	router.PATCH("/api/v1/billing-addresses/:id", func(c *gin.Context) { gc.EditEntity(c) })

	body, _ := json.Marshal(map[string]any{"siret": "1234567890123"}) // 13 digits
	req := httptest.NewRequest("PATCH", "/api/v1/billing-addresses/"+addr.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code,
		"a rejected siret patch must be a 400 client error, not 404; got body: %s", w.Body.String())
}
