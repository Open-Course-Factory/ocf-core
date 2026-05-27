package dto

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	paymentDto "soli/formations/src/payment/dto"

	"github.com/google/uuid"
)

// Terminal DTOs for generic system
type CreateTerminalInput struct {
	SessionID            string    `binding:"required" json:"session_id"`
	UserID               string    `binding:"required" json:"user_id"`
	Name                 string    `json:"name"` // User-friendly name for the terminal session
	ExpiresAt            time.Time `binding:"required" json:"expires_at"`
	TerminalTrainerKeyID uuid.UUID `binding:"required" json:"terminal_trainer_key_id"`
}

type UpdateTerminalInput struct {
	Name *string `json:"name,omitempty" mapstructure:"name"`
	// State is the lifecycle field (running, stopped, hibernating, deleted, etc.).
	// SSOT — the legacy `status` field has been removed.
	State *string `json:"state,omitempty" mapstructure:"state"`
	// PersistenceMode allows switching a session between ephemeral and persistent
	// when the plan permits it. Values: "ephemeral", "persistent".
	PersistenceMode *string `json:"persistence_mode,omitempty" mapstructure:"persistence_mode"`
}

type TerminalOutput struct {
	ID              uuid.UUID  `json:"id"`
	SessionID       string     `json:"session_id"`
	UserID          string     `json:"user_id"`
	Name            string     `json:"name"` // User-friendly name for the terminal session
	// State is the session lifecycle field (running, stopped, hibernating, deleted, etc.).
	// SSOT — the legacy `status` field has been removed from the wire contract.
	State           string     `json:"state"`
	// PersistenceMode is "ephemeral" or "persistent".
	PersistenceMode string     `json:"persistence_mode,omitempty"`
	// IdleUntil is the absolute deadline after which the session may be reaped or
	// hibernated. Nil = no idle policy currently applies.
	IdleUntil       *time.Time `json:"idle_until,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at"`
	InstanceType    string     `json:"instance_type"`
	MachineSize     string     `json:"machine_size"` // XS, S, M, L, XL
	Backend            string     `json:"backend,omitempty"`
	OrganizationID     *uuid.UUID `json:"organization_id,omitempty"`
	SubscriptionPlanID *uuid.UUID `json:"subscription_plan_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// UserTerminalKey DTOs for generic system
type CreateUserTerminalKeyInput struct {
	UserID      string `binding:"required" json:"user_id"`
	KeyName     string `binding:"required" json:"key_name"`
	MaxSessions int    `json:"max_sessions"`
}

type UpdateUserTerminalKeyInput struct {
	KeyName     *string `json:"key_name,omitempty" mapstructure:"key_name"`
	IsActive    *bool   `json:"is_active,omitempty" mapstructure:"is_active"`
	MaxSessions *int    `json:"max_sessions,omitempty" mapstructure:"max_sessions"`
}

type UserTerminalKeyOutput struct {
	ID          uuid.UUID `json:"id"`
	UserID      string    `json:"user_id"`
	KeyName     string    `json:"key_name"`
	IsActive    bool      `json:"is_active"`
	MaxSessions int       `json:"max_sessions"`
	CreatedAt   time.Time `json:"created_at"`
}


type TerminalSessionResponse struct {
	SessionID  string    `json:"session_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	ConsoleURL string    `json:"console_url"`
	Status     string    `json:"status"`
	Backend    string    `json:"backend,omitempty"`
}

// Response du Terminal Trainer API (pour mapping)
type TerminalTrainerAPIKeyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		ID                    int64  `json:"id"`
		KeyValue              string `json:"key_value"`
		Name                  string `json:"name"`
		IsAdmin               bool   `json:"is_admin"`
		IsActive              bool   `json:"is_active"`
		CreatedAt             int64  `json:"created_at"`
		UpdatedAt             int64  `json:"updated_at"`
		LastUsedAt            *int64 `json:"last_used_at"`
		MaxConcurrentSessions int    `json:"max_concurrent_sessions"`
	} `json:"data"`
	Message string `json:"message"`
}

// FlexibleInt is a custom type that can unmarshal both string and int JSON values
type FlexibleInt int

// UnmarshalJSON implements custom JSON unmarshaling to handle both string and int
func (fi *FlexibleInt) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*fi = FlexibleInt(i)
		return nil
	}

	// If that fails, try as string and convert to int
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("status must be either int or string: %v", err)
	}

	// Convert string to int
	i, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("cannot convert status string to int: %v", err)
	}

	*fi = FlexibleInt(i)
	return nil
}

type TerminalTrainerSessionResponse struct {
	SessionID   string      `json:"id,omitempty"`
	Status      FlexibleInt `json:"status"`               // 0 = success, non-zero = error (can be string or int)
	ExpiresAt   int64       `json:"expires_at,omitempty"` // timestamp Unix
	CreatedAt   int64       `json:"created_at,omitempty"`
	MachineSize string      `json:"machine_size,omitempty"` // XS, S, M, L, XL
	Backend     string      `json:"backend,omitempty"`
}

// TerminalTrainerSession représente une session retournée par l'endpoint /1.0/sessions
type TerminalTrainerSession struct {
	ID          string      `json:"id"`
	SessionID   string      `json:"session_id"`
	Name        string      `json:"name"`
	Status      FlexibleInt `json:"status"` // Terminal Trainer returns integer status values
	ExpiresAt   int64       `json:"expires_at"`
	CreatedAt   int64       `json:"created_at"`
	IP          string      `json:"ip"`
	Username    string      `json:"username,omitempty"`
	Password    string      `json:"password,omitempty"`
	FQDN        string      `json:"fqdn,omitempty"`
	MachineSize string      `json:"machine_size,omitempty"` // XS, S, M, L, XL
	Backend     string      `json:"backend,omitempty"`
	// State is the lifecycle field driven by tt-backend (running, stopped,
	// hibernating, ...). tt-backend is the source of truth; ocf-core caches it.
	State string `json:"state,omitempty" mapstructure:"state"`
	// PersistenceMode is "ephemeral" or "persistent". A stopped persistent
	// session is still resumable and the frontend uses this to decide between
	// a "Resume" affordance and a "Session expirée" message.
	PersistenceMode string `json:"persistence_mode,omitempty" mapstructure:"persistence_mode"`
	// IdleUntil is the absolute deadline (Unix seconds) after which the session
	// may be reaped or hibernated by tt-backend.
	IdleUntil int64 `json:"idle_until,omitempty" mapstructure:"idle_until"`
}

// TerminalTrainerSessionsResponse réponse de l'endpoint /1.0/sessions
type TerminalTrainerSessionsResponse struct {
	Sessions       []TerminalTrainerSession `json:"sessions"`
	Count          int                      `json:"count"`
	APIKeyID       int64                    `json:"api_key_id"`
	IncludeExpired bool                     `json:"include_expired"`
	Limit          int                      `json:"limit"`
}

// TerminalTrainerSessionInfo informations détaillées d'une session depuis /1.0/info
type TerminalTrainerSessionInfo struct {
	SessionID   string      `json:"id"`
	Status      FlexibleInt `json:"status"` // Terminal Trainer returns integer status values
	ExpiresAt   int64       `json:"expiry,omitempty"`
	StartedAt   int64       `json:"started_at,omitempty"`
	IP          string      `json:"ip,omitempty"`
	Username    string      `json:"username,omitempty"`
	Password    string      `json:"password,omitempty"`
	FQDN        string      `json:"fqdn,omitempty"`
	MachineSize string      `json:"machine_size,omitempty"` // XS, S, M, L, XL
	Backend     string      `json:"backend,omitempty"`
}

// SyncSessionResponse représente le résultat de synchronisation d'une session.
// PreviousState/CurrentState carry the SSOT state names (running, stopped,
// deleted, ...). The legacy `previous_status`/`current_status` JSON keys were
// renamed to make the namespace explicit.
type SyncSessionResponse struct {
	SessionID    string    `json:"session_id"`
	Action       string    `json:"action"` // "created", "updated", "expired", "no_change"
	PreviousState string    `json:"previous_state"`
	CurrentState  string    `json:"current_state"`
	Updated      bool      `json:"updated"`
	Source       string    `json:"source"` // "api", "local", "both"
	ErrorMessage string    `json:"error_message,omitempty"`
	LastSyncAt   time.Time `json:"last_sync_at"`
}

// SyncAllSessionsResponse représente la réponse de synchronisation de toutes les sessions
type SyncAllSessionsResponse struct {
	TotalSessions   int                   `json:"total_sessions"`
	SyncedSessions  int                   `json:"synced_sessions"`
	UpdatedSessions int                   `json:"updated_sessions"`
	CreatedSessions int                   `json:"created_sessions"`
	ExpiredSessions int                   `json:"expired_sessions"`
	ErrorCount      int                   `json:"error_count"`
	Errors          []string              `json:"errors,omitempty"`
	SessionResults  []SyncSessionResponse `json:"session_results"`
	LastSyncAt      time.Time             `json:"last_sync_at"`
}

// ExtendedSessionStatusResponse pour les vérifications de statut détaillées
type ExtendedSessionStatusResponse struct {
	SessionID   string    `json:"session_id"`
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	LastChecked time.Time `json:"last_checked"`

	// Statuts comparés
	LocalStatus  string    `json:"local_status"`
	APIStatus    string    `json:"api_status"`
	APIExpiresAt time.Time `json:"api_expires_at,omitempty"`
	APIError     string    `json:"api_error,omitempty"`

	// État de la synchronisation
	ExistsInAPI     bool `json:"exists_in_api"`
	ExistsLocally   bool `json:"exists_locally"`
	SyncRecommended bool `json:"sync_recommended"`
	StatusMatch     bool `json:"status_match"`
}

// SessionStatusRequest pour les requêtes de vérification de statut
type SessionStatusRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

// SessionStatusResponse pour les réponses de statut simple
type SessionStatusResponse struct {
	SessionID   string    `json:"session_id"`
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	LastChecked time.Time `json:"last_checked"`
	APIStatus   string    `json:"api_status,omitempty"`
	LocalStatus string    `json:"local_status,omitempty"`
	NeedSync    bool      `json:"need_sync"`
}

// SyncStatisticsResponse pour les statistiques de synchronisation
type SyncStatisticsResponse struct {
	UserID      string    `json:"user_id,omitempty"`
	GeneratedAt time.Time `json:"generated_at"`

	// Compteurs par statut
	SessionsByStatus map[string]int `json:"sessions_by_status"`
	TotalSessions    int            `json:"total_sessions"`
	ActiveSessions   int            `json:"active_sessions"`
	ExpiredSessions  int            `json:"expired_sessions"`

	// Informations de synchronisation
	LastSyncAt    *time.Time `json:"last_sync_at,omitempty"`
	SyncFrequency string     `json:"sync_frequency"`

	// Utilisation des ressources
	APIKeyInfo APIKeyUsageInfo `json:"api_key_info"`
}

// APIKeyUsageInfo informations sur l'utilisation de la clé API
type APIKeyUsageInfo struct {
	KeyID           int64     `json:"key_id"`
	IsActive        bool      `json:"is_active"`
	MaxSessions     int       `json:"max_sessions"`
	CurrentSessions int       `json:"current_sessions"`
	UsagePercentage float64   `json:"usage_percentage"`
	LastUsed        time.Time `json:"last_used,omitempty"`
}

// CompareSessionsRequest pour comparer les sessions entre local et API
type CompareSessionsRequest struct {
	UserID         string `json:"user_id,omitempty"`
	IncludeExpired bool   `json:"include_expired"`
}

// CompareSessionsResponse résultat de la comparaison
type CompareSessionsResponse struct {
	UserID              string    `json:"user_id"`
	ComparisonTimestamp time.Time `json:"comparison_timestamp"`

	// Résultats de comparaison
	OnlyInAPI      []SessionComparison `json:"only_in_api"`
	OnlyLocal      []SessionComparison `json:"only_local"`
	StatusMismatch []SessionComparison `json:"status_mismatch"`
	InSync         []SessionComparison `json:"in_sync"`

	// Statistiques
	APISessions     int  `json:"api_sessions"`
	LocalSessions   int  `json:"local_sessions"`
	SyncRecommended bool `json:"sync_recommended"`
}

// SessionComparison détails de comparaison d'une session
type SessionComparison struct {
	SessionID      string    `json:"session_id"`
	LocalStatus    string    `json:"local_status,omitempty"`
	APIStatus      string    `json:"api_status,omitempty"`
	LocalExpiry    time.Time `json:"local_expiry,omitempty"`
	APIExpiry      time.Time `json:"api_expiry,omitempty"`
	Recommendation string    `json:"recommendation"` // "create_local", "update_local", "expire_local", "no_action"
}

// BatchSyncRequest pour les synchronisations par lot
type BatchSyncRequest struct {
	SessionIDs []string `json:"session_ids"`
	Action     string   `json:"action"` // "sync", "expire", "force_update"
	DryRun     bool     `json:"dry_run"`
}

// BatchSyncResponse résultat de synchronisation par lot
type BatchSyncResponse struct {
	RequestID      string            `json:"request_id"`
	Action         string            `json:"action"`
	DryRun         bool              `json:"dry_run"`
	ProcessedCount int               `json:"processed_count"`
	SuccessCount   int               `json:"success_count"`
	ErrorCount     int               `json:"error_count"`
	Results        []BatchSyncResult `json:"results"`
	StartedAt      time.Time         `json:"started_at"`
	CompletedAt    time.Time         `json:"completed_at"`
	Duration       time.Duration     `json:"duration"`
}

// BatchSyncResult résultat pour une session dans un lot
type BatchSyncResult struct {
	SessionID    string `json:"session_id"`
	Success      bool   `json:"success"`
	Action       string `json:"action"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// FullSyncRequest pour les requêtes de synchronisation complète
type FullSyncRequest struct {
	UserID         string `json:"user_id,omitempty"`
	IncludeExpired bool   `json:"include_expired"`
	DryRun         bool   `json:"dry_run"`
}

// FullSyncResponse réponse détaillée d'une synchronisation complète
type FullSyncResponse struct {
	RequestID string `json:"request_id"`
	UserID    string `json:"user_id"`
	SyncType  string `json:"sync_type"`
	DryRun    bool   `json:"dry_run"`

	// Statistiques générales
	TotalSessions   int `json:"total_sessions"`
	SyncedSessions  int `json:"synced_sessions"`
	UpdatedSessions int `json:"updated_sessions"`
	CreatedSessions int `json:"created_sessions"`
	ExpiredSessions int `json:"expired_sessions"`
	ErrorCount      int `json:"error_count"`

	// Détails par session
	SessionResults []SyncSessionResponse `json:"session_results"`
	Errors         []string              `json:"errors,omitempty"`

	// Informations sur l'API
	APISessionCount int           `json:"api_session_count"`
	APIReachable    bool          `json:"api_reachable"`
	APIResponseTime time.Duration `json:"api_response_time"`

	// Timing
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at"`
	Duration    time.Duration `json:"duration"`
}

// ServerMetricsResponse représente les métriques du serveur Terminal Trainer
type ServerMetricsResponse struct {
	CPUPercent     float64 `json:"cpu_percent"`
	RAMPercent     float64 `json:"ram_percent"`
	RAMAvailableGB float64 `json:"ram_available_gb"`
	Timestamp      int64   `json:"timestamp"`
	Backend        string  `json:"backend,omitempty"`
}

// BulkCreateTerminalsRequest for creating multiple terminals for a group
type BulkCreateTerminalsRequest struct {
	Terms          string `binding:"required" json:"terms"`
	Expiry         int    `json:"expiry,omitempty"`
	InstanceType   string `json:"instance_type,omitempty"`
	NameTemplate   string `json:"name_template,omitempty"` // Template with placeholders: {group_name}, {user_email}, {user_id}
	Backend          string `json:"backend,omitempty"`
	OrganizationID   string `json:"organization_id,omitempty"`
	RecordingEnabled int    `json:"recording_enabled,omitempty"`
	ExternalRef      string `json:"external_ref,omitempty"` // Optional exercise/training session reference
	Hostname         string `json:"hostname,omitempty"`
}

// BulkCreateTerminalsResponse response for bulk terminal creation
type BulkCreateTerminalsResponse struct {
	Success      bool                         `json:"success"`
	CreatedCount int                          `json:"created_count"`
	FailedCount  int                          `json:"failed_count"`
	TotalMembers int                          `json:"total_members"`
	Terminals    []BulkTerminalCreationResult `json:"terminals"`
	Errors       []string                     `json:"errors,omitempty"`
}

// BulkTerminalCreationResult individual result for each terminal created
type BulkTerminalCreationResult struct {
	UserID     string  `json:"user_id"`
	UserEmail  string  `json:"user_email,omitempty"`
	TerminalID *string `json:"terminal_id,omitempty"`
	SessionID  *string `json:"session_id,omitempty"`
	Name       string  `json:"name"`
	Success    bool    `json:"success"`
	Error      string  `json:"error,omitempty"`
}

// EnumValue represents a single enum value with its metadata
type EnumValue struct {
	Value       int    `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// EnumDefinition represents a complete enum type
type EnumDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Values      []EnumValue `json:"values"`
}

// TerminalTrainerEnumsResponse response from /1.0/enums endpoint
type TerminalTrainerEnumsResponse struct {
	Enums []EnumDefinition `json:"enums"`
}

// BackendInfo represents a Terminal Trainer backend
type BackendInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Connected   bool   `json:"connected"`
	IsDefault   bool   `json:"is_default"`
}

// EnumServiceStatus represents the status of the enum service
type EnumServiceStatus struct {
	Initialized   bool     `json:"initialized"`
	LastFetch     time.Time `json:"last_fetch,omitempty"`
	Source        string    `json:"source"` // "local", "api", "mixed"
	EnumCount     int       `json:"enum_count"`
	HasMismatches bool      `json:"has_mismatches"`
	Mismatches    []string  `json:"mismatches,omitempty"`
}

// --- Composed Session DTOs (Phase 4) ---

// CreateComposedSessionInput is the request body for POST /terminals/start-composed-session
type CreateComposedSessionInput struct {
	Distribution     string          `binding:"required" json:"distribution"`
	Size             string          `binding:"required" json:"size"`
	Features         map[string]bool `json:"features"`
	Terms            string          `binding:"required" json:"terms"`
	Name             string          `json:"name,omitempty"`
	Expiry           int             `json:"expiry,omitempty"`
	Backend          string          `json:"backend,omitempty"`
	OrganizationID   string          `json:"organization_id,omitempty"`
	Hostname         string          `json:"hostname,omitempty"`
	Packages         []string        `json:"packages,omitempty"`
	RecordingEnabled int             `json:"recording_enabled,omitempty"`
	ExternalRef      string          `json:"external_ref,omitempty"`
	// PersistenceMode requests an "ephemeral" (default) or "persistent" session.
	// Tier enforcement (free tier cannot request persistent) is applied by
	// StartComposedSession. The value is forwarded to tt-backend.
	PersistenceMode string `json:"persistence_mode,omitempty"`
	// Set by service layer
	HistoryRetentionDays   int        `json:"-"`
	SubscriptionPlanID     *uuid.UUID `json:"-"`
	DistributionPrefix     string     `json:"-"` // Resolved from TTDistribution.Prefix
	// IdleWindowSeconds is the org-level idle window override forwarded to
	// tt-backend (nil = let tt-backend use its global default for the mode).
	// Set by the service layer based on the resolved persistence mode + org
	// override; not accepted from the public request body.
	IdleWindowSeconds *int `json:"-"`
	// SizeCPU / SizeMemoryMB are snapshot from the size catalog by the
	// service layer so the persisted Terminal row carries its budget
	// footprint without a downstream catalog lookup. Only meaningful when
	// the size key was resolvable in the catalog; zero otherwise (legacy
	// behaviour). Mirrors what TerminalBudgetHook does on the generic
	// Create path.
	SizeCPU      int `json:"-"`
	SizeMemoryMB int `json:"-"`
}

// TTDistribution mirrors tt-backend's Distribution struct
type TTDistribution struct {
	Name              string   `json:"name"`
	Prefix            string   `json:"prefix"`
	Description       string   `json:"description"`
	OsType            string   `json:"os_type,omitempty"`
	IsGlobal          bool     `json:"is_global"`
	MinSizeKey        string   `json:"min_size_key,omitempty"`
	DefaultSizeKey    string   `json:"default_size_key,omitempty"`
	SupportedFeatures []string `json:"supported_features,omitempty"`
}

// TTSize mirrors tt-backend's Size struct
type TTSize struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	CPU          int    `json:"cpu"`
	CPUAllowance string `json:"cpu_allowance"`
	Memory       string `json:"memory"`
	Disk         string `json:"disk"`
	Processes    int    `json:"processes"`
	SortOrder    int    `json:"sort_order"`
}

// TTFeature mirrors tt-backend's Feature struct
type TTFeature struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ProfileName    string `json:"profile_name"`
	MinSizeKey     string `json:"min_size_key,omitempty"`
	DefaultEnabled bool   `json:"default_enabled"`
	SortOrder      int    `json:"sort_order"`
}

// SessionOptionsResponse is returned by GET /terminals/session-options.
//
// Budget fields (Quota, AllowedSizes[i].RemainingCount, AllowedSizes[i].MemoryMB)
// reflect the user's (or org's) current CPU/RAM footprint against the
// effective plan's caps. Plans with zero caps report Quota.Scope="unlimited"
// and per-size RemainingCount=0 — the frontend renders an unconstrained UI
// in that case.
type SessionOptionsResponse struct {
	Distribution    TTDistribution         `json:"distribution"`
	AllowedSizes    []SessionOptionSize    `json:"allowed_sizes"`
	AllowedFeatures []SessionOptionFeature `json:"allowed_features"`
	// Quota carries the user's (or org's) budget snapshot. Always present
	// so the frontend can render generically; Scope="unlimited" signals
	// "no budget enforcement, ignore the numeric fields".
	Quota *SessionQuotaInfo `json:"quota,omitempty"`
}

// SessionQuotaInfo is the budget snapshot embedded in session-options and
// org-usage responses. MaxCPU / MaxMemoryMB of 0 paired with Scope="unlimited"
// means the plan has no budget cap (or the feature flag is off).
type SessionQuotaInfo struct {
	MaxCPU            int    `json:"max_cpu"`
	MaxMemoryMB       int    `json:"max_memory_mb"`
	UsedCPU           int    `json:"used_cpu"`
	UsedMemoryMB      int    `json:"used_memory_mb"`
	RemainingCPU      int    `json:"remaining_cpu"`
	RemainingMemoryMB int    `json:"remaining_memory_mb"`
	Scope             string `json:"scope"` // "user" | "organization" | "unlimited"
}

// SessionOptionSize describes a size with its allowed status. RemainingCount
// and MemoryMB are populated in budget mode so the frontend can disable
// sizes whose remaining count is 0 and skip re-parsing the Memory string.
type SessionOptionSize struct {
	TTSize
	Allowed        bool   `json:"allowed"`
	Reason         string `json:"reason,omitempty"`
	RemainingCount int    `json:"remaining_count"`
	MemoryMB       int    `json:"memory_mb"`
}

// SessionOptionFeature describes a feature with its allowed status (no ProfileName exposed)
type SessionOptionFeature struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Allowed     bool   `json:"allowed"`
	Reason      string `json:"reason,omitempty"`
}

// CommandHistoryEntry represents a single command from a terminal session
type CommandHistoryEntry struct {
	SequenceNum int    `json:"sequence_num"`
	CommandText string `json:"command_text"`
	ExecutedAt  int64  `json:"executed_at"`
}

// CommandHistoryResponse wraps the history entries list
type CommandHistoryResponse struct {
	SessionID string                `json:"session_id"`
	Commands  []CommandHistoryEntry `json:"commands"`
	Count     int                   `json:"count"`
	Limit     int                   `json:"limit,omitempty"`
	Offset    int                   `json:"offset,omitempty"`
}

// OrgTerminalUsageUser holds per-user terminal counts for an organization.
//
// ActiveCount reports running sessions only (state='running'). OccupyingSlots
// reports every session that still consumes a slot under the canonical rule
// in models.TerminalStatesOccupyingSlot (currently running+stopped). The two
// values differ because a stopped session keeps disk and a quota slot until
// it is deleted — see models.OccupiesSlotScope for the single source of truth.
//
// ActiveCPU / ActiveMemoryMB sum the budget-counted resources for this user
// (state='running' OR persistence_mode='persistent', not past expiry). In
// count-mode they are reported alongside the slot count for parity but
// quotas are still enforced by the slot counter.
type OrgTerminalUsageUser struct {
	UserID         string `json:"user_id"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email"`
	ActiveCount    int    `json:"active_count"`
	OccupyingSlots int    `json:"occupying_slots"`
	ActiveCPU      int    `json:"active_cpu"`
	ActiveMemoryMB int    `json:"active_memory_mb"`
}

// SizeRemainingDTO is an alias to paymentDto.SizeRemaining — the
// canonical shape lives in src/payment/dto/sizeRemaining.go. The alias
// preserves existing call-site spellings while removing the duplicate.
type SizeRemainingDTO = paymentDto.SizeRemaining

// OrgTerminalUsageResponse is returned by GET /organizations/:id/terminal-usage.
//
// ActiveTerminals is the "what's running right now" display count. OccupyingSlots
// is the quota-relevant count and matches MaxTerminals semantics (stopped sessions
// still occupy a slot — see models.TerminalStatesOccupyingSlot). Both are
// surfaced so the dashboard can show "5 running / 7 slots used / 10 max".
//
// Budget fields (Quota, RemainingBySize) reflect the org's current CPU/RAM
// footprint against the effective plan's caps. Plans with zero caps report
// Quota.Scope="unlimited" and an empty RemainingBySize slice — the dashboard
// renders an unconstrained view in that case.
type OrgTerminalUsageResponse struct {
	OrganizationID  string                 `json:"organization_id"`
	ActiveTerminals int                    `json:"active_terminals"`
	OccupyingSlots  int                    `json:"occupying_slots"`
	MaxTerminals    int                    `json:"max_terminals"`
	PlanName        string                 `json:"plan_name"`
	IsFallback      bool                   `json:"is_fallback"`
	Users           []OrgTerminalUsageUser `json:"users"`
	Quota           *SessionQuotaInfo      `json:"quota,omitempty"`
	RemainingBySize []SizeRemainingDTO     `json:"remaining_by_size,omitempty"`
}
