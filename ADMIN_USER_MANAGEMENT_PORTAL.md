# Admin User Management Portal

This document describes the admin-facing user management portal: a single
page (`/admin/users` in ocf-front) where a platform administrator can look up
any user and change their access without touching the database directly.

It builds on the admin "users with memberships" listing that already existed
(`GET /admin/users-with-memberships` + `AdminUsers.vue`, previously used only
for impersonation) and adds eight write actions plus inline role editing.

## What an admin can do from `/admin/users`

| Action | What it does | How |
|---|---|---|
| **Create a user** | Creates an account directly — username, name, email, a password the admin sets and hands over themselves, and an optional "email already verified" checkbox. No self-signup ToS step. | `POST /admin/users` |
| **Verify email** | Force-verifies an account's email with no token — for accounts created before this checkbox existed, or whose owner never clicked the self-service link. Hidden once an account is verified. | `POST /admin/users/:id/verify-email` |
| **Reset password** | Sends the user the standard "forgot password" email (token link, 1h expiry). The admin never sees or sets the new password. | `POST /admin/users/:id/send-password-reset-email` |
| **Set password** | Sets an *existing* account's password directly — no token, no email. The admin types it (masked, with a confirmation field) and hands it to the user themselves. | `PUT /admin/users/:id/password` |
| **Grant / revoke admin** | Promotes a user to platform `administrator`, or demotes them back to a regular `member`. | `POST` / `DELETE /admin/users/:id/admin-role` |
| **Assign a subscription plan** | Attributes a plan (Free / Small machine / Large machine / etc. — i.e. a `SubscriptionPlan` with its CPU/RAM budget) to the user for N days, bypassing Stripe. | `POST /user-subscriptions/admin-assign` (pre-existing endpoint, now reachable from the user's row via `AdminAssignPlanModal`) |
| **Block / unblock account** | Flips Casdoor's `is_forbidden` flag — the account can no longer sign in (or can again). Reversible. | `PUT /admin/users/:id/forbidden` |
| **Delete a user** | Permanently erases the account: Stripe cancellation, Casdoor identity, RBAC policies and all OCF-side data. Irreversible. | `DELETE /users/:id` (pre-existing admin-only endpoint, now reachable from the row) |
| **Change org / group role** | Inline `<select>` on each organization/group chip — member, teacher (org only), manager, owner. | `PATCH /organizations/:id/members/:userId` (orgs, pre-existing) and generic `PATCH /group-members/:id` (groups) |

Block and delete answer different questions and are not substitutes for one
another: blocking is reversible and keeps every record (useful for a
suspension, a dispute, "come back when you've paid your invoice"); deleting
is the GDPR right-to-erasure flow, permanent, and refused (409) while the
account still owns a non-personal organization or group. The frontend asks
the admin to type the target's exact username before enabling the delete
button — there is no "undo" to fall back on if they click the wrong row.

There are now three distinct password-handling actions, each answering a
different question:

- **Create** (new account): the admin types a password and hands it over.
  No existing password to protect yet.
- **Reset** (existing account, self-service style): only ever emails a
  link — the admin never sees or sets the password.
- **Set password** (existing account, admin-driven): the admin types a new
  password directly — same trust model as creation, but for an account that
  already exists. Added because an admin doesn't always want to wait on an
  email round-trip (the user is standing right there, or email delivery is
  broken, etc.).

All three are deliberate, independent choices — not modes of one another.
The two "admin types it" actions (create, set password) always require the
value twice (password + confirmation, both masked) before the submit button
even enables, to catch typos before they lock someone out.

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
| POST | `/api/v1/admin/users` | `administrator` | `AdminOnly` | `NewCreateUserHandler` |
| POST | `/api/v1/admin/users/:id/verify-email` | `administrator` | `AdminOnly` | `NewVerifyEmailHandler` |
| POST | `/api/v1/admin/users/:id/send-password-reset-email` | `administrator` | `AdminOnly` | `NewSendPasswordResetHandler` |
| PUT | `/api/v1/admin/users/:id/password` | `administrator` | `AdminOnly` | `NewSetPasswordHandler` |
| POST | `/api/v1/admin/users/:id/admin-role` | `administrator` | `AdminOnly` | `NewGrantAdminRoleHandler` |
| DELETE | `/api/v1/admin/users/:id/admin-role` | `administrator` | `AdminOnly` | `NewRevokeAdminRoleHandler` |
| PUT | `/api/v1/admin/users/:id/forbidden` | `administrator` | `AdminOnly` | `NewSetForbiddenHandler` |

Registered the same way as the pre-existing listing route: `permissions.go`
declares all seven via one `access.RegisterEnforced(...)` call (Layer 1 +
Layer 2 together), `routes.go` wires the gin handlers.

### How each action works

- **Create a user** (`actions.go`) builds a Casdoor account directly — no
  ToS acceptance step, since the admin isn't the one accepting terms on the
  new user's behalf. The ToS properties (`tos_accepted_at`, `tos_version`)
  are synthesized at creation time instead, the same way the organization
  bulk-import path already does for its own admin-created accounts
  (`src/organizations/services/importService.go`); a `created_by_admin`
  property marks the account's origin. The account gets the same starter
  resources a self-registered account gets — Casbin's `member` role (plus an
  optional `default_role`), a terminal trainer key, default settings, and
  the free Trial plan — via `AddDefaultRoleToUser`/`CreateDefaultUserSettings`
  (newly exported from `userService.go` for reuse) and the existing
  `AssignFreeTrialPlan`. The `email_verified` checkbox marks the account
  verified immediately (`casdoor.MarkEmailVerified`) when checked; left
  unchecked, the account goes through the normal self-service verification
  flow like any other. The password is whatever the admin typed — this is
  the one write action in the module that doesn't hide a secret from the
  admin, because they are the one who typed it.
- **Verify email** (`actions.go`) force-verifies an *existing* account with
  no token, via the same `casdoor.MarkEmailVerified` +
  `CasdoorUserClient.UpdateUserForColumns(user, casdoor.EmailVerifiedColumns)`
  pair the token-based self-service flow
  (`src/auth/services/emailVerificationService.go`) and the organization
  bulk-import path both use — "verified" means the same thing everywhere.
  For accounts that predate this admin panel, or whose owner never clicked
  the emailed link.
- **Send password reset email** (`actions.go`) looks the user's email up in
  Casdoor (`GetCasdoorUserByID`, a swappable var for tests) and calls the
  exact same `PasswordResetService.RequestPasswordReset(email, resetURL)`
  the self-service "forgot password" flow uses — see
  `PASSWORD_RESET_SETUP.md`. The admin triggers it; only the email recipient
  ever sees the new password.
- **Set password** (`actions.go`) sets an *existing* account's password
  directly via `CasdoorUserClient.SetPassword` — no token, no email. A
  deliberately separate endpoint from the reset-email one (not a "mode" of
  it): the two answer different questions, and the frontend enforces the
  same password+confirmation pattern as account creation before enabling
  submit.
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
- **Delete a user** also needed no new backend code — `DELETE /users/:id`
  (`src/auth/routes/usersRoutes/deleteUser.go`) already existed, already
  admin-gated, and already ran the full erasure flow
  (`UserDeletionService.EraseUser`): Stripe cancellation, Casdoor identity,
  RBAC policies, and every OCF-side record. It answers 409 (with a message
  naming the reason) while the account still owns a non-personal
  organization or group, and 404 for an unknown id. This slice only adds
  the row action in `AdminUsers.vue`.

Every write action is recorded to the audit log (`AuditService.LogSecurityEvent`)
with the acting admin, the target user, and a human-readable action string —
visible wherever the platform's audit trail is reviewed.

### Tests

- `tests/admin/users_test.go` — pre-existing listing tests, now also
  asserting the new `member_id` field.
- `tests/admin/userActions_test.go` — one test per handler added in the
  first slice: success path, the admin-only gate, the two self-action
  guards (can't revoke own admin role, can't block own account), and the
  "unknown user" 404 for password reset.
- `tests/admin/userCreation_test.go` — create-user success (asserts the
  Casdoor payload: ToS properties synthesized, `created_by_admin` stamped,
  password passed through verbatim), the `email_verified` flag actually
  marking the account verified, the admin-only gate, a Casdoor rejection
  surfacing as 400, verify-email's success/404/admin-only cases, and
  set-password's success/too-short/404/admin-only cases. Also asserts
  `email_verified` is surfaced by `BuildUserListings`.
- `tests/authorization/all_permissions_test.go` —
  `TestSetupAdminUsersPermissions` asserts the Layer 1 Casbin policy exists
  for all eight admin/users routes.

Delete-user added no backend tests of its own: `DELETE /users/:id` is
pre-existing, already covered wherever `EraseUser` is tested. The new
coverage lives entirely on the frontend (`tests/components/AdminUsers.test.ts`):
the self-delete guard, the submit button staying disabled until the typed
confirmation matches the username, the success path removing the row, and
a 409 conflict surfacing the backend's message.

**A gotcha worth knowing**: `AddDefaultRoleToUser` calls the Casdoor SDK's
*package-level* client directly (`casdoorsdk.GetRole`), which panics with a
nil-pointer dereference rather than returning an error when that client
isn't configured — exactly the state a unit test runs in. It's wrapped in
its own swappable var, `AddCasdoorNativeRole`, purely so tests can stub it
out; production always calls the real function.

## Frontend

Everything lives on the existing `AdminUsers.vue` page
(`src/components/Pages/Admin/AdminUsers.vue`, route `/admin/users`).

- **Row actions** gained several buttons next to the existing "Impersonate":
  Verify email (hidden once verified), Reset password, Set password,
  Grant/Revoke admin (label flips based on `is_admin`), Assign plan,
  Block/Unblock (label flips based on `is_active`). Verify email / reset
  password / grant-revoke-admin / block-unblock share one confirmation
  modal (`showActionModal` / `pendingAction`), keyed by action type so the
  title/message/success text come from one translation lookup
  (`adminUsers.actionTitle.<type>` etc.) instead of near-duplicate modals.
  Set password gets its own dedicated modal instead, since it needs two
  masked input fields rather than a yes/no confirmation.
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
- **Create user** is a button in the page header (`.page-header`, next to
  the title) rather than a row action, opening a dedicated `BaseModal` form
  (first/last/display name, username, email, password, the "email already
  verified" checkbox). On success the new row is appended to the table
  locally — no full refetch needed.
- **Verify email** is a row action, conditionally rendered
  (`v-if="!row.email_verified"`) — it disappears once the account is
  verified, and shares the same generic action-confirm modal as reset
  password / grant admin / block account.
- The email cell gained a small verified/unverified icon badge next to the
  address (`email_verified`).
- **Set password**'s dedicated modal reuses the same masked
  password+confirmation pattern as account creation (two `type="password"`
  fields, submit disabled until they match and meet the length minimum).
- **Delete user** gets its own dedicated modal rather than the generic
  confirm one: it shows what gets erased vs. anonymized, and the delete
  button stays disabled until the admin types the target's exact username
  into a confirmation field — the same "type to confirm" pattern the
  self-service account-deletion page already uses, adapted since the admin
  is confirming someone else's account rather than their own. A 409
  response (still owns an organization or group) surfaces the backend's
  message verbatim rather than a generic failure string. On success the row
  is removed from the table locally.
- `src/services/admin/usersService.ts` gained `createUser`, `verifyEmail`,
  `setPassword`, `deleteUser`, `sendPasswordResetEmail`, `grantAdminRole`,
  `revokeAdminRole`, `setForbidden`; `AdminUserListing` gained
  `email_verified`; `OrgMembership` / `GroupMembership` gained `member_id`.

All new strings are translated (English + French), following the project's
`useTranslations()` convention.

## Safety rails

- Every write endpoint re-checks the `administrator` role inside the handler
  (`requireAdmin`), in addition to the Layer 1/Layer 2 gate — matching the
  rest of the codebase's admin-only handlers (e.g.
  `AdminAssignSubscription`).
- An admin cannot revoke their own `administrator` role, block their own
  account, or delete their own account, through this portal — all three
  would be a one-way trip to a no-admin-left (or no-admin-at-all) state.
- Deleting a user requires typing their exact username first; there is no
  confirmation dialog alone standing between a misclick and an irreversible
  erasure.
- Password reset never shows the admin a password: only the user, through
  the emailed link, can set one. Account *creation* is the deliberate
  exception — see the note above.
- Every action is audit-logged with both the acting admin and the target
  user.
