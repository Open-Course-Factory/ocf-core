// tests/payment/orgInvoiceListing_test.go
//
// RED-phase tests for issue #384: a new org-scoped invoice listing endpoint
//
//     GET /api/v1/organizations/:id/invoices
//
// gated at Layer 2 by OrgRole with MinRole "manager" (mirrors the existing
// /organizations/:id/subscribe declaration in the payment module). Org managers
// (and owners) may list their organization's invoices; plain members and
// managers of OTHER orgs must be rejected.
//
// Two layers are pinned:
//
//   - Authorization contract: RegisterPaymentPermissions must declare the route
//     in the RouteRegistry with OrgRole / MinRole manager. A missing declaration
//     means Layer2Enforcement silently passes the route through (no gate).
//
//   - Enforcement behavior: driven through the REAL Layer2Enforcement middleware
//     with the REAL RegisterPaymentPermissions declaration and a
//     GormMembershipChecker, mirroring setupOrgSubscriptionRouter. The leaf
//     handler is a sentinel: forbidden requests must be stopped by the
//     middleware BEFORE reaching it, so the sentinel is only reached when the
//     caller is authorized.
//
// The rows-payload assertion (a manager receives the seeded org invoices in the
// body) is intentionally NOT covered here: it requires the production controller
// method, which does not exist yet, and referencing it would break compilation
// of the whole payment_tests package. The GREEN implementer must add that
// controller and a payload test alongside it. The manager path is pinned here at
// the authorization layer (declaration contract) plus a positive control that
// the manager is not wrongly blocked.
package payment_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	paymentController "soli/formations/src/payment/routes"

	"github.com/google/uuid"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const orgInvoicesRoutePath = "/api/v1/organizations/:id/invoices"

// setupOrgInvoiceListingRouter mirrors setupOrgSubscriptionRouter: it loads the
// REAL payment permission declarations, wires the REAL Layer2Enforcement with a
// GormMembershipChecker, and mounts GET /organizations/:id/invoices behind a
// sentinel handler. Because the controller does not exist yet, the sentinel
// stands in for the real handler: it returns 200 with a marker body and is only
// reached when enforcement authorizes the caller.
func setupOrgInvoiceListingRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	mockEnforcer := mocks.NewMockEnforcer()
	paymentController.RegisterPaymentPermissions(mockEnforcer)
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	api.Use(access.Layer2Enforcement())

	api.GET("/organizations/:id/invoices", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"reached": true})
	})

	return r
}

// TestOrgInvoices_ListRoute_DeclaredForManagerOrgRole pins the authorization
// contract: RegisterPaymentPermissions must declare GET
// /organizations/:id/invoices in the RouteRegistry as OrgRole / MinRole
// "manager", param "id". RED today (no declaration → Lookup returns not-found,
// and Layer2Enforcement would pass the route through ungated).
func TestOrgInvoices_ListRoute_DeclaredForManagerOrgRole(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	paymentController.RegisterPaymentPermissions(mocks.NewMockEnforcer())

	perm, found := access.RouteRegistry.Lookup("GET", orgInvoicesRoutePath)
	require.True(t, found,
		"RegisterPaymentPermissions must declare a RoutePermission for GET %s — "+
			"without it Layer2Enforcement silently passes the route through with no "+
			"org-role gate.", orgInvoicesRoutePath)

	assert.Equal(t, access.OrgRole, perm.Access.Type,
		"org invoice listing must be gated by OrgRole")
	assert.Equal(t, "manager", perm.Access.MinRole,
		"listing an org's invoices is a manager+ operation (mirrors "+
			"/organizations/:id/subscribe)")
	assert.Equal(t, "id", perm.Access.Param,
		"the org id must be resolved from the :id URL param")
}

// TestOrgInvoices_ListByOrg_ManagerAllowed is the positive control: an org
// manager must NOT be blocked by enforcement and must reach the handler (200).
// GREEN today (route ungated → pass-through → sentinel 200) and must stay GREEN
// after the declaration lands (manager >= manager → allowed). Guards against a
// future over-broad denial that wrongly rejects legitimate managers.
func TestOrgInvoices_ListByOrg_ManagerAllowed(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "orginv-owner-allowed"
	managerID := "orginv-manager-allowed"
	org := seedSharedTeamOrg(t, db, ownerID, managerID, orgModels.OrgRoleManager)

	router := setupOrgInvoiceListingRouter(t, db, managerID, []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+org.ID.String()+"/invoices", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"an org manager must be allowed to reach the org invoice listing endpoint")
}

// TestOrgInvoices_ListByOrg_MemberForbidden — a plain member of the org must be
// rejected with 403 by the Layer 2 gate, never reaching the handler. RED today:
// with no declaration the route is ungated, so the member falls through to the
// sentinel and gets 200 instead of 403.
func TestOrgInvoices_ListByOrg_MemberForbidden(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "orginv-owner-member403"
	memberID := "orginv-member-member403"
	org := seedSharedTeamOrg(t, db, ownerID, memberID, orgModels.OrgRoleMember)

	router := setupOrgInvoiceListingRouter(t, db, memberID, []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+org.ID.String()+"/invoices", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a plain org member must NOT list the org's invoices (manager+ only). "+
			"Today the route is undeclared so enforcement passes it through — the "+
			"missing RouteRegistry entry is the bug this pins.")
}

// TestOrgInvoices_ListByOrg_OtherOrgManagerForbidden — a manager of a DIFFERENT
// org has no membership in the target org and must be rejected with 403. RED
// today (ungated route → sentinel 200). Guards against the org id being ignored
// so that any manager could read any org's invoices.
func TestOrgInvoices_ListByOrg_OtherOrgManagerForbidden(t *testing.T) {
	db := freshTestDB(t)

	// Target org the invoices belong to; the requester is NOT a member of it.
	targetOrg := seedSharedTeamOrg(t, db, "orginv-target-owner", "orginv-target-member", orgModels.OrgRoleMember)

	// A separate org where the requester IS a manager — irrelevant to the target.
	otherManagerID := "orginv-other-org-manager"
	seedSharedTeamOrg(t, db, "orginv-other-owner", otherManagerID, orgModels.OrgRoleManager)

	router := setupOrgInvoiceListingRouter(t, db, otherManagerID, []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+targetOrg.ID.String()+"/invoices", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a manager of another org must NOT list the target org's invoices — "+
			"enforcement must scope to the :id org, not the caller's role elsewhere")
}

// setupOrgInvoiceListingRouterWithController is the same real Layer-2 harness as
// setupOrgInvoiceListingRouter but mounts the REAL invoice controller instead of
// a sentinel, so the payload (which invoices are returned, in what order) is
// exercised end-to-end through GetOrganizationInvoices.
func setupOrgInvoiceListingRouterWithController(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	mockEnforcer := mocks.NewMockEnforcer()
	paymentController.RegisterPaymentPermissions(mockEnforcer)
	access.RegisterBuiltinEnforcers(nil, access.NewGormMembershipChecker(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	api.Use(access.Layer2Enforcement())

	controller := paymentController.NewInvoiceController(db)
	api.GET("/organizations/:id/invoices", controller.GetOrganizationInvoices)

	return r
}

// seedOrgInvoice inserts an organization-scoped invoice (UserID empty,
// UserSubscriptionID NULL, OrganizationID set). invoiceDate drives the
// newest-first ordering the endpoint promises.
func seedOrgInvoice(t *testing.T, db *gorm.DB, orgID uuid.UUID, stripeID, number string, invoiceDate time.Time) {
	t.Helper()
	orgSubID := uuid.New()
	require.NoError(t, db.Create(&models.Invoice{
		OrganizationID:             &orgID,
		OrganizationSubscriptionID: &orgSubID,
		StripeInvoiceID:            stripeID,
		Amount:                     1200,
		Currency:                   "eur",
		Status:                     "paid",
		InvoiceNumber:              number,
		InvoiceDate:                invoiceDate,
	}).Error)
}

// TestOrgInvoices_ListByOrg_ManagerGets200WithRows is the post-implementation
// payload pin (the RED I deferred until the controller existed). It seeds
// invoices for TWO orgs plus a user-scoped invoice, lists org A as its manager
// through the real Layer-2 harness + real controller, and asserts the body
// contains EXACTLY org A's invoices — org B's invoice and the user-scoped one
// EXCLUDED (the critical assert), each carrying organization_id in the DTO,
// newest invoice_date first (mirrors GetOrganizationInvoices' invoice_date DESC).
//
// Red-if-gutted: dropping the repository's `WHERE organization_id = ?` filter
// makes the query return all four invoices, so len==2, the org-B/user exclusion,
// and the per-row organization_id assertions all fail.
func TestOrgInvoices_ListByOrg_ManagerGets200WithRows(t *testing.T) {
	db := freshTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Invoice{}))

	managerID := "orginv-rows-manager"
	orgA := seedSharedTeamOrg(t, db, "orginv-rows-ownerA", managerID, orgModels.OrgRoleManager)
	orgB := seedSharedTeamOrg(t, db, "orginv-rows-ownerB", "orginv-rows-memberB", orgModels.OrgRoleMember)

	now := time.Now()
	orgAOlderStripeID := "in_orgA_older_" + uuid.NewString()
	orgANewerStripeID := "in_orgA_newer_" + uuid.NewString()
	orgBStripeID := "in_orgB_" + uuid.NewString()

	// Two invoices for org A with distinct dates (newer / older) to pin ordering.
	seedOrgInvoice(t, db, orgA.ID, orgAOlderStripeID, "INV-A-OLD", now.Add(-48*time.Hour))
	seedOrgInvoice(t, db, orgA.ID, orgANewerStripeID, "INV-A-NEW", now.Add(-1*time.Hour))
	// One invoice for org B — must never leak into org A's listing.
	seedOrgInvoice(t, db, orgB.ID, orgBStripeID, "INV-B", now.Add(-12*time.Hour))
	// One user-scoped invoice (no org) — must never appear either.
	require.NoError(t, db.Create(&models.Invoice{
		UserID: "orginv-rows-user", UserSubscriptionID: uuidPtr(uuid.New()),
		StripeInvoiceID: "in_user_" + uuid.NewString(), Amount: 999, Currency: "eur",
		Status: "paid", InvoiceNumber: "INV-USER", InvoiceDate: now.Add(-2 * time.Hour),
	}).Error)

	router := setupOrgInvoiceListingRouterWithController(t, db, managerID, []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+orgA.ID.String()+"/invoices", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "org manager must get 200; body: %s", w.Body.String())

	type invoiceDTO struct {
		OrganizationID  *string   `json:"organization_id"`
		UserID          string    `json:"user_id"`
		StripeInvoiceID string    `json:"stripe_invoice_id"`
		InvoiceDate     time.Time `json:"invoice_date"`
	}
	var body []invoiceDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	returnedIDs := make([]string, 0, len(body))
	for _, inv := range body {
		returnedIDs = append(returnedIDs, inv.StripeInvoiceID)
	}

	// Critical assert: exactly org A's two invoices, org B's and the user's excluded.
	require.Len(t, body, 2,
		"the listing must return exactly org A's invoices (2), not org B's or the "+
			"user-scoped one — got %v", returnedIDs)
	assert.ElementsMatch(t, []string{orgANewerStripeID, orgAOlderStripeID}, returnedIDs,
		"only org A's invoices may be listed")
	assert.NotContains(t, returnedIDs, orgBStripeID,
		"another org's invoice must never leak into org A's listing")

	for _, inv := range body {
		require.NotNil(t, inv.OrganizationID,
			"each listed org invoice must expose organization_id in the DTO")
		assert.Equal(t, orgA.ID.String(), *inv.OrganizationID,
			"listed invoices must be scoped to the requested org")
		assert.Empty(t, inv.UserID, "an org invoice carries no user_id")
	}

	// Ordering: newest invoice_date first.
	assert.Equal(t, orgANewerStripeID, body[0].StripeInvoiceID,
		"invoices must be ordered newest invoice_date first (invoice_date DESC)")
	assert.True(t, !body[0].InvoiceDate.Before(body[1].InvoiceDate),
		"row 0 must not be older than row 1")
}
