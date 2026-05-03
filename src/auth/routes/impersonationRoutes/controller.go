package impersonationRoutes

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	authMiddleware "soli/formations/src/auth/middleware"
	services "soli/formations/src/auth/services"
)

// UserValidator abstracts the existence check for a target user. The
// production implementation calls Casdoor; tests inject an in-memory fake.
type UserValidator interface {
	UserExists(userID string) (bool, error)
}

// Controller holds the dependencies for the three impersonation HTTP
// handlers. It is intentionally small: all stateful work lives in the
// service layer.
type Controller struct {
	Service       services.ImpersonationService
	UserValidator UserValidator
}

// NewController builds a new impersonation Controller.
func NewController(svc services.ImpersonationService, v UserValidator) *Controller {
	return &Controller{Service: svc, UserValidator: v}
}

// StartImpersonation handles POST /admin/impersonate/start.
//
// Contract (returned status / error code on failure):
//   - 409 impersonation_chaining_forbidden — caller is already inside an
//     impersonation context (X-Impersonate-User header is present).
//   - 400 invalid_request — body missing or target_user_id empty.
//   - 400 self_impersonation_forbidden — target_user_id equals the caller.
//   - 404 target_not_found — UserValidator reports the target does not exist.
//   - 500 user_validation_failed — UserValidator returned an error.
//   - 409 already_impersonating — service reports an existing active session.
//   - 500 start_failed — any other service error.
//   - 201 + StartImpersonationResponse on success.
func (c *Controller) StartImpersonation(ctx *gin.Context) {
	// 1. Reject chaining: if the caller already carries the impersonation
	// header, they are already acting as someone else.
	if ctx.GetHeader(authMiddleware.ImpersonationHeader) != "" {
		ctx.JSON(http.StatusConflict, gin.H{"error": "impersonation_chaining_forbidden"})
		return
	}

	// 2. Bind the request body. `binding:"required"` covers the empty-string
	// case for target_user_id.
	var req StartImpersonationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":  "invalid_request",
			"detail": err.Error(),
		})
		return
	}

	callerID := ctx.GetString("userId")

	// 3. Defence-in-depth self-check before hitting the validator or service.
	if req.TargetUserID == callerID {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "self_impersonation_forbidden"})
		return
	}

	// 4. Confirm the target exists in the identity provider.
	exists, err := c.UserValidator.UserExists(req.TargetUserID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "user_validation_failed"})
		return
	}
	if !exists {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "target_not_found"})
		return
	}

	// 5. Open the session via the service. Service-level guards translate
	// to specific HTTP responses.
	session, err := c.Service.StartSession(callerID, req.TargetUserID, ctx.ClientIP(), ctx.Request.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAlreadyImpersonating):
			ctx.JSON(http.StatusConflict, gin.H{"error": "already_impersonating"})
		case errors.Is(err, services.ErrSelfImpersonation):
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "self_impersonation_forbidden"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "start_failed"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, StartImpersonationResponse{
		SessionID:    session.ID.String(),
		TargetUserID: session.TargetID,
		StartedAt:    session.StartedAt.Format(time.RFC3339Nano),
	})
}

// StopImpersonation handles POST /admin/impersonate/stop.
//
// The handler relies on the upstream ImpersonationMiddleware to populate
// `impersonatorId` on the gin context when the request is part of an active
// impersonation session.
//
// Contract:
//   - 400 not_impersonating — context carries no impersonatorId (the caller
//     is not currently impersonating anyone).
//   - 404 no_active_session — context says we are impersonating, but the DB
//     row is gone (e.g. another tab already stopped it).
//   - 500 stop_failed — any other service error.
//   - 204 No Content on success.
func (c *Controller) StopImpersonation(ctx *gin.Context) {
	impersonatorID := ctx.GetString("impersonatorId")
	if impersonatorID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "not_impersonating"})
		return
	}

	if err := c.Service.StopSession(impersonatorID, "manual"); err != nil {
		switch {
		case errors.Is(err, services.ErrNoActiveSession):
			ctx.JSON(http.StatusNotFound, gin.H{"error": "no_active_session"})
		default:
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "stop_failed"})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}

// GetActiveImpersonation handles GET /admin/impersonate/active.
//
// The lookup uses the calling user's id (the admin), not the impersonation
// target — admins must call this WITHOUT the X-Impersonate-User header to
// see their own active session.
//
// Contract:
//   - 204 No Content — no active session for the caller.
//   - 500 lookup_failed — any service error other than ErrNoActiveSession.
//   - 200 + ActiveImpersonationResponse on success.
func (c *Controller) GetActiveImpersonation(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	session, err := c.Service.GetActiveSession(userID)
	if err != nil {
		if errors.Is(err, services.ErrNoActiveSession) {
			ctx.Status(http.StatusNoContent)
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
		return
	}

	ctx.JSON(http.StatusOK, ActiveImpersonationResponse{
		SessionID:      session.ID.String(),
		TargetUserID:   session.TargetID,
		StartedAt:      session.StartedAt.Format(time.RFC3339Nano),
		LastActivityAt: session.LastActivityAt.Format(time.RFC3339Nano),
	})
}
