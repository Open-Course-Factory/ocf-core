package dto

import "time"

// StopParticipantSessionsInput is the body of
// POST /class-groups/:id/participants/:userId/stop-sessions. Without
// session_ids, every running session of the participant in the class.
type StopParticipantSessionsInput struct {
	SessionIDs []string `json:"session_ids"`
}

// SetParticipantNetworkInput is the body of
// PUT /class-groups/:id/participants/:userId/network. Enabled is required: a
// missing value must not read as "off".
type SetParticipantNetworkInput struct {
	Enabled    *bool    `json:"enabled" binding:"required"`
	SessionIDs []string `json:"session_ids"`
}

// SetSessionBlockInput is the body of the three session-block routes (class,
// organization, platform). StopRunning also stops the participant's running
// sessions in the scope.
type SetSessionBlockInput struct {
	Blocked     *bool  `json:"blocked" binding:"required"`
	Reason      string `json:"reason"`
	StopRunning bool   `json:"stop_running"`
}

// ParticipantBlockView is a session block shown to the trainer.
type ParticipantBlockView struct {
	Reason    string    `json:"reason"`
	BlockedBy string    `json:"blocked_by"`
	CreatedAt time.Time `json:"created_at"`
}

// ParticipantSessionView is one session of a participant in the class.
// The scenario fields are set while a scenario run is open on the session.
type ParticipantSessionView struct {
	SessionID         string  `json:"session_id"`
	State             string  `json:"state"`
	NetworkFeature    bool    `json:"network_feature"`
	NetworkDisabled   bool    `json:"network_disabled"`
	ScenarioSessionID *string `json:"scenario_session_id"`
	ScenarioTitle     *string `json:"scenario_title"`
}

// ParticipantControlsRow is one active member of the class, as
// GET /class-groups/:id/participant-controls lists them: their blocks in this
// class, in its organization and on the platform, and their sessions.
type ParticipantControlsRow struct {
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	// Role is the participant's role in the class: "member", "manager" or
	// "owner". Trainer actions apply to members only (a platform
	// administrator excepted).
	Role        string                   `json:"role"`
	ClassBlock  *ParticipantBlockView    `json:"class_block"`
	OrgBlock    *ParticipantBlockView    `json:"org_block"`
	GlobalBlock bool                     `json:"global_block"`
	Sessions    []ParticipantSessionView `json:"sessions"`
}
