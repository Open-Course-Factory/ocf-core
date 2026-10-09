package authorization_tests

// Role access matrix: who may do what in an organisation, pinned in one table.
//
// Every cell is one HTTP request through the production chain — Layer 2
// (Layer2Enforcement, the global middleware), Layer 1 (the real
// AuthManagement against a real Casbin enforcer loaded with the production
// policies), then the real handler or generic entity controller with the
// production hooks, on a real database. Only the JWT signature check is
// stubbed: the bearer token IS the user id. Two routes whose handlers need
// Stripe or tt-backend before they authorise anything are mounted behind a
// sentinel instead, so they test the gates only (see gateOnlyRoutes).
//
// The expectations are the product spec — help page "Roles and permissions",
// CanManageScenario / CanRunScenario / CanAssignScenario / CanTeachInOrg,
// CanUserManageGroup, RoleMinimumForClassrooms — never the code. Where the code
// disagrees, the cell carries a KNOWN GAP: it is skipped so the suite stays
// green, and it FAILS the day the gap closes, so the marker cannot outlive the
// bug.
//
// Every cell runs on a freshly seeded world (seedMatrixWorld), so a write in
// one cell never leaks into the next.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	auditModels "soli/formations/src/audit/models"
	authController "soli/formations/src/auth"
	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	configModels "soli/formations/src/configuration/models"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/entityManagementService"
	"soli/formations/src/entityManagement/hooks"
	entityManagementModels "soli/formations/src/entityManagement/models"
	swaggerGenerator "soli/formations/src/entityManagement/swagger"
	groupRegistration "soli/formations/src/groups/entityRegistration"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"
	orgRegistration "soli/formations/src/organizations/entityRegistration"
	orgHooks "soli/formations/src/organizations/hooks"
	orgModels "soli/formations/src/organizations/models"
	organizationRoutes "soli/formations/src/organizations/routes"
	orgServices "soli/formations/src/organizations/services"
	paymentModels "soli/formations/src/payment/models"
	paymentController "soli/formations/src/payment/routes"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	scenarioModels "soli/formations/src/scenarios/models"
	scenarioController "soli/formations/src/scenarios/routes"
	terminalModels "soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/utils"
)

// The actors. Everyone but the platform admin holds the platform role member.
const (
	mxOrgOwner   = "mx-org-owner"
	mxOrgManager = "mx-org-manager"
	mxTeacherA   = "mx-teacher-a"  // org teacher, owns classA, wrote scenA
	mxTeacherB   = "mx-teacher-b"  // org teacher, owns classB
	mxCoTrainer  = "mx-co-trainer" // org member, manager of classA
	mxLearner    = "mx-learner"    // org member, member of classA
	mxOutsider   = "mx-outsider"   // no membership anywhere
	mxAdmin      = "mx-platform-admin"

	mxPeer          = "mx-peer"     // a second learner of classA
	mxNewcomer      = "mx-newcomer" // org member enrolled in nothing yet
	mxOtherOwner    = "mx-other-org-owner"
	mxCatalogAuthor = "mx-catalog-author" // platform author of scenPub
)

var matrixActors = []string{mxOrgOwner, mxOrgManager, mxTeacherA, mxTeacherB, mxCoTrainer, mxLearner, mxOutsider, mxAdmin}

// matrixWorld holds the ids of one seeded world.
type matrixWorld struct {
	ecole, otherOrg                  uuid.UUID
	classA, classB                   uuid.UUID
	scenA, scenM, scenPub, scenOther uuid.UUID
	stepA, questionA                 uuid.UUID
	learnerSession, peerSession      uuid.UUID
	learnerOrgMembership             uuid.UUID
	learnerClassMembership           uuid.UUID
}

type matrixRequest struct {
	method, path string
	body         any
}

// accessRow is one line of the matrix: an action, and the actors the spec lets
// do it. Every other actor in `judged` (all of matrixActors by default) must be
// refused.
type accessRow struct {
	action  string
	request func(w matrixWorld) matrixRequest
	allowed []string
	judged  []string
	// verdict reads the response; nil means defaultVerdict.
	verdict func(w matrixWorld, code int, body string) (allowed, decided bool)
	gaps    map[string]string
}

// defaultVerdict: a 2xx is "allowed", 401/403/404 is "refused" (handlers hide
// scenarios and sessions the caller may not see behind a 404). A before-hook
// that refuses with a plain error instead of a permission error surfaces as a
// 500 "Hook execution failed"; it is still a refusal. Anything else means the
// request never reached a decision and fails the cell loudly.
func defaultVerdict(_ matrixWorld, code int, body string) (bool, bool) {
	switch {
	case code >= 200 && code < 300:
		return true, true
	case code == http.StatusUnauthorized, code == http.StatusForbidden, code == http.StatusNotFound:
		return false, true
	case code == http.StatusInternalServerError && strings.Contains(body, "Hook execution failed"):
		return false, true
	}
	return false, false
}

// pastAuthorization is the verdict for handlers that call tt-backend once the
// caller is authorised: any answer but a refusal means the gates let it through.
func pastAuthorization(_ matrixWorld, code int, _ string) (bool, bool) {
	refused := code == http.StatusUnauthorized || code == http.StatusForbidden || code == http.StatusNotFound
	return !refused, true
}

// listsClassA is the verdict for GET /teacher/groups: the class is "managed"
// when it appears in the caller's own list.
func listsClassA(w matrixWorld, code int, body string) (bool, bool) {
	if code != http.StatusOK {
		return false, code == http.StatusForbidden
	}
	return strings.Contains(body, w.classA.String()), true
}

func allow(actors ...string) []string { return actors }

func except(actors []string, excluded string) []string {
	var kept []string
	for _, a := range actors {
		if a != excluded {
			kept = append(kept, a)
		}
	}
	return kept
}

// Every place the code disagrees with the spec gets a gapXxx constant naming
// its root cause, set on the cells it breaks with gap(gapXxx, actors...).
// There are none today.

func gap(reason string, actors ...string) map[string]string {
	gaps := make(map[string]string, len(actors))
	for _, a := range actors {
		gaps[a] = reason
	}
	return gaps
}

func get(path func(w matrixWorld) string) func(w matrixWorld) matrixRequest {
	return func(w matrixWorld) matrixRequest { return matrixRequest{http.MethodGet, path(w), nil} }
}

func send(method string, path func(w matrixWorld) string, body func(w matrixWorld) any) func(w matrixWorld) matrixRequest {
	return func(w matrixWorld) matrixRequest {
		var b any
		if body != nil {
			b = body(w)
		}
		return matrixRequest{method, path(w), b}
	}
}

func entityPath(entity string, id func(w matrixWorld) uuid.UUID, suffix ...string) func(w matrixWorld) string {
	return func(w matrixWorld) string {
		p := "/api/v1" + ems.ResourceBasePath(entity)
		if id != nil {
			p += "/" + id(w).String()
		}
		return p + strings.Join(suffix, "")
	}
}

func route(format string, ids ...func(w matrixWorld) uuid.UUID) func(w matrixWorld) string {
	return func(w matrixWorld) string {
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id(w).String()
		}
		return fmt.Sprintf(format, args...)
	}
}

var (
	ecole       = func(w matrixWorld) uuid.UUID { return w.ecole }
	classA      = func(w matrixWorld) uuid.UUID { return w.classA }
	scenA       = func(w matrixWorld) uuid.UUID { return w.scenA }
	scenM       = func(w matrixWorld) uuid.UUID { return w.scenM }
	scenPub     = func(w matrixWorld) uuid.UUID { return w.scenPub }
	scenOther   = func(w matrixWorld) uuid.UUID { return w.scenOther }
	stepA       = func(w matrixWorld) uuid.UUID { return w.stepA }
	questionA   = func(w matrixWorld) uuid.UUID { return w.questionA }
	learnerRun  = func(w matrixWorld) uuid.UUID { return w.learnerSession }
	peerRun     = func(w matrixWorld) uuid.UUID { return w.peerSession }
	learnerOrgM = func(w matrixWorld) uuid.UUID { return w.learnerOrgMembership }
	learnerGrpM = func(w matrixWorld) uuid.UUID { return w.learnerClassMembership }
)

func assignTo(scenario func(w matrixWorld) uuid.UUID, class func(w matrixWorld) uuid.UUID) func(w matrixWorld) any {
	return func(w matrixWorld) any {
		return map[string]any{"scenario_id": scenario(w), "group_id": class(w), "scope": "group"}
	}
}

// classManagers is the spec's "may run classA": its owner, its class manager,
// and the organisation's managers, who manage every class of their org
// without being on its roster.
var classManagers = allow(mxTeacherA, mxCoTrainer, mxOrgOwner, mxOrgManager, mxAdmin)

// orgTeachers is the spec's "teaches in École": org teacher rank or above, or
// managing one of its classes (CanTeachInOrg).
var orgTeachers = allow(mxOrgOwner, mxOrgManager, mxTeacherA, mxTeacherB, mxCoTrainer, mxAdmin)

func organizationRows() []accessRow {
	return []accessRow{
		{action: "org: list members", request: get(route("/api/v1/organizations/%s/members", ecole)),
			allowed: allow(mxOrgOwner, mxOrgManager, mxTeacherA, mxTeacherB, mxCoTrainer, mxLearner, mxAdmin)},
		{action: "org: add a member", request: send(http.MethodPost, entityPath("OrganizationMember", nil),
			func(w matrixWorld) any {
				return map[string]any{"organization_id": w.ecole, "user_id": "mx-fresh-member", "role": "member"}
			}),
			allowed: allow(mxOrgOwner, mxOrgManager, mxAdmin)},
		{action: "org: make the learner a teacher", request: send(http.MethodPatch, entityPath("OrganizationMember", learnerOrgM),
			func(matrixWorld) any { return map[string]any{"role": "teacher"} }),
			allowed: allow(mxOrgOwner, mxOrgManager, mxAdmin)},
		{action: "org: make the learner an owner", request: send(http.MethodPatch, entityPath("OrganizationMember", learnerOrgM),
			func(matrixWorld) any { return map[string]any{"role": "owner"} }),
			allowed: allow(mxOrgOwner, mxAdmin)},
		{action: "org: remove the learner", request: send(http.MethodDelete, entityPath("OrganizationMember", learnerOrgM), nil),
			allowed: allow(mxOrgOwner, mxOrgManager, mxAdmin)},
		{action: "org: rename", request: send(http.MethodPatch, entityPath("Organization", ecole),
			func(matrixWorld) any { return map[string]any{"display_name": "École renamed"} }),
			allowed: allow(mxOrgOwner, mxOrgManager, mxAdmin)},
		{action: "org: subscribe to a plan (gates only)", request: send(http.MethodPost, route("/api/v1/organizations/%s/subscribe", ecole), nil),
			allowed: allow(mxOrgOwner, mxOrgManager, mxAdmin)},
		{action: "org: delete", request: send(http.MethodDelete, entityPath("Organization", ecole), nil),
			allowed: allow(mxOrgOwner, mxAdmin)},
	}
}

func classRows() []accessRow {
	teacherGroup := func(suffix string, ids ...func(w matrixWorld) uuid.UUID) func(w matrixWorld) string {
		return route("/api/v1/teacher/groups/%s"+suffix, append([]func(w matrixWorld) uuid.UUID{classA}, ids...)...)
	}
	return []accessRow{
		{action: "class: create in École", request: send(http.MethodPost, entityPath("ClassGroup", nil),
			func(w matrixWorld) any {
				return map[string]any{"name": "new-class", "display_name": "New class", "organization_id": w.ecole, "max_members": 10}
			}),
			allowed: allow(mxOrgOwner, mxOrgManager, mxTeacherA, mxTeacherB, mxAdmin)},
		{action: "classA: rename", request: send(http.MethodPatch, entityPath("ClassGroup", classA),
			func(matrixWorld) any { return map[string]any{"display_name": "Renamed class"} }),
			allowed: classManagers},
		{action: "classA: archive", request: send(http.MethodPost, entityPath("ClassGroup", classA, "/archive"), nil),
			allowed: classManagers},
		// A co-trainer may archive the class but not delete it (decided 2026-10-08).
		{action: "classA: delete", request: send(http.MethodDelete, entityPath("ClassGroup", classA), nil),
			allowed: allow(mxTeacherA, mxOrgOwner, mxOrgManager, mxAdmin)},
		{action: "classA: enrol a member", request: send(http.MethodPost, entityPath("GroupMember", nil),
			func(w matrixWorld) any {
				return map[string]any{"group_id": w.classA, "user_id": mxNewcomer, "role": "member"}
			}),
			allowed: classManagers},
		{action: "classA: remove the learner", request: send(http.MethodDelete, entityPath("GroupMember", learnerGrpM), nil),
			allowed: classManagers},
		{action: "classA: make the learner a class manager", request: send(http.MethodPatch, entityPath("GroupMember", learnerGrpM),
			func(matrixWorld) any { return map[string]any{"role": "manager"} }),
			allowed: classManagers},
		// A class manager's grant is capped at their own rank. Org managers may
		// name a class owner: they hand a class over when its teacher leaves
		// (decided 2026-10-08).
		{action: "classA: make the learner the class owner", request: send(http.MethodPatch, entityPath("GroupMember", learnerGrpM),
			func(matrixWorld) any { return map[string]any{"role": "owner"} }),
			allowed: allow(mxTeacherA, mxOrgOwner, mxOrgManager, mxAdmin)},
		{action: "classA: listed among my classes", request: get(func(matrixWorld) string { return "/api/v1/teacher/groups" }),
			// A personal list: the platform admin teaches no class, so is not judged.
			allowed: classManagers, judged: except(matrixActors, mxAdmin), verdict: listsClassA},
		{action: "classA: live progress", request: get(teacherGroup("/live-progress")), allowed: classManagers},
		{action: "classA: assignments progress", request: get(teacherGroup("/assignments-progress")), allowed: classManagers},
		{action: "classA: scenA results", request: get(teacherGroup("/scenarios/%s/results", scenA)), allowed: classManagers},
		{action: "classA: scenA analytics", request: get(teacherGroup("/scenarios/%s/analytics", scenA)), allowed: classManagers},
		{action: "classA: a learner's session detail", request: get(teacherGroup("/sessions/%s/detail", learnerRun)), allowed: classManagers},
		{action: "classA: a learner's terminal commands", request: get(teacherGroup("/sessions/%s/commands", peerRun)),
			allowed: classManagers, verdict: pastAuthorization},
		{action: "classA: bulk-start scenA", request: send(http.MethodPost, teacherGroup("/scenarios/%s/bulk-start", scenA), nil),
			allowed: classManagers, verdict: pastAuthorization},
		{action: "classA: reset scenA sessions", request: send(http.MethodPost, teacherGroup("/scenarios/%s/reset-sessions", scenA), nil),
			allowed: classManagers},
		{action: "classA: list its scenarios", request: get(route("/api/v1/groups/%s/scenarios", classA)), allowed: classManagers},
		{action: "classA: bulk-create terminals (gates only)", request: send(http.MethodPost, route("/api/v1/class-groups/%s/bulk-create-terminals", classA), nil),
			allowed: classManagers},
	}
}

func scenarioRows() []accessRow {
	authors := allow(mxTeacherA, mxOrgManager, mxOrgOwner, mxAdmin)
	importBody := func(matrixWorld) any {
		return map[string]any{"title": "Imported lab", "instance_type": "ubuntu:22.04",
			"steps": []map[string]any{{"title": "Step 1", "text_content": "Do it", "verify_script": "#!/bin/bash\ntrue"}}}
	}
	copyInEcole := func(scenario func(w matrixWorld) uuid.UUID) func(w matrixWorld) matrixRequest {
		return send(http.MethodPost, route("/api/v1/organizations/%s/scenarios/%s/duplicate", ecole, scenario), nil)
	}
	return []accessRow{
		{action: "scenario: create in École", request: send(http.MethodPost, route("/api/v1/organizations/%s/scenarios", ecole),
			func(matrixWorld) any {
				return map[string]any{"name": "new-lab", "title": "New lab", "instance_type": "ubuntu:22.04"}
			}),
			allowed: allow(mxOrgOwner, mxOrgManager, mxTeacherA, mxTeacherB, mxAdmin)},
		{action: "scenario: import JSON into École", request: send(http.MethodPost, route("/api/v1/organizations/%s/scenarios/import-json", ecole), importBody),
			allowed: allow(mxOrgOwner, mxOrgManager, mxTeacherA, mxTeacherB, mxAdmin)},

		{action: "scenA: edit its fields", request: send(http.MethodPatch, entityPath("Scenario", scenA),
			func(matrixWorld) any { return map[string]any{"title": "Rewritten"} }), allowed: authors},
		{action: "scenA: edit a step", request: send(http.MethodPatch, entityPath("ScenarioStep", stepA),
			func(matrixWorld) any { return map[string]any{"title": "Rewritten step"} }), allowed: authors},
		{action: "scenA: edit a question", request: send(http.MethodPatch, entityPath("ScenarioStepQuestion", questionA),
			func(matrixWorld) any { return map[string]any{"question_text": "Rewritten?"} }), allowed: authors},
		{action: "scenA: add a translation", request: send(http.MethodPost, entityPath("ScenarioTranslation", nil),
			func(w matrixWorld) any {
				return map[string]any{"scenario_id": w.scenA, "locale": "en", "title": "Lab A"}
			}),
			allowed: authors},
		{action: "scenA: archive", request: send(http.MethodPost, entityPath("Scenario", scenA, "/archive"), nil), allowed: authors},
		{action: "scenA: delete", request: send(http.MethodDelete, route("/api/v1/organizations/%s/scenarios/%s", ecole, scenA), nil),
			allowed: authors},

		{action: "scenA: read its full content", request: get(entityPath("Scenario", scenA, "/steps/read-only")), allowed: orgTeachers},
		{action: "scenA: export", request: get(entityPath("Scenario", scenA, "/export")), allowed: orgTeachers},
		{action: "scenA: copy within École", request: copyInEcole(scenA), allowed: orgTeachers},
		{action: "scenM: read its full content", request: get(entityPath("Scenario", scenM, "/steps/read-only")), allowed: orgTeachers},
		{action: "scenM: export", request: get(entityPath("Scenario", scenM, "/export")), allowed: orgTeachers},
		{action: "scenM: copy within École", request: copyInEcole(scenM), allowed: orgTeachers},

		// The spec says every teacher and manager may use the public catalogue;
		// it says nothing of learners reading it, so they are not judged there.
		// Copying writes into École, which only its teachers may do.
		{action: "scenPub: read its full content", request: get(entityPath("Scenario", scenPub, "/steps/read-only")),
			allowed: orgTeachers, judged: orgTeachers},
		{action: "scenPub: copy into École", request: copyInEcole(scenPub), allowed: orgTeachers},

		{action: "scenOther: read its full content", request: get(entityPath("Scenario", scenOther, "/steps/read-only")), allowed: allow(mxAdmin)},
		{action: "scenOther: export", request: get(entityPath("Scenario", scenOther, "/export")), allowed: allow(mxAdmin)},
		{action: "scenOther: edit its fields", request: send(http.MethodPatch, entityPath("Scenario", scenOther),
			func(matrixWorld) any { return map[string]any{"title": "Hijacked"} }), allowed: allow(mxAdmin)},
	}
}

func assignmentRows() []accessRow {
	assign := func(scenario func(w matrixWorld) uuid.UUID) func(w matrixWorld) matrixRequest {
		return send(http.MethodPost, entityPath("ScenarioAssignment", nil), assignTo(scenario, classA))
	}
	return []accessRow{
		{action: "assign scenM to classA", request: assign(scenM), allowed: classManagers},
		{action: "assign scenPub to classA", request: assign(scenPub), allowed: classManagers},
		// A scenario never leaves its organisation, not even for a platform
		// administrator (refuseCrossOrgAssignment).
		{action: "assign scenOther to classA", request: assign(scenOther), allowed: nil},
		{action: "assign scenA to classB", request: send(http.MethodPost, entityPath("ScenarioAssignment", nil),
			assignTo(scenA, func(w matrixWorld) uuid.UUID { return w.classB })),
			allowed: allow(mxTeacherB, mxOrgOwner, mxOrgManager, mxAdmin)},
		{action: "assign scenM to the whole of École", request: send(http.MethodPost, entityPath("ScenarioAssignment", nil),
			func(w matrixWorld) any {
				return map[string]any{"scenario_id": w.scenM, "organization_id": w.ecole, "scope": "org"}
			}),
			allowed: allow(mxOrgOwner, mxOrgManager, mxAdmin)},
	}
}

func learnerRows() []accessRow {
	learnerView := allow(mxLearner, mxOutsider, mxAdmin)
	return []accessRow{
		{action: "launch scenA (assigned to classA)", request: send(http.MethodPost, func(matrixWorld) string { return "/api/v1/scenario-sessions/launch" },
			func(w matrixWorld) any { return map[string]any{"scenario_id": w.scenA} }),
			allowed: allow(mxLearner, mxAdmin), judged: learnerView, verdict: pastAuthorization},
		{action: "read own session", request: get(route("/api/v1/scenario-sessions/%s/info", learnerRun)),
			allowed: allow(mxLearner, mxAdmin), judged: learnerView},
		{action: "read a classmate's session", request: get(route("/api/v1/scenario-sessions/%s/info", peerRun)),
			allowed: allow(mxAdmin), judged: learnerView},
	}
}

func TestRoleAccessMatrix(t *testing.T) {
	stack := newMatrixStack(t)

	groups := []struct {
		name string
		rows []accessRow
	}{
		{"Organization", organizationRows()},
		{"Classes", classRows()},
		{"Scenarios", scenarioRows()},
		{"Assignments", assignmentRows()},
		{"Learners", learnerRows()},
	}
	for _, g := range groups {
		t.Run(g.name, func(t *testing.T) {
			for _, row := range g.rows {
				t.Run(row.action, func(t *testing.T) {
					judged := row.judged
					if judged == nil {
						judged = matrixActors
					}
					for _, actor := range judged {
						t.Run(actor, func(t *testing.T) {
							stack.runCell(t, row, actor)
						})
					}
				})
			}
		})
	}
}

func (s *matrixStack) runCell(t *testing.T, row accessRow, actor string) {
	world := s.freshWorld(t)
	req := row.request(world)
	code, body := s.do(t, actor, req)

	verdict := row.verdict
	if verdict == nil {
		verdict = defaultVerdict
	}
	got, decided := verdict(world, code, body)
	require.True(t, decided, "%s %s answered %d before any authorization decision: %s", req.method, req.path, code, body)

	want := slices.Contains(row.allowed, actor)
	if reason, known := row.gaps[actor]; known {
		require.NotEqual(t, want, got, "KNOWN GAP closed, remove its marker: %s", reason)
		t.Skip("KNOWN GAP: " + reason)
	}
	require.Equal(t, want, got, "%s %s → %d (spec says allowed=%v): %s", req.method, req.path, code, want, body)
}

// ---------------------------------------------------------------------------
// The stack: production wiring with the JWT signature as the only seam.
// ---------------------------------------------------------------------------

type matrixStack struct {
	db, casbinDB *gorm.DB
	router       *gin.Engine
	// lastRoutePolicy is the highest casbin_rule id once the route policies
	// are loaded; every row above it is a per-entity grant made by a world.
	lastRoutePolicy uint
}

// freshWorld drops the previous world's rows and its Casbin grants (which
// AuthManagement would otherwise reload on every request, growing with each
// cell), then seeds a new one.
func (s *matrixStack) freshWorld(t *testing.T) matrixWorld {
	t.Helper()
	require.NoError(t, s.casbinDB.Exec("DELETE FROM casbin_rule WHERE id > ?", s.lastRoutePolicy).Error)
	require.NoError(t, casdoor.Enforcer.LoadPolicy())
	return seedMatrixWorld(t, s.db)
}

// gateOnlyRoutes call Stripe or tt-backend (and check a plan) before their
// own authorization, so their handler cannot run here. A sentinel stands in
// behind the real Layer 2 and Layer 1, which is where their authorization
// lives.
var gateOnlyRoutes = []struct{ method, path string }{
	{http.MethodPost, "/organizations/:id/subscribe"},
	{http.MethodPost, "/class-groups/:id/bulk-create-terminals"},
}

func newMatrixStack(t *testing.T) *matrixStack {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := matrixDB(t, "role_matrix_data")
	migrateMatrixDB(t, db)
	casbinDB := matrixDB(t, "role_matrix_casbin")

	prevEnforcer, prevParser, prevDB, prevRegistry := casdoor.Enforcer, casdoor.JwtTokenParser, sqldb.DB, ems.GlobalEntityRegistrationService
	t.Cleanup(func() {
		casdoor.Enforcer, casdoor.JwtTokenParser, sqldb.DB, ems.GlobalEntityRegistrationService = prevEnforcer, prevParser, prevDB, prevRegistry
		access.RouteRegistry.Reset()
		access.ResetEnforcers()
		hooks.GlobalHookRegistry.ClearAllHooks()
	})

	casdoor.InitCasdoorEnforcer(casbinDB, basePath())
	require.NoError(t, casdoor.Enforcer.LoadPolicy())
	// Dashboards resolve learner names through the Casdoor SDK; an unreachable
	// endpoint makes the lookups fail soft instead of dereferencing a nil client.
	casdoorsdk.InitConfig("http://localhost:0", "dummy", "dummy", "dummy", "dummy", "dummy")
	casdoor.JwtTokenParser = func(token string) (*casdoorsdk.Claims, error) {
		return &casdoorsdk.Claims{User: casdoorsdk.User{Id: token}}, nil
	}
	sqldb.DB = db
	ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
	access.RouteRegistry.Reset()
	access.ResetEnforcers()
	hooks.GlobalHookRegistry.ClearAllHooks()

	registry := ems.GlobalEntityRegistrationService
	groupRegistration.RegisterGroup(registry)
	groupRegistration.RegisterGroupMember(registry)
	orgRegistration.RegisterOrganization(registry)
	orgRegistration.RegisterOrganizationMember(registry)
	scenarioRegistration.RegisterScenario(registry)
	scenarioRegistration.RegisterScenarioStep(registry)
	scenarioRegistration.RegisterScenarioStepQuestion(registry)
	scenarioRegistration.RegisterScenarioTranslation(registry)
	scenarioRegistration.RegisterScenarioAssignment(registry)
	scenarioRegistration.RegisterScenarioSession(registry)

	scenarioController.RegisterScenarioPermissions(casdoor.Enforcer)
	terminalController.RegisterTerminalPermissions(casdoor.Enforcer)
	organizationRoutes.RegisterOrganizationPermissions(casdoor.Enforcer)
	paymentController.RegisterPaymentPermissions(casdoor.Enforcer)
	access.RegisterBuiltinEnforcers(access.NewGormEntityLoader(db), access.NewGormMembershipChecker(db))

	groupHooks.InitGroupHooks(db)
	orgHooks.InitOrganizationHooks(db)
	scenarioHooks.InitScenarioHooks(db)
	ems.RegisterOwnershipHooks(db)

	// The startup role sync gives every user member; operators hold
	// administrator on top of it.
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = false
	for _, user := range append(matrixActors, mxPeer, mxNewcomer, mxOtherOwner, mxCatalogAuthor) {
		require.NoError(t, utils.AddGroupingPolicy(casdoor.Enforcer, user, string(authModels.Member), opts))
	}
	require.NoError(t, utils.AddGroupingPolicy(casdoor.Enforcer, mxAdmin, string(authModels.Administrator), opts))

	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(access.Layer2Enforcement())
	authMiddleware := authController.NewAuthMiddleware(db)
	swaggerGenerator.NewSwaggerRouteGenerator(db).RegisterDocumentedRoutes(api, authMiddleware.AuthManagement(), authMiddleware.IdentifyIfPresent())
	scenarioController.ScenarioRoutes(api, db)
	organizationRoutes.OrganizationRoutes(api, db)
	for _, route := range gateOnlyRoutes {
		api.Handle(route.method, route.path, authMiddleware.AuthManagement(), func(c *gin.Context) { c.Status(http.StatusOK) })
	}

	var lastRoutePolicy uint
	require.NoError(t, casbinDB.Table("casbin_rule").Select("COALESCE(MAX(id), 0)").Scan(&lastRoutePolicy).Error)
	return &matrixStack{db: db, casbinDB: casbinDB, router: r, lastRoutePolicy: lastRoutePolicy}
}

func matrixDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// SQLite cannot bind the jsonb Metadata maps Postgres stores; drop the
	// column on every write, as the rest of the suite does with Omit("Metadata").
	omitMetadata := func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.LookUpField("Metadata") != nil {
			tx.Statement.Omits = append(tx.Statement.Omits, "Metadata")
		}
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("matrix:omit_metadata", omitMetadata))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("matrix:omit_metadata", omitMetadata))
	return db
}

func (s *matrixStack) do(t *testing.T, actor string, req matrixRequest) (int, string) {
	t.Helper()
	var payload []byte
	if req.body != nil {
		var err error
		payload, err = json.Marshal(req.body)
		require.NoError(t, err)
	}
	httpReq := httptest.NewRequest(req.method, req.path, bytes.NewReader(payload))
	httpReq.Header.Set("Authorization", "Bearer "+actor)
	if payload != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, httpReq)
	return rec.Code, rec.Body.String()
}

// ---------------------------------------------------------------------------
// The world. Rows are inserted directly, then given the Casbin grants their
// production creation path gives them, so Layer 1 sees what it sees in prod.
// ---------------------------------------------------------------------------

var matrixTables = []string{
	"scenario_step_progress", "scenario_flags", "scenario_sessions", "scenario_assignments",
	"scenario_instance_types", "scenario_step_questions", "scenario_step_hints", "scenario_lexicon_names",
	"scenario_lexicon_entries", "scenario_step_translations", "scenario_translations", "scenario_steps",
	"scenarios", "project_files", "group_members", "class_groups", "organization_members", "organizations",
	"terminals", "user_terminal_keys", "user_subscriptions", "organization_subscriptions",
	"organization_role_plans", "subscription_plans", "usage_metrics", "features", "audit_logs",
}

func migrateMatrixDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(
		&scenarioModels.ProjectFile{}, &scenarioModels.Scenario{}, &scenarioModels.ScenarioStep{},
		&scenarioModels.ScenarioStepHint{}, &scenarioModels.ScenarioStepQuestion{}, &scenarioModels.ScenarioSession{},
		&scenarioModels.ScenarioStepProgress{}, &scenarioModels.ScenarioFlag{}, &scenarioModels.ScenarioAssignment{},
		&scenarioModels.ScenarioInstanceType{}, &scenarioModels.ScenarioTranslation{}, &scenarioModels.ScenarioStepTranslation{},
		&scenarioModels.ScenarioLexiconEntry{}, &scenarioModels.ScenarioLexiconName{},
		&groupModels.ClassGroup{}, &groupModels.GroupMember{},
		&orgModels.Organization{}, &orgModels.OrganizationMember{},
		&terminalModels.Terminal{}, &terminalModels.UserTerminalKey{},
		&paymentModels.SubscriptionPlan{}, &paymentModels.UserSubscription{}, &paymentModels.OrganizationSubscription{},
		&paymentModels.OrganizationRolePlan{}, &paymentModels.UsageMetrics{},
		&configModels.Feature{}, &auditModels.AuditLog{}, &authModels.TokenBlacklist{},
	))
}

func seedMatrixWorld(t *testing.T, db *gorm.DB) matrixWorld {
	t.Helper()
	for _, table := range matrixTables {
		require.NoError(t, db.Exec("DELETE FROM "+table).Error)
	}

	orgSvc := orgServices.NewOrganizationService(db)
	groupSvc := groupServices.NewGroupService(db)
	var w matrixWorld

	w.ecole = matrixOrg(t, db, "ecole", mxOrgOwner)
	matrixOrgPlan(t, db, w.ecole)
	w.otherOrg = matrixOrg(t, db, "autre", mxOtherOwner)
	enrol := func(org uuid.UUID, user string, role orgModels.OrganizationMemberRole) uuid.UUID {
		m := orgModels.OrganizationMember{OrganizationID: org, UserID: user, Role: role, InvitedBy: user, JoinedAt: time.Now(), IsActive: true}
		require.NoError(t, db.Omit("Metadata").Create(&m).Error)
		require.NoError(t, orgSvc.GrantOrganizationPermissions(user, org))
		if m.IsManager() {
			require.NoError(t, orgSvc.GrantOrganizationManagerPermissions(user, org))
		}
		return m.ID
	}
	enrol(w.ecole, mxOrgOwner, orgModels.OrgRoleOwner)
	enrol(w.ecole, mxOrgManager, orgModels.OrgRoleManager)
	enrol(w.ecole, mxTeacherA, orgModels.OrganizationMemberRole(access.RoleTeacher))
	enrol(w.ecole, mxTeacherB, orgModels.OrganizationMemberRole(access.RoleTeacher))
	enrol(w.ecole, mxCoTrainer, orgModels.OrgRoleMember)
	w.learnerOrgMembership = enrol(w.ecole, mxLearner, orgModels.OrgRoleMember)
	enrol(w.ecole, mxPeer, orgModels.OrgRoleMember)
	enrol(w.ecole, mxNewcomer, orgModels.OrgRoleMember)
	enrol(w.otherOrg, mxOtherOwner, orgModels.OrgRoleOwner)
	for _, user := range append(matrixActors, mxPeer) {
		matrixPersonalPlan(t, db, user)
	}

	class := func(owner string) uuid.UUID {
		g := groupModels.ClassGroup{Name: "class-" + owner, DisplayName: "Class of " + owner, OwnerUserID: owner, OrganizationID: &w.ecole, MaxMembers: 50}
		require.NoError(t, db.Omit("Metadata").Create(&g).Error)
		require.NoError(t, groupSvc.EnrolMember(g.ID, owner, groupModels.GroupMemberRoleOwner, owner))
		return g.ID
	}
	w.classA = class(mxTeacherA)
	w.classB = class(mxTeacherB)
	require.NoError(t, groupSvc.EnrolMember(w.classA, mxCoTrainer, groupModels.GroupMemberRoleManager, mxTeacherA))
	require.NoError(t, groupSvc.EnrolMember(w.classA, mxLearner, groupModels.GroupMemberRoleMember, mxTeacherA))
	require.NoError(t, groupSvc.EnrolMember(w.classA, mxPeer, groupModels.GroupMemberRoleMember, mxTeacherA))
	var learnerRow groupModels.GroupMember
	require.NoError(t, db.Where("group_id = ? AND user_id = ?", w.classA, mxLearner).First(&learnerRow).Error)
	w.learnerClassMembership = learnerRow.ID

	scenario := func(name, author string, org *uuid.UUID, public bool) *scenarioModels.Scenario {
		s := &scenarioModels.Scenario{
			Name: name, Title: "Lab " + name, Description: "matrix fixture", InstanceType: "ubuntu:22.04",
			SourceType: "seed", CreatedByID: author, OrganizationID: org, IsPublic: public,
			Steps: []scenarioModels.ScenarioStep{{Order: 0, Title: "Step 1", TextContent: "Do step 1"}},
		}
		require.NoError(t, db.Create(s).Error)
		return s
	}
	a := scenario("scen-a", mxTeacherA, &w.ecole, false)
	w.scenA, w.stepA = a.ID, a.Steps[0].ID
	w.scenM = scenario("scen-m", mxOrgManager, &w.ecole, false).ID
	w.scenPub = scenario("scen-pub", mxCatalogAuthor, nil, true).ID
	w.scenOther = scenario("scen-other", mxOtherOwner, &w.otherOrg, false).ID

	question := scenarioModels.ScenarioStepQuestion{StepID: w.stepA, Order: 0, QuestionText: "Why?", QuestionType: "free_text"}
	require.NoError(t, db.Create(&question).Error)
	w.questionA = question.ID

	require.NoError(t, db.Create(&scenarioModels.ScenarioAssignment{
		ScenarioID: w.scenA, GroupID: &w.classA, Scope: "group", CreatedByID: mxTeacherA, IsActive: true,
	}).Error)

	run := func(user, status string) uuid.UUID {
		terminal := "tt-" + user
		s := scenarioModels.ScenarioSession{ScenarioID: w.scenA, UserID: user, Status: status, StartedAt: time.Now(), TerminalSessionID: &terminal}
		require.NoError(t, db.Create(&s).Error)
		return s.ID
	}
	// The learner's own run is finished, so launching scenA again is not
	// refused as "already running".
	w.learnerSession = run(mxLearner, "completed")
	w.peerSession = run(mxPeer, "active")
	return w
}

func matrixOrg(t *testing.T, db *gorm.DB, name, owner string) uuid.UUID {
	t.Helper()
	org := orgModels.Organization{
		Name: name, DisplayName: name, OwnerUserID: owner, OrganizationType: orgModels.OrgTypeTeam,
		MaxMembers: 100, IsActive: true,
	}
	require.NoError(t, db.Omit("Metadata").Create(&org).Error)
	return org.ID
}

// matrixOrgPlan gives École the school plan every member inherits: classes
// allowed, so only the role separates staff from students.
func matrixOrgPlan(t *testing.T, db *gorm.DB, org uuid.UUID) {
	t.Helper()
	plan := paymentModels.SubscriptionPlan{BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name: "school-" + org.String()[:8], Priority: 30, IsActive: true, GroupManagementEnabled: true}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&paymentModels.OrganizationSubscription{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()}, OrganizationID: org, SubscriptionPlanID: plan.ID,
		StripeCustomerID: "cus_matrix", Status: "active",
		CurrentPeriodStart: time.Now().Add(-time.Hour), CurrentPeriodEnd: time.Now().Add(24 * time.Hour),
	}).Error)
}

// matrixPersonalPlan gives a user the plan a launch requires, so a launch is
// decided by its access check rather than refused for want of a plan.
func matrixPersonalPlan(t *testing.T, db *gorm.DB, user string) {
	t.Helper()
	plan := paymentModels.SubscriptionPlan{BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name: "personal-" + user, Priority: 10, IsActive: true}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&paymentModels.UserSubscription{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()}, UserID: user, SubscriptionPlanID: plan.ID,
		SubscriptionType: "personal", Status: "active",
		CurrentPeriodStart: time.Now().Add(-time.Hour), CurrentPeriodEnd: time.Now().Add(24 * time.Hour),
	}).Error)
}
