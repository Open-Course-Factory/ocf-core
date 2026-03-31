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

## Permission Architecture (Casbin RBAC)

Permissions are **decentralized** — each module registers its own Casbin policies in a `permissions.go` file next to its routes.

### Adding a new route with AuthManagement()

When you add a new route that uses `middleware.AuthManagement()`, you **MUST** also register a Casbin policy for it, or it will return 403 for everyone (including admins).

1. Find the `permissions.go` in the module's route package (e.g., `src/payment/routes/permissions.go`)
2. Add a `casbinUtils.ReconcilePolicy(enforcer, role, path, method)` call for the new route
3. Use `"member"` for routes accessible to all authenticated users, `"administrator"` for admin-only
4. Add a test case in `tests/authorization/all_permissions_test.go`

### Module permission files

| Module | File | Registration function |
|---|---|---|
| Auth (core) | `src/auth/routes/usersRoutes/permissions.go` | `RegisterAuthPermissions`, `RegisterUserPermissions`, `RegisterFeedbackPermissions` |
| Terminal | `src/terminalTrainer/routes/permissions.go` | `RegisterTerminalPermissions` |
| Security admin | `src/auth/routes/securityAdminRoutes/permissions.go` | `RegisterSecurityAdminPermissions` |
| Payment | `src/payment/routes/permissions.go` | `RegisterPaymentPermissions` |
| Scenarios | `src/scenarios/routes/permissions.go` | `RegisterScenarioPermissions` |
| Courses | `src/courses/routes/courseRoutes/permissions.go` | `RegisterCoursePermissions` |
| Organizations | `src/organizations/routes/permissions.go` | `RegisterOrganizationPermissions` |

### Key rules

- `ReconcilePolicy` lives in `src/auth/access/reconcile.go` — **never import `initialization` from module permission files** (it pulls in swagger.go → generated docs/ → CI failure)
- Entity CRUD routes get policies automatically via entity registration (`/:id` pattern, not `/*`)
- Custom routes need manual policy registration in the module's `permissions.go`
- Admin routes should have **both** Casbin `administrator` policy AND handler-level `isAdmin()` check (defense-in-depth)
