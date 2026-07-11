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
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/mocks"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

// ─────────────────────────────────────────────────────────────────────────────
// Payload tests (GREEN landed in commit 7fc611b). These exercise the real
// controller GetOrgUsageExport through NewTerminalController — the deferred pins
// from this file's header, now that the method compiles. The controller-level
// IsUserOrgManagerOrAdmin guard 403s a non-manager BEFORE the payload runs, so
// every payload caller is seeded as a manager/owner of the target org.
// ─────────────────────────────────────────────────────────────────────────────

// mountOrgUsageExport wires the REAL controller behind a mock-auth middleware
// that stamps userId + userRoles, mirroring makeOrgUsageRequest in
// orgTerminalUsage_test.go. No Layer2Enforcement here: authorization is pinned
// by the sentinel tests above; these payload tests drive the handler directly.
func mountOrgUsageExport(db *gorm.DB, userID string, roles []string) *gin.Engine {
	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Set("userRoles", roles)
		c.Next()
	})
	router.GET("/organizations/:id/usage-export", ctrl.GetOrgUsageExport)
	return router
}

// seedExportSession inserts a terminal attributed to orgID for userID with a
// forced created_at (GORM would otherwise stamp now()). expires_at drives the
// duration; created_at drives both ordering and the period filter. UpdateColumn
// bypasses hooks and leaves updated_at untouched so the row's timestamps are
// exactly what the export must read back.
func seedExportSession(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID, size string, cpu, mem int, createdAt, expiresAt time.Time) *models.Terminal {
	t.Helper()
	key, err := createTestUserKey(db, userID)
	require.NoError(t, err)

	term := &models.Terminal{
		SessionID:         "export-" + uuid.New().String(),
		UserID:            userID,
		State:             models.StateDeleted, // finished tombstone — must still be exported
		ExpiresAt:         expiresAt,
		InstanceType:      "test",
		MachineSize:       size,
		SizeCPU:           cpu,
		SizeMemoryMB:      mem,
		UserTerminalKeyID: key.ID,
		OrganizationID:    &orgID,
	}
	require.NoError(t, db.Create(term).Error)
	require.NoError(t, db.Model(&models.Terminal{}).
		Where("id = ?", term.ID).
		UpdateColumn("created_at", createdAt).Error)
	return term
}

// parseCSV decodes the response body into header + data records.
func parseCSV(t *testing.T, body []byte) (header []string, rows [][]string) {
	t.Helper()
	records, err := csv.NewReader(bytes.NewReader(body)).ReadAll()
	require.NoError(t, err, "response body must be valid CSV; got: %s", string(body))
	require.NotEmpty(t, records, "CSV must have at least a header row")
	return records[0], records[1:]
}

// expectedHeader is the exact column contract pinned in this file's header.
var expectedHeader = []string{
	"member_id", "member_email", "session_id", "machine_size",
	"cpu_millicores", "memory_mb", "started_at", "expires_at", "duration_hours",
}

// TestOrgUsageExport_ManagerGets200WithCSVRows is the payload pin: seed sessions
// for TWO orgs plus an out-of-period session, export org A over a window as its
// manager, and assert the CSV body contains EXACTLY org A's in-period sessions —
// org B's row and the out-of-period row EXCLUDED (the critical assert), ordered
// created_at ASC, with computed durations, RFC3339 timestamps, and member_email
// resolved through the swapped Casdoor seam.
//
// Red-if-gutted: dropping the repository's `organization_id = ?` filter would
// leak org B's row (len becomes 3, the exclusion assert fails); dropping the
// created_at range would leak the out-of-period row (same failure).
func TestOrgUsageExport_ManagerGets200WithCSVRows(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	orgA := createTestOrgForHistory(t, db, "export-rows-ownerA")
	createTestOrgMember(t, db, orgA.ID, "export-rows-manager", orgModels.OrgRoleManager)
	orgB := createTestOrgForHistory(t, db, "export-rows-ownerB")

	// Window: 2026-03-01 .. 2026-03-31 (to inclusive of the whole day).
	utc := time.UTC
	aliceCreated := time.Date(2026, 3, 5, 10, 0, 0, 0, utc)
	aliceExpires := aliceCreated.Add(1 * time.Hour) // duration 1.00
	bobCreated := time.Date(2026, 3, 10, 9, 0, 0, 0, utc)
	bobExpires := bobCreated.Add(2*time.Hour + 30*time.Minute) // duration 2.50

	alice := seedExportSession(t, db, orgA.ID, "alice", "S", 1000, 1024, aliceCreated, aliceExpires)
	bob := seedExportSession(t, db, orgA.ID, "bob", "M", 2000, 2048, bobCreated, bobExpires)
	// Out-of-period session for org A (created in February) — must be EXCLUDED.
	seedExportSession(t, db, orgA.ID, "alice", "S", 1000, 1024,
		time.Date(2026, 2, 1, 12, 0, 0, 0, utc), time.Date(2026, 2, 1, 13, 0, 0, 0, utc))
	// In-period session belonging to org B — must be EXCLUDED (cross-org leak).
	orgBSession := seedExportSession(t, db, orgB.ID, "carol", "L", 4000, 4096,
		time.Date(2026, 3, 15, 8, 0, 0, 0, utc), time.Date(2026, 3, 15, 12, 0, 0, 0, utc))

	// Swap the Casdoor seam so member_email is deterministic.
	original := services.LookupCasdoorUserForOrgUsage
	services.LookupCasdoorUserForOrgUsage = func(id string) (*casdoorsdk.User, error) {
		switch id {
		case "alice":
			return &casdoorsdk.User{Id: "alice", Email: "alice@corp.example"}, nil
		case "bob":
			return &casdoorsdk.User{Id: "bob", Email: "bob@corp.example"}, nil
		}
		return nil, fmt.Errorf("user not found")
	}
	t.Cleanup(func() { services.LookupCasdoorUserForOrgUsage = original })

	router := mountOrgUsageExport(db, "export-rows-manager", []string{"member"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET",
		"/organizations/"+orgA.ID.String()+"/usage-export?from=2026-03-01&to=2026-03-31", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "manager must get 200; body: %s", w.Body.String())

	header, rows := parseCSV(t, w.Body.Bytes())
	assert.Equal(t, expectedHeader, header, "CSV header must match the pinned column contract")

	require.Len(t, rows, 2,
		"export must contain exactly org A's two in-period sessions — the "+
			"out-of-period row and org B's row must be excluded; got %v", rows)

	sessionIDs := []string{rows[0][2], rows[1][2]}
	assert.NotContains(t, sessionIDs, orgBSession.SessionID,
		"another org's session must never leak into org A's export")

	// Ordering: created_at ASC → alice (03-05) before bob (03-10).
	assert.Equal(t, alice.SessionID, rows[0][2], "rows must be ordered created_at ASC")
	assert.Equal(t, bob.SessionID, rows[1][2], "rows must be ordered created_at ASC")

	// Row 0 (alice): member_id, email, size, cpu, mem, timestamps, duration.
	assert.Equal(t, "alice", rows[0][0], "member_id must be the session's user_id")
	assert.Equal(t, "alice@corp.example", rows[0][1], "member_email must come from the Casdoor seam")
	assert.Equal(t, "S", rows[0][3], "machine_size column")
	assert.Equal(t, "1000", rows[0][4], "cpu_millicores column")
	assert.Equal(t, "1024", rows[0][5], "memory_mb column")
	assert.Equal(t, "1.00", rows[0][8], "duration_hours = expires-created, 2 decimals (allocated window)")

	// started_at / expires_at are RFC3339 and equal the seeded instants
	// (compared with .Equal so a UTC/offset representation difference can't flake).
	gotStart, err := time.Parse(time.RFC3339, rows[0][6])
	require.NoError(t, err, "started_at must be RFC3339")
	assert.True(t, gotStart.Equal(aliceCreated), "started_at must equal created_at")
	gotExpires, err := time.Parse(time.RFC3339, rows[0][7])
	require.NoError(t, err, "expires_at must be RFC3339")
	assert.True(t, gotExpires.Equal(aliceExpires), "expires_at must equal the session's expiry")

	// Row 1 (bob): the 2.5h allocated window.
	assert.Equal(t, "bob", rows[1][0])
	assert.Equal(t, "bob@corp.example", rows[1][1])
	assert.Equal(t, "2.50", rows[1][8], "duration_hours must be 2.50 for a 2h30m window")
}

// TestOrgUsageExport_ResponseHeaders pins the download headers: text/csv content
// type and an attachment Content-Disposition naming org-usage-export_<from>_<to>.csv.
func TestOrgUsageExport_ResponseHeaders(t *testing.T) {
	db := setupTestDBWithOrgs(t)
	org := createTestOrgForHistory(t, db, "export-headers-owner")
	createTestOrgMember(t, db, org.ID, "export-headers-manager", orgModels.OrgRoleManager)

	router := mountOrgUsageExport(db, "export-headers-manager", []string{"member"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET",
		"/organizations/"+org.ID.String()+"/usage-export?from=2026-03-01&to=2026-03-31", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv",
		"Content-Type must mark the response as CSV")
	assert.Equal(t,
		`attachment; filename="org-usage-export_2026-03-01_2026-03-31.csv"`,
		w.Header().Get("Content-Disposition"),
		"Content-Disposition must offer a dated org-usage-export .csv download")
}

// TestOrgUsageExport_InvalidDateParams_400 is table-driven: with the caller
// already authorized (manager), a missing or unparseable from/to must yield 400.
// The guard 403s first, so the manager seeding is what lets these reach the date
// validation.
func TestOrgUsageExport_InvalidDateParams_400(t *testing.T) {
	db := setupTestDBWithOrgs(t)
	org := createTestOrgForHistory(t, db, "export-400-owner")
	createTestOrgMember(t, db, org.ID, "export-400-manager", orgModels.OrgRoleManager)
	router := mountOrgUsageExport(db, "export-400-manager", []string{"member"})

	cases := []struct {
		name  string
		query string
	}{
		{"missing both", ""},
		{"missing from", "?to=2026-03-31"},
		{"missing to", "?from=2026-03-01"},
		{"invalid from", "?from=03-2026-01&to=2026-03-31"},
		{"invalid to", "?from=2026-03-01&to=notadate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET",
				"/organizations/"+org.ID.String()+"/usage-export"+tc.query, nil)
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"missing/unparseable from/to must be rejected with 400 (query %q)", tc.query)
		})
	}
}

// TestOrgUsageExport_EmptyPeriod_HeaderOnly200 verifies that a window with no
// attributed sessions returns 200 with a header-only CSV (no data rows) rather
// than an error or an empty body.
func TestOrgUsageExport_EmptyPeriod_HeaderOnly200(t *testing.T) {
	db := setupTestDBWithOrgs(t)
	org := createTestOrgForHistory(t, db, "export-empty-owner")
	createTestOrgMember(t, db, org.ID, "export-empty-manager", orgModels.OrgRoleManager)

	// A session exists but OUTSIDE the requested window, so the export is empty.
	seedExportSession(t, db, org.ID, "ghost", "S", 1000, 1024,
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC))

	router := mountOrgUsageExport(db, "export-empty-manager", []string{"member"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET",
		"/organizations/"+org.ID.String()+"/usage-export?from=2026-03-01&to=2026-03-31", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	header, rows := parseCSV(t, w.Body.Bytes())
	assert.Equal(t, expectedHeader, header, "empty export must still carry the header row")
	assert.Empty(t, rows, "an empty window must produce a header-only CSV with no data rows")
}
