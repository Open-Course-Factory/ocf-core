package adminUsersRoutes

import (
	"errors"
	"fmt"
	"net/http"
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
	"soli/formations/src/auth/interfaces"
	authServices "soli/formations/src/auth/services"
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

	// CreateCasdoorUser creates the account in the identity provider.
	CreateCasdoorUser = func(user *casdoorsdk.User) error {
		return authServices.NewCasdoorUserClient().AddUser(user)
	}

	// GetCasdoorUserByEmail looks the just-created account back up, the same
	// way the self-signup flow does, to get the id Casdoor assigned it.
	GetCasdoorUserByEmail = func(email string) (*casdoorsdk.User, error) {
		return casdoorsdk.GetUserByEmail(email)
	}

	// ProvisionNewUserExtras performs the best-effort side effects every new
	// account gets on self-signup (see AddUser in
	// src/auth/services/userService.go): a terminal trainer key, default
	// user settings, and the free Trial subscription plan. Each failure is
	// logged and swallowed rather than failing account creation — same
	// resilience as the self-signup path, since none of the three is
	// essential to having a working account.
	ProvisionNewUserExtras = func(db *gorm.DB, userID, username string) {
		if err := ttServices.NewTerminalTrainerService(db).CreateUserKey(userID, username); err != nil {
			utils.Warn("admin-created user %s: could not create terminal trainer key: %v", userID, err)
		}
		if err := authServices.CreateDefaultUserSettings(userID); err != nil {
			utils.Warn("admin-created user %s: could not create default settings: %v", userID, err)
		}
		if err := authServices.AssignFreeTrialPlan(userID); err != nil {
			utils.Warn("admin-created user %s: could not assign free Trial plan: %v", userID, err)
		}
	}

	// AddCasdoorNativeRole adds the account to Casdoor's own "member" Role
	// object (used elsewhere for display/sync — authorization itself runs on
	// the Casbin grouping policy, granted separately). Swappable because it
	// calls the Casdoor SDK's package-level client directly, which panics
	// rather than erroring when unconfigured (role.go GetRole dereferences a
	// nil client) — tests must never reach the real function.
	AddCasdoorNativeRole = authServices.AddDefaultRoleToUser

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
	ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin role required"})
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
			ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
			return
		}

		if err := SendPasswordResetEmail(db, user.Email, resetPasswordURL()); err != nil {
			utils.Error("admin password-reset email failed for %s: %v", targetID, err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "send_failed"})
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

		if err := utils.AddGroupingPolicy(enforcer, targetID, access.RoleAdministrator, utils.DefaultPermissionOptions()); err != nil {
			utils.Error("grant administrator role failed for %s: %v", targetID, err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "grant_failed"})
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
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "cannot_revoke_own_admin_role"})
			return
		}

		if err := utils.RemoveGroupingPolicy(enforcer, targetID, access.RoleAdministrator, utils.DefaultPermissionOptions()); err != nil {
			utils.Error("revoke administrator role failed for %s: %v", targetID, err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "revoke_failed"})
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
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_input"})
			return
		}

		if input.Forbidden && targetID == adminID {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "cannot_block_own_account"})
			return
		}

		if err := SetAccountForbidden(targetID, input.Forbidden); err != nil {
			utils.Error("set forbidden=%t failed for %s: %v", input.Forbidden, targetID, err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "update_failed"})
			return
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

// NewCreateUserHandler returns the handler for POST /admin/users. It creates
// an account directly — no self-signup ToS step, since the admin isn't the
// one accepting terms — and grants it the same "member" role and starter
// resources (terminal key, settings, free Trial plan) a self-registered
// account gets. The password is whatever the admin typed in the form; unlike
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
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_input"})
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
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "create_failed", "message": err.Error()})
			return
		}

		created, err := GetCasdoorUserByEmail(input.Email)
		if err != nil || created == nil {
			utils.Error("admin create user: account created but lookup by email failed for %s: %v", input.Email, err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "lookup_after_create_failed"})
			return
		}

		// Casdoor's own "member" Role object (used elsewhere for display/sync)
		// — best-effort, same as self-signup.
		if err := AddCasdoorNativeRole(*created); err != nil {
			utils.Warn("admin-created user %s: could not add Casdoor default role: %v", created.Id, err)
		}

		// The Casbin grouping policy is what actually drives authorization
		// (AuthMiddleware reads roles from here) — same roles self-signup grants.
		rolesToAdd := []string{"member"}
		if input.DefaultRole != "" {
			rolesToAdd = append(rolesToAdd, input.DefaultRole)
		}
		opts := utils.DefaultPermissionOptions()
		opts.WarnOnError = true
		for _, role := range rolesToAdd {
			if err := utils.AddGroupingPolicy(enforcer, created.Id, role, opts); err != nil {
				utils.Warn("admin-created user %s: could not grant role %s: %v", created.Id, role, err)
			}
		}

		ProvisionNewUserExtras(db, created.Id, created.Name)

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
				ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "user_not_found"})
				return
			}
			utils.Error("admin verify-email failed for %s: %v", targetID, err)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "verify_failed"})
			return
		}

		adminID := ctx.GetString("userId")
		logAdminUserAction(ctx, db, auditModels.AuditEventUserUpdated, adminID, targetID,
			"Admin marked the account's email as verified")

		ctx.JSON(http.StatusOK, ActionResult{Success: true})
	}
}
