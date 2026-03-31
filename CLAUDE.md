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

### Self-documenting reference

`GET /api/v1/permissions/reference` returns all route permissions + entity CRUD permissions as JSON. The frontend renders this at `/help/account/permissions-reference`. Adding a route declaration automatically updates the reference page.

### Key rules

- `ReconcilePolicy` lives in `src/auth/access/reconcile.go` — **never import `initialization` from module permission files** (pulls in swagger → CI failure)
- Entity CRUD routes get policies automatically via entity registration (`/:id` pattern, not `/*`)
- Custom routes need manual policy + RouteRegistry declaration in the module's `permissions.go`
- The enforcement middleware handles `AdminOnly`, `EntityOwner`, `GroupRole`, `OrgRole` automatically. `SelfScoped` is documentation-only — handlers must verify userId scoping themselves.
- `ValidatePermissionSetup(router)` runs at startup and warns about routes without declarations or access types without enforcers
