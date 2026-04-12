## Git Workflow Rules

1. **Always fetch before comparing**: Run `git fetch origin` before any branch comparison
2. **MR target is always `main`**: Never target a feature branch unless I explicitly say otherwise
3. **Validate before MR creation**: Before creating any MR, run `git log --oneline origin/main..HEAD` and show me the commit count. If it exceeds 10 commits, STOP and ask me to confirm.
4. **Never use git filter-branch**: For history cleanup, use `git rebase -i` scoped to only commits ahead of origin/main
5. **Branch naming**: Use `fix/` for bugs, `feat/` for features, `chore/` for maintenance
6. **Confirm remote state**: Before creating an MR, verify the branch doesn't already have an open MR with `glab mr list --source-branch=$(git branch --show-current)`
7. **Commits**: Must follow conventional patterns and not mention Claude

## GitLab (not GitHub)

This project uses GitLab, not GitHub. Use `glab` CLI for merge requests and issues. MR workflow:
1. `glab issue create --title "..." --description "..."`
2. `glab mr create --source-branch <branch> --target-branch main --title "..." --description "..."`
Never use `gh` or assume GitHub APIs.

## Debugging Approach

- When diagnosing bugs, do NOT celebrate or declare success until the user confirms the fix works in their environment.
- Exhaust the simplest explanations first before diving into framework internals (e.g., check middleware, environment config, null values before blaming GORM or library bugs).
- If a fix attempt doesn't resolve the issue, step back and re-read the user's original problem statement before trying the next hypothesis.

## Go Project Conventions

- This is a Go backend project. Always run `go build ./...` after code changes to catch compile errors.
- The API server runs on port 8080 (not 3333 or other ports).
- Swagger docs are generated — check the correct output directory before regenerating.

## Permission Architecture

OCF has a two-layer, fully extensible permission system. All code lives in `src/auth/access/`.

### Two Layers

**Layer 1 — RBAC Gateway** (`AuthManagement()` middleware): Controls which HTTP methods a role can use on a route. Only two built-in platform roles: `member` (all real users) and `administrator` (platform ops). Policies are registered per module in `permissions.go` files via `access.ReconcilePolicy()`.

**Layer 2 — Business Logic** (`Layer2Enforcement()` middleware): Enforces fine-grained access rules (ownership, group role, org role) from declarative `RoutePermission` entries in the `RouteRegistry`. Applied globally — acts only on registered routes, passes through unregistered ones.

### Adding a new route

When you add a route with `AuthManagement()`:

1. **Register the RBAC policy** in your module's `permissions.go`:
   ```go
   access.ReconcilePolicy(enforcer, "member", "/api/v1/my-route", "POST")
   ```
2. **Declare the access rule** in the same file:
   ```go
   access.RouteRegistry.Register("MyModule",
       access.RoutePermission{
           Path: "/api/v1/my-route", Method: "POST",
           Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
           Description: "Do something for the authenticated user",
       },
   )
   ```
3. **Add a test** in `tests/authorization/all_permissions_test.go`

### Module permission files

| Module | File |
|---|---|
| Auth | `src/auth/routes/usersRoutes/permissions.go` |
| Terminal | `src/terminalTrainer/routes/permissions.go` |
| Security admin | `src/auth/routes/securityAdminRoutes/permissions.go` |
| Payment | `src/payment/routes/permissions.go` |
| Scenarios | `src/scenarios/routes/permissions.go` |
| Courses | `src/courses/routes/courseRoutes/permissions.go` |
| Organizations | `src/organizations/routes/permissions.go` |

### Role hierarchy

Built-in roles: `member(10) < manager(50) < owner(100)`. Platform `administrator` bypasses all role checks.

**Extending**: Add custom roles at startup:
```go
access.RegisterRole("supervisor", 75) // between manager and owner
access.RegisterRole("viewer", 5)      // below member
```

The `IsRoleAtLeast(userRole, requiredRole)` helper uses these priorities. The hierarchy is shared by both groups and organizations.

### Access rule types

Built-in types: `Public`, `AdminOnly`, `SelfScoped`, `EntityOwner`, `GroupRole`, `OrgRole`.

**Extending**: Define a custom type and register its enforcer:
```go
// In your plugin/module:
const TenantScoped access.AccessRuleType = "tenant_scoped"

access.RegisterAccessEnforcer(TenantScoped, func(ctx *gin.Context, rule access.AccessRule, userID string, roles []string) bool {
    tenantID := ctx.Param("tenantId")
    // ... your custom authorization logic
    return allowed
})
```

Then use it in route declarations:
```go
access.RouteRegistry.Register("MyModule",
    access.RoutePermission{
        Path: "/api/v1/tenants/:tenantId/data", Method: "GET",
        Role: "member", Access: access.AccessRule{Type: TenantScoped, Param: "tenantId"},
        Description: "Get tenant data (tenant members only)",
    },
)
```

**Simplifying for a basic project**: Don't call `RegisterBuiltinEnforcers()` — register only the handlers you need. Unused types simply pass through.

### Entity ownership hooks

Entities can declare ownership in their registration:
```go
OwnershipConfig: &access.OwnershipConfig{
    OwnerField: "UserID",
    Operations: []string{"create", "update", "delete"},
    AdminBypass: true,
},
```

This auto-generates hooks: `BeforeCreate` forces the field to the authenticated user, `BeforeUpdate`/`BeforeDelete` verify ownership. No hand-written hook files needed.

---

## Terminal Session Composition (Phase 4)

### Overview

The `terminalTrainer` module gained a composed session API that lets the frontend present a filtered list of distributions, sizes, and features based on the user's effective plan, then launch a terminal through a single endpoint.

### New Endpoints (`src/terminalTrainer/routes/`)

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/terminals/distributions` | `member` | List all distributions available on the backend (cached) |
| `GET` | `/terminals/session-options` | `member` + effectivePlanMiddleware | Compute allowed sizes and features for a given distribution + plan |
| `POST` | `/terminals/start-composed-session` | `member` + effectivePlanMiddleware + CheckLimit + CheckRAMAvailability | Start a composed terminal session |
| `GET` | `/terminals/catalog-sizes` | `administrator` | Full catalog of resource sizes (for admin scenario editing) |
| `GET` | `/terminals/catalog-features` | `administrator` | Full catalog of features (for admin scenario editing) |

### Service Layer (`src/terminalTrainer/services/terminalTrainerService.go`)

- `GetDistributions(backend string)` — calls tt-backend and returns available distributions (with caching)
- `GetCatalogSizes()` — full size catalog from tt-backend (admin use)
- `GetCatalogFeatures()` — full feature catalog from tt-backend (admin use)
- `ComputeSessionOptions(distro, sizes, features, plan)` — pure function, intersects catalog data with plan limits (`AllowedMachineSizes`); exported for testing
- `GetSessionOptions(plan, distribution, backend)` — validates distribution exists, then calls `ComputeSessionOptions`; used by the GET endpoint
- `StartComposedSession(userID, input, planInterface)` — validates size/feature choices against the plan, then POSTs to tt-backend `/compose` endpoint and saves the `Terminal` record

### effectivePlanMiddleware Chain (`src/payment/middleware/`)

The composed session routes use a middleware chain that threads plan resolution through the request context, avoiding redundant DB round-trips:

```
InjectOrgContext → InjectEffectivePlan → RequirePlan → CheckLimit → CheckRAMAvailability → handler
```

| Middleware | File | Responsibility |
|---|---|---|
| `InjectOrgContext()` | `effectivePlanMiddleware.go` | Reads `organization_id` from query param or JSON body, stores in context as `org_context_id` |
| `InjectEffectivePlan(svc, db)` | `effectivePlanMiddleware.go` | Resolves effective plan per user+org, stores `effective_plan_result` and `subscription_plan` in context |
| `RequirePlan()` | `effectivePlanMiddleware.go` | Aborts 403 if no plan was resolved |
| `CheckLimit(svc, db, metricType)` | `effectivePlanMiddleware.go` | Reads pre-resolved plan from context, checks concurrent limit (e.g. `"concurrent_terminals"`), increments metric after success |
| `CheckRAMAvailability(svc)` | `ramCheckMiddleware.go` | Reads `subscription_plan` from context to estimate RAM needed (from `AllowedMachineSizes`), checks real-time server metrics, aborts 503 if insufficient |

`CheckLimit` falls back to full plan resolution if `InjectEffectivePlan` was not in the chain.

### Plan Gating (`src/payment/models/subscriptionPlan.go`)

`SubscriptionPlan` has terminal-specific fields used for composed session gating:

- `AllowedMachineSizes []string` — e.g. `["XS", "S"]`; empty = no restriction. The `"all"` value means any size is allowed.
- `MaxConcurrentTerminals int` — enforced by `CheckLimit("concurrent_terminals")`
- `MaxSessionDurationMinutes int`
- `NetworkAccessEnabled`, `DataPersistenceEnabled`, `DataPersistenceGB`
- `DefaultBackend`, `AllowedBackends []string`

### EffectivePlanService (`src/payment/services/effectivePlanService.go`)

Single source of truth for plan resolution:

- `GetUserEffectivePlanForOrg(userID, orgID)` — resolves personal plan vs org plan, returns the higher-priority one
- `CheckEffectiveUsageLimitFromResult(result, userID, metricType, increment)` — checks limits using an already-resolved `EffectivePlanResult`, skipping DB round-trip
- `EffectivePlanResult.Source` — either `"personal"` or `"organization"`; admin bypass resolves org plan directly

### Trial Plan Auto-Assignment

New organizations automatically receive a Trial subscription:
- `assignOrgTrialPlan` in `src/organizations/services/organizationService.go` is called in the `AfterCreate` org lifecycle
- `ensureOrganizationsHaveTrialPlan` in `src/initialization/database.go` runs at startup to backfill any orgs missing a subscription
- Mirrors `ensureUsersHaveTrialPlan` / `AssignFreeTrialPlan` for the user path

---

## Org-Scoped Scenarios (`src/scenarios/`)

### Overview

Scenarios now support organization-level scoping. A scenario owned by an org has `OrganizationID *uuid.UUID` set. Org managers can manage their own scenario library independently from the platform-wide catalog.

### `Scenario` Model Field (`src/scenarios/models/scenario.go`)

```go
OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
```

### New Org Endpoints (`src/scenarios/routes/scenarioRoutes.go`)

All mounted under `/organizations/:id/scenarios`:

| Method | Path | MinRole | Description |
|---|---|---|---|
| `GET` | `/organizations/:id/scenarios` | `manager` | List org's scenarios (`OrgListScenarios`) |
| `POST` | `/organizations/:id/scenarios/upload` | `manager` | Upload scenario archive to org (`OrgUploadScenario`) |
| `POST` | `/organizations/:id/scenarios/import-json` | `manager` | Import scenario from JSON into org (`OrgImportJSON`) |
| `GET` | `/organizations/:id/scenarios/:scenarioId/export` | `manager` | Export org scenario (`OrgExportScenario`) |
| `DELETE` | `/organizations/:id/scenarios/:scenarioId` | `manager` | Delete org scenario (`OrgDeleteScenario`) |
| `POST` | `/organizations/:id/scenarios/:scenarioId/duplicate` | `manager` | Duplicate org scenario (`OrgDuplicateScenario`) |

Layer 2 enforcement uses `OrgRole` with `MinRole: "manager"` — declared in `src/scenarios/routes/permissions.go`.

### Scenario Launch — Org-Aware Backend Resolution

`LaunchScenario` and `POST /scenario-sessions/launch` resolve the backend and distribution using org context:
1. `organization_id` extracted from body by `InjectOrgContext`
2. `resolveScenarioBackendAndDistribution(scenario, orgID)` selects a backend and distribution compatible with the scenario's `CompatibleInstanceTypes` and the org's backend configuration
3. Calls `StartComposedSession` with the resolved distribution/backend

---

### Self-documenting reference

`GET /api/v1/permissions/reference` returns all route permissions + entity CRUD permissions as JSON. The frontend renders this at `/help/account/permissions-reference`. Adding a route declaration automatically updates the reference page.

### Key rules

- `ReconcilePolicy` lives in `src/auth/access/reconcile.go` — **never import `initialization` from module permission files** (pulls in swagger → CI failure)
- Entity CRUD routes get policies automatically via entity registration (`/:id` pattern, not `/*`)
- Custom routes need manual policy + RouteRegistry declaration in the module's `permissions.go`
- The enforcement middleware handles `AdminOnly`, `EntityOwner`, `GroupRole`, `OrgRole` automatically. `SelfScoped` is documentation-only — handlers must verify userId scoping themselves.
- `ValidatePermissionSetup(router)` runs at startup and warns about routes without declarations or access types without enforcers
