package adminUsersRoutes

import (
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/auth/access"
	auditModels "soli/formations/src/audit/models"
	auditServices "soli/formations/src/audit/services"
	"soli/formations/src/auth/casdoor"
	apiErrors "soli/formations/src/auth/errors"
	"soli/formations/src/auth/interfaces"
	authServices "soli/formations/src/auth/services"
	paymentServices "soli/formations/src/payment/services"
	ttServices "soli/formations/src/terminalTrainer/services"
	"soli/formations/src/utils"
)

// Swappable seams for testing — same pattern as ListAllCasdoorUsers in
// controller.go: production code delegates to the real SDK/service, tests
// swap these vars to avoid hitting Casdoor or sending real email.
var (
	// GetCasdoorUserByID looks a single account up by its Casdoor user id.
	GetCasdoorUserByID = func(userID string) (*casdoorsdk.User, error) {
		return casdoorsdk.GetUserByUserId(userID)
	}

	// SendPasswordResetEmail triggers the same token+email flow the
	// self-service "forgot password" request uses (see
	// src/auth/services/passwordResetService.go), on behalf of an admin. The
	// admin never sees or sets the new password themselves — only the
	// account holder, via the emailed link, can.
	SendPasswordResetEmail = func(db *gorm.DB, email, resetURL string) error {
		return authServices.NewPasswordResetService(db).RequestPasswordReset(email, resetURL)
	}

	// SetAccountForbidden flips Casdoor's is_forbidden column, blocking or
	// restoring the account's ability to sign in.
	SetAccountForbidden = func(userID string, forbidden bool) error {
		return authServices.NewCasdoorUserClient().SetForbidden(userID, forbidden)
	}

	// DisableUserTerminalKey deactivates the account's tt-backend API key —
	// a long-lived credential the frontend holds and calls tt-backend with
	// directly, independent of the Casdoor JWT. Blocking sign-in alone
	// (SetAccountForbidden) stops a NEW login from ever getting that JWT,
	// but does nothing to a key the browser already has: without this call,
	// a blocked user keeps every terminal capability tt-backend grants that
	// key until it expires on its own. Same fault-tolerant call the
	// self-service key-regenerate endpoint and account erasure already use
	// (src/terminalTrainer/routes/userTerminalKeyController.go,
	// src/auth/services/userDeletionService.go) — a tt-backend-side failure
	// is only ever a warning; the local row still flips to inactive.
	DisableUserTerminalKey = func(db *gorm.DB, userID string) error {
		return ttServices.NewTerminalTrainerService(db).DisableUserKey(userID)
	}

	// CreateCasdoorUser creates the account in the identity provider.
	CreateCasdoorUser = func(user *casdoorsdk.User) error {
		return authServices.NewCasdoorUserClient().AddUser(user)
	}

	// GetCasdoorUserByEmail looks the just-created account back up, the same
	// way the self-signup flow does, to get the id Casdoor assigned it.
	GetCasdoorUserByEmail = func(email string) (*casdoorsdk.User, error) {
		return casdoorsdk.GetUserByEmail(email)
	}

	// ProvisionNewUser is the post-creation setup self-signup runs too
	// (authServices.ProvisionNewUser), swappable because its steps reach
	// tt-backend, Casdoor and SMTP.
	ProvisionNewUser = authServices.ProvisionNewUser

	// AddCasdoorNativeRole adds the account to Casdoor's own "member" Role
	// object (used elsewhere for display/sync — authorization itself runs on
	// the Casbin grouping policy, granted separately). Swappable because it
	// calls the Casdoor SDK's package-level client directly, which panics
	// rather than erroring when unconfigured (role.go GetRole dereferences a
	// nil client) — tests must never reach the real function.
	AddCasdoorNativeRole = authServices.AddDefaultRoleToUser

	// SetUserPassword sets an existing account's password directly, with no
	// token and no email — the admin hands the password to the user
	// themselves, the same trust model as the password field on account
	// creation (NewCreateUserHandler). This is deliberately a second,
	// independent path from SendPasswordResetEmail rather than a mode of
	// it: the two answer different questions (does the user pick their own
	// password via a link, or does the admin set one and hand it over) and
	// gating one behind the other would make the no-email path feel like a
	// workaround instead of a first-class choice.
	SetUserPassword = func(userID, newPassword string) error {
		client := authServices.NewCasdoorUserClient()
		user, err := client.GetUserByUserId(userID)
		if err != nil {
			return fmt.Errorf("failed to load account %s: %w", userID, err)
		}
		if user == nil {
			return fmt.Errorf("%w: %s", authServices.ErrUserNotFound, userID)
		}
		return client.SetPassword(user, newPassword)
	}

	// MarkUserEmailVerified force-verifies an existing account's email with
	// no token — the admin is vouching for the address directly. Shares
	// casdoor.MarkEmailVerified with the token-based self-service flow
	// (src/auth/services/emailVerificationService.go) and the organization
	// bulk-import path, so "verified" means the same thing everywhere.
	MarkUserEmailVerified = func(userID string) error {
		client := authServices.NewCasdoorUserClient()
		user, err := client.GetUserByUserId(userID)
		if err != nil {
			return fmt.Errorf("failed to load account %s: %w", userID, err)
		}
		if user == nil {
			return fmt.Errorf("%w: %s", authServices.ErrUserNotFound, userID)
		}
		casdoor.MarkEmailVerified(user, time.Now())
		affected, err := client.UpdateUserForColumns(user, casdoor.EmailVerifiedColumns)
		if err != nil {
			return fmt.Errorf("failed to update account %s: %w", userID, err)
		}
		if !affected {
			return authServices.ErrCasdoorWriteNotPersisted
		}
		return nil
	}
)

const defaultFrontendURL = "http://localhost:4000"

// resetPasswordURL builds the link the reset email points to, the same way
// passwordResetRoutes.RequestPasswordReset does for the self-service flow.
func resetPasswordURL() string {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = defaultFrontendURL
	}
	return fmt.Sprintf("%s/reset-password", frontendURL)
}

// requireAdmin aborts the request with 403 and returns false unless the
// caller carries the "administrator" role.
//
// Layer2Enforcement already blocks non-admins for every route in this file
// (declared AdminOnly in permissions.go) before the handler ever runs; this
// repeats the check as defense-in-depth, matching every other admin-only
// handler in the codebase (e.g. SubscriptionController.AdminAssignSubscription).
func requireAdmin(ctx *gin.Context) bool {
	if access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		return true
	}
	apiErrors.Abort(ctx, http.StatusForbidden, "admin_role_required")
	return false
}

// logAdminUserAction records a security-relevant admin action against a
// target account. actorUUID/targetUUID are best-effort: Casdoor user ids are
// not UUIDs, so a failed parse simply logs a zero UUID — the readable ids
// are also embedded in the action string for that reason.
func logAdminUserAction(ctx *gin.Context, db *gorm.DB, eventType auditModels.AuditEventType, adminID, targetID, action string) {
	actorUUID, _ := uuid.Parse(adminID)
	targetUUID, _ := uuid.Parse(targetID)
	auditServices.NewAuditService(db).LogSecurityEvent(ctx, eventType, &actorUUID, &targetUUID,
		fmt.Sprintf("%s (admin=%s, target=%s)", action, adminID, targetID),
		auditModels.AuditSeverityInfo)
}

// NewSendPasswordResetHandler returns the handler for
// POST /admin/users/:id/send-password-reset-email. It looks the target
// account's email up in Casdoor and sends the standard password-reset email
// — it does not generate or reveal a password.
func NewSendPasswordResetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireAdmin(ctx) {
			return
		}
		targetID := ctx.Param("id")

		user, err := GetCasdoorUserByID(targetID)
		if err != nil || user == nil {
			apiErrors.Abort(ctx, http.StatusNotFound, "user_not_found")
			return
		}

		if err := SendPasswordResetEmail(db, user.Email, resetPasswordURL()); err != nil {
			utils.Error("admin password-reset email failed for %s: %v", targetID, err)
			apiErrors.Abort(ctx, http.StatusInternalServerError, "send_failed")
			return
		}

		adminID := ctx.GetString("userId")
		logAdminUserAction(ctx, db, auditModels.AuditEventPasswordReset, adminID, targetID,
			"Admin triggered a password reset email")

		ctx.JSON(http.StatusOK, ActionResult{Success: true})
	}
}

// NewGrantAdminRoleHandler returns the handler for
// POST /admin/users/:id/admin-role. It binds the target account to the
// platform "administrator" role via the Casbin grouping policy — the same
// mechanism used at account creation (see userService.go).
func NewGrantAdminRoleHandler(enforcer interfaces.EnforcerInterface, db *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireAdmin(ctx) {
			return
		}
		targetID := ctx.Param("id")

		if user, err := GetCasdoorUserByID(targetID); err != nil || user == nil {
			apiErrors.Abort(ctx, http.StatusNotFound, "user_not_found")
			return
		}

		if err := utils.AddGroupingPolicy(enforcer, targetID, access.RoleAdministrator, utils.DefaultPermissionOptions()); err != nil {
			utils.Error("grant administrator role failed for %s: %v", targetID, err)
			apiErrors.Abort(ctx, http.StatusInternalServerError, "grant_failed")
			return
		}

		adminID := ctx.GetString("userId")
		logAdminUserAction(ctx, db, auditModels.AuditEventUserRoleAssigned, adminID, targetID,
			"Admin granted the administrator role")

		ctx.JSON(http.StatusOK, ActionResult{Success: true})
	}
}

// NewRevokeAdminRoleHandler returns the handler for
// DELETE /admin/users/:id/admin-role. It refuses to let an admin revoke
// their own administrator role — the one scenario this endpoint could use
// to lock the platform out of every admin account.
func NewRevokeAdminRoleHandler(enforcer interfaces.EnforcerInterface, db *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireAdmin(ctx) {
			return
		}
		targetID := ctx.Param("id")
		adminID := ctx.GetString("userId")

		if targetID == adminID {
			apiErrors.Abort(ctx, http.StatusBadRequest, "cannot_revoke_own_admin_role")
			return
		}

		if err := utils.RemoveGroupingPolicy(enforcer, targetID, access.RoleAdministrator, utils.DefaultPermissionOptions()); err != nil {
			utils.Error("revoke administrator role failed for %s: %v", targetID, err)
			apiErrors.Abort(ctx, http.StatusInternalServerError, "revoke_failed")
			return
		}

		logAdminUserAction(ctx, db, auditModels.AuditEventUserRoleRevoked, adminID, targetID,
			"Admin revoked the administrator role")

		ctx.JSON(http.StatusOK, ActionResult{Success: true})
	}
}

// NewSetForbiddenHandler returns the handler for
// PUT /admin/users/:id/forbidden. It blocks or restores the account's
// ability to sign in, via Casdoor's is_forbidden column. An admin cannot
// block their own account through this endpoint.
func NewSetForbiddenHandler(db *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireAdmin(ctx) {
			return
		}
		targetID := ctx.Param("id")
		adminID := ctx.GetString("userId")

		var input SetForbiddenInput
		if err := ctx.ShouldBindJSON(&input); err != nil {
			apiErrors.Abort(ctx, http.StatusBadRequest, "invalid_input")
			return
		}

		if input.Forbidden && targetID == adminID {
			apiErrors.Abort(ctx, http.StatusBadRequest, "cannot_block_own_account")
			return
		}

		if err := SetAccountForbidden(targetID, input.Forbidden); err != nil {
			if errors.Is(err, authServices.ErrUserNotFound) {
				apiErrors.Abort(ctx, http.StatusNotFound, "user_not_found")
				return
			}
			utils.Error("set forbidden=%t failed for %s: %v", input.Forbidden, targetID, err)
			apiErrors.Abort(ctx, http.StatusInternalServerError, "update_failed")
			return
		}

		if input.Forbidden {
			cutOffTerminalAccess(db, targetID)
		}

		eventType := auditModels.AuditEventUserReactivated
		action := "Admin unblocked the account"
		if input.Forbidden {
			eventType = auditModels.AuditEventUserSuspended
			action = "Admin blocked the account"
		}
		logAdminUserAction(ctx, db, eventType, adminID, targetID, action)

		ctx.JSON(http.StatusOK, ActionResult{Success: true})
	}
}

// cutOffTerminalAccess disables the blocked account's terminal key and revokes
// its running terminals. Best-effort: a blocked sign-in is the point of the
// block even if tt-backend is unreachable, but an already-issued key or a
// running terminal would otherwise outlive it. Most blocked accounts never
// opened a terminal and carry no key at all, an expected no-op.
func cutOffTerminalAccess(db *gorm.DB, userID string) {
	if err := DisableUserTerminalKey(db, userID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		utils.Warn("could not disable terminal trainer key for blocked user %s: %v", userID, err)
	}
	if err := paymentServices.TerminateUserTerminals(db, userID, nil); err != nil {
		utils.Warn("could not terminate terminals of blocked user %s: %v", userID, err)
	}
}

// minAdminSetPasswordLength mirrors the frontend form's minlength — enforced
// here too since the frontend check is only ever a courtesy.
const minAdminSetPasswordLength = 8

// NewSetPasswordHandler returns the handler for PUT /admin/users/:id/password.
// It sets the account's password directly — no token, no email. The admin
// is expected to hand the password to the user themselves, same as the
// password field on account creation.
func NewSetPasswordHandler(db *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireAdmin(ctx) {
			return
		}
		targetID := ctx.Param("id")

		var input SetPasswordInput
		if err := ctx.ShouldBindJSON(&input); err != nil || len(input.Password) < minAdminSetPasswordLength {
			apiErrors.Abort(ctx, http.StatusBadRequest, "invalid_input")
			return
		}

		if err := SetUserPassword(targetID, input.Password); err != nil {
			if errors.Is(err, authServices.ErrUserNotFound) {
				apiErrors.Abort(ctx, http.StatusNotFound, "user_not_found")
				return
			}
			utils.Error("admin set-password failed for %s: %v", targetID, err)
			apiErrors.Abort(ctx, http.StatusInternalServerError, "update_failed")
			return
		}

		adminID := ctx.GetString("userId")
		logAdminUserAction(ctx, db, auditModels.AuditEventPasswordChange, adminID, targetID,
			"Admin set the account's password directly (no email sent)")

		ctx.JSON(http.StatusOK, ActionResult{Success: true})
	}
}

// NewCreateUserHandler returns the handler for POST /admin/users. It creates
// an account directly — no self-signup ToS step, since the admin isn't the
// one accepting terms — and runs it through the same post-creation setup a
// self-registered account gets (authServices.ProvisionNewUser). The password is whatever the admin typed in the form; unlike
// every other write action in this module, this one leaves the admin holding
// a secret on purpose; they typed it, so there is nothing to protect by
// hiding it back from them.
func NewCreateUserHandler(enforcer interfaces.EnforcerInterface, db *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireAdmin(ctx) {
			return
		}

		var input AdminCreateUserInput
		if err := ctx.ShouldBindJSON(&input); err != nil {
			apiErrors.Abort(ctx, http.StatusBadRequest, "invalid_input")
			return
		}
		if _, err := mail.ParseAddress(input.Email); err != nil {
			apiErrors.Abort(ctx, http.StatusBadRequest, "invalid_email")
			return
		}
		if len(input.Password) < minAdminSetPasswordLength {
			apiErrors.Abort(ctx, http.StatusBadRequest, "password_too_short")
			return
		}

		now := time.Now()
		newUser := &casdoorsdk.User{
			Name:              input.Username,
			DisplayName:       input.DisplayName,
			Email:             input.Email,
			Password:          input.Password,
			FirstName:         input.FirstName,
			LastName:          input.LastName,
			SignupApplication: "ocf",
			CreatedTime:       casdoorsdk.GetCurrentTime(),
			Properties: map[string]string{
				// Synthesized rather than collected: the admin is creating this
				// account, not accepting terms on the new user's behalf. Matches
				// the organization bulk-import path's own synthesized stamp.
				"tos_accepted_at":  now.Format(time.RFC3339),
				"tos_version":      now.Format("2006-01-02"),
				"created_by_admin": "true",
			},
		}
		if input.EmailVerified {
			casdoor.MarkEmailVerified(newUser, now)
		}

		if err := CreateCasdoorUser(newUser); err != nil {
			utils.Error("admin create user failed for %s: %v", input.Email, err)
			apiErrors.Abort(ctx, http.StatusBadRequest, "create_failed")
			return
		}

		created, err := GetCasdoorUserByEmail(input.Email)
		if err != nil || created == nil {
			utils.Error("admin create user: account created but lookup by email failed for %s: %v", input.Email, err)
			apiErrors.Abort(ctx, http.StatusInternalServerError, "lookup_after_create_failed")
			return
		}

		// Casdoor's own "member" Role object (used elsewhere for display/sync)
		// — best-effort, unlike self-signup: the account already exists and
		// the Casbin grouping policy is what drives authorization.
		if err := AddCasdoorNativeRole(*created); err != nil {
			utils.Warn("admin-created user %s: could not add Casdoor default role: %v", created.Id, err)
		}

		ProvisionNewUser(enforcer, db, created, input.DefaultRole)

		adminID := ctx.GetString("userId")
		logAdminUserAction(ctx, db, auditModels.AuditEventUserCreated, adminID, created.Id,
			fmt.Sprintf("Admin created user account (email_verified=%t)", input.EmailVerified))

		ctx.JSON(http.StatusCreated, AdminCreateUserOutput{ID: created.Id, Username: created.Name, Email: created.Email})
	}
}

// NewVerifyEmailHandler returns the handler for
// POST /admin/users/:id/verify-email. It force-verifies an existing
// account's email with no token — for accounts created before this admin
// panel existed, or whose owner never completed the self-service link.
func NewVerifyEmailHandler(db *gorm.DB) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !requireAdmin(ctx) {
			return
		}
		targetID := ctx.Param("id")

		if err := MarkUserEmailVerified(targetID); err != nil {
			if errors.Is(err, authServices.ErrUserNotFound) {
				apiErrors.Abort(ctx, http.StatusNotFound, "user_not_found")
				return
			}
			utils.Error("admin verify-email failed for %s: %v", targetID, err)
			apiErrors.Abort(ctx, http.StatusInternalServerError, "verify_failed")
			return
		}

		adminID := ctx.GetString("userId")
		logAdminUserAction(ctx, db, auditModels.AuditEventUserUpdated, adminID, targetID,
			"Admin marked the account's email as verified")

		ctx.JSON(http.StatusOK, ActionResult{Success: true})
	}
}
