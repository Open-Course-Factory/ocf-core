// tests/payment/orgRolePlanListing_test.go
//
// RED-phase tests for issue #386 (frontend finding I2): the org role→plan
// entitlement listing leaks across organizations.
//
// Today the only listing surface is the generic entity route
//
//	GET /api/v1/organization-role-plans
//
// served by the generic entity controller. It is Casbin-gated to platform
// Administrators only (the registration grants Member "()" — no methods), and
// the generic controller applies NO per-organization scoping: an administrator
// receives EVERY organization's mappings. The frontend admin panel fetches that
// full list and filters by organization_id client-side (see
// ocf-front stores/organizationRolePlans.ts loadOrganizationRolePlans). There is
// no server-side, org-scoped surface an org MANAGER could use to read only their
// own organization's mappings without seeing everyone else's.
//
// Chosen contract (mirrors the freshly merged org invoice listing, !285):
// a sibling, org-scoped route
//
//	GET /api/v1/organizations/:id/role-plans
//
// gated at Layer 2 by OrgRole with MinRole "manager" (param "id"). Managers and
// owners of THAT org — plus platform administrators via admin bypass — may list
// the org's role→plan mappings; plain members and managers of OTHER orgs are
// rejected. This is the only clean fit for Layer 2 OrgRole enforcement, which
// resolves the org from a URL :id param (the flat /organization-role-plans route
// has no such param, so it cannot be gated by org role). The flat route stays
// platform-admin-only and unchanged.
//
// Two layers are pinned, exactly as orgInvoiceListing_test.go does:
//
//   - Authorization contract: RegisterPaymentPermissions must declare the route
//     in the RouteRegistry with OrgRole / MinRole manager / Param "id". A missing
//     declaration means Layer2Enforcement silently passes the route through with
//     no org-role gate.
//
//   - Enforcement behavior: driven through the REAL Layer2Enforcement middleware
//     with the REAL RegisterPaymentPermissions declaration and a
//     GormMembershipChecker, mirroring setupOrgInvoiceListingRouter. The leaf
//     handler is a sentinel: forbidden requests must be stopped by the middleware
//     BEFORE reaching it, so the sentinel is only reached when the caller is
//     authorized.
//
// The rows-payload assertion (a manager receives ONLY their org's mappings, with
// another org's rows EXCLUDED) is intentionally NOT covered here: it requires the
// production list-by-org repository query + controller, which do not exist yet,
// and referencing them would break compilation of the whole payment_tests
// package. The GREEN implementer MUST add that controller and, alongside it, a
// payload test named TestOrgRolePlans_ListByOrg_ManagerGetsOnlyOwnOrgRows that
// seeds role→plan mappings for TWO orgs, lists org A as its manager through the
// real Layer-2 harness + real controller, and asserts the body contains EXACTLY
// org A's mappings — org B's row EXCLUDED (the critical assert). That test must
// be red-if-gutted: dropping the `WHERE organization_id = ?` filter makes it
// return both orgs' rows and fail. The manager path is pinned here at the
// authorization layer (declaration contract) plus a positive control that the
// manager is not wrongly blocked.
package payment_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"
	paymentController "soli/formations/src/payment/routes"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const orgRolePlansRoutePath = "/api/v1/organizations/:id/role-plans"

// setupOrgRolePlanListingRouter mirrors setupOrgInvoiceListingRouter: it loads
// the REAL payment permission declarations, wires the REAL Layer2Enforcement
// with a GormMembershipChecker, and mounts GET /organizations/:id/role-plans
// behind a sentinel handler. Because the org-scoped controller does not exist
// yet, the sentinel stands in for the real handler: it returns 200 with a marker
// body and is only reached when enforcement authorizes the caller.
func setupOrgRolePlanListingRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
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

	api.GET("/organizations/:id/role-plans", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"reached": true})
	})

	return r
}

// TestOrgRolePlans_ListRoute_DeclaredForManagerOrgRole pins the authorization
// contract: RegisterPaymentPermissions must declare GET
// /organizations/:id/role-plans in the RouteRegistry as OrgRole / MinRole
// "manager", param "id". RED today (no declaration → Lookup returns not-found,
// and Layer2Enforcement would pass the route through ungated).
func TestOrgRolePlans_ListRoute_DeclaredForManagerOrgRole(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	paymentController.RegisterPaymentPermissions(mocks.NewMockEnforcer())

	perm, found := access.RouteRegistry.Lookup("GET", orgRolePlansRoutePath)
	require.True(t, found,
		"RegisterPaymentPermissions must declare a RoutePermission for GET %s — "+
			"without it Layer2Enforcement silently passes the route through with no "+
			"org-role gate, so any authenticated user could read the org's mappings.",
		orgRolePlansRoutePath)

	assert.Equal(t, access.OrgRole, perm.Access.Type,
		"org role-plan listing must be gated by OrgRole")
	assert.Equal(t, "manager", perm.Access.MinRole,
		"listing an org's role→plan mappings is a manager+ operation (mirrors "+
			"/organizations/:id/invoices)")
	assert.Equal(t, "id", perm.Access.Param,
		"the org id must be resolved from the :id URL param")
}

// TestOrgRolePlans_ListByOrg_ManagerAllowed is the positive control: an org
// manager must NOT be blocked by enforcement and must reach the handler (200).
// GREEN today (route ungated → pass-through → sentinel 200) and must stay GREEN
// after the declaration lands (manager >= manager → allowed). Guards against a
// future over-broad denial that wrongly rejects legitimate managers.
func TestOrgRolePlans_ListByOrg_ManagerAllowed(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "orgrp-owner-allowed"
	managerID := "orgrp-manager-allowed"
	org := seedSharedTeamOrg(t, db, ownerID, managerID, orgModels.OrgRoleManager)

	router := setupOrgRolePlanListingRouter(t, db, managerID, []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+org.ID.String()+"/role-plans", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"an org manager must be allowed to reach the org role-plan listing endpoint")
}

// TestOrgRolePlans_ListByOrg_MemberForbidden — a plain member of the org must be
// rejected with 403 by the Layer 2 gate, never reaching the handler. RED today:
// with no declaration the route is ungated, so the member falls through to the
// sentinel and gets 200 instead of 403.
func TestOrgRolePlans_ListByOrg_MemberForbidden(t *testing.T) {
	db := freshTestDB(t)
	ownerID := "orgrp-owner-member403"
	memberID := "orgrp-member-member403"
	org := seedSharedTeamOrg(t, db, ownerID, memberID, orgModels.OrgRoleMember)

	router := setupOrgRolePlanListingRouter(t, db, memberID, []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+org.ID.String()+"/role-plans", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a plain org member must NOT list the org's role→plan mappings (manager+ "+
			"only). Today the route is undeclared so enforcement passes it through — "+
			"the missing RouteRegistry entry is the bug this pins.")
}

// TestOrgRolePlans_ListByOrg_OtherOrgManagerForbidden — a manager of a DIFFERENT
// org has no membership in the target org and must be rejected with 403. RED
// today (ungated route → sentinel 200). This is the cross-org leak from finding
// I2: enforcement must scope to the :id org, not the caller's role elsewhere, so
// an admin of org B can never read org A's mappings.
func TestOrgRolePlans_ListByOrg_OtherOrgManagerForbidden(t *testing.T) {
	db := freshTestDB(t)

	// Target org the mappings belong to; the requester is NOT a member of it.
	targetOrg := seedSharedTeamOrg(t, db, "orgrp-target-owner", "orgrp-target-member", orgModels.OrgRoleMember)

	// A separate org where the requester IS a manager — irrelevant to the target.
	otherManagerID := "orgrp-other-org-manager"
	seedSharedTeamOrg(t, db, "orgrp-other-owner", otherManagerID, orgModels.OrgRoleManager)

	router := setupOrgRolePlanListingRouter(t, db, otherManagerID, []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+targetOrg.ID.String()+"/role-plans", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a manager of another org must NOT list the target org's role→plan "+
			"mappings — enforcement must scope to the :id org, not the caller's role "+
			"elsewhere. This is the cross-org info exposure from finding I2.")
}

// TestOrgRolePlans_ListByOrg_AdminBypassAllowed — a platform administrator is not
// a member of the target org yet must still reach the handler via the Layer 2
// admin bypass (administrators manage every org's mappings, as the admin panel
// does today). GREEN now (route ungated → pass-through) and must stay GREEN after
// the declaration lands (admin bypass). Guards against the declaration wrongly
// locking administrators out of the org-scoped route.
func TestOrgRolePlans_ListByOrg_AdminBypassAllowed(t *testing.T) {
	db := freshTestDB(t)
	targetOrg := seedSharedTeamOrg(t, db, "orgrp-admin-owner", "orgrp-admin-member", orgModels.OrgRoleMember)

	adminID := "orgrp-platform-admin"
	router := setupOrgRolePlanListingRouter(t, db, adminID, []string{"administrator"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+targetOrg.ID.String()+"/role-plans", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"a platform administrator must reach the org role-plan listing for any org "+
			"via admin bypass, even without membership")
}
