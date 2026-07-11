// tests/terminalTrainer/orgUsageExport_test.go
//
// RED-phase tests for issue #385: an org-scoped terminal usage export
//
//	GET /api/v1/organizations/:id/usage-export?from=YYYY-MM-DD&to=YYYY-MM-DD
//
// Organisations need « hours / terminals consumed by member X over period Z »
// as a CSV so they can re-invoice an OPCO. The endpoint is gated at Layer 2 by
// OrgRole with MinRole "manager", mirroring the existing sibling
// /organizations/:id/terminal-usage (declared in RegisterTerminalPermissions).
// Managers and owners of the org may export; plain members and managers of
// OTHER orgs must be rejected.
//
// ── The CSV contract (decided from what the Terminal row ACTUALLY records) ──
//
// Source of truth: src/terminalTrainer/models/terminal.go. Each Terminal row
// carries, per session:
//   - user_id                      → who ran it (the member)
//   - organization_id (*uuid)      → which org's context/budget it consumed
//   - machine_size                 → human-readable size tier (XS/S/M/L/XL)
//   - size_cpu (millicores)        → denormalised CPU footprint
//   - size_memory_mb               → denormalised RAM footprint
//   - created_at                   → when the session started
//   - expires_at                   → the session's scheduled end
//
// One row per session (the factual base; aggregation/rollups are left to the
// accountant's spreadsheet). Header + column order:
//
//	member_id,member_email,session_id,machine_size,cpu_millicores,memory_mb,started_at,expires_at,duration_hours
//
// Semantics pinned for the GREEN implementer:
//
//   - started_at   = Terminal.created_at (RFC3339).
//   - expires_at   = Terminal.expires_at (RFC3339).
//   - duration_hours = (expires_at - created_at) in hours, 2 decimals. This is
//     the *provisioned/allocated* window, NOT wall-clock-consumed time: the
//     Terminal model keeps no cumulative-runtime accumulator and no stop
//     timestamp (finished sessions become state='deleted' TOMBSTONES with
//     deleted_at left NULL — see terminalLifecycleService.go), so the
//     created_at→expires_at span is the only server-derivable factual bound.
//   - member_email = resolved from Casdoor via the existing
//     services.LookupCasdoorUserForOrgUsage seam (same enrichment the
//     terminal-usage dashboard uses). Left empty when the lookup fails —
//     member_id (always present) is the stable identifier.
//   - Scope: terminals.organization_id = :id (direct billing attribution: the
//     sessions that consumed THIS org's budget). This deliberately differs from
//     the live terminal-usage dashboard's organization_members join — a member
//     who has since left the org still consumed its budget during the period,
//     so the export attributes by the row's organization_id, not by current
//     membership. Sessions belonging to another org are EXCLUDED.
//   - Period filter: a session is included when created_at is in
//     [from 00:00:00, to+1day) — i.e. `from` inclusive, whole `to` day
//     inclusive. Both params are REQUIRED; there is no default period (an OPCO
//     export always names its billing window). Missing or unparseable from/to
//     → 400. No state filter: running, stopped, and deleted (tombstone)
//     sessions all count if their created_at falls in the window.
//   - Response headers: Content-Type "text/csv; charset=utf-8" and a
//     Content-Disposition "attachment; filename=..." naming a .csv file.
//   - Empty result (no sessions in window, or empty org): header-only CSV, 200.
//
// ── DROPPED columns (data does not exist — refused to fabricate) ──
//
//   - group / per-group breakdown: the Terminal row carries NO group dimension
//     (no group_id, no group join). A per-group column would be invented data.
//   - estimated_cost / price: there is no per-session cost model. Plans carry a
//     subscription price, not a per-session unit rate, so any monetary figure
//     would be fabricated. The org multiplies duration_hours × size externally.
//   - member_display_name: member_email + member_id already identify the member
//     for re-invoicing; the display name adds no billing signal (kept minimal).
//
// ── Test layering (why the payload tests are DEFERRED) ──
//
// The authorization contract is pinned here as compiling RED: the declaration
// test fails today (no RouteRegistry entry), and the member / other-org-manager
// 403 tests fail today (an undeclared route is passed THROUGH by
// Layer2Enforcement, so the caller wrongly reaches the sentinel with 200). A
// sentinel handler stands in for the real controller — it is only reached when
// enforcement authorises the caller.
//
// The CSV-body / duration / period-filter / content-type / 400-validation /
// empty-org payload assertions are intentionally NOT written here: they require
// the production controller method (GetOrgUsageExport), which does not exist
// yet. Referencing it would break compilation of the WHOLE terminalTrainer_tests
// package (identical constraint to tests/payment/orgInvoiceListing_test.go,
// whose rows-payload test was deferred the same way). The GREEN implementer must
// add that controller, wire the route in TerminalRoutes, and add the payload
// tests alongside it — the contract above is the spec they pin.
package terminalTrainer_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	terminalController "soli/formations/src/terminalTrainer/routes"
)

const orgUsageExportRoutePath = "/api/v1/organizations/:id/usage-export"

// setupOrgUsageExportRouter mirrors setupOrgInvoiceListingRouter
// (tests/payment/orgInvoiceListing_test.go): it loads the REAL terminal
// permission declarations, wires the REAL Layer2Enforcement with a
// GormMembershipChecker, and mounts GET /organizations/:id/usage-export behind a
// sentinel. Because the controller does not exist yet, the sentinel stands in
// for the real handler: it returns 200 with a marker body and is only reached
// when enforcement authorises the caller.
func setupOrgUsageExportRouter(t *testing.T, db *gorm.DB, userID string, roles []string) *gin.Engine {
	t.Helper()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	t.Cleanup(func() {
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
	})

	mockEnforcer := mocks.NewMockEnforcer()
	terminalController.RegisterTerminalPermissions(mockEnforcer)
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

	api.GET("/organizations/:id/usage-export", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"reached": true})
	})

	return r
}

// TestOrgUsageExport_Route_DeclaredForManagerOrgRole pins the authorization
// contract: RegisterTerminalPermissions must declare GET
// /organizations/:id/usage-export in the RouteRegistry as OrgRole / MinRole
// "manager", param "id". RED today — no declaration → Lookup returns not-found,
// and Layer2Enforcement would pass the route through ungated.
func TestOrgUsageExport_Route_DeclaredForManagerOrgRole(t *testing.T) {
	access.RouteRegistry.Reset()
	t.Cleanup(func() { access.RouteRegistry.Reset() })

	terminalController.RegisterTerminalPermissions(mocks.NewMockEnforcer())

	perm, found := access.RouteRegistry.Lookup("GET", orgUsageExportRoutePath)
	require.True(t, found,
		"RegisterTerminalPermissions must declare a RoutePermission for GET %s — "+
			"without it Layer2Enforcement silently passes the route through with no "+
			"org-role gate.", orgUsageExportRoutePath)

	assert.Equal(t, access.OrgRole, perm.Access.Type,
		"the usage export must be gated by OrgRole")
	assert.Equal(t, "manager", perm.Access.MinRole,
		"exporting an org's usage is a manager+ operation (mirrors "+
			"/organizations/:id/terminal-usage)")
	assert.Equal(t, "id", perm.Access.Param,
		"the org id must be resolved from the :id URL param")
}

// TestOrgUsageExport_ManagerAllowed is the positive control: an org manager must
// NOT be blocked by enforcement and must reach the handler (200). GREEN today
// (route ungated → pass-through → sentinel 200) and must stay GREEN after the
// declaration lands (manager >= manager → allowed). Guards against a future
// over-broad denial that wrongly rejects legitimate managers.
func TestOrgUsageExport_ManagerAllowed(t *testing.T) {
	db := setupTestDBWithOrgs(t)
	org := createTestOrgForHistory(t, db, "usageexport-owner-allowed")
	createTestOrgMember(t, db, org.ID, "usageexport-manager-allowed", orgModels.OrgRoleManager)

	router := setupOrgUsageExportRouter(t, db, "usageexport-manager-allowed", []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+org.ID.String()+"/usage-export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"an org manager must be allowed to reach the usage export endpoint")
}

// TestOrgUsageExport_MemberForbidden — a plain member of the org must be rejected
// with 403 by the Layer 2 gate, never reaching the handler. RED today: with no
// declaration the route is ungated, so the member falls through to the sentinel
// and gets 200 instead of 403.
func TestOrgUsageExport_MemberForbidden(t *testing.T) {
	db := setupTestDBWithOrgs(t)
	org := createTestOrgForHistory(t, db, "usageexport-owner-member403")
	createTestOrgMember(t, db, org.ID, "usageexport-member-member403", orgModels.OrgRoleMember)

	router := setupOrgUsageExportRouter(t, db, "usageexport-member-member403", []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+org.ID.String()+"/usage-export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a plain org member must NOT export the org's usage (manager+ only). "+
			"Today the route is undeclared so enforcement passes it through — the "+
			"missing RouteRegistry entry is the bug this pins.")
}

// TestOrgUsageExport_OtherOrgManagerForbidden — a manager of a DIFFERENT org has
// no membership in the target org and must be rejected with 403. RED today
// (ungated route → sentinel 200). Guards against the org id being ignored so
// that any manager could export any org's usage.
func TestOrgUsageExport_OtherOrgManagerForbidden(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	// Target org whose usage is requested; the requester is NOT a member of it.
	targetOrg := createTestOrgForHistory(t, db, "usageexport-target-owner")
	createTestOrgMember(t, db, targetOrg.ID, "usageexport-target-member", orgModels.OrgRoleMember)

	// A separate org where the requester IS a manager — irrelevant to the target.
	otherOrg := createTestOrgForHistory(t, db, "usageexport-other-owner")
	createTestOrgMember(t, db, otherOrg.ID, "usageexport-other-manager", orgModels.OrgRoleManager)

	router := setupOrgUsageExportRouter(t, db, "usageexport-other-manager", []string{"member"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/organizations/"+targetOrg.ID.String()+"/usage-export", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"a manager of another org must NOT export the target org's usage — "+
			"enforcement must scope to the :id org, not the caller's role elsewhere")
}
