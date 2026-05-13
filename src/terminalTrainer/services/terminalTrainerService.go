package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	authModels "soli/formations/src/auth/models"
	groupModels "soli/formations/src/groups/models"
	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

type TerminalTrainerService interface {
	// User key management
	CreateUserKey(userID, userName string) error
	GetUserKey(userID string) (*models.UserTerminalKey, error)
	DisableUserKey(userID string) error

	// Session management
	GetSessionInfo(sessionID string) (*models.Terminal, error)
	GetTerminalByUUID(terminalUUID string) (*models.Terminal, error)
	GetActiveUserSessions(userID string) (*[]models.Terminal, error)
	StopSession(sessionID string) error
	StartSession(sessionID string) error
	DeleteSession(sessionID string) error
	HasTerminalAccess(terminalIDOrSessionID, userID string) (bool, error)

	// Synchronization methods (nouvelle approche avec API comme source de vérité)
	GetAllSessionsFromAPI(userAPIKey string) (*dto.TerminalTrainerSessionsResponse, error)
	SyncUserSessions(userID string) (*dto.SyncAllSessionsResponse, error)
	SyncAllActiveSessions() error
	GetSessionInfoFromAPI(sessionID string) (*dto.TerminalTrainerSessionInfo, error)

	// Utilities
	GetRepository() repositories.TerminalRepository
	CleanupExpiredSessions() error

	// Configuration
	GetTerms() (string, error)

	// Metrics
	GetServerMetrics(nocache bool, backend string) (*dto.ServerMetricsResponse, error)

	// Backend management
	GetBackends() ([]dto.BackendInfo, error)
	GetBackendsForOrganization(orgID uuid.UUID) ([]dto.BackendInfo, error)
	GetBackendsForContext(orgID uuid.UUID, userID string) ([]dto.BackendInfo, error)
	IsBackendOnline(backendName string) (bool, error)
	SetSystemDefaultBackend(backendID string) (*dto.BackendInfo, error)

	// Bulk operations
	BulkCreateTerminalsForGroup(groupID string, requestingUserID string, userRoles []string, request dto.BulkCreateTerminalsRequest, planInterface any) (*dto.BulkCreateTerminalsResponse, error)

	// Enum service access
	GetEnumService() TerminalTrainerEnumService

	// Session validation
	ValidateSessionAccess(sessionID string, checkAPI bool) (bool, string, error)

	// Command history
	GetSessionCommandHistory(sessionID string, since *int64, format string, limit, offset int) ([]byte, string, error)
	GetSessionCommandHistoryAdmin(sessionUUID string, limit, offset int) ([]byte, string, error)
	DeleteSessionCommandHistory(sessionID string) error
	DeleteAllUserCommandHistory(apiKey string) (int64, error)

	// Organization session management
	GetOrganizationTerminalSessions(orgID uuid.UUID) (*[]models.Terminal, error)
	GetOrgTerminalUsage(orgID uuid.UUID) (*dto.OrgTerminalUsageResponse, error)

	// Group command history
	GetGroupCommandHistory(groupID string, userID string, since *int64, format string, limit, offset int, includeStopped bool, search string) ([]byte, string, error)

	// Group command history stats
	GetGroupCommandHistoryStats(groupID string, userID string, includeStopped bool) ([]byte, string, error)

	// Consent status
	GetUserConsentStatus(userID string) (consentHandled bool, source string, err error)

	// Authorization helpers
	IsUserAuthorizedForSession(userID string, terminal *models.Terminal, isAdmin bool) bool
	IsUserOrgManagerOrAdmin(userID string, orgID uuid.UUID, isAdmin bool) bool

	// Composed session (Phase 4)
	GetDistributions(backend string) ([]dto.TTDistribution, error)
	GetCatalogSizes() ([]dto.TTSize, error)
	GetCatalogFeatures() ([]dto.TTFeature, error)
	GetSessionOptions(plan *paymentModels.SubscriptionPlan, distribution string, backend string) (*dto.SessionOptionsResponse, error)
	StartComposedSession(userID string, input dto.CreateComposedSessionInput, planInterface any) (*dto.TerminalSessionResponse, error)
}

type terminalTrainerService struct {
	adminKey                string
	baseURL                 string
	apiVersion              string
	terminalType            string
	repository              repositories.TerminalRepository
	subscriptionService     paymentServices.UserSubscriptionService
	orgSubscriptionService  paymentServices.OrganizationSubscriptionService
	enumService             TerminalTrainerEnumService
	db                      *gorm.DB
	backendCache            []dto.BackendInfo
	backendCacheTime        time.Time
	backendCacheMu          sync.RWMutex
	backendCacheSF          singleflight.Group
	catalogSizesCache       []dto.TTSize
	catalogSizesCacheTime   time.Time
	catalogSizesMu          sync.RWMutex
	catalogFeaturesCache     []dto.TTFeature
	catalogFeaturesCacheTime time.Time
	catalogFeaturesMu        sync.RWMutex
}

func NewTerminalTrainerService(db *gorm.DB) TerminalTrainerService {
	apiVersion := os.Getenv("TERMINAL_TRAINER_API_VERSION")
	if apiVersion == "" {
		apiVersion = "1.0" // default version
	}

	terminalType := os.Getenv("TERMINAL_TRAINER_TYPE")
	if terminalType == "" {
		terminalType = "" // no prefix by default
	}

	baseURL := os.Getenv("TERMINAL_TRAINER_URL") // http://localhost:8090

	return &terminalTrainerService{
		adminKey:               os.Getenv("TERMINAL_TRAINER_ADMIN_KEY"),
		baseURL:                baseURL,
		apiVersion:             apiVersion,
		terminalType:           terminalType,
		repository:             repositories.NewTerminalRepository(db),
		subscriptionService:    paymentServices.NewSubscriptionService(db),
		orgSubscriptionService: paymentServices.NewOrganizationSubscriptionService(db),
		enumService:            NewTerminalTrainerEnumService(baseURL, apiVersion),
		db:                     db,
	}
}

// CreateUserKey crée une clé Terminal Trainer et la stocke en DB
func (tts *terminalTrainerService) CreateUserKey(userID, keyName string) error {
	// Skip if Terminal Trainer is not configured
	if tts.baseURL == "" || tts.adminKey == "" {
		return fmt.Errorf("terminal trainer not configured")
	}

	// Vérifier si l'utilisateur a déjà une clé
	existingKey, err := tts.repository.GetUserTerminalKeyByUserID(userID, true)
	if err == nil && existingKey.IsActive {
		return fmt.Errorf("user already has an active terminal trainer key")
	}

	// Appel à l'API Terminal Trainer
	payload := map[string]any{
		"name":                    keyName,
		"is_admin":                false,
		"max_concurrent_sessions": 5,
	}

	url := fmt.Sprintf("%s/%s/admin/api-keys", tts.baseURL, tts.apiVersion)
	var apiResponse dto.TerminalTrainerAPIKeyResponse

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))
	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, payload, &apiResponse, opts)
	if err != nil {
		return err
	}

	// Sauvegarder en base
	userTerminalKey := &models.UserTerminalKey{
		UserID:      userID,
		APIKey:      apiResponse.Data.KeyValue,
		KeyName:     apiResponse.Data.Name,
		IsActive:    true,
		MaxSessions: apiResponse.Data.MaxConcurrentSessions,
	}

	return tts.repository.CreateUserTerminalKey(userTerminalKey)
}

// GetUserKey récupère la clé Terminal Trainer d'un utilisateur
func (tts *terminalTrainerService) GetUserKey(userID string) (*models.UserTerminalKey, error) {
	return tts.repository.GetUserTerminalKeyByUserID(userID, true)
}

// DisableUserKey désactive la clé d'un utilisateur
// FAULT-TOLERANT: If Terminal Trainer rejects the request (key doesn't exist in TT),
// we still disable it locally to allow creating a new key (auto-repair)
func (tts *terminalTrainerService) DisableUserKey(userID string) error {
	key, err := tts.repository.GetUserTerminalKeyByUserID(userID, true)
	if err != nil {
		return err
	}

	// Désactiver côté Terminal Trainer
	payload := map[string]any{
		"is_active": false,
	}

	url := fmt.Sprintf("%s/%s/admin/api-keys/%s", tts.baseURL, tts.apiVersion, key.APIKey)
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	_, err = utils.MakeExternalAPIRequest("Terminal Trainer", "PUT", url, payload, opts)
	if err != nil {
		// FAULT TOLERANCE: If Terminal Trainer doesn't have this key (databases out of sync),
		// log a warning but continue to disable locally (auto-repair)
		utils.Warn("Failed to disable key in Terminal Trainer (possibly orphaned): %v", err)
		utils.Debug("Continuing to disable key locally for auto-repair...")
	}

	// Désactiver en base locale (always do this, even if Terminal Trainer failed)
	key.IsActive = false
	return tts.repository.UpdateUserTerminalKey(key)
}


// GetSessionInfo récupère les informations d'une session
func (tts *terminalTrainerService) GetSessionInfo(sessionID string) (*models.Terminal, error) {
	return tts.repository.GetTerminalSessionByID(sessionID)
}

func (tts *terminalTrainerService) GetTerminalByUUID(terminalUUID string) (*models.Terminal, error) {
	return tts.repository.GetTerminalByUUID(terminalUUID)
}

// GetActiveUserSessions récupère toutes les sessions actives d'un utilisateur
func (tts *terminalTrainerService) GetActiveUserSessions(userID string) (*[]models.Terminal, error) {
	return tts.repository.GetTerminalSessionsByUserID(userID, true)
}

// StopSession arrête une session en appelant le nouvel endpoint /stop de
// tt-backend. Contrairement à l'ancien /expire, le disque est préservé pour
// permettre un redémarrage ultérieur via StartSession.
//
// Le compteur concurrent_terminals N'est PAS décrémenté ici — une session
// arrêtée occupe toujours un slot. La décrémentation se fait dans
// DeleteSession.
func (tts *terminalTrainerService) StopSession(sessionID string) error {
	utils.Debug("StopSession called for session %s", sessionID)

	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	utils.Debug("Session %s current status: %s", sessionID, terminal.Status)

	// 1. Appeler le nouvel endpoint /sessions/{id}/stop de tt-backend
	idleUntil, err := tts.stopSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey)
	if err != nil {
		// Log mais on continue : le state local reflètera "stopped" même si
		// tt-backend est temporairement injoignable.
		utils.Warn("failed to stop session in Terminal Trainer API: %v", err)
	}

	// 2. Marquer la session comme arrêtée localement
	utils.Debug("Updating session %s status/state to 'stopped'", sessionID)
	terminal.Status = "stopped"
	terminal.State = "stopped"
	if idleUntil != nil {
		terminal.IdleUntil = idleUntil
	}
	if err := tts.repository.UpdateTerminalSession(terminal); err != nil {
		utils.Error("Failed to update session %s status: %v", sessionID, err)
		return err
	}

	// 3. Auto-abandon any active scenario sessions linked to this terminal
	result := tts.db.Model(&struct{}{}).Table("scenario_sessions").
		Where("terminal_session_id = ? AND status IN ?", sessionID, []string{"active", "provisioning", "in_progress"}).
		Update("status", "abandoned")
	if result.Error != nil {
		utils.Warn("failed to abandon scenario sessions for terminal %s: %v", sessionID, result.Error)
	} else if result.RowsAffected > 0 {
		utils.Debug("Auto-abandoned %d scenario session(s) for stopped terminal %s", result.RowsAffected, sessionID)
	}

	return nil
}

// StartSession reprend une session précédemment arrêtée via l'endpoint
// /sessions/{id}/start de tt-backend. Le disque est restauré côté backend.
//
// La réponse de tt-backend porte le nouveau expires_at (unix seconds) calculé
// côté backend après réinitialisation du timer d'auto-stop. On le mire dans
// terminal.ExpiresAt pour que le front voie immédiatement la nouvelle échéance
// (sinon il affiche "Session expirée" sur une session qui vient d'être reprise,
// jusqu'à la prochaine synchro). Si tt-backend ne renvoie pas d'expires_at
// (instance sans expiry fini), on conserve la valeur actuelle.
func (tts *terminalTrainerService) StartSession(sessionID string) error {
	utils.Debug("StartSession called for session %s", sessionID)

	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	expiresAtUnix, err := tts.startSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey)
	if err != nil {
		return fmt.Errorf("failed to start session in Terminal Trainer API: %w", err)
	}

	terminal.Status = "active"
	terminal.State = "running"
	terminal.LastStartedAt = time.Now()
	terminal.IdleUntil = nil
	if expiresAtUnix > 0 {
		terminal.ExpiresAt = time.Unix(expiresAtUnix, 0)
	}
	if err := tts.repository.UpdateTerminalSession(terminal); err != nil {
		utils.Error("Failed to update session %s after start: %v", sessionID, err)
		return err
	}

	return nil
}

// DeleteSession supprime définitivement une session via DELETE /sessions/{id}
// de tt-backend. Décrémente la métrique concurrent_terminals et abandonne
// tout scenario session lié.
func (tts *terminalTrainerService) DeleteSession(sessionID string) error {
	utils.Debug("DeleteSession called for session %s", sessionID)

	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if err := tts.deleteSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey); err != nil {
		// Log mais on continue : la session locale doit être marquée "deleted"
		// même si tt-backend est injoignable, sinon l'utilisateur reste bloqué
		// avec un slot consommé.
		utils.Warn("failed to delete session in Terminal Trainer API: %v", err)
	}

	terminal.Status = "deleted"
	terminal.State = "deleted"
	if err := tts.repository.UpdateTerminalSession(terminal); err != nil {
		utils.Error("Failed to update session %s after delete: %v", sessionID, err)
		return err
	}

	// Décrémenter le compteur de slots — c'est ici que ça se passe désormais
	// (et plus dans StopSession), car une session "stopped" occupe encore un slot.
	if decErr := tts.subscriptionService.IncrementUsage(terminal.UserID, "concurrent_terminals", -1); decErr != nil {
		utils.Warn("failed to decrement concurrent_terminals for user %s: %v", terminal.UserID, decErr)
	}

	// Auto-abandon any active scenario sessions linked to this terminal
	result := tts.db.Model(&struct{}{}).Table("scenario_sessions").
		Where("terminal_session_id = ? AND status IN ?", sessionID, []string{"active", "provisioning", "in_progress"}).
		Update("status", "abandoned")
	if result.Error != nil {
		utils.Warn("failed to abandon scenario sessions for terminal %s: %v", sessionID, result.Error)
	} else if result.RowsAffected > 0 {
		utils.Debug("Auto-abandoned %d scenario session(s) for deleted terminal %s", result.RowsAffected, sessionID)
	}

	return nil
}

// stopSessionInAPI appelle POST /sessions/{id}/stop sur tt-backend.
// Retourne idle_until si tt-backend en propose un.
func (tts *terminalTrainerService) stopSessionInAPI(sessionID, userAPIKey string) (*time.Time, error) {
	url := fmt.Sprintf("%s/%s/sessions/%s/stop", tts.baseURL, tts.apiVersion, sessionID)

	utils.Debug("stopSessionInAPI - calling %s", url)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	var resp struct {
		IdleUntil *time.Time `json:"idle_until,omitempty"`
	}
	if err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, nil, &resp, opts); err != nil {
		return nil, err
	}
	return resp.IdleUntil, nil
}

// startSessionInAPI appelle POST /sessions/{id}/start sur tt-backend.
// Retourne le nouveau expires_at (unix seconds) tel que tt-backend l'a recalculé
// au redémarrage de l'instance. Retourne 0 si le champ est absent (instance
// sans expiry fini côté backend) — l'appelant doit alors conserver la valeur
// locale actuelle.
func (tts *terminalTrainerService) startSessionInAPI(sessionID, userAPIKey string) (int64, error) {
	url := fmt.Sprintf("%s/%s/sessions/%s/start", tts.baseURL, tts.apiVersion, sessionID)

	utils.Debug("startSessionInAPI - calling %s", url)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	var resp struct {
		ExpiresAt int64 `json:"expires_at,omitempty"`
	}
	if err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, nil, &resp, opts); err != nil {
		return 0, err
	}
	return resp.ExpiresAt, nil
}

// deleteSessionInAPI appelle DELETE /sessions/{id} sur tt-backend.
func (tts *terminalTrainerService) deleteSessionInAPI(sessionID, userAPIKey string) error {
	url := fmt.Sprintf("%s/%s/sessions/%s", tts.baseURL, tts.apiVersion, sessionID)

	utils.Debug("deleteSessionInAPI - calling %s", url)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	_, err := utils.MakeExternalAPIRequest("Terminal Trainer", "DELETE", url, nil, opts)
	return err
}

// GetAllSessionsFromAPI récupère toutes les sessions depuis l'API Terminal Trainer
func (tts *terminalTrainerService) GetAllSessionsFromAPI(userAPIKey string) (*dto.TerminalTrainerSessionsResponse, error) {
	// Utiliser le type d'instance par défaut configuré pour récupérer toutes les sessions
	path := tts.buildAPIPath("/sessions", "")
	url := fmt.Sprintf("%s%s?include_expired=true&limit=1000", tts.baseURL, path)

	var sessionsResp dto.TerminalTrainerSessionsResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sessionsResp, opts)
	if err != nil {
		return nil, err
	}

	return &sessionsResp, nil
}

// GetSessionInfoFromAPI récupère les infos d'une session directement depuis l'API
func (tts *terminalTrainerService) GetSessionInfoFromAPI(sessionID string) (*dto.TerminalTrainerSessionInfo, error) {
	// Récupérer la session locale pour obtenir la clé API
	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found locally: %w", err)
	}

	// Construire le chemin avec version et type d'instance dynamique
	path := tts.buildAPIPath("/info", terminal.InstanceType)
	url := fmt.Sprintf("%s%s?id=%s", tts.baseURL, path, sessionID)

	var sessionInfo dto.TerminalTrainerSessionInfo
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(terminal.UserTerminalKey.APIKey))

	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sessionInfo, opts)
	if err != nil {
		// Check for 404 specifically
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("session not found on Terminal Trainer")
		}
		return nil, err
	}

	return &sessionInfo, nil
}

// SyncUserSessions synchronise toutes les sessions d'un utilisateur avec l'API comme source de vérité
func (tts *terminalTrainerService) SyncUserSessions(userID string) (*dto.SyncAllSessionsResponse, error) {
	// 1. Récupérer la clé utilisateur
	userKey, err := tts.repository.GetUserTerminalKeyByUserID(userID, true)
	if err != nil {
		return nil, fmt.Errorf("no terminal trainer key found for user: %w", err)
	}

	if !userKey.IsActive {
		return nil, fmt.Errorf("user terminal trainer key is disabled")
	}

	// 2. Récupérer TOUTES les sessions depuis l'API Terminal Trainer pour tous les types d'instances
	apiSessions, err := tts.getAllSessionsFromAllInstanceTypes(userKey.APIKey, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions from Terminal Trainer API: %w", err)
	}

	// 3. Récupérer les sessions locales pour cet utilisateur
	localSessions, err := tts.repository.GetTerminalSessionsByUserID(userID, false)
	if err != nil {
		localSessions = &[]models.Terminal{} // Traiter comme liste vide si erreur
	}

	// 4. Créer des maps pour faciliter la comparaison
	localSessionsMap := make(map[string]*models.Terminal)
	for i := range *localSessions {
		session := &(*localSessions)[i]
		localSessionsMap[session.SessionID] = session
	}

	apiSessionsMap := make(map[string]*dto.TerminalTrainerSession)
	for i := range apiSessions.Sessions {
		session := &apiSessions.Sessions[i]
		apiSessionsMap[session.SessionID] = session
	}

	// 5. Synchronisation bidirectionnelle
	sessionResults := make([]dto.SyncSessionResponse, 0, len(apiSessionsMap)+len(localSessionsMap))
	errors := make([]string, 0, 8)
	syncedCount := 0
	updatedCount := 0
	createdCount := 0

	// 5a. Traiter les sessions qui existent côté API (source de vérité)
	for sessionID, apiSession := range apiSessionsMap {
		localSession := localSessionsMap[sessionID]

		// Map SessionStatus from /sessions endpoint to terminal lifecycle status.
		// /sessions returns SessionStatus: 0=active, 1=expired, 2=preallocated, 3+=deleted.
		// This is different from InstanceCreationStatus (0=started, 1=invalid_terms) used by /start and /info.
		apiStatusName := "expired"
		if apiSession.Status == 0 {
			apiStatusName = "active"
		}

		if localSession == nil {
			// Session existe côté API mais pas côté local
			// Ne recréer que les sessions actives, pas les expirées/arrêtées
			if apiStatusName == "active" {
				utils.Debug("SyncUserSessions - Creating missing active session %s (status=%d, name=%s)",
					sessionID, apiSession.Status, apiStatusName)
				err := tts.createMissingLocalSession(userID, userKey, apiSession)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to create missing session %s: %v", sessionID, err))
				} else {
					sessionResults = append(sessionResults, dto.SyncSessionResponse{
						SessionID:      sessionID,
						PreviousStatus: "missing",
						CurrentStatus:  apiStatusName,
						Updated:        true,
						LastSyncAt:     time.Now(),
					})
					syncedCount++
					updatedCount++
					createdCount++
				}
			} else {
				utils.Debug("SyncUserSessions - Ignoring non-active session %s (status=%d, name=%s) from API",
					sessionID, apiSession.Status, apiStatusName)
				// Ajouter quand même aux résultats pour le suivi
				sessionResults = append(sessionResults, dto.SyncSessionResponse{
					SessionID:      sessionID,
					PreviousStatus: "missing",
					CurrentStatus:  fmt.Sprintf("ignored-%s", apiStatusName),
					Updated:        false,
					LastSyncAt:     time.Now(),
				})
				syncedCount++
			}
		} else {
			// Session existe des deux côtés -> synchroniser le statut
			previousStatus := localSession.Status
			needsUpdate := false

			utils.Debug("SyncUserSessions - Session %s: local='%s', api='%d' (name='%s')",
				sessionID, localSession.Status, apiSession.Status, apiStatusName)

			// Vérifier si le statut a changé
			// Ne pas écraser les sessions arrêtées manuellement (status "stopped")
			if localSession.Status != apiStatusName && localSession.Status != "stopped" {
				utils.Debug("SyncUserSessions - Status mismatch for session %s: changing '%s' -> '%s'",
					sessionID, localSession.Status, apiStatusName)
				localSession.Status = apiStatusName
				needsUpdate = true
			} else if localSession.Status == "stopped" {
				utils.Debug("SyncUserSessions - Session %s is manually stopped, keeping local status", sessionID)
			}

			// Vérifier si la session a expiré selon la date
			expiryTime := time.Unix(apiSession.ExpiresAt, 0)
			if time.Now().After(expiryTime) && apiStatusName == "active" {
				utils.Debug("SyncUserSessions - Session %s expired by date, marking as expired", sessionID)
				localSession.Status = "expired"
				needsUpdate = true
			}

			// Propager les champs de cycle de vie depuis tt-backend (source de vérité).
			// Sans ça, une session auto-arrêtée côté tt-backend reste affichée comme
			// "running" en local et le front continue d'afficher "Session expirée"
			// au lieu d'offrir un bouton Reprendre.
			if apiSession.State != "" && localSession.State != apiSession.State {
				utils.Debug("SyncUserSessions - State mismatch for session %s: changing '%s' -> '%s'",
					sessionID, localSession.State, apiSession.State)
				localSession.State = apiSession.State
				needsUpdate = true
			}
			if apiSession.PersistenceMode != "" && localSession.PersistenceMode != apiSession.PersistenceMode {
				utils.Debug("SyncUserSessions - PersistenceMode mismatch for session %s: changing '%s' -> '%s'",
					sessionID, localSession.PersistenceMode, apiSession.PersistenceMode)
				localSession.PersistenceMode = apiSession.PersistenceMode
				needsUpdate = true
			}
			if apiSession.IdleUntil > 0 {
				t := time.Unix(apiSession.IdleUntil, 0).UTC()
				if localSession.IdleUntil == nil || !localSession.IdleUntil.Equal(t) {
					localSession.IdleUntil = &t
					needsUpdate = true
				}
			}

			if needsUpdate {
				utils.Debug("SyncUserSessions - Updating session %s status to '%s'", sessionID, localSession.Status)
				err := tts.repository.UpdateTerminalSession(localSession)
				if err != nil {
					utils.Error("SyncUserSessions - Failed to update session %s: %v", sessionID, err)
					errors = append(errors, fmt.Sprintf("Failed to update session %s: %v", sessionID, err))
				} else {
					utils.Debug("SyncUserSessions - Successfully updated session %s", sessionID)
					updatedCount++
				}
			}

			sessionResults = append(sessionResults, dto.SyncSessionResponse{
				SessionID:      sessionID,
				PreviousStatus: previousStatus,
				CurrentStatus:  localSession.Status,
				Updated:        needsUpdate,
				LastSyncAt:     time.Now(),
			})
			syncedCount++
		}
	}

	// 5b. Traiter les sessions qui existent côté local mais pas côté API
	expiredCount := 0
	for sessionID, localSession := range localSessionsMap {
		if _, exists := apiSessionsMap[sessionID]; !exists {
			// Session existe côté local mais pas côté API -> la marquer comme expirée
			if localSession.Status != "expired" && localSession.Status != "stopped" {
				previousStatus := localSession.Status
				localSession.Status = "expired"

				err := tts.repository.UpdateTerminalSession(localSession)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to expire orphaned session %s: %v", sessionID, err))
				} else {
					sessionResults = append(sessionResults, dto.SyncSessionResponse{
						SessionID:      sessionID,
						PreviousStatus: previousStatus,
						CurrentStatus:  "expired",
						Updated:        true,
						LastSyncAt:     time.Now(),
					})
					updatedCount++
					expiredCount++
				}
			}
			syncedCount++
		}
	}

	// 6. Construire la réponse
	response := &dto.SyncAllSessionsResponse{
		TotalSessions:   len(apiSessions.Sessions),
		SyncedSessions:  syncedCount,
		UpdatedSessions: updatedCount,
		ErrorCount:      len(errors),
		Errors:          errors,
		SessionResults:  sessionResults,
		LastSyncAt:      time.Now(),
	}

	return response, nil
}

// createMissingLocalSession crée une session locale manquante basée sur les données de l'API
func (tts *terminalTrainerService) createMissingLocalSession(userID string, userKey *models.UserTerminalKey, apiSession *dto.TerminalTrainerSession) error {
	expiresAt := time.Unix(apiSession.ExpiresAt, 0)

	// Map SessionStatus from /sessions endpoint to terminal lifecycle status
	statusName := "expired"
	if apiSession.Status == 0 {
		statusName = "active"
	}

	terminal := &models.Terminal{
		SessionID:         apiSession.SessionID,
		UserID:            userID,
		Status:            statusName, // Terminal lifecycle status: "active" or "expired"
		ExpiresAt:         expiresAt,
		MachineSize:       apiSession.MachineSize, // Taille réelle depuis l'API
		Backend:           apiSession.Backend,
		UserTerminalKeyID: userKey.ID,
		UserTerminalKey:   *userKey,
	}

	// Propager les champs de cycle de vie depuis tt-backend (source de vérité)
	// dès la création de la ligne locale, sinon le front verra des valeurs par
	// défaut ("running"/"ephemeral") qui ne reflètent pas l'état réel.
	if apiSession.State != "" {
		terminal.State = apiSession.State
	}
	if apiSession.PersistenceMode != "" {
		terminal.PersistenceMode = apiSession.PersistenceMode
	}
	if apiSession.IdleUntil > 0 {
		t := time.Unix(apiSession.IdleUntil, 0).UTC()
		terminal.IdleUntil = &t
	}

	return tts.repository.CreateTerminalSessionFromAPI(terminal)
}

// SyncAllActiveSessions - version améliorée qui utilise la nouvelle logique
func (tts *terminalTrainerService) SyncAllActiveSessions() error {
	// Récupérer tous les utilisateurs ayant des clés actives
	activeKeys, err := tts.repository.GetAllActiveUserKeys()
	if err != nil {
		return fmt.Errorf("failed to get active user keys: %w", err)
	}

	var globalErrors []string
	for _, userKey := range *activeKeys {
		_, err := tts.SyncUserSessions(userKey.UserID)
		if err != nil {
			globalErrors = append(globalErrors, fmt.Sprintf("User %s: %v", userKey.UserID, err))
		}
	}

	if len(globalErrors) > 0 {
		return fmt.Errorf("sync completed with errors: %v", globalErrors)
	}

	return nil
}

// GetRepository expose le repository pour les contrôleurs
func (tts *terminalTrainerService) GetRepository() repositories.TerminalRepository {
	return tts.repository
}

// CleanupExpiredSessions nettoie les sessions expirées
func (tts *terminalTrainerService) CleanupExpiredSessions() error {
	return tts.repository.CleanupExpiredSessions()
}

// GetTerms fetches the terms of service text from Terminal Trainer
func (tts *terminalTrainerService) GetTerms() (string, error) {
	path := fmt.Sprintf("/%s/terms", tts.apiVersion)
	url := fmt.Sprintf("%s%s", tts.baseURL, path)

	var termsResp struct {
		Terms string `json:"terms"`
		Hash  string `json:"hash"`
	}
	opts := utils.DefaultHTTPClientOptions()
	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &termsResp, opts)
	if err != nil {
		return "", err
	}

	return termsResp.Terms, nil
}

// buildAPIPath construit le chemin API avec version et type d'instance optionnel
func (tts *terminalTrainerService) buildAPIPath(endpoint string, instanceType string) string {
	path := fmt.Sprintf("/%s", tts.apiVersion)

	// Utiliser le type d'instance fourni, sinon celui par défaut du service
	if instanceType != "" {
		path += fmt.Sprintf("/%s", instanceType)
	} else if tts.terminalType != "" {
		path += fmt.Sprintf("/%s", tts.terminalType)
	}

	path += endpoint
	return path
}

// getAllSessionsFromAllInstanceTypes récupère les sessions de tous les types d'instances utilisés par l'utilisateur
func (tts *terminalTrainerService) getAllSessionsFromAllInstanceTypes(userAPIKey, userID string) (*dto.TerminalTrainerSessionsResponse, error) {
	// 1. Récupérer toutes les sessions locales de l'utilisateur pour connaître les types d'instances utilisés
	localSessions, err := tts.repository.GetTerminalSessionsByUserID(userID, false)
	if err != nil {
		localSessions = &[]models.Terminal{} // Traiter comme liste vide si erreur
	}

	// 2. Créer un set des types d'instances utilisés (incluant le type par défaut)
	instanceTypesUsed := make(map[string]bool)
	instanceTypesUsed[""] = true // Toujours inclure le type par défaut

	for _, session := range *localSessions {
		if session.InstanceType != "" {
			instanceTypesUsed[session.InstanceType] = true
		}
	}

	// 3. Récupérer les sessions depuis chaque type d'instance utilisé
	allSessions := make([]dto.TerminalTrainerSession, 0, len(instanceTypesUsed)*10)
	totalCount := 0

	for instanceType := range instanceTypesUsed {
		apiResponse, err := tts.getSessionsFromInstanceType(userAPIKey, instanceType)
		if err != nil {
			// Log l'erreur mais continuer avec les autres types d'instances
			utils.Warn("failed to get sessions from instance type '%s': %v", instanceType, err)
			continue
		}
		allSessions = append(allSessions, apiResponse.Sessions...)
		totalCount += apiResponse.Count
	}

	// 4. Retourner une réponse combinée
	return &dto.TerminalTrainerSessionsResponse{
		Sessions:       allSessions,
		Count:          totalCount,
		APIKeyID:       0, // Valeur par défaut car on combine plusieurs réponses
		IncludeExpired: true,
		Limit:          1000,
	}, nil
}

// getSessionsFromInstanceType récupère les sessions d'un type d'instance spécifique
func (tts *terminalTrainerService) getSessionsFromInstanceType(userAPIKey, instanceType string) (*dto.TerminalTrainerSessionsResponse, error) {
	path := tts.buildAPIPath("/sessions", instanceType)
	url := fmt.Sprintf("%s%s?include_expired=true&limit=1000", tts.baseURL, path)

	var sessionsResp dto.TerminalTrainerSessionsResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sessionsResp, opts)
	if err != nil {
		return nil, err
	}

	return &sessionsResp, nil
}


// HasTerminalAccess checks if a user has access to a terminal.
// Only terminal owners and group owners of the owner's group have access.
func (tts *terminalTrainerService) HasTerminalAccess(terminalIDOrSessionID, userID string) (bool, error) {
	// Try to get terminal by UUID first (most common case from API)
	terminal, err := tts.repository.GetTerminalByUUID(terminalIDOrSessionID)
	if err != nil {
		// If UUID lookup fails, try SessionID lookup
		terminal, err = tts.repository.GetTerminalSessionBySessionID(terminalIDOrSessionID)
		if err != nil {
			return false, fmt.Errorf("failed to get terminal: %w", err)
		}
		if terminal == nil {
			return false, fmt.Errorf("terminal not found")
		}
	}

	// The terminal owner always has access
	if terminal.UserID == userID {
		return true, nil
	}

	// Check if requesting user is a group owner with the terminal owner as member
	isGroupOwner, err := tts.checkGroupOwnerAccess(terminal.UserID, userID)
	if err == nil && isGroupOwner {
		return true, nil
	}

	return false, nil
}

// GetServerMetrics récupère les métriques du serveur Terminal Trainer
func (tts *terminalTrainerService) GetServerMetrics(nocache bool, backend string) (*dto.ServerMetricsResponse, error) {
	// Skip if Terminal Trainer is not configured
	if tts.baseURL == "" {
		return nil, fmt.Errorf("terminal trainer not configured")
	}

	// Construire l'URL des métriques
	path := fmt.Sprintf("/%s/metrics", tts.apiVersion)
	url := fmt.Sprintf("%s%s", tts.baseURL, path)

	// Ajouter les paramètres
	params := []string{}
	if nocache {
		params = append(params, "nocache=true")
	}
	if backend != "" {
		params = append(params, fmt.Sprintf("backend=%s", backend))
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	// Exécuter la requête (pas besoin d'authentification selon les specs)
	var metrics dto.ServerMetricsResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithTimeout(10*time.Second))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &metrics, opts)
	if err != nil {
		return nil, err
	}

	metrics.Backend = backend
	return &metrics, nil
}

// GetBackends retrieves all available backends from Terminal Trainer
func (tts *terminalTrainerService) GetBackends() ([]dto.BackendInfo, error) {
	if tts.baseURL == "" {
		return nil, fmt.Errorf("terminal trainer not configured")
	}

	url := fmt.Sprintf("%s/%s/backends", tts.baseURL, tts.apiVersion)

	var backends []dto.BackendInfo
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &backends, opts)
	if err != nil {
		return nil, err
	}

	for i := range backends {
		// Default Name to ID if upstream doesn't provide one
		if backends[i].Name == "" {
			backends[i].Name = backends[i].ID
		}
	}

	return backends, nil
}

// getBackendsCached returns cached backends or refreshes if older than 30s.
// Uses singleflight to coalesce concurrent cache misses into a single upstream call.
func (tts *terminalTrainerService) getBackendsCached() ([]dto.BackendInfo, error) {
	tts.backendCacheMu.RLock()
	if tts.backendCache != nil && time.Since(tts.backendCacheTime) < 30*time.Second {
		cached := make([]dto.BackendInfo, len(tts.backendCache))
		copy(cached, tts.backendCache)
		tts.backendCacheMu.RUnlock()
		return cached, nil
	}
	tts.backendCacheMu.RUnlock()

	// Use singleflight to prevent cache stampede: concurrent callers that find
	// stale cache will share a single upstream GetBackends() call.
	v, err, _ := tts.backendCacheSF.Do("backends", func() (interface{}, error) {
		backends, err := tts.GetBackends()
		if err != nil {
			return nil, err
		}

		tts.backendCacheMu.Lock()
		tts.backendCache = backends
		tts.backendCacheTime = time.Now()
		tts.backendCacheMu.Unlock()

		return backends, nil
	})
	if err != nil {
		return nil, err
	}

	return v.([]dto.BackendInfo), nil
}

// getSystemDefault returns the backend ID marked as default by tt-backend.
// Returns empty string if no backend is marked as default.
func (tts *terminalTrainerService) getSystemDefault() string {
	backends, err := tts.getBackendsCached()
	if err != nil || len(backends) == 0 {
		return ""
	}
	for _, b := range backends {
		if b.IsDefault {
			return b.ID
		}
	}
	return ""
}

// invalidateBackendCache clears the cached backends so the next read
// fetches fresh data from tt-backend.
func (tts *terminalTrainerService) invalidateBackendCache() {
	tts.backendCacheMu.Lock()
	tts.backendCache = nil
	tts.backendCacheTime = time.Time{}
	tts.backendCacheMu.Unlock()
	// Cancel any in-flight singleflight request that could repopulate the
	// cache with stale data.
	tts.backendCacheSF.Forget("backends")
}

// GetBackendsForContext returns backends filtered by org config (if set) or plan config (fallback).
// This ensures the frontend backend selector shows the correct options for the current org/plan context.
func (tts *terminalTrainerService) GetBackendsForContext(orgID uuid.UUID, userID string) ([]dto.BackendInfo, error) {
	var org orgModels.Organization
	if err := tts.db.First(&org, "id = ?", orgID).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	// If org has explicit backend config, use org rules (existing behavior)
	if len(org.AllowedBackends) > 0 || org.DefaultBackend != "" {
		return tts.GetBackendsForOrganization(orgID)
	}

	// No org config → resolve the user's effective plan for this org to get plan-level backends
	effectivePlanService := paymentServices.NewEffectivePlanService(tts.db)
	result, err := effectivePlanService.GetUserEffectivePlan(userID, &orgID)
	if err != nil || result == nil || result.Plan == nil {
		// No plan resolved — fall back to system default
		return tts.GetBackendsForOrganization(orgID)
	}

	plan := result.Plan
	if len(plan.AllowedBackends) == 0 && plan.DefaultBackend == "" {
		// Plan has no backend config either — fall back to org logic
		return tts.GetBackendsForOrganization(orgID)
	}

	// Filter all backends by plan's AllowedBackends
	allBackends, err := tts.getBackendsCached()
	if err != nil {
		return nil, err
	}

	if len(plan.AllowedBackends) > 0 {
		allowedSet := make(map[string]bool, len(plan.AllowedBackends))
		for _, b := range plan.AllowedBackends {
			allowedSet[b] = true
		}
		var filtered []dto.BackendInfo
		for _, b := range allBackends {
			if allowedSet[b.ID] {
				b.IsDefault = (b.ID == plan.DefaultBackend)
				filtered = append(filtered, b)
			}
		}
		if len(filtered) > 0 {
			return filtered, nil
		}
	}

	// Plan has DefaultBackend but no AllowedBackends — return just the default
	if plan.DefaultBackend != "" {
		for _, b := range allBackends {
			if b.ID == plan.DefaultBackend {
				b.IsDefault = true
				return []dto.BackendInfo{b}, nil
			}
		}
	}

	return tts.GetBackendsForOrganization(orgID)
}

// GetBackendsForOrganization returns backends filtered by org's AllowedBackends/DefaultBackend
func (tts *terminalTrainerService) GetBackendsForOrganization(orgID uuid.UUID) ([]dto.BackendInfo, error) {
	var org orgModels.Organization
	if err := tts.db.First(&org, "id = ?", orgID).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	allBackends, err := tts.getBackendsCached()
	if err != nil {
		return nil, err
	}

	// Determine which backends this org can access
	if len(org.AllowedBackends) == 0 {
		// No manual config: return only the default backend.
		// Priority: org default → system default → first backend in list
		defaultID := org.DefaultBackend
		if defaultID == "" {
			defaultID = tts.getSystemDefault()
		}
		if defaultID == "" && len(allBackends) > 0 {
			defaultID = allBackends[0].ID
		}
		var filtered []dto.BackendInfo
		for _, b := range allBackends {
			if b.ID == defaultID {
				b.IsDefault = true
				filtered = append(filtered, b)
			}
		}
		if len(filtered) == 0 {
			return allBackends, nil
		}
		return filtered, nil
	}

	// Explicit config: return only the allowed backends
	allowedSet := make(map[string]bool, len(org.AllowedBackends))
	for _, b := range org.AllowedBackends {
		allowedSet[b] = true
	}

	defaultID := org.DefaultBackend
	if defaultID == "" {
		defaultID = tts.getSystemDefault()
	}

	var filtered []dto.BackendInfo
	for _, b := range allBackends {
		if allowedSet[b.ID] {
			// Reset IsDefault based on org's effective default, not system default
			b.IsDefault = (b.ID == defaultID)
			filtered = append(filtered, b)
		}
	}

	return filtered, nil
}

// IsBackendOnline checks if a specific backend is connected.
// An empty backendName means "use system default", which is assumed online
// since tt-backend routes empty backend to its own default.
func (tts *terminalTrainerService) IsBackendOnline(backendName string) (bool, error) {
	if backendName == "" {
		return true, nil
	}

	backends, err := tts.getBackendsCached()
	if err != nil {
		return false, err
	}

	for _, b := range backends {
		if b.ID == backendName {
			return b.Connected, nil
		}
	}

	// Backend not found in list - assume offline
	return false, nil
}

// validateBackendForContext resolves the backend using a multi-level chain:
// 1. If orgID != nil and the org has backend config → delegate to validateBackendForOrg
// 2. Otherwise, apply plan-level backend rules
// 3. Final fallback: system default from tt-backend
func (tts *terminalTrainerService) validateBackendForContext(orgID *uuid.UUID, plan *paymentModels.SubscriptionPlan, requestedBackend string) (string, error) {
	// If org context is present, check if the org has its own backend config
	if orgID != nil {
		var org orgModels.Organization
		if err := tts.db.First(&org, "id = ?", *orgID).Error; err != nil {
			return "", fmt.Errorf("failed to get organization: %w", err)
		}

		// If the org has its own backend config, delegate to org rules
		if len(org.AllowedBackends) > 0 || org.DefaultBackend != "" {
			return tts.validateBackendForOrg(orgID, requestedBackend)
		}
	}

	// No org backend config (or no org / personal org) → apply plan-level rules
	if plan != nil {
		// No backend requested → use plan default, fallback to system default
		if requestedBackend == "" {
			if plan.DefaultBackend != "" {
				return plan.DefaultBackend, nil
			}
			return tts.getSystemDefault(), nil
		}

		// Backend requested → check against plan's AllowedBackends
		if len(plan.AllowedBackends) > 0 {
			for _, allowed := range plan.AllowedBackends {
				if allowed == requestedBackend {
					return requestedBackend, nil
				}
			}
			// Requested backend not in allowed list — fall back to plan default
			// (the user likely didn't explicitly choose; the frontend auto-selected from a stale list)
			if plan.DefaultBackend != "" {
				return plan.DefaultBackend, nil
			}
			return "", fmt.Errorf("backend '%s' is not allowed by your subscription plan. Allowed backends: %v",
				requestedBackend, plan.AllowedBackends)
		}

		// Plan has no AllowedBackends restriction → use plan default or system default
		if plan.DefaultBackend != "" {
			return plan.DefaultBackend, nil
		}
	}

	// Final fallback: no org config, no plan config — only system default is allowed
	systemDefault := tts.getSystemDefault()
	if requestedBackend == "" || requestedBackend == systemDefault {
		return systemDefault, nil
	}
	return "", fmt.Errorf("backend '%s' is not allowed: no backend restrictions configured, only system default is available", requestedBackend)
}

// validateBackendForOrg validates and resolves the backend for an organization
func (tts *terminalTrainerService) validateBackendForOrg(orgID *uuid.UUID, requestedBackend string) (string, error) {
	if orgID == nil {
		return requestedBackend, nil // No org context, allow any backend
	}

	var org orgModels.Organization
	if err := tts.db.First(&org, "id = ?", *orgID).Error; err != nil {
		return "", fmt.Errorf("failed to get organization: %w", err)
	}

	// Resolve org's effective default: org default → system default → ""
	effectiveDefault := org.DefaultBackend
	if effectiveDefault == "" {
		effectiveDefault = tts.getSystemDefault()
	}

	// If no backend requested, use the effective default
	if requestedBackend == "" {
		return effectiveDefault, nil
	}

	// If AllowedBackends is empty, only the default backend is allowed
	// If no default is configured either, allow any backend (no restrictions)
	if len(org.AllowedBackends) == 0 {
		if effectiveDefault == "" {
			return requestedBackend, nil
		}
		if requestedBackend == effectiveDefault {
			return requestedBackend, nil
		}
		return "", fmt.Errorf("backend '%s' is not allowed for your organization (no backends configured, default only)",
			requestedBackend)
	}

	// Check if requested backend is in allowed list
	for _, allowed := range org.AllowedBackends {
		if allowed == requestedBackend {
			return requestedBackend, nil
		}
	}

	return "", fmt.Errorf("backend '%s' is not allowed for your organization. Allowed backends: %v",
		requestedBackend, org.AllowedBackends)
}

// applyNameTemplate applies template placeholders to generate terminal names
func (tts *terminalTrainerService) applyNameTemplate(template, groupName, userEmail, userID string) string {
	if template == "" {
		template = "{group_name} - {user_email}"
	}

	result := template
	result = strings.ReplaceAll(result, "{group_name}", groupName)
	result = strings.ReplaceAll(result, "{user_email}", userEmail)
	result = strings.ReplaceAll(result, "{user_id}", userID)

	return result
}

// ErrBulkInsufficientRAM is returned by BulkCreateTerminalsForGroup when the
// Terminal Trainer server lacks enough RAM to provision terminals for all active
// group members. Callers should map this to HTTP 503.
var ErrBulkInsufficientRAM = errors.New("server at capacity: insufficient RAM for bulk terminal creation")

// BulkCreateTerminalsForGroup creates terminals for all members of a group
func (tts *terminalTrainerService) BulkCreateTerminalsForGroup(
	groupID string,
	requestingUserID string,
	userRoles []string,
	request dto.BulkCreateTerminalsRequest,
	planInterface any,
) (*dto.BulkCreateTerminalsResponse, error) {
	// Parse groupID
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, fmt.Errorf("invalid group ID: %w", err)
	}

	// Get group details
	var group groupModels.ClassGroup
	if err := tts.db.Preload("Members").Where("id = ?", groupUUID).First(&group).Error; err != nil {
		return nil, fmt.Errorf("group not found: %w", err)
	}

	// Check permissions - only group owner, group admin, or system administrator can bulk create terminals
	canManage := false
	if group.OwnerUserID == requestingUserID {
		canManage = true
	} else {
		// Check if user is a system administrator
		for _, role := range userRoles {
			if authModels.IsSystemAdmin(authModels.RoleName(role)) {
				canManage = true
				break
			}
		}
		// Check if user is an admin of the group
		if !canManage {
			for _, member := range group.Members {
				if member.UserID == requestingUserID && (member.Role == groupModels.GroupMemberRoleManager || member.Role == groupModels.GroupMemberRoleOwner) {
					canManage = true
					break
				}
			}
		}
	}

	if !canManage {
		return nil, fmt.Errorf("only group owner or admin can create bulk terminals")
	}

	// Filter active members only
	activeMembers := make([]groupModels.GroupMember, 0, len(group.Members))
	for _, member := range group.Members {
		if member.IsActive {
			activeMembers = append(activeMembers, member)
		}
	}

	// Pre-flight RAM check: refuse up-front if the server lacks capacity for all terminals.
	// Mirrors the logic in src/payment/middleware/ramCheckMiddleware.go.
	// Fail-open: if metrics are unavailable, skip the check and proceed.
	if len(activeMembers) > 0 {
		metrics, metricsErr := tts.GetServerMetrics(true, "")
		if metricsErr != nil {
			utils.Warn("bulk terminal creation: could not fetch server metrics, skipping RAM check: %v", metricsErr)
		} else {
			plan, _ := planInterface.(*paymentModels.SubscriptionPlan)
			if plan != nil {
				machineSizeToRAM := map[string]float64{
					"XS": 0.25,
					"S":  0.5,
					"M":  1.0,
					"L":  2.0,
					"XL": 4.0,
				}

				// Estimate RAM per terminal: max of allowed sizes (mirrors middleware logic)
				perTerminalRAM := 0.5 // default for S
				if len(plan.AllowedMachineSizes) > 0 {
					maxRAM := 0.0
					for _, size := range plan.AllowedMachineSizes {
						if size == "all" {
							perTerminalRAM = 1.0 // use M as average for "all"
							maxRAM = 0            // signal: already set
							break
						}
						if ram, found := machineSizeToRAM[size]; found && ram > maxRAM {
							maxRAM = ram
						}
					}
					if maxRAM > 0 {
						perTerminalRAM = maxRAM
					}
				}

				totalRequiredRAM := float64(len(activeMembers)) * perTerminalRAM
				totalRAM := metrics.RAMAvailableGB / (1.0 - metrics.RAMPercent/100.0)
				minReservedRAM := totalRAM * 0.05

				if metrics.RAMPercent >= 99.0 || metrics.RAMAvailableGB-totalRequiredRAM < minReservedRAM {
					utils.Warn("bulk terminal creation blocked: insufficient RAM (%d terminals × %.2f GB = %.2f GB required, %.2f GB available)",
						len(activeMembers), perTerminalRAM, totalRequiredRAM, metrics.RAMAvailableGB)
					return nil, fmt.Errorf("%w: %d terminals need %.2f GB, %.2f GB available",
						ErrBulkInsufficientRAM, len(activeMembers), totalRequiredRAM, metrics.RAMAvailableGB)
				}
			}
		}
	}

	// Initialize response
	response := &dto.BulkCreateTerminalsResponse{
		Success:      true,
		CreatedCount: 0,
		FailedCount:  0,
		TotalMembers: len(activeMembers),
		Terminals:    make([]dto.BulkTerminalCreationResult, 0, len(activeMembers)),
		Errors:       make([]string, 0, len(activeMembers)/4),
	}

	// Get user details from Casdoor for email addresses
	userEmails := make(map[string]string) // userID -> email
	for _, member := range activeMembers {
		user, err := casdoorsdk.GetUserByUserId(member.UserID)
		if err != nil || user == nil {
			utils.Warn("failed to get user details for %s: %v", member.UserID, err)
			userEmails[member.UserID] = member.UserID // Fallback to userID
		} else {
			userEmails[member.UserID] = user.Email
		}
	}

	// Auto-provision terminal keys for members who don't have one
	for _, member := range activeMembers {
		_, err := tts.repository.GetUserTerminalKeyByUserID(member.UserID, true)
		if err != nil {
			keyName := "auto-" + userEmails[member.UserID]
			if createErr := tts.CreateUserKey(member.UserID, keyName); createErr != nil {
				utils.Warn("failed to auto-provision terminal key for user %s: %v", member.UserID, createErr)
			}
		}
	}

	// Create terminals for each member
	for _, member := range activeMembers {
		userEmail := userEmails[member.UserID]

		// Generate terminal name using template
		terminalName := tts.applyNameTemplate(request.NameTemplate, group.DisplayName, userEmail, member.UserID)

		// Create composed session input for this user
		composedInput := dto.CreateComposedSessionInput{
			Distribution:     request.InstanceType, // InstanceType now maps to distribution name
			Size:             "S",                  // Default size for bulk creation
			Terms:            request.Terms,
			Name:             terminalName,
			Expiry:           request.Expiry,
			Backend:          request.Backend,
			OrganizationID:   request.OrganizationID,
			RecordingEnabled: request.RecordingEnabled,
			ExternalRef:      request.ExternalRef,
			Hostname:         request.Hostname,
		}

		// Try to create terminal via composed session
		sessionResp, err := tts.StartComposedSession(member.UserID, composedInput, planInterface)

		result := dto.BulkTerminalCreationResult{
			UserID:    member.UserID,
			UserEmail: userEmail,
			Name:      terminalName,
			Success:   err == nil,
		}

		if err != nil {
			result.Error = err.Error()
			response.FailedCount++
			response.Errors = append(response.Errors, fmt.Sprintf("Failed for user %s (%s): %v", userEmail, member.UserID, err))
		} else {
			result.SessionID = &sessionResp.SessionID
			// Get the terminal record to get the UUID
			terminal, terr := tts.repository.GetTerminalSessionByID(sessionResp.SessionID)
			if terr == nil {
				terminalID := terminal.ID.String()
				result.TerminalID = &terminalID
			}
			response.CreatedCount++
		}

		response.Terminals = append(response.Terminals, result)
	}

	// If all failed, mark as not successful
	if response.FailedCount > 0 && response.CreatedCount == 0 {
		response.Success = false
	}

	return response, nil
}

// GetEnumService returns the enum service for external access
func (tts *terminalTrainerService) GetEnumService() TerminalTrainerEnumService {
	return tts.enumService
}

// ValidateSessionAccess checks if a session is accessible for console operations
// Returns: (isValid bool, reason string, error)
// - isValid: true if session can be accessed, false otherwise
// - reason: "active", "stopped", "expired", or other status
// - error: any error encountered during validation
func (tts *terminalTrainerService) ValidateSessionAccess(sessionID string, checkAPI bool) (bool, string, error) {
	// 1. Get session from local database
	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return false, "", fmt.Errorf("session not found: %w", err)
	}

	// 2. Check local lifecycle state.
	//
	// State is the canonical SSOT, populated by SyncUserSessions from
	// tt-backend's authoritative session.state. Status is a legacy field
	// (see models/terminal.go) that drifts to "expired" when ExpiresAt
	// passes, even on persistent sessions that tt-backend has correctly
	// auto-stopped (State="stopped"). Reading Status first 410-Gone'd the
	// user out of the Resume flow.
	switch terminal.State {
	case "running":
		// continue to backend + expiration checks below
	case "stopped":
		return false, "stopped", nil
	case "deleted":
		// Preserve the existing wire format the FE maps to "Session ended".
		return false, "expired", nil
	case "":
		// Legacy row that pre-dates the State column being populated.
		// Fall back to the old Status check so old data doesn't break.
		if terminal.Status != "active" {
			return false, terminal.Status, nil
		}
	default:
		// Surface other lifecycle states (paused, hibernating, resuming, ...)
		// to the caller; the middleware will 403 with the state name.
		return false, terminal.State, nil
	}

	// 3. Check backend online status
	if terminal.Backend != "" {
		online, err := tts.IsBackendOnline(terminal.Backend)
		if err != nil {
			utils.Warn("failed to check backend status: %v", err)
		} else if !online {
			return false, "backend_offline", nil
		}
	}

	// 4. Check expiration time.
	//
	// For State="running" sessions, ExpiresAt in the past means the sync
	// hasn't caught up yet — tt-backend's auto-stop will land on the next
	// poll. Returning "expired" here is conservative; we don't write the
	// legacy Status field anymore — the next sync owns the State update.
	if time.Now().After(terminal.ExpiresAt) {
		return false, "expired", nil
	}

	// 4. Optional API verification (for critical operations)
	if checkAPI {
		apiInfo, err := tts.GetSessionInfoFromAPI(sessionID)
		if err != nil {
			// Handle 404 = session doesn't exist in Terminal Trainer
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
				terminal.Status = "expired"
				updateErr := tts.repository.UpdateTerminalSession(terminal)
				if updateErr != nil {
					utils.Warn("failed to update session %s status after API 404: %v", sessionID, updateErr)
				}
				return false, "expired", nil
			}
			// For other API errors, return error but don't block access
			// This allows fail-open behavior when API is unavailable
			utils.Warn("API validation failed for session %s: %v", sessionID, err)
			return false, "", fmt.Errorf("failed to validate session with API: %w", err)
		}

		// Map InstanceCreationStatus from /info endpoint to terminal lifecycle status.
		// /info returns InstanceCreationStatus: 0=started (instance running), 6=expired (instance gone).
		// These are different from SessionStatus (0=active, 1=expired) used by /sessions.
		apiStatusName := "expired"
		if apiInfo.Status == 0 {
			apiStatusName = "active"
		}

		// Sync status if mismatch detected
		if apiStatusName != terminal.Status {
			previousStatus := terminal.Status
			terminal.Status = apiStatusName
			err := tts.repository.UpdateTerminalSession(terminal)
			if err != nil {
				utils.Warn("failed to sync session %s status from '%s' to '%s': %v",
					sessionID, previousStatus, apiStatusName, err)
			}
			return terminal.Status == "active", terminal.Status, nil
		}
	}

	return true, "active", nil
}

// GetSessionCommandHistory retrieves command history from tt-backend
func (tts *terminalTrainerService) GetSessionCommandHistory(sessionID string, since *int64, format string, limit, offset int) ([]byte, string, error) {
	// Validate format against whitelist to prevent URL parameter injection
	if format != "" && format != "json" && format != "csv" {
		format = "json" // default to json for unknown formats
	}

	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("session not found: %w", err)
	}

	path := tts.buildAPIPath("/history", terminal.InstanceType)
	url := fmt.Sprintf("%s%s?id=%s", tts.baseURL, path, neturl.QueryEscape(sessionID))
	if since != nil {
		url += fmt.Sprintf("&since=%d", *since)
	}
	if format != "" {
		url += fmt.Sprintf("&format=%s", format)
	}
	if limit > 0 {
		url += fmt.Sprintf("&limit=%d", limit)
	}
	if offset > 0 {
		url += fmt.Sprintf("&offset=%d", offset)
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(terminal.UserTerminalKey.APIKey))

	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "GET", url, nil, opts)
	if err != nil {
		return nil, "", err
	}

	// Enforce a 10MB cap on response body to prevent OOM from oversized payloads
	const maxResponseSize = 10 * 1024 * 1024 // 10MB
	if len(resp.Body) > maxResponseSize {
		return nil, "", fmt.Errorf("response body exceeds maximum allowed size (%d bytes > %d bytes)", len(resp.Body), maxResponseSize)
	}

	// Read content-type from tt-backend response when available; fall back to
	// format-based heuristic when the upstream does not provide a header.
	contentType := resp.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
		if format == "csv" {
			contentType = "text/csv"
		}
	}

	return resp.Body, contentType, nil
}

// GetSessionCommandHistoryAdmin retrieves command history for a single tt-backend
// session UUID using the OCF admin API key. Used by trainer endpoints where the
// requesting user is a group manager and does not own the student's session key.
//
// It proxies to tt-backend's bulk history endpoint with a single UUID, returning
// the response body verbatim ({commands, total, limit, offset}). Returns the
// raw JSON bytes plus the content type. limit defaults to 50 and is capped at
// 1000 by tt-backend; offset defaults to 0.
func (tts *terminalTrainerService) GetSessionCommandHistoryAdmin(sessionUUID string, limit, offset int) ([]byte, string, error) {
	if tts.baseURL == "" || tts.adminKey == "" {
		return nil, "", fmt.Errorf("terminal trainer not configured")
	}

	url := fmt.Sprintf("%s/%s/admin/history/bulk", tts.baseURL, tts.apiVersion)

	reqBody := map[string]interface{}{
		"session_uuids": []string{sessionUUID},
		"limit":         limit,
		"offset":        offset,
		"format":        "json",
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "POST", url, reqBody, opts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch session history: %w", err)
	}

	const maxResponseSize = 10 * 1024 * 1024 // 10MB
	if len(resp.Body) > maxResponseSize {
		return nil, "", fmt.Errorf("response body exceeds maximum allowed size (%d bytes > %d bytes)", len(resp.Body), maxResponseSize)
	}

	contentType := resp.Headers.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	return resp.Body, contentType, nil
}

// DeleteSessionCommandHistory deletes command history (RGPD right to erasure)
func (tts *terminalTrainerService) DeleteSessionCommandHistory(sessionID string) error {
	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	path := tts.buildAPIPath("/history", terminal.InstanceType)
	url := fmt.Sprintf("%s%s?id=%s", tts.baseURL, path, neturl.QueryEscape(sessionID))

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(terminal.UserTerminalKey.APIKey))

	_, err = utils.MakeExternalAPIRequest("Terminal Trainer", "DELETE", url, nil, opts)
	return err
}

// DeleteAllUserCommandHistory deletes all command history across all sessions for an API key (RGPD bulk erasure)
func (tts *terminalTrainerService) DeleteAllUserCommandHistory(apiKey string) (int64, error) {
	path := fmt.Sprintf("/%s/history/all", tts.apiVersion)
	url := fmt.Sprintf("%s%s", tts.baseURL, path)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(apiKey))

	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "DELETE", url, nil, opts)
	if err != nil {
		return 0, err
	}

	var result struct {
		SessionsCleared int64 `json:"sessions_cleared"`
	}
	if err := resp.DecodeJSON(&result); err != nil {
		return 0, fmt.Errorf("failed to decode bulk delete response: %w", err)
	}

	return result.SessionsCleared, nil
}

// SetSystemDefaultBackend sets the system-wide default backend by calling
// tt-backend's admin API to mark the backend as default.
func (tts *terminalTrainerService) SetSystemDefaultBackend(backendID string) (*dto.BackendInfo, error) {
	// Verify backend exists and is connected
	backends, err := tts.getBackendsCached()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch backends: %w", err)
	}

	var target *dto.BackendInfo
	for i := range backends {
		if backends[i].ID == backendID {
			target = &backends[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("backend not found: %s", backendID)
	}
	if !target.Connected {
		return nil, fmt.Errorf("backend is offline: %s", backendID)
	}

	// Find the numeric DB ID by listing admin backends
	adminBackends, err := tts.getAdminBackends()
	if err != nil {
		return nil, fmt.Errorf("failed to list admin backends: %w", err)
	}

	var adminEntry *adminBackendEntry
	for i := range adminBackends {
		if adminBackends[i].BackendID == backendID {
			adminEntry = &adminBackends[i]
			break
		}
	}
	if adminEntry == nil {
		return nil, fmt.Errorf("backend not found in admin API: %s", backendID)
	}

	// Call PUT /admin/backends/{id} with is_default=true, preserving all existing fields
	isDefault := true
	updateReq := struct {
		Name              string `json:"name"`
		Description       string `json:"description,omitempty"`
		IsDefault         *bool  `json:"is_default"`
		IsActive          bool   `json:"is_active"`
		ServerURL         string `json:"server_url,omitempty"`
		ServerCertificate string `json:"server_certificate,omitempty"`
		ClientCertificate string `json:"client_certificate,omitempty"`
		Project           string `json:"project,omitempty"`
		Target            string `json:"target,omitempty"`
	}{
		Name:              adminEntry.Name,
		Description:       adminEntry.Description,
		IsDefault:         &isDefault,
		IsActive:          adminEntry.IsActive,
		ServerURL:         adminEntry.ServerURL,
		ServerCertificate: adminEntry.ServerCertificate,
		ClientCertificate: adminEntry.ClientCertificate,
		Project:           adminEntry.Project,
		Target:            adminEntry.Target,
	}

	url := fmt.Sprintf("%s/%s/admin/backends/%d", tts.baseURL, tts.apiVersion, adminEntry.ID)
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	_, err = utils.MakeExternalAPIRequest("Terminal Trainer", "PUT", url, updateReq, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to set default backend on tt-backend: %w", err)
	}

	// Invalidate backend cache so next read picks up the change
	tts.invalidateBackendCache()

	target.IsDefault = true
	return target, nil
}

// adminBackendEntry represents a backend from tt-backend's admin API
type adminBackendEntry struct {
	ID                int64  `json:"id"`
	BackendID         string `json:"backend_id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	IsDefault         bool   `json:"is_default"`
	IsActive          bool   `json:"is_active"`
	ServerURL         string `json:"server_url,omitempty"`
	ServerCertificate string `json:"server_certificate,omitempty"`
	ClientCertificate string `json:"client_certificate,omitempty"`
	Project           string `json:"project,omitempty"`
	Target            string `json:"target,omitempty"`
	Connected         bool   `json:"connected"`
}

type adminAPIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

func (tts *terminalTrainerService) getAdminBackends() ([]adminBackendEntry, error) {
	url := fmt.Sprintf("%s/%s/admin/backends", tts.baseURL, tts.apiVersion)
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	var resp adminAPIResponse
	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &resp, opts)
	if err != nil {
		return nil, err
	}

	var backends []adminBackendEntry
	if err := json.Unmarshal(resp.Data, &backends); err != nil {
		return nil, fmt.Errorf("failed to decode admin backends: %w", err)
	}
	return backends, nil
}

func (tts *terminalTrainerService) GetOrganizationTerminalSessions(orgID uuid.UUID) (*[]models.Terminal, error) {
	return tts.repository.GetTerminalSessionsByOrganizationID(orgID)
}

// LookupCasdoorUserForOrgUsage is a swappable seam for the Casdoor user
// lookup used by GetOrgTerminalUsage to enrich the per-user breakdown with
// DisplayName + Email. Defaults to the package-level Casdoor SDK function;
// tests replace it with a fake to avoid standing up a real Casdoor server.
//
// Mirrors the pattern used in src/auth/routes/usersRoutes/getCurrentUser.go
// (LookupCasdoorUser). Resolution logically duplicates a slice-loop over
// auth/services.UserService.GetUsersByIds, but importing auth/services here
// would create a cycle (auth/services already imports this package). The
// seam keeps the call site honest about graceful degradation without
// introducing the cycle — see code comment in GetOrgTerminalUsage below.
var LookupCasdoorUserForOrgUsage = casdoorsdk.GetUserByUserId

// GetOrgTerminalUsage returns aggregated active terminal usage for an organization:
// the org's effective plan limits, and a per-user breakdown of active terminal counts.
func (tts *terminalTrainerService) GetOrgTerminalUsage(orgID uuid.UUID) (*dto.OrgTerminalUsageResponse, error) {
	// 1. Resolve the org's effective plan via EffectivePlanService (owner's plan in the org context).
	effectivePlanSvc := paymentServices.NewEffectivePlanService(tts.db)

	// Load the org to find owner
	var org orgModels.Organization
	if err := tts.db.First(&org, "id = ?", orgID).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	orgIDPtr := orgID
	planResult, err := effectivePlanSvc.GetUserEffectivePlan(org.OwnerUserID, &orgIDPtr)

	planName := "unknown"
	maxTerminals := 0
	isFallback := false
	if err == nil && planResult != nil && planResult.Plan != nil {
		planName = planResult.Plan.Name
		maxTerminals = planResult.Plan.MaxConcurrentTerminals
		isFallback = planResult.IsFallback
	}

	// 2. Count active terminals (what's running) per user — display field.
	//
	// SSOT: "currently running terminals" is owned by
	// models.RunningDisplayScope. Both this counter and the slot counter
	// below now route through their respective scopes so the dashboard
	// stays internally consistent (no "1 active / 0 occupying" zombie
	// drift). RunningDisplayScope intentionally narrows further than
	// OccupiesSlotScope (status='active' only, not active+stopped)
	// because "active_terminals" reports what is currently running; the
	// slot count uses OccupiesSlotScope for quota semantics.
	type userCount struct {
		UserID      string
		ActiveCount int64
	}
	var activeRows []userCount
	err = tts.db.Table("terminals").
		Scopes(models.RunningDisplayScope).
		Where("terminals.organization_id = ?", orgID).
		Select("terminals.user_id as user_id, COUNT(*) as active_count").
		Group("terminals.user_id").
		Scan(&activeRows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count active terminals: %w", err)
	}

	// 3. Count quota-relevant slot occupancy per user using the SSOT scope
	// (models.OccupiesSlotScope). Stopped sessions still occupy a slot and
	// must be reflected separately from the running-only display count.
	//
	// NOTE: this uses the direct terminals.organization_id column, NOT the
	// member-join shape of models.CountOrgOccupiedSlots. The two semantics
	// disagree:
	//   - dashboard (here): "terminals attributed to this org via the
	//     terminals.organization_id column" — what dashboards historically
	//     displayed per-user for the org.
	//   - quota (CountOrgOccupiedSlots): "terminals owned by any member of
	//     the org" — the production quota gate semantics.
	// Aligning the two is a real product-behavior change; tracked separately
	// so this security-fix branch stays behavior-preserving. We deliberately
	// do NOT use CountOrgOccupiedSlots here for that reason, and we cannot
	// use a flat helper anyway because this query is a GROUP BY user_id.
	type userSlotCount struct {
		UserID         string
		OccupyingCount int64
	}
	var slotRows []userSlotCount
	err = tts.db.Table("terminals").
		Scopes(models.OccupiesSlotScope).
		Select("terminals.user_id as user_id, COUNT(*) as occupying_count").
		Where("terminals.organization_id = ?", orgID).
		Group("terminals.user_id").
		Scan(&slotRows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count slot-occupying terminals: %w", err)
	}

	// 4. Build per-user entries merging both counts. A user may appear in
	// slotRows but not in activeRows (e.g. all their sessions are stopped),
	// so we key by UserID and union the two sets.
	type userEntry struct {
		active    int
		occupying int
	}
	byUser := make(map[string]*userEntry)
	for _, row := range activeRows {
		e, ok := byUser[row.UserID]
		if !ok {
			e = &userEntry{}
			byUser[row.UserID] = e
		}
		e.active = int(row.ActiveCount)
	}
	for _, row := range slotRows {
		e, ok := byUser[row.UserID]
		if !ok {
			e = &userEntry{}
			byUser[row.UserID] = e
		}
		e.occupying = int(row.OccupyingCount)
	}

	// 5. Resolve DisplayName + Email per user via Casdoor. The Casdoor SDK
	// has no bulk endpoint, so this is an N+1 loop. Acceptable for a
	// dashboard with a handful of users on a ~30s auto-refresh; if the
	// shape ever grows to hundreds of users, introduce caching at the
	// seam. Fallback chain: DisplayName -> Name -> Email -> userID, so
	// users with missing or partial Casdoor records still get a sensible
	// label.
	//
	// SSOT note: this mirrors authServices.UserService.GetUsersByIds
	// (auth/services/userService.go) which loops over GetUserByUserId
	// with the same graceful-degradation contract. We cannot reuse that
	// helper here because auth/services already imports this package
	// (would create an import cycle). The seam below keeps the resolution
	// rule visible in one place. If a third caller appears, lift this
	// loop into a shared helper in a leaf package both can import.
	type userInfo struct {
		DisplayName string
		Email       string
	}
	infoByUser := make(map[string]userInfo, len(byUser))
	for userID := range byUser {
		user, lookupErr := LookupCasdoorUserForOrgUsage(userID)
		if lookupErr != nil || user == nil {
			infoByUser[userID] = userInfo{DisplayName: userID, Email: ""}
			continue
		}
		label := user.DisplayName
		if label == "" {
			label = user.Name
		}
		if label == "" {
			label = user.Email
		}
		if label == "" {
			label = userID
		}
		infoByUser[userID] = userInfo{DisplayName: label, Email: user.Email}
	}

	totalActive := 0
	totalOccupying := 0
	users := make([]dto.OrgTerminalUsageUser, 0, len(byUser))
	for userID, e := range byUser {
		totalActive += e.active
		totalOccupying += e.occupying
		info := infoByUser[userID]
		users = append(users, dto.OrgTerminalUsageUser{
			UserID:         userID,
			DisplayName:    info.DisplayName,
			Email:          info.Email,
			ActiveCount:    e.active,
			OccupyingSlots: e.occupying,
		})
	}

	return &dto.OrgTerminalUsageResponse{
		OrganizationID:  orgID.String(),
		ActiveTerminals: totalActive,
		OccupyingSlots:  totalOccupying,
		MaxTerminals:    maxTerminals,
		PlanName:        planName,
		IsFallback:      isFallback,
		Users:           users,
	}, nil
}

// IsUserAuthorizedForSession checks if a user is authorized to access a terminal session.
// Returns true if the user is the session owner, an admin in the session's org,
// or an org owner/manager of the session's org.
func (tts *terminalTrainerService) IsUserAuthorizedForSession(userID string, terminal *models.Terminal, isAdmin bool) bool {
	// Session owner always has access
	if terminal.UserID == userID {
		return true
	}
	// Check organization-scoped access (admin, org owner, or org manager)
	if terminal.OrganizationID != nil {
		var orgMember orgModels.OrganizationMember
		err := tts.db.Where(
			"organization_id = ? AND user_id = ? AND is_active = ?",
			*terminal.OrganizationID, userID, true,
		).First(&orgMember).Error
		if err == nil {
			if orgMember.IsManager() || isAdmin {
				return true
			}
		}
		// Also check if user is the organization owner directly
		var org orgModels.Organization
		err = tts.db.Where("id = ?", *terminal.OrganizationID).First(&org).Error
		if err == nil && org.OwnerUserID == userID {
			return true
		}
	}
	return false
}

// History row-count caps.
//
// JSON pagination is 1k — loading more rows into the DOM is a UX/perf issue.
// CSV export is 100k — sized for exam-scale exports (~5k observed in the field)
// with two orders of magnitude of headroom while still bounded for memory safety.
const (
	defaultHistoryLimit = 50
	maxJSONHistoryLimit = 1000
	maxCSVHistoryLimit  = 100000
)

// ClampHistoryLimit returns the effective row limit for a command-history
// request, applying defaults (when limit <= 0) and a format-aware ceiling
// (1000 for JSON/default, 100000 for CSV).
func ClampHistoryLimit(limit int, format string) int {
	if limit <= 0 {
		return defaultHistoryLimit
	}
	cap := maxJSONHistoryLimit
	if format == "csv" {
		cap = maxCSVHistoryLimit
	}
	if limit > cap {
		return cap
	}
	return limit
}

// GetGroupCommandHistory aggregates command history for all active members of a group.
// Only group owner, admin, or assistant can access this endpoint.
func (tts *terminalTrainerService) GetGroupCommandHistory(groupID string, userID string, since *int64, format string, limit, offset int, includeStopped bool, search string) ([]byte, string, error) {
	// Validate and default format
	if format != "" && format != "json" && format != "csv" {
		format = "json"
	}
	if format == "" {
		format = "json"
	}

	limit = ClampHistoryLimit(limit, format)

	// Parse groupID to UUID
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, "", fmt.Errorf("invalid group ID: %w", err)
	}

	// Fetch group with members from DB
	var group groupModels.ClassGroup
	if err := tts.db.Preload("Members").Where("id = ?", groupUUID).First(&group).Error; err != nil {
		return nil, "", fmt.Errorf("group not found")
	}

	// Check authorization - user must be owner, admin, or assistant
	var userMember *groupModels.GroupMember
	for i := range group.Members {
		if group.Members[i].UserID == userID && group.Members[i].IsActive {
			userMember = &group.Members[i]
			break
		}
	}
	if userMember == nil || !(userMember.Role == groupModels.GroupMemberRoleOwner || userMember.Role == groupModels.GroupMemberRoleManager) {
		return nil, "", fmt.Errorf("unauthorized: only group owner or manager can view group command history")
	}

	// Collect active member user IDs
	var memberUserIDs []string
	for _, m := range group.Members {
		if m.IsActive {
			memberUserIDs = append(memberUserIDs, m.UserID)
		}
	}

	// Fetch terminals for all members, scoped to group's organization
	var terminals []models.Terminal
	query := tts.db.Where("user_id IN ?", memberUserIDs)
	if group.OrganizationID != nil {
		query = query.Where("organization_id = ?", *group.OrganizationID)
	}
	if !includeStopped {
		// SSOT: "running display" lives in models.RunningDisplayScope.
		// Inline `status = "active"` predicates drift away from the UI's
		// per-second-aware view (past-expiry zombies remain in results
		// even though their proxy session is gone).
		query = query.Scopes(models.RunningDisplayScope)
	}
	if err := query.Find(&terminals).Error; err != nil {
		return nil, "", fmt.Errorf("failed to query terminals: %w", err)
	}

	// Collect session UUIDs and track unique user IDs
	sessionUUIDs := make([]string, 0, len(terminals))
	userIDSet := make(map[string]bool)
	for _, t := range terminals {
		if t.SessionID != "" {
			sessionUUIDs = append(sessionUUIDs, t.SessionID)
			userIDSet[t.UserID] = true
		}
	}

	// If no sessions found, return empty result
	if len(sessionUUIDs) == 0 {
		if format == "csv" {
			var buf bytes.Buffer
			writer := csv.NewWriter(&buf)
			_ = writer.Write([]string{"student_name", "student_email", "session_uuid", "sequence_num", "command", "executed_at"})
			writer.Flush()
			return buf.Bytes(), "text/csv", nil
		}
		result := map[string]interface{}{
			"commands": []interface{}{},
			"total":    0,
			"limit":    limit,
			"offset":   offset,
		}
		body, _ := json.Marshal(result)
		return body, "application/json", nil
	}

	// Fetch user info for enrichment using Casdoor SDK
	type userInfo struct {
		DisplayName string
		Email       string
	}
	userInfoMap := make(map[string]userInfo)
	for uid := range userIDSet {
		user, err := casdoorsdk.GetUserByUserId(uid)
		if err == nil && user != nil {
			userInfoMap[uid] = userInfo{
				DisplayName: user.DisplayName,
				Email:       user.Email,
			}
		}
	}

	// Build sessionUUID -> userInfo map
	sessionUserMap := make(map[string]userInfo)
	for _, t := range terminals {
		if t.SessionID != "" {
			sessionUserMap[t.SessionID] = userInfoMap[t.UserID]
		}
	}

	// Call tt-backend bulk endpoint
	url := fmt.Sprintf("%s/%s/admin/history/bulk", tts.baseURL, tts.apiVersion)

	reqBody := map[string]interface{}{
		"session_uuids": sessionUUIDs,
		"limit":         limit,
		"offset":        offset,
		"format":        "json", // Always get JSON from tt-backend, we transform to CSV ourselves if needed
	}
	if since != nil {
		reqBody["since"] = *since
	}
	if search != "" {
		reqBody["search"] = search
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	var bulkResponse struct {
		Commands []struct {
			SessionUUID string `json:"session_uuid"`
			SequenceNum int    `json:"sequence_num"`
			CommandText string `json:"command_text"`
			ExecutedAt  int64  `json:"executed_at"`
		} `json:"commands"`
		Total  int `json:"total"`
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}

	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, reqBody, &bulkResponse, opts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch bulk history: %w", err)
	}

	// Enrich commands with student info
	type enrichedCommand struct {
		StudentName  string `json:"student_name"`
		StudentEmail string `json:"student_email"`
		SessionUUID  string `json:"session_uuid"`
		SequenceNum  int    `json:"sequence_num"`
		CommandText  string `json:"command_text"`
		ExecutedAt   int64  `json:"executed_at"`
	}

	enriched := make([]enrichedCommand, 0, len(bulkResponse.Commands))
	for _, cmd := range bulkResponse.Commands {
		info := sessionUserMap[cmd.SessionUUID]
		enriched = append(enriched, enrichedCommand{
			StudentName:  info.DisplayName,
			StudentEmail: info.Email,
			SessionUUID:  cmd.SessionUUID,
			SequenceNum:  cmd.SequenceNum,
			CommandText:  cmd.CommandText,
			ExecutedAt:   cmd.ExecutedAt,
		})
	}

	// Return in requested format
	if format == "csv" {
		var buf bytes.Buffer
		writer := csv.NewWriter(&buf)
		_ = writer.Write([]string{"student_name", "student_email", "session_uuid", "sequence_num", "command", "executed_at"})
		for _, cmd := range enriched {
			_ = writer.Write([]string{
				cmd.StudentName,
				cmd.StudentEmail,
				cmd.SessionUUID,
				strconv.Itoa(cmd.SequenceNum),
				cmd.CommandText,
				time.Unix(cmd.ExecutedAt, 0).UTC().Format(time.RFC3339),
			})
		}
		writer.Flush()
		return buf.Bytes(), "text/csv", nil
	}

	// JSON format (default)
	result := map[string]interface{}{
		"commands": enriched,
		"total":    bulkResponse.Total,
		"limit":    bulkResponse.Limit,
		"offset":   bulkResponse.Offset,
	}
	body, _ := json.Marshal(result)
	return body, "application/json", nil
}

// GetGroupCommandHistoryStats returns aggregate command history statistics for all members of a group.
// Only group owner, admin, or assistant can access this endpoint.
func (tts *terminalTrainerService) GetGroupCommandHistoryStats(groupID string, userID string, includeStopped bool) ([]byte, string, error) {
	// Parse groupID to UUID
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, "", fmt.Errorf("invalid group ID: %w", err)
	}

	// Fetch group with members from DB
	var group groupModels.ClassGroup
	if err := tts.db.Preload("Members").Where("id = ?", groupUUID).First(&group).Error; err != nil {
		return nil, "", fmt.Errorf("group not found")
	}

	// Check authorization - user must be owner, admin, or assistant
	var userMember *groupModels.GroupMember
	for i := range group.Members {
		if group.Members[i].UserID == userID && group.Members[i].IsActive {
			userMember = &group.Members[i]
			break
		}
	}
	if userMember == nil || !(userMember.Role == groupModels.GroupMemberRoleOwner || userMember.Role == groupModels.GroupMemberRoleManager) {
		return nil, "", fmt.Errorf("unauthorized: only group owner or manager can view group command history stats")
	}

	// Collect active member user IDs
	var memberUserIDs []string
	for _, m := range group.Members {
		if m.IsActive {
			memberUserIDs = append(memberUserIDs, m.UserID)
		}
	}

	// Fetch terminals for all members, scoped to group's organization
	var terminals []models.Terminal
	query := tts.db.Where("user_id IN ?", memberUserIDs)
	if group.OrganizationID != nil {
		query = query.Where("organization_id = ?", *group.OrganizationID)
	}
	if !includeStopped {
		// SSOT alignment — see RunningDisplayScope rationale on the
		// matching block in GetGroupCommandHistory above.
		query = query.Scopes(models.RunningDisplayScope)
	}
	if err := query.Find(&terminals).Error; err != nil {
		return nil, "", fmt.Errorf("failed to query terminals: %w", err)
	}

	// Collect session UUIDs and build terminal -> user mapping
	sessionUUIDs := make([]string, 0, len(terminals))
	userIDSet := make(map[string]bool)
	sessionToUserID := make(map[string]string)
	for _, t := range terminals {
		if t.SessionID != "" {
			sessionUUIDs = append(sessionUUIDs, t.SessionID)
			userIDSet[t.UserID] = true
			sessionToUserID[t.SessionID] = t.UserID
		}
	}

	// If no sessions found, return empty stats
	if len(sessionUUIDs) == 0 {
		result := map[string]interface{}{
			"summary": map[string]interface{}{
				"total_commands":              0,
				"total_sessions":              0,
				"active_students":             0,
				"avg_commands_per_student":     0.0,
				"avg_time_per_student_seconds": 0,
			},
			"students": []interface{}{},
		}
		body, _ := json.Marshal(result)
		return body, "application/json", nil
	}

	// Fetch user info for enrichment using Casdoor SDK
	type userInfo struct {
		DisplayName string
		Email       string
	}
	userInfoMap := make(map[string]userInfo)
	for uid := range userIDSet {
		user, err := casdoorsdk.GetUserByUserId(uid)
		if err == nil && user != nil {
			userInfoMap[uid] = userInfo{
				DisplayName: user.DisplayName,
				Email:       user.Email,
			}
		}
	}

	// Call tt-backend bulk-stats endpoint
	url := fmt.Sprintf("%s/%s/admin/history/bulk-stats", tts.baseURL, tts.apiVersion)

	reqBody := map[string]interface{}{
		"session_uuids": sessionUUIDs,
	}

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	var bulkStatsResponse struct {
		Sessions []struct {
			SessionUUID    string `json:"session_uuid"`
			CommandCount   int    `json:"command_count"`
			FirstCommandAt int64  `json:"first_command_at"`
			LastCommandAt  int64  `json:"last_command_at"`
		} `json:"sessions"`
		TotalCommands int `json:"total_commands"`
		TotalSessions int `json:"total_sessions"`
	}

	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "POST", url, reqBody, &bulkStatsResponse, opts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch bulk stats: %w", err)
	}

	// Build per-student stats
	type studentStats struct {
		StudentName      string `json:"student_name"`
		StudentEmail     string `json:"student_email"`
		TotalCommands    int    `json:"total_commands"`
		SessionCount     int    `json:"session_count"`
		TotalTimeSeconds int64  `json:"total_time_seconds"`
		LastActiveAt     int64  `json:"last_active_at"`
	}

	studentMap := make(map[string]*studentStats)
	for _, sess := range bulkStatsResponse.Sessions {
		uid, ok := sessionToUserID[sess.SessionUUID]
		if !ok {
			continue
		}
		st, exists := studentMap[uid]
		if !exists {
			info := userInfoMap[uid]
			st = &studentStats{
				StudentName:  info.DisplayName,
				StudentEmail: info.Email,
			}
			studentMap[uid] = st
		}
		st.TotalCommands += sess.CommandCount
		st.SessionCount++
		if sess.LastCommandAt > sess.FirstCommandAt {
			st.TotalTimeSeconds += sess.LastCommandAt - sess.FirstCommandAt
		}
		if sess.LastCommandAt > st.LastActiveAt {
			st.LastActiveAt = sess.LastCommandAt
		}
	}

	// Convert to slice
	students := make([]studentStats, 0, len(studentMap))
	for _, st := range studentMap {
		students = append(students, *st)
	}

	// Build summary
	activeStudents := len(studentMap)
	var avgCommandsPerStudent float64
	var avgTimePerStudentSecs int64
	if activeStudents > 0 {
		avgCommandsPerStudent = float64(bulkStatsResponse.TotalCommands) / float64(activeStudents)
		var totalTime int64
		for _, st := range students {
			totalTime += st.TotalTimeSeconds
		}
		avgTimePerStudentSecs = totalTime / int64(activeStudents)
	}

	type statsSummary struct {
		TotalCommands         int     `json:"total_commands"`
		TotalSessions         int     `json:"total_sessions"`
		ActiveStudents        int     `json:"active_students"`
		AvgCommandsPerStudent float64 `json:"avg_commands_per_student"`
		AvgTimePerStudentSecs int64   `json:"avg_time_per_student_seconds"`
	}

	result := map[string]interface{}{
		"summary": statsSummary{
			TotalCommands:         bulkStatsResponse.TotalCommands,
			TotalSessions:         bulkStatsResponse.TotalSessions,
			ActiveStudents:        activeStudents,
			AvgCommandsPerStudent: avgCommandsPerStudent,
			AvgTimePerStudentSecs: avgTimePerStudentSecs,
		},
		"students": students,
	}

	body, _ := json.Marshal(result)
	return body, "application/json", nil
}

// IsUserOrgManagerOrAdmin checks if a user is an org owner/manager or a system admin
// who is also a member of the given organization.
func (tts *terminalTrainerService) IsUserOrgManagerOrAdmin(userID string, orgID uuid.UUID, isAdmin bool) bool {
	var orgMember orgModels.OrganizationMember
	err := tts.db.Where(
		"organization_id = ? AND user_id = ? AND is_active = ?",
		orgID, userID, true,
	).First(&orgMember).Error
	if err == nil {
		if orgMember.IsManager() || isAdmin {
			return true
		}
	}
	return false
}

// GetUserConsentStatus checks if recording consent policy is handled at the org or group level
// for a given user. Returns consentHandled=true if any org or group the user belongs to
// has recording_consent_handled set. The source indicates "organization" or "group".
// Note: recording is always enabled (RGPD Art. 6.1.f), this checks org/group policy status.
func (tts *terminalTrainerService) GetUserConsentStatus(userID string) (bool, string, error) {
	// Check organizations: find orgs where user is an active member with consent handled
	var orgMembers []orgModels.OrganizationMember
	if err := tts.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&orgMembers).Error; err != nil {
		return false, "", fmt.Errorf("failed to check organization membership: %w", err)
	}

	for _, member := range orgMembers {
		var org orgModels.Organization
		if err := tts.db.Where("id = ? AND is_active = ?", member.OrganizationID, true).First(&org).Error; err != nil {
			continue
		}
		if org.RecordingConsentHandled {
			return true, "organization", nil
		}
	}

	// Check groups: find groups where user is an active member
	var groupMembers []groupModels.GroupMember
	if err := tts.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&groupMembers).Error; err != nil {
		return false, "", fmt.Errorf("failed to check group membership: %w", err)
	}

	for _, member := range groupMembers {
		var group groupModels.ClassGroup
		if err := tts.db.Where("id = ? AND is_active = ?", member.GroupID, true).First(&group).Error; err != nil {
			continue
		}
		// Group-level override: explicit true means consent handled
		if group.RecordingConsentHandled != nil && *group.RecordingConsentHandled {
			return true, "group", nil
		}
		// Group inherits from org if nil and org has consent handled
		if group.RecordingConsentHandled == nil && group.OrganizationID != nil {
			var org orgModels.Organization
			if err := tts.db.Where("id = ? AND is_active = ?", *group.OrganizationID, true).First(&org).Error; err == nil {
				if org.RecordingConsentHandled {
					return true, "organization", nil
				}
			}
		}
	}

	return false, "", nil
}

// checkGroupOwnerAccess checks if requestingUserID is the owner of any active group
// where terminalOwnerUserID is an active member. This gives group owners implicit
// access to their members' terminals.
func (tts *terminalTrainerService) checkGroupOwnerAccess(terminalOwnerUserID, requestingUserID string) (bool, error) {
	var count int64
	err := tts.db.Table("class_groups").
		Joins("JOIN group_members ON class_groups.id = group_members.group_id").
		Where("class_groups.owner_user_id = ?", requestingUserID).
		Where("group_members.user_id = ?", terminalOwnerUserID).
		Where("group_members.is_active = ?", true).
		Where("class_groups.is_active = ?", true).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ==========================================
// Composed Session (Phase 4)
// ==========================================

const catalogCacheTTL = 60 * time.Second

// featurePlanMapping maps feature keys to plan predicates.
//
// Persistence is intentionally NOT in this map: the persistent-vs-ephemeral
// choice is surfaced as a persistence_mode radio (in TerminalAdvancedOptions),
// not as a "feature" chip in the SessionComposer. It is gated upstream by
// resolvePersistenceMode reading plan.DataPersistenceEnabled (SSOT).
var featurePlanMapping = map[string]func(*paymentModels.SubscriptionPlan) bool{
	"network": func(p *paymentModels.SubscriptionPlan) bool { return p.NetworkAccessEnabled },
}

// NormalizeSizeKey uppercases and trims a size key for comparison
func NormalizeSizeKey(key string) string {
	return strings.ToUpper(strings.TrimSpace(key))
}

// GetDistributions fetches available distributions from tt-backend
func (tts *terminalTrainerService) GetDistributions(backend string) ([]dto.TTDistribution, error) {
	url := fmt.Sprintf("%s/%s/distributions", tts.baseURL, tts.apiVersion)
	if backend != "" {
		url += fmt.Sprintf("?backend=%s", backend)
	}

	var distributions []dto.TTDistribution
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &distributions, opts)
	if err != nil {
		return nil, err
	}
	return distributions, nil
}

// GetCatalogSizes fetches sizes from tt-backend with a 60s TTL cache
func (tts *terminalTrainerService) GetCatalogSizes() ([]dto.TTSize, error) {
	tts.catalogSizesMu.RLock()
	if tts.catalogSizesCache != nil && time.Since(tts.catalogSizesCacheTime) < catalogCacheTTL {
		cached := tts.catalogSizesCache
		tts.catalogSizesMu.RUnlock()
		return cached, nil
	}
	tts.catalogSizesMu.RUnlock()

	url := fmt.Sprintf("%s/%s/sizes", tts.baseURL, tts.apiVersion)
	var sizes []dto.TTSize
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sizes, opts)
	if err != nil {
		return nil, err
	}

	tts.catalogSizesMu.Lock()
	tts.catalogSizesCache = sizes
	tts.catalogSizesCacheTime = time.Now()
	tts.catalogSizesMu.Unlock()

	return sizes, nil
}

// GetCatalogFeatures fetches features from tt-backend with a 60s TTL cache
func (tts *terminalTrainerService) GetCatalogFeatures() ([]dto.TTFeature, error) {
	tts.catalogFeaturesMu.RLock()
	if tts.catalogFeaturesCache != nil && time.Since(tts.catalogFeaturesCacheTime) < catalogCacheTTL {
		cached := tts.catalogFeaturesCache
		tts.catalogFeaturesMu.RUnlock()
		return cached, nil
	}
	tts.catalogFeaturesMu.RUnlock()

	url := fmt.Sprintf("%s/%s/features", tts.baseURL, tts.apiVersion)
	var features []dto.TTFeature
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(tts.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &features, opts)
	if err != nil {
		return nil, err
	}

	tts.catalogFeaturesMu.Lock()
	tts.catalogFeaturesCache = features
	tts.catalogFeaturesCacheTime = time.Now()
	tts.catalogFeaturesMu.Unlock()

	return features, nil
}

// ComputeSessionOptions computes allowed sizes and features given catalogs, a distribution, and a plan.
// Exported for testing.
func ComputeSessionOptions(
	distro dto.TTDistribution,
	allSizes []dto.TTSize,
	allFeatures []dto.TTFeature,
	plan *paymentModels.SubscriptionPlan,
) *dto.SessionOptionsResponse {
	// Build a lookup of size sort orders by normalized key
	sizeSortOrder := make(map[string]int, len(allSizes))
	for _, s := range allSizes {
		sizeSortOrder[NormalizeSizeKey(s.Key)] = s.SortOrder
	}

	// Determine the minimum size sort order for this distribution
	minSortOrder := 0
	if distro.MinSizeKey != "" {
		if so, ok := sizeSortOrder[NormalizeSizeKey(distro.MinSizeKey)]; ok {
			minSortOrder = so
		}
	}

	// Build the set of plan-allowed size keys
	planAllowsAll := false
	planSizeSet := make(map[string]bool, len(plan.AllowedMachineSizes))
	for _, s := range plan.AllowedMachineSizes {
		norm := NormalizeSizeKey(s)
		if norm == "ALL" {
			planAllowsAll = true
		}
		planSizeSet[norm] = true
	}

	// Evaluate each size
	allowedSizes := make([]dto.SessionOptionSize, 0, len(allSizes))
	for _, s := range allSizes {
		opt := dto.SessionOptionSize{TTSize: s, Allowed: true}
		normKey := NormalizeSizeKey(s.Key)

		if s.SortOrder < minSortOrder {
			continue
		}
		if !planAllowsAll && !planSizeSet[normKey] {
			opt.Allowed = false
			opt.Reason = "plan_limit"
		}

		allowedSizes = append(allowedSizes, opt)
	}

	// Build a set of the distribution's supported features
	supportedFeatures := make(map[string]bool, len(distro.SupportedFeatures))
	for _, f := range distro.SupportedFeatures {
		supportedFeatures[f] = true
	}

	// Find the minimum sort order among allowed sizes (for min_size_key feature check)
	maxAllowedSortOrder := 0
	for _, s := range allowedSizes {
		if s.Allowed && s.SortOrder > maxAllowedSortOrder {
			maxAllowedSortOrder = s.SortOrder
		}
	}

	// Evaluate each feature
	allowedFeatures := make([]dto.SessionOptionFeature, 0, len(allFeatures))
	for _, f := range allFeatures {
		opt := dto.SessionOptionFeature{
			Key:         f.Key,
			Name:        f.Name,
			Description: f.Description,
			Allowed:     true,
		}

		if !supportedFeatures[f.Key] {
			opt.Allowed = false
			opt.Reason = "not_supported"
		} else if checker, ok := featurePlanMapping[f.Key]; ok && !checker(plan) {
			opt.Allowed = false
			opt.Reason = "plan_disabled"
		} else if f.MinSizeKey != "" {
			// Check if at least one allowed size meets the feature's minimum
			featureMinSortOrder := 0
			if so, ok := sizeSortOrder[NormalizeSizeKey(f.MinSizeKey)]; ok {
				featureMinSortOrder = so
			}
			if maxAllowedSortOrder < featureMinSortOrder {
				opt.Allowed = false
				opt.Reason = "size_too_small"
			}
		}

		allowedFeatures = append(allowedFeatures, opt)
	}

	return &dto.SessionOptionsResponse{
		Distribution:    distro,
		AllowedSizes:    allowedSizes,
		AllowedFeatures: allowedFeatures,
	}
}

// GetSessionOptions validates a distribution and computes plan-intersected options
func (tts *terminalTrainerService) GetSessionOptions(plan *paymentModels.SubscriptionPlan, distribution string, backend string) (*dto.SessionOptionsResponse, error) {
	distributions, err := tts.GetDistributions(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to get distributions: %w", err)
	}

	var distro *dto.TTDistribution
	for i := range distributions {
		if distributions[i].Name == distribution || distributions[i].Prefix == distribution {
			distro = &distributions[i]
			break
		}
	}
	if distro == nil {
		return nil, fmt.Errorf("distribution '%s' not found", distribution)
	}

	sizes, err := tts.GetCatalogSizes()
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog sizes: %w", err)
	}

	features, err := tts.GetCatalogFeatures()
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog features: %w", err)
	}

	return ComputeSessionOptions(*distro, sizes, features, plan), nil
}

// StartComposedSession validates inputs against the plan and starts a composed session
func (tts *terminalTrainerService) StartComposedSession(userID string, input dto.CreateComposedSessionInput, planInterface any) (*dto.TerminalSessionResponse, error) {
	plan, ok := planInterface.(*paymentModels.SubscriptionPlan)
	if !ok {
		return nil, fmt.Errorf("invalid subscription plan type")
	}

	// Resolve effective persistence mode (free tier hard-fails; empty defaults to ephemeral).
	// Done up-front so we never hit tt-backend with a request the plan forbids.
	effectiveMode, persistErr := resolvePersistenceMode(input.PersistenceMode, plan)
	if persistErr != nil {
		return nil, persistErr
	}
	input.PersistenceMode = effectiveMode

	// Compute session options to validate the request
	options, err := tts.GetSessionOptions(plan, input.Distribution, input.Backend)
	if err != nil {
		return nil, err
	}

	// Store the distribution prefix for console URL and InstanceType
	input.DistributionPrefix = options.Distribution.Prefix

	// Validate requested size
	requestedSizeNorm := NormalizeSizeKey(input.Size)
	sizeAllowed := false
	for _, s := range options.AllowedSizes {
		if NormalizeSizeKey(s.Key) == requestedSizeNorm {
			if !s.Allowed {
				return nil, fmt.Errorf("size '%s' is not allowed: %s", input.Size, s.Reason)
			}
			sizeAllowed = true
			break
		}
	}
	if !sizeAllowed {
		return nil, fmt.Errorf("size '%s' not found in catalog", input.Size)
	}

	// Validate requested features
	if input.Features != nil {
		featureAllowedMap := make(map[string]*dto.SessionOptionFeature, len(options.AllowedFeatures))
		for i := range options.AllowedFeatures {
			featureAllowedMap[options.AllowedFeatures[i].Key] = &options.AllowedFeatures[i]
		}
		for featureKey, enabled := range input.Features {
			if !enabled {
				continue
			}
			opt, exists := featureAllowedMap[featureKey]
			if !exists {
				return nil, fmt.Errorf("feature '%s' not found in catalog", featureKey)
			}
			if !opt.Allowed {
				return nil, fmt.Errorf("feature '%s' is not allowed: %s", featureKey, opt.Reason)
			}
		}
	}

	// Validate backend
	var orgID *uuid.UUID
	if input.OrganizationID != "" {
		parsed, err := uuid.Parse(input.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("invalid organization_id: %w", err)
		}
		orgID = &parsed
	}

	validatedBackend, err := tts.validateBackendForContext(orgID, plan, input.Backend)
	if err != nil {
		return nil, err
	}
	input.Backend = validatedBackend

	// Resolve effective idle window from the org override (if any). nil means
	// "let tt-backend fall back to its global default".
	input.IdleWindowSeconds = tts.resolveIdleWindowSeconds(orgID, input.PersistenceMode)

	// Enforce max session duration
	maxDurationSeconds := plan.MaxSessionDurationMinutes * 60
	if input.Expiry == 0 || input.Expiry > maxDurationSeconds {
		input.Expiry = maxDurationSeconds
	}

	// Set plan-derived fields
	input.HistoryRetentionDays = plan.CommandHistoryRetentionDays
	input.SubscriptionPlanID = &plan.ID

	return tts.startComposedSession(userID, input)
}

// resolveIdleWindowSeconds returns the org-level idle window override for the
// requested persistence mode, or nil if the org has no override (in which case
// tt-backend falls back to its globally-configured default).
func (tts *terminalTrainerService) resolveIdleWindowSeconds(orgID *uuid.UUID, mode string) *int {
	if orgID == nil {
		return nil
	}
	var org orgModels.Organization
	if err := tts.db.First(&org, "id = ?", *orgID).Error; err != nil {
		return nil
	}
	return computeIdleWindowSeconds(&org, mode)
}

// startComposedSession is the internal method that calls tt-backend's POST /sessions endpoint
func (tts *terminalTrainerService) startComposedSession(userID string, input dto.CreateComposedSessionInput) (*dto.TerminalSessionResponse, error) {
	// Get user key
	userKey, err := tts.repository.GetUserTerminalKeyByUserID(userID, true)
	if err != nil {
		return nil, fmt.Errorf("no terminal trainer key found for user: %w", err)
	}
	if !userKey.IsActive {
		return nil, fmt.Errorf("user terminal trainer key is disabled")
	}

	// Compute terms hash
	hash := sha256.New()
	io.WriteString(hash, input.Terms)
	termsHash := fmt.Sprintf("%x", hash.Sum(nil))

	// Clamp recording_enabled
	if input.RecordingEnabled > 1 {
		input.RecordingEnabled = 1
	}
	if input.RecordingEnabled < 0 {
		input.RecordingEnabled = 0
	}

	// Build POST body for tt-backend
	ttReqBody := map[string]interface{}{
		"distribution":         input.Distribution,
		"size":                 strings.ToLower(input.Size),
		"features":             input.Features,
		"terms":                termsHash,
		"expiry":               input.Expiry,
		"hostname":             input.Hostname,
		"packages":             input.Packages,
		"history_retention_days": input.HistoryRetentionDays,
		"recording_enabled":     input.RecordingEnabled,
		"external_ref":          input.ExternalRef,
	}
	if input.Name != "" {
		ttReqBody["name"] = input.Name
	}
	if input.PersistenceMode != "" {
		ttReqBody["persistence_mode"] = input.PersistenceMode
	}
	if input.IdleWindowSeconds != nil {
		ttReqBody["idle_window_seconds"] = *input.IdleWindowSeconds
	}

	// Build URL
	url := fmt.Sprintf("%s/%s/sessions", tts.baseURL, tts.apiVersion)
	if input.Backend != "" {
		url += fmt.Sprintf("?backend=%s", input.Backend)
	}

	utils.Debug("StartComposedSession - POST %s", url)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userKey.APIKey))

	// tt-backend may stream NDJSON, use the same pattern as startSession
	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "POST", url, ttReqBody, opts)
	if err != nil {
		return nil, err
	}

	var sessionResp dto.TerminalTrainerSessionResponse
	if err := resp.DecodeLastJSON(&sessionResp); err != nil {
		return nil, utils.ExternalAPIError("Terminal Trainer", "decode response", err)
	}

	if sessionResp.Status != 0 {
		errorMsg := tts.enumService.FormatError("session_status", int(sessionResp.Status), "Failed to start composed session")
		return nil, fmt.Errorf("%s", errorMsg)
	}

	// Build local terminal record
	expiresAt := time.Unix(sessionResp.ExpiresAt, 0)

	var orgID *uuid.UUID
	if input.OrganizationID != "" {
		parsed, err := uuid.Parse(input.OrganizationID)
		if err == nil {
			orgID = &parsed
		}
	}

	// Serialize enabled features as JSON
	composedFeaturesJSON := ""
	if input.Features != nil {
		enabledFeatures := make(map[string]bool)
		for k, v := range input.Features {
			if v {
				enabledFeatures[k] = true
			}
		}
		if len(enabledFeatures) > 0 {
			if b, err := json.Marshal(enabledFeatures); err == nil {
				composedFeaturesJSON = string(b)
			}
		}
	}

	terminal := &models.Terminal{
		SessionID:            sessionResp.SessionID,
		UserID:               userID,
		Name:                 input.Name,
		Status:               "active",
		PersistenceMode:      input.PersistenceMode,
		ExpiresAt:            expiresAt,
		InstanceType:         input.DistributionPrefix,
		MachineSize:          strings.ToUpper(input.Size),
		Backend:              sessionResp.Backend,
		OrganizationID:       orgID,
		SubscriptionPlanID:   input.SubscriptionPlanID,
		UserTerminalKeyID:    userKey.ID,
		UserTerminalKey:      *userKey,
		ComposedDistribution: input.Distribution,
		ComposedSize:         input.Size,
		ComposedFeatures:     composedFeaturesJSON,
	}

	if err := tts.repository.CreateTerminalSession(terminal); err != nil {
		return nil, fmt.Errorf("failed to save terminal session: %w", err)
	}

	// Build console URL
	consolePath := tts.buildAPIPath("/console", input.DistributionPrefix)
	response := &dto.TerminalSessionResponse{
		SessionID:  sessionResp.SessionID,
		ExpiresAt:  expiresAt,
		ConsoleURL: fmt.Sprintf("%s%s?id=%s", tts.baseURL, consolePath, sessionResp.SessionID),
		Status:     "active",
		Backend:    sessionResp.Backend,
	}

	return response, nil
}
