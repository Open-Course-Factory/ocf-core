package dto

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

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
	Name   *string `json:"name,omitempty" mapstructure:"name"`
	Status *string `json:"status,omitempty" mapstructure:"status"`
}

type TerminalOutput struct {
	ID              uuid.UUID  `json:"id"`
	SessionID       string     `json:"session_id"`
	UserID          string     `json:"user_id"`
	Name            string     `json:"name"` // User-friendly name for the terminal session
	Status          string     `json:"status"`
	ExpiresAt       time.Time  `json:"expires_at"`
	InstanceType    string     `json:"instance_type"`
	MachineSize     string     `json:"machine_size"` // XS, S, M, L, XL
	Backend         string     `json:"backend,omitempty"`
	OrganizationID  *uuid.UUID `json:"organization_id,omitempty"`
	IsHiddenByOwner bool       `json:"is_hidden_by_owner"`
	HiddenByOwnerAt *time.Time `json:"hidden_by_owner_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
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

// TerminalShare DTOs for generic system
type CreateTerminalShareInput struct {
	TerminalID        uuid.UUID  `binding:"required" json:"terminal_id"`
	SharedWithUserID  *string    `json:"shared_with_user_id,omitempty"`   // Share to specific user
	SharedWithGroupID *uuid.UUID `json:"shared_with_group_id,omitempty"`  // OR share to group
	AccessLevel       string     `binding:"required" json:"access_level"` // read, write, owner
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

type UpdateTerminalShareInput struct {
	AccessLevel *string    `json:"access_level,omitempty" mapstructure:"access_level"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" mapstructure:"expires_at"`
	IsActive    *bool      `json:"is_active,omitempty" mapstructure:"is_active"`
}

type TerminalShareOutput struct {
	ID                  uuid.UUID  `json:"id"`
	TerminalID          uuid.UUID  `json:"terminal_id"`
	SharedWithUserID    *string    `json:"shared_with_user_id,omitempty"`
	SharedWithGroupID   *uuid.UUID `json:"shared_with_group_id,omitempty"`
	SharedByUserID      string     `json:"shared_by_user_id"`
	AccessLevel         string     `json:"access_level"`
	ShareType           string     `json:"share_type"` // "user" or "group"
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	IsActive            bool       `json:"is_active"`
	IsHiddenByRecipient bool       `json:"is_hidden_by_recipient"`
	HiddenAt            *time.Time `json:"hidden_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

// Terminal sharing specific DTOs
type ShareTerminalRequest struct {
	SharedWithUserID  *string    `json:"shared_with_user_id,omitempty"`
	SharedWithGroupID *uuid.UUID `json:"shared_with_group_id,omitempty"`
	AccessLevel       string     `binding:"required" json:"access_level"` // read, write, owner
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

type SharedTerminalInfo struct {
	Terminal            TerminalOutput        `json:"terminal"`
	SharedBy            string                `json:"shared_by"`
	SharedByDisplayName string                `json:"shared_by_display_name"`
	AccessLevel         string                `json:"access_level"`
	ExpiresAt           *time.Time            `json:"expires_at,omitempty"`
	SharedAt            time.Time             `json:"shared_at"`
	Shares              []TerminalShareOutput `json:"shares,omitempty"`
}

// Terminal Service DTOs (pour les appels au Terminal Trainer)
type CreateTerminalSessionInput struct {
	Terms                string `binding:"required" json:"terms" form:"terms"`
	Name                 string `json:"name,omitempty" form:"name"`                   // User-friendly name for the terminal session
	Expiry               int    `json:"expiry,omitempty" form:"expiry"`               // optionnel
	InstanceType         string `json:"instance_type,omitempty" form:"instance_type"` // préfixe du type d'instance
	Backend              string `json:"backend,omitempty" form:"backend"`             // Backend ID to use
	OrganizationID       string `json:"organization_id,omitempty" form:"organization_id"`
	HistoryRetentionDays int    `json:"history_retention_days,omitempty" form:"history_retention_days"`
	RecordingConsent     int    `json:"recording_consent,omitempty" form:"recording_consent"` // 1 = learner accepted recording
	ExternalRef          string `json:"external_ref,omitempty" form:"external_ref"`           // Optional training session reference
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

// SyncSessionResponse représente le résultat de synchronisation d'une session
type SyncSessionResponse struct {
	SessionID      string    `json:"session_id"`
	Action         string    `json:"action"` // "created", "updated", "expired", "no_change"
	PreviousStatus string    `json:"previous_status"`
	CurrentStatus  string    `json:"current_status"`
	Updated        bool      `json:"updated"`
	Source         string    `json:"source"` // "api", "local", "both"
	ErrorMessage   string    `json:"error_message,omitempty"`
	LastSyncAt     time.Time `json:"last_sync_at"`
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

// InstanceType représente un type d'instance disponible pour les clients
type InstanceType struct {
	Name        string `json:"name"`
	Prefix      string `json:"prefix"`
	Description string `json:"description"`
	Size        string `json:"size"` // XS, S, M, L, XL, etc.
}

// InstanceTypesResponse réponse contenant la liste des types d'instances
type InstanceTypesResponse struct {
	InstanceTypes []InstanceType `json:"instance_types"`
}

// FixPermissionsResponse réponse de la correction des permissions de masquage
type FixPermissionsResponse struct {
	UserID             string   `json:"user_id"`
	Success            bool     `json:"success"`
	Message            string   `json:"message"`
	ProcessedTerminals int      `json:"processed_terminals"`
	ProcessedShares    int      `json:"processed_shares"`
	Errors             []string `json:"errors,omitempty"`
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
	RecordingConsent int    `json:"recording_consent,omitempty"`
	ExternalRef      string `json:"external_ref,omitempty"` // Optional exercise/training session reference
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
