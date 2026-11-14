package dto

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

// ParticipantBlockView is the participant's block in the class, as the
// trainer sees it: its reason only.
type ParticipantBlockView struct {
	Reason string `json:"reason"`
}

// ParticipantSessionView is one running or stopped session of a participant
// in the class's organization. ScenarioSessionID is set while a scenario run
// is open on the session.
type ParticipantSessionView struct {
	SessionID         string  `json:"session_id"`
	State             string  `json:"state"`
	NetworkFeature    bool    `json:"network_feature"`
	NetworkDisabled   bool    `json:"network_disabled"`
	PersistenceMode   string  `json:"persistence_mode"`
	ScenarioSessionID *string `json:"scenario_session_id,omitempty"`
}

// ParticipantControlsRow is one active member of the class, as
// GET /class-groups/:id/participant-controls lists them: their block in this
// class, whether they are blocked in its organization or on the platform,
// whether their internet is cut in this class, and their sessions.
type ParticipantControlsRow struct {
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	// Role is the participant's role in the class: "member", "manager" or
	// "owner". Trainer actions apply to members only (a platform
	// administrator excepted).
	Role        string                   `json:"role"`
	ClassBlock  *ParticipantBlockView    `json:"class_block"`
	OrgBlocked  bool                     `json:"org_blocked"`
	GlobalBlock bool                     `json:"global_block"`
	NetworkCut  bool                     `json:"network_cut"`
	Sessions    []ParticipantSessionView `json:"sessions"`
}
