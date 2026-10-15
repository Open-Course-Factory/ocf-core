# Admin User Management Portal

This document describes the admin-facing user management portal: a single
page (`/admin/users` in ocf-front) where a platform administrator can look up
any user and change their access without touching the database directly.

It builds on the admin "users with memberships" listing that already existed
(`GET /admin/users-with-memberships` + `AdminUsers.vue`, previously used only
for impersonation) and adds four write actions plus inline role editing.

## What an admin can do from `/admin/users`

| Action | What it does | How |
|---|---|---|
| **Reset password** | Sends the user the standard "forgot password" email (token link, 1h expiry). The admin never sees or sets the new password. | `POST /admin/users/:id/send-password-reset-email` |
| **Grant / revoke admin** | Promotes a user to platform `administrator`, or demotes them back to a regular `member`. | `POST` / `DELETE /admin/users/:id/admin-role` |
| **Assign a subscription plan** | Attributes a plan (Free / Small machine / Large machine / etc. — i.e. a `SubscriptionPlan` with its CPU/RAM budget) to the user for N days, bypassing Stripe. | `POST /user-subscriptions/admin-assign` (pre-existing endpoint, now reachable from the user's row via `AdminAssignPlanModal`) |
| **Block / unblock account** | Flips Casdoor's `is_forbidden` flag — the account can no longer sign in (or can again). | `PUT /admin/users/:id/forbidden` |
| **Change org / group role** | Inline `<select>` on each organization/group chip — member, teacher (org only), manager, owner. | `PATCH /organizations/:id/members/:userId` (orgs, pre-existing) and generic `PATCH /group-members/:id` (groups) |

"Teacher" ("prof") is not a separate system role — it is one of the four
business roles a user can hold within a specific organization (`member` <
`teacher`/`manager` < `owner`), same as "Small machine" / "Large machine" is
not a role but a `SubscriptionPlan`'s CPU/RAM budget. The portal surfaces
both existing concepts next to the two genuinely new levers this slice adds:
the platform `administrator` role and the account block switch.

## Backend

All new code lives in `src/admin/routes/adminUsersRoutes/`, alongside the
pre-existing listing handler.

### New endpoints

| Method | Path | Layer 1 role | Layer 2 | Handler |
|---|---|---|---|---|
| POST | `/api/v1/admin/users/:id/send-password-reset-email` | `administrator` | `AdminOnly` | `NewSendPasswordResetHandler` |
| POST | `/api/v1/admin/users/:id/admin-role` | `administrator` | `AdminOnly` | `NewGrantAdminRoleHandler` |
| DELETE | `/api/v1/admin/users/:id/admin-role` | `administrator` | `AdminOnly` | `NewRevokeAdminRoleHandler` |
| PUT | `/api/v1/admin/users/:id/forbidden` | `administrator` | `AdminOnly` | `NewSetForbiddenHandler` |

Registered the same way as the pre-existing listing route: `permissions.go`
declares all four via one `access.RegisterEnforced(...)` call (Layer 1 +
Layer 2 together), `routes.go` wires the gin handlers.

### How each action works

- **Send password reset email** (`actions.go`) looks the user's email up in
  Casdoor (`GetCasdoorUserByID`, a swappable var for tests) and calls the
  exact same `PasswordResetService.RequestPasswordReset(email, resetURL)`
  the self-service "forgot password" flow uses — see
  `PASSWORD_RESET_SETUP.md`. The admin triggers it; only the email recipient
  ever sees the new password.
- **Grant / revoke admin role** bind or unbind the target account from the
  Casbin `administrator` role group, via `utils.AddGroupingPolicy` /
  `RemoveGroupingPolicy` — the same mechanism `userService.go` uses at
  account creation. An admin cannot revoke their own admin role through this
  endpoint (the one path that could lock the platform out of every admin
  account).
- **Block / unblock account** flips Casdoor's `is_forbidden` column via the
  existing `CasdoorUserClient.SetForbidden`, already used for member
  offboarding. An admin cannot block their own account through this
  endpoint.
- **Assign a subscription plan** needed no new backend code — the
  `POST /user-subscriptions/admin-assign` endpoint
  (`SubscriptionController.AdminAssignSubscription`) already existed and is
  admin-gated; this slice only wires the frontend modal to a user's row.
- **Org/group role changes** also needed no new backend code — org member
  roles already have a dedicated endpoint
  (`PATCH /organizations/:id/members/:userId`,
  `organizationService.UpdateMemberRole`), and group member roles go through
  the generic entity framework's `PATCH /group-members/:id`. The admin
  listing now additionally returns each membership row's own id
  (`member_id`) so the frontend can target the group PATCH directly — org
  memberships don't need it since that endpoint takes the org id + user id
  instead.

Every write action is recorded to the audit log (`AuditService.LogSecurityEvent`)
with the acting admin, the target user, and a human-readable action string —
visible wherever the platform's audit trail is reviewed.

### Tests

- `tests/admin/users_test.go` — pre-existing listing tests, now also
  asserting the new `member_id` field.
- `tests/admin/userActions_test.go` — one test per new handler: success
  path, the admin-only gate, the two self-action guards (can't revoke own
  admin role, can't block own account), and the "unknown user" 404 for
  password reset.
- `tests/authorization/all_permissions_test.go` —
  `TestSetupAdminUsersPermissions` asserts the Layer 1 Casbin policy exists
  for all five admin/users routes (the pre-existing GET plus the four new
  ones).

## Frontend

Everything lives on the existing `AdminUsers.vue` page
(`src/components/Pages/Admin/AdminUsers.vue`, route `/admin/users`).

- **Row actions** gained four buttons next to the existing "Impersonate":
  Reset password, Grant/Revoke admin (label flips based on `is_admin`),
  Assign plan, Block/Unblock (label flips based on `is_active`). The first
  and last two share one confirmation modal (`showActionModal` /
  `pendingAction`), keyed by action type so the title/message/success text
  come from one translation lookup (`adminUsers.actionTitle.<type>` etc.)
  instead of four near-duplicate modals.
- **Assign plan** reuses `AdminAssignPlanModal.vue` (previously reachable
  only from the subscription-plans page with a hand-typed user id). It
  gained two optional props, `presetUserId` / `presetUserName`, that
  pre-fill and lock the user id field when opened from a known row.
- **Org/group chips** gained an inline role `<select>` next to the chip
  name. Its `:value` is bound straight to the source membership object (not
  a local copy), so a failed update simply re-renders the original role —
  no manual revert logic needed. Organizations use
  `organizationService.updateMemberRole`; groups PATCH
  `/group-members/:member_id` directly (no dedicated group-role service
  existed, unlike organizations).
- `src/services/admin/usersService.ts` gained `sendPasswordResetEmail`,
  `grantAdminRole`, `revokeAdminRole`, `setForbidden`, and the `OrgMembership`
  / `GroupMembership` interfaces gained `member_id`.

All new strings are translated (English + French), following the project's
`useTranslations()` convention.

## Safety rails

- Every write endpoint re-checks the `administrator` role inside the handler
  (`requireAdmin`), in addition to the Layer 1/Layer 2 gate — matching the
  rest of the codebase's admin-only handlers (e.g.
  `AdminAssignSubscription`).
- An admin cannot revoke their own `administrator` role, nor block their own
  account, through this portal — both would be a one-way trip to a
  no-admin-left state.
- Password reset never shows the admin a password: only the user, through
  the emailed link, can set one.
- Every action is audit-logged with both the acting admin and the target
  user.
