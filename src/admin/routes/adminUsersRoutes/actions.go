package adminUsersRoutes

import (
	"fmt"
	"net/http"
	"os"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/auth/access"
	auditModels "soli/formations/src/audit/models"
	auditServices "soli/formations/src/audit/services"
	"soli/formations/src/auth/interfaces"
	authServices "soli/formations/src/auth/services"
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
