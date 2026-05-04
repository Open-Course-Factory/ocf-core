package impersonationRoutes

// StartImpersonationRequest is the JSON body accepted by
// POST /admin/impersonate/start.
type StartImpersonationRequest struct {
	TargetUserID string `json:"target_user_id" binding:"required"`
}

// TargetUser is a minimal profile of an impersonation target,
// returned alongside the session info on /start so the frontend can
// populate the impersonation banner without a follow-up lookup.
type TargetUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar,omitempty"`
}

// StartImpersonationResponse is the JSON body returned on a successful
// POST /admin/impersonate/start.
type StartImpersonationResponse struct {
	SessionID    string      `json:"session_id"`
	TargetUserID string      `json:"target_user_id"`
	StartedAt    string      `json:"started_at"`
	Target       *TargetUser `json:"target,omitempty"`
}

// ActiveImpersonationResponse is the JSON body returned by
// GET /admin/impersonate/active when an active session exists.
type ActiveImpersonationResponse struct {
	SessionID      string `json:"session_id"`
	TargetUserID   string `json:"target_user_id"`
	StartedAt      string `json:"started_at"`
	LastActivityAt string `json:"last_activity_at"`
}
