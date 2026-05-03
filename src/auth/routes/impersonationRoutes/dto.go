package impersonationRoutes

// StartImpersonationRequest is the JSON body accepted by
// POST /admin/impersonate/start.
type StartImpersonationRequest struct {
	TargetUserID string `json:"target_user_id" binding:"required"`
}

// StartImpersonationResponse is the JSON body returned on a successful
// POST /admin/impersonate/start.
type StartImpersonationResponse struct {
	SessionID    string `json:"session_id"`
	TargetUserID string `json:"target_user_id"`
	StartedAt    string `json:"started_at"`
}

// ActiveImpersonationResponse is the JSON body returned by
// GET /admin/impersonate/active when an active session exists.
type ActiveImpersonationResponse struct {
	SessionID      string `json:"session_id"`
	TargetUserID   string `json:"target_user_id"`
	StartedAt      string `json:"started_at"`
	LastActivityAt string `json:"last_activity_at"`
}
