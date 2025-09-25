package dto

import (
	"time"

	"github.com/google/uuid"
)

// Terminal DTOs for generic system
type CreateTerminalInput struct {
	SessionID            string    `binding:"required" json:"session_id"`
	UserID               string    `binding:"required" json:"user_id"`
	ExpiresAt            time.Time `binding:"required" json:"expires_at"`
	TerminalTrainerKeyID uuid.UUID `binding:"required" json:"terminal_trainer_key_id"`
}

type UpdateTerminalInput struct {
	Status string `json:"status,omitempty"`
}

type TerminalOutput struct {
	ID           uuid.UUID `json:"id"`
	SessionID    string    `json:"session_id"`
	UserID       string    `json:"user_id"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expires_at"`
	InstanceType string    `json:"instance_type"`
	CreatedAt    time.Time `json:"created_at"`
}

// UserTerminalKey DTOs for generic system
type CreateUserTerminalKeyInput struct {
	UserID      string `binding:"required" json:"user_id"`
	KeyName     string `binding:"required" json:"key_name"`
	MaxSessions int    `json:"max_sessions"`
}

type UpdateUserTerminalKeyInput struct {
	KeyName     string `json:"key_name,omitempty"`
	IsActive    *bool  `json:"is_active,omitempty"`
	MaxSessions *int   `json:"max_sessions,omitempty"`
}

type UserTerminalKeyOutput struct {
	ID          uuid.UUID `json:"id"`
	UserID      string    `json:"user_id"`
	KeyName     string    `json:"key_name"`
	IsActive    bool      `json:"is_active"`
	MaxSessions int       `json:"max_sessions"`
	CreatedAt   time.Time `json:"created_at"`
}

// Terminal Service DTOs (pour les appels au Terminal Trainer)
type CreateTerminalSessionInput struct {
	Terms        string `binding:"required" json:"terms" form:"terms"`
	Expiry       int    `json:"expiry,omitempty" form:"expiry"`        // optionnel
	InstanceType string `json:"instance_type,omitempty" form:"instance_type"` // préfixe du type d'instance
}

type TerminalSessionResponse struct {
	SessionID  string    `json:"session_id"`
	ExpiresAt  time.Time `json:"expires_at"`
	ConsoleURL string    `json:"console_url"`
	Status     string    `json:"status"`
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

type TerminalTrainerSessionResponse struct {
	SessionID string `json:"id,omitempty"`
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expires_at,omitempty"` // timestamp Unix
	CreatedAt int64  `json:"created_at,omitempty"`
}

// TerminalTrainerSession représente une session retournée par l'endpoint /1.0/sessions
type TerminalTrainerSession struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedAt int64  `json:"created_at"`
	IP        string `json:"ip"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	FQDN      string `json:"fqdn,omitempty"`
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
	SessionID string `json:"id"`
	Status    string `json:"status"`
	ExpiresAt int64  `json:"expiry,omitempty"`
	StartedAt int64  `json:"started_at,omitempty"`
	IP        string `json:"ip,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	FQDN      string `json:"fqdn,omitempty"`
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
}

// InstanceTypesResponse réponse contenant la liste des types d'instances
type InstanceTypesResponse struct {
	InstanceTypes []InstanceType `json:"instance_types"`
}
