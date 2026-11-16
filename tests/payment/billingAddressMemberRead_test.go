// tests/payment/billingAddressMemberRead_test.go
//
// Issue #590: the billing-addresses page lists through the generic
// GET /billing-addresses, but the BillingAddress registration gives the Member
// role no GET (Layer 1 Casbin answers 403) and declares no "read" owner scope.
// A member must be able to list and fetch their OWN addresses, and only those.
package payment_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	controller "soli/formations/src/entityManagement/routes"
	registration "soli/formations/src/payment/entityRegistration"
)

func TestBillingAddressRegistration_Member_HasGet(t *testing.T) {
	service := ems.NewEntityRegistrationService()
	registration.RegisterBillingAddress(service)

	roles, ok := service.GetAllEntityRoles()["BillingAddress"]
	require.True(t, ok, "BillingAddress should be registered")
	memberMethods := roles.Roles[string(authModels.Member)]

	assert.Contains(t, memberMethods, http.MethodGet,
		"Member needs GET on billing-addresses to list their own addresses")
}

// billingAddressReadRouter drives the REAL generic list/get handlers over the
// REAL BillingAddress registration, as the given member.
func billingAddressReadRouter(t *testing.T, userID string) *gin.Engine {
	t.Helper()
	if _, ok := ems.GlobalEntityRegistrationService.GetEntityOps("BillingAddress"); !ok {
		registration.RegisterBillingAddress(ems.GlobalEntityRegistrationService)
		t.Cleanup(func() { ems.GlobalEntityRegistrationService.UnregisterEntity("BillingAddress") })
	}
	gin.SetMode(gin.TestMode)
	gc := controller.NewGenericController(sharedTestDB, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", []string{string(authModels.Member)})
		c.Next()
	})
	r.GET("/api/v1/billing-addresses/", func(c *gin.Context) { gc.GetEntities(c) })
	r.GET("/api/v1/billing-addresses/:id", func(c *gin.Context) { gc.GetEntity(c) })
	return r
}

func TestBillingAddressList_Member_SeesOnlyOwnAddresses(t *testing.T) {
	_ = freshTestDB(t)
	mine := createTestBillingAddress(t, "member-a")
	theirs := createTestBillingAddress(t, "member-b")
	r := billingAddressReadRouter(t, "member-a")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/billing-addresses/", nil))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Data []struct {
			ID     string `json:"id"`
			UserID string `json:"user_id"`
		} `json:"data"`
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := []string{}
	for _, d := range resp.Data {
		ids = append(ids, d.ID)
	}
	assert.Contains(t, ids, mine.ID.String(), "member must see their own address")
	assert.NotContains(t, ids, theirs.ID.String(), "member must not see another member's address")
	assert.Equal(t, int64(1), resp.Total)
}

func TestBillingAddressGet_Member_OtherMembersAddressNotFound(t *testing.T) {
	_ = freshTestDB(t)
	mine := createTestBillingAddress(t, "member-a")
	theirs := createTestBillingAddress(t, "member-b")
	r := billingAddressReadRouter(t, "member-a")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/billing-addresses/"+theirs.ID.String(), nil))
	assert.Equal(t, http.StatusNotFound, w.Code, "another member's address must be a 404; body=%s", w.Body.String())

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/billing-addresses/"+mine.ID.String(), nil))
	assert.Equal(t, http.StatusOK, w.Code, "own address must be readable; body=%s", w.Body.String())
}
