package services

import (
	"crypto/sha256"
	"fmt"
	"io"
	neturl "net/url"
	"os"
	"strings"
	"sync"
	"time"

	"soli/formations/src/auth/casdoor"
	authModels "soli/formations/src/auth/models"
	configRepositories "soli/formations/src/configuration/repositories"
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
	StartSession(userID string, sessionInput dto.CreateTerminalSessionInput) (*dto.TerminalSessionResponse, error)
	StartSessionWithPlan(userID string, sessionInput dto.CreateTerminalSessionInput, planInterface any) (*dto.TerminalSessionResponse, error)
	GetSessionInfo(sessionID string) (*models.Terminal, error)
	GetTerminalByUUID(terminalUUID string) (*models.Terminal, error)
	GetActiveUserSessions(userID string) (*[]models.Terminal, error)
	StopSession(sessionID string) error

	// Terminal sharing methods
	ShareTerminal(sessionID, sharedByUserID, sharedWithUserID, accessLevel string, expiresAt *time.Time) error
	RevokeTerminalAccess(sessionID, sharedWithUserID, requestingUserID string) error
	GetTerminalShares(sessionID, requestingUserID string) (*[]models.TerminalShare, error)
	GetSharedTerminals(userID string) (*[]models.Terminal, error)
	GetSharedTerminalsWithHidden(userID string, includeHidden bool) (*[]models.Terminal, error)
	HasTerminalAccess(sessionID, userID string, requiredLevel string) (bool, error)
	GetSharedTerminalInfo(sessionID, userID string) (*dto.SharedTerminalInfo, error)

	// Terminal hiding methods
	HideTerminal(terminalID, userID string) error
	UnhideTerminal(terminalID, userID string) error

	// Synchronization methods (nouvelle approche avec API comme source de vérité)
	GetAllSessionsFromAPI(userAPIKey string) (*dto.TerminalTrainerSessionsResponse, error)
	SyncUserSessions(userID string) (*dto.SyncAllSessionsResponse, error)
	SyncAllActiveSessions() error
	GetSessionInfoFromAPI(sessionID string) (*dto.TerminalTrainerSessionInfo, error)

	// Utilities
	GetRepository() repositories.TerminalRepository
	CleanupExpiredSessions() error

	// Configuration
	GetInstanceTypes(backend string) ([]dto.InstanceType, error)

	// Metrics
	GetServerMetrics(nocache bool, backend string) (*dto.ServerMetricsResponse, error)

	// Backend management
	GetBackends() ([]dto.BackendInfo, error)
	GetBackendsForOrganization(orgID uuid.UUID) ([]dto.BackendInfo, error)
	IsBackendOnline(backendName string) (bool, error)
	SetSystemDefaultBackend(backendID string) (*dto.BackendInfo, error)

	// Correction des permissions
	FixTerminalHidePermissions(userID string) (*dto.FixPermissionsResponse, error)

	// Bulk operations
	BulkCreateTerminalsForGroup(groupID string, requestingUserID string, userRoles []string, request dto.BulkCreateTerminalsRequest, planInterface any) (*dto.BulkCreateTerminalsResponse, error)

	// Enum service access
	GetEnumService() TerminalTrainerEnumService

	// Session validation
	ValidateSessionAccess(sessionID string, checkAPI bool) (bool, string, error)

	// Command history
	GetSessionCommandHistory(sessionID string, since *int64, format string, limit, offset int) ([]byte, string, error)
	DeleteSessionCommandHistory(sessionID string) error
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
	featureRepo             configRepositories.FeatureRepository
	db                      *gorm.DB
	backendCache            []dto.BackendInfo
	backendCacheTime        time.Time
	backendCacheMu          sync.RWMutex
	backendCacheSF          singleflight.Group
	systemDefaultBackend    string
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

	featureRepo := configRepositories.NewFeatureRepository(db)

	return &terminalTrainerService{
		adminKey:               os.Getenv("TERMINAL_TRAINER_ADMIN_KEY"),
		baseURL:                baseURL,
		apiVersion:             apiVersion,
		terminalType:           terminalType,
		repository:             repositories.NewTerminalRepository(db),
		subscriptionService:    paymentServices.NewSubscriptionService(db),
		orgSubscriptionService: paymentServices.NewOrganizationSubscriptionService(db),
		enumService:            NewTerminalTrainerEnumService(baseURL, apiVersion),
		featureRepo:            featureRepo,
		db:                     db,
		systemDefaultBackend:   loadSystemDefaultBackend(featureRepo),
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

// StartSession démarre une nouvelle session
func (tts *terminalTrainerService) StartSession(userID string, sessionInput dto.CreateTerminalSessionInput) (*dto.TerminalSessionResponse, error) {
	// Récupérer la clé utilisateur
	userKey, err := tts.repository.GetUserTerminalKeyByUserID(userID, true)
	if err != nil {
		return nil, fmt.Errorf("no terminal trainer key found for user: %w", err)
	}

	if !userKey.IsActive {
		return nil, fmt.Errorf("user terminal trainer key is disabled")
	}

	// NOTE: Concurrent terminal checks are now handled by middleware
	// The subscription plan limits are enforced in CheckTerminalCreationLimit() middleware
	// Machine size, template, and network validation should be added here based on subscription plan

	// Appel à l'API Terminal Trainer pour démarrer la session
	hash := sha256.New()
	io.WriteString(hash, sessionInput.Terms)

	// Construire le chemin avec version et type d'instance dynamique
	path := tts.buildAPIPath("/start", sessionInput.InstanceType)

	// Construire l'URL avec les paramètres
	url := fmt.Sprintf("%s%s?terms=%s", tts.baseURL, path, fmt.Sprintf("%x", hash.Sum(nil)))
	if sessionInput.Expiry > 0 {
		url += fmt.Sprintf("&expiry=%d", sessionInput.Expiry)
	}
	if sessionInput.Backend != "" {
		url += fmt.Sprintf("&backend=%s", sessionInput.Backend)
	}
	if sessionInput.HistoryRetentionDays > 0 {
		url += fmt.Sprintf("&history_retention_days=%d", sessionInput.HistoryRetentionDays)
	}
	if sessionInput.RecordingConsent > 0 {
		url += fmt.Sprintf("&recording_consent=%d", sessionInput.RecordingConsent)
	}
	if sessionInput.ExternalRef != "" {
		url += fmt.Sprintf("&external_ref=%s", neturl.QueryEscape(sessionInput.ExternalRef))
	}

	// Parser la réponse du Terminal Trainer
	var sessionResp dto.TerminalTrainerSessionResponse
	opts := utils.DefaultHTTPClientOptions()

	// Debug: Log API key usage (without exposing the key itself)
	if len(userKey.APIKey) > 0 {
		utils.Debug("StartSession - Using API key for user %s (length: %d)", userID, len(userKey.APIKey))
	} else {
		utils.Debug("StartSession - WARNING: Empty API key for user %s!", userID)
	}

	utils.ApplyOptions(&opts, utils.WithAPIKey(userKey.APIKey))

	// Use MakeExternalAPIRequest + DecodeLastJSON because tt-backend's /start
	// endpoint streams progress messages as NDJSON before the final session JSON.
	resp, err := utils.MakeExternalAPIRequest("Terminal Trainer", "GET", url, nil, opts)
	if err != nil {
		return nil, err
	}
	if err := resp.DecodeLastJSON(&sessionResp); err != nil {
		return nil, utils.ExternalAPIError("Terminal Trainer", "decode response", err)
	}

	if sessionResp.Status != 0 {
		// Use enum service to provide detailed error message
		errorMsg := tts.enumService.FormatError("session_status", int(sessionResp.Status), "Failed to start session")
		return nil, fmt.Errorf("%s", errorMsg)
	}

	// Créer l'enregistrement local
	expiresAt := time.Unix(sessionResp.ExpiresAt, 0)

	// Parse organization ID if provided
	var orgID *uuid.UUID
	if sessionInput.OrganizationID != "" {
		parsed, err := uuid.Parse(sessionInput.OrganizationID)
		if err == nil {
			orgID = &parsed
		}
	}

	terminal := &models.Terminal{
		SessionID:         sessionResp.SessionID,
		UserID:            userID,
		Name:              sessionInput.Name,
		Status:            "active",
		ExpiresAt:         expiresAt,
		InstanceType:      sessionInput.InstanceType,
		MachineSize:       sessionResp.MachineSize, // Taille réelle retournée par Terminal Trainer
		Backend:           sessionResp.Backend,
		OrganizationID:    orgID,
		UserTerminalKeyID: userKey.ID,
		UserTerminalKey:   *userKey,
	}

	if err := tts.repository.CreateTerminalSession(terminal); err != nil {
		return nil, fmt.Errorf("failed to save terminal session: %w", err)
	}

	// Ajouter les permissions Casbin pour que le propriétaire puisse masquer des terminaux
	err = tts.addTerminalHidePermissions(userID)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer la création du terminal
		utils.Warn("failed to add hide permissions for terminal %s: %v", terminal.ID.String(), err)
	}

	// Ajouter les permissions Casbin pour que le propriétaire puisse accéder à la console WebSocket
	err = tts.addTerminalConsolePermissions(userID)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer la création du terminal
		utils.Warn("failed to add console permissions for terminal %s: %v", terminal.ID.String(), err)
	}

	// Construire la réponse
	consolePath := tts.buildAPIPath("/console", sessionInput.InstanceType)
	response := &dto.TerminalSessionResponse{
		SessionID:  sessionResp.SessionID,
		ExpiresAt:  expiresAt,
		ConsoleURL: fmt.Sprintf("%s%s?id=%s", tts.baseURL, consolePath, sessionResp.SessionID),
		Status:     "active",
		Backend:    sessionResp.Backend,
	}

	return response, nil
}

// StartSessionWithPlan démarre une nouvelle session avec validation du plan d'abonnement
func (tts *terminalTrainerService) StartSessionWithPlan(userID string, sessionInput dto.CreateTerminalSessionInput, planInterface any) (*dto.TerminalSessionResponse, error) {
	// Convertir l'interface en SubscriptionPlan
	plan, ok := planInterface.(*paymentModels.SubscriptionPlan)
	if !ok {
		return nil, fmt.Errorf("invalid subscription plan type")
	}

	// Valider la taille de la machine
	if sessionInput.InstanceType != "" {
		// Récupérer les types d'instances disponibles depuis l'API Terminal Trainer
		instanceTypes, err := tts.GetInstanceTypes(sessionInput.Backend)
		if err != nil {
			return nil, fmt.Errorf("failed to get instance types: %w", err)
		}

		// Trouver la taille correspondant au type d'instance demandé
		var instanceSizes string
		for _, it := range instanceTypes {
			if it.Prefix == sessionInput.InstanceType || it.Name == sessionInput.InstanceType {
				instanceSizes = it.Size
				break
			}
		}

		if instanceSizes == "" {
			return nil, fmt.Errorf("instance type '%s' not found", sessionInput.InstanceType)
		}

		// Parse les tailles disponibles pour cette instance (format: "XS|S|M")
		availableSizes := strings.Split(instanceSizes, "|")

		// Vérifier qu'au moins une des tailles de l'instance est autorisée dans le plan
		allowed := false
		for _, instanceSize := range availableSizes {
			for _, allowedSize := range plan.AllowedMachineSizes {
				if allowedSize == strings.TrimSpace(instanceSize) || allowedSize == "all" {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("instance '%s' with sizes [%s] not allowed in your plan. Allowed sizes: %v",
				sessionInput.InstanceType, instanceSizes, plan.AllowedMachineSizes)
		}
	}

	// Validate backend against organization's plan
	var orgID *uuid.UUID
	if sessionInput.OrganizationID != "" {
		parsed, err := uuid.Parse(sessionInput.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("invalid organization_id: %w", err)
		}
		orgID = &parsed
	}

	validatedBackend, err := tts.validateBackendForOrg(orgID, sessionInput.Backend)
	if err != nil {
		return nil, err
	}
	sessionInput.Backend = validatedBackend

	// Appliquer la durée maximale de session depuis le plan
	maxDurationSeconds := plan.MaxSessionDurationMinutes * 60
	if sessionInput.Expiry == 0 || sessionInput.Expiry > maxDurationSeconds {
		sessionInput.Expiry = maxDurationSeconds
	}

	// Pass command history retention days from subscription plan
	sessionInput.HistoryRetentionDays = plan.CommandHistoryRetentionDays

	// Appeler la méthode StartSession originale avec les paramètres validés
	return tts.StartSession(userID, sessionInput)
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

// StopSession arrête une session ET appelle l'API externe pour expirer
func (tts *terminalTrainerService) StopSession(sessionID string) error {
	utils.Debug("StopSession called for session %s", sessionID)

	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	utils.Debug("Session %s current status: %s", sessionID, terminal.Status)

	// 1. Appeler l'API Terminal Trainer pour expirer la session
	utils.Debug("Calling expireSessionInAPI for session %s", sessionID)
	err = tts.expireSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey, terminal.InstanceType)
	if err != nil {
		// Log l'erreur complète pour debugging
		utils.Warn("failed to expire session in Terminal Trainer API: %v", err)
	} else {
		utils.Debug("Successfully called expireSessionInAPI for session %s", sessionID)
	}

	// 2. Marquer la session comme arrêtée localement
	// L'utilisateur pourra la masquer s'il le souhaite
	utils.Debug("Updating session %s status to 'stopped'", sessionID)
	terminal.Status = "stopped"
	err = tts.repository.UpdateTerminalSession(terminal)
	if err != nil {
		utils.Error("Failed to update session %s status: %v", sessionID, err)
		return err
	}

	// 3. Décrémenter la métrique concurrent_terminals
	utils.Debug("Decrementing concurrent_terminals for user %s", terminal.UserID)
	decrementErr := tts.subscriptionService.IncrementUsage(terminal.UserID, "concurrent_terminals", -1)
	if decrementErr != nil {
		// Log l'erreur mais ne pas faire échouer l'arrêt du terminal
		utils.Warn("failed to decrement concurrent_terminals for user %s: %v", terminal.UserID, decrementErr)
	}

	utils.Debug("Successfully updated session %s status to 'stopped'", sessionID)
	return nil
}

// expireSessionInAPI appelle l'endpoint /expire de l'API Terminal Trainer
func (tts *terminalTrainerService) expireSessionInAPI(sessionID, userAPIKey, instanceType string) error {
	// Construire le chemin avec version et type d'instance dynamique
	path := tts.buildAPIPath("/expire", instanceType)
	url := fmt.Sprintf("%s%s?id=%s", tts.baseURL, path, sessionID)

	utils.Debug("expireSessionInAPI - calling %s", url)

	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userAPIKey))

	_, err := utils.MakeExternalAPIRequest("Terminal Trainer", "PUT", url, nil, opts)
	if err != nil {
		return err
	}

	return nil
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

		// Convert numeric status to semantic name using enum service
		apiStatusName := tts.enumService.GetEnumName("session_status", int(apiSession.Status))

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

	// Convert numeric status to semantic name
	statusName := tts.enumService.GetEnumName("session_status", int(apiSession.Status))

	terminal := &models.Terminal{
		SessionID:         apiSession.SessionID,
		UserID:            userID,
		Status:            statusName, // Use semantic name (e.g., "active", "expired")
		ExpiresAt:         expiresAt,
		MachineSize:       apiSession.MachineSize, // Taille réelle depuis l'API
		Backend:           apiSession.Backend,
		UserTerminalKeyID: userKey.ID,
		UserTerminalKey:   *userKey,
	}

	return tts.repository.CreateTerminalSession(terminal)
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

// GetInstanceTypes récupère la liste des types d'instances disponibles depuis Terminal Trainer
// Si backend est non-vide, filtre par backend (retourne uniquement les instances disponibles sur ce backend)
func (tts *terminalTrainerService) GetInstanceTypes(backend string) ([]dto.InstanceType, error) {
	// Utiliser le type par défaut pour récupérer la liste des instances disponibles
	path := tts.buildAPIPath("/instances", "")
	url := fmt.Sprintf("%s%s", tts.baseURL, path)
	if backend != "" {
		url += fmt.Sprintf("?backend=%s", backend)
	}

	var instanceTypes []dto.InstanceType
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithHeader("X-Admin-Key", tts.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &instanceTypes, opts)
	if err != nil {
		return nil, err
	}

	return instanceTypes, nil
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

// Terminal sharing methods implementation

// ShareTerminal partage un terminal avec un autre utilisateur
func (tts *terminalTrainerService) ShareTerminal(sessionID, sharedByUserID, sharedWithUserID, accessLevel string, expiresAt *time.Time) error {
	// Validate that sharedWithUserID is not empty
	if sharedWithUserID == "" {
		return fmt.Errorf("shared_with_user_id cannot be empty")
	}

	// Vérifier que le terminal existe
	terminal, err := tts.repository.GetTerminalSessionBySessionID(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get terminal: %w", err)
	}
	if terminal == nil {
		return fmt.Errorf("terminal not found")
	}

	// Vérifier que l'utilisateur qui partage est le propriétaire du terminal
	if terminal.UserID != sharedByUserID {
		return utils.OwnerOnlyError("terminal", "share")
	}

	// Vérifier que l'utilisateur ne partage pas avec lui-même
	if sharedByUserID == sharedWithUserID {
		return fmt.Errorf("cannot share terminal with yourself")
	}

	// Valider le niveau d'accès
	if !models.IsValidAccessLevel(accessLevel) {
		return fmt.Errorf("invalid access level: %s", accessLevel)
	}

	// Vérifier si un partage existe déjà
	existingShare, err := tts.repository.GetTerminalShare(terminal.ID.String(), sharedWithUserID)
	if err != nil {
		return fmt.Errorf("failed to check existing share: %w", err)
	}

	if existingShare != nil {
		// Si le niveau d'accès change, supprimer d'abord les anciennes permissions
		if existingShare.AccessLevel != accessLevel {
			err := tts.removeTerminalSharePermissions(sharedWithUserID, existingShare.AccessLevel)
			if err != nil {
				utils.Warn("failed to remove old permissions for terminal %s from user %s: %v", terminal.ID.String(), sharedWithUserID, err)
			}
		}

		// Mettre à jour le partage existant
		existingShare.AccessLevel = accessLevel
		existingShare.ExpiresAt = expiresAt
		existingShare.IsActive = true
		err := tts.repository.UpdateTerminalShare(existingShare)
		if err != nil {
			return err
		}

		// Ajouter les permissions Casbin pour le nouveau niveau d'accès
		err = tts.addTerminalHidePermissions(sharedWithUserID)
		if err != nil {
			utils.Warn("failed to add hide permissions for updated shared terminal %s to user %s: %v", terminal.ID.String(), sharedWithUserID, err)
		}

		// Ajouter les permissions console WebSocket
		err = tts.addTerminalConsolePermissions(sharedWithUserID)
		if err != nil {
			utils.Warn("failed to add console permissions for updated shared terminal %s to user %s: %v", terminal.ID.String(), sharedWithUserID, err)
		}

		// Ajouter les nouvelles permissions d'édition selon le niveau d'accès
		err = tts.addTerminalSharePermissions(sharedWithUserID, accessLevel)
		if err != nil {
			utils.Warn("failed to add share permissions for updated shared terminal %s to user %s: %v", terminal.ID.String(), sharedWithUserID, err)
		}

		return nil
	}

	// Créer un nouveau partage
	share := &models.TerminalShare{
		TerminalID:       terminal.ID,
		SharedWithUserID: &sharedWithUserID,
		SharedByUserID:   sharedByUserID,
		AccessLevel:      accessLevel,
		ExpiresAt:        expiresAt,
		IsActive:         true,
	}

	err = tts.repository.CreateTerminalShare(share)
	if err != nil {
		return err
	}

	// Ajouter les permissions Casbin pour que le destinataire puisse masquer des terminaux
	err = tts.addTerminalHidePermissions(sharedWithUserID)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer le partage
		utils.Warn("failed to add hide permissions for shared terminal %s to user %s: %v", terminal.ID.String(), sharedWithUserID, err)
	}

	// Ajouter les permissions console WebSocket
	err = tts.addTerminalConsolePermissions(sharedWithUserID)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer le partage
		utils.Warn("failed to add console permissions for shared terminal %s to user %s: %v", terminal.ID.String(), sharedWithUserID, err)
	}

	// Ajouter les permissions d'édition pour les utilisateurs avec accès "owner"
	err = tts.addTerminalSharePermissions(sharedWithUserID, accessLevel)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer le partage
		utils.Warn("failed to add share permissions for shared terminal %s to user %s: %v", terminal.ID.String(), sharedWithUserID, err)
	}

	return nil
}

// RevokeTerminalAccess révoque l'accès d'un utilisateur à un terminal
func (tts *terminalTrainerService) RevokeTerminalAccess(sessionID, sharedWithUserID, requestingUserID string) error {
	// Vérifier que le terminal existe
	terminal, err := tts.repository.GetTerminalSessionBySessionID(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get terminal: %w", err)
	}
	if terminal == nil {
		return fmt.Errorf("terminal not found")
	}

	// Vérifier que l'utilisateur qui révoque est le propriétaire du terminal
	if terminal.UserID != requestingUserID {
		return utils.OwnerOnlyError("terminal", "revoke")
	}

	// Récupérer le partage
	share, err := tts.repository.GetTerminalShare(terminal.ID.String(), sharedWithUserID)
	if err != nil {
		return fmt.Errorf("failed to get share: %w", err)
	}
	if share == nil {
		return fmt.Errorf("no active share found")
	}

	// Révoquer les permissions Casbin avant de désactiver le partage
	err = tts.removeTerminalSharePermissions(sharedWithUserID, share.AccessLevel)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer la révocation
		utils.Warn("failed to remove permissions for terminal %s from user %s: %v", terminal.ID.String(), sharedWithUserID, err)
	}

	// Désactiver le partage
	share.IsActive = false
	return tts.repository.UpdateTerminalShare(share)
}

// GetTerminalShares récupère les partages d'un terminal
func (tts *terminalTrainerService) GetTerminalShares(sessionID, requestingUserID string) (*[]models.TerminalShare, error) {
	// Vérifier que le terminal existe
	terminal, err := tts.repository.GetTerminalSessionBySessionID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get terminal: %w", err)
	}
	if terminal == nil {
		return nil, fmt.Errorf("terminal not found")
	}

	// Vérifier que l'utilisateur est le propriétaire du terminal
	if terminal.UserID != requestingUserID {
		return nil, fmt.Errorf("only terminal owner can view shares")
	}

	return tts.repository.GetTerminalSharesByTerminalID(terminal.ID.String())
}

// GetSharedTerminals récupère tous les terminaux partagés avec un utilisateur
func (tts *terminalTrainerService) GetSharedTerminals(userID string) (*[]models.Terminal, error) {
	return tts.repository.GetSharedTerminalsForUser(userID)
}

// HasTerminalAccess vérifie si un utilisateur a accès à un terminal avec le niveau requis
func (tts *terminalTrainerService) HasTerminalAccess(terminalIDOrSessionID, userID string, requiredLevel string) (bool, error) {
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

	// Le propriétaire a toujours accès
	if terminal.UserID == userID {
		return true, nil
	}

	// Vérifier les partages
	return tts.repository.HasTerminalAccess(terminal.ID.String(), userID, requiredLevel)
}

// GetSharedTerminalInfo récupère les informations détaillées d'un terminal partagé
func (tts *terminalTrainerService) GetSharedTerminalInfo(sessionID, userID string) (*dto.SharedTerminalInfo, error) {
	// Vérifier que le terminal existe
	terminal, err := tts.repository.GetTerminalSessionBySessionID(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get terminal: %w", err)
	}
	if terminal == nil {
		return nil, fmt.Errorf("terminal not found")
	}

	// Vérifier si l'utilisateur a accès
	hasAccess, err := tts.HasTerminalAccess(sessionID, userID, models.AccessLevelRead)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, fmt.Errorf("access denied")
	}

	// Construire les informations du terminal
	terminalOutput := dto.TerminalOutput{
		ID:              terminal.ID,
		SessionID:       terminal.SessionID,
		UserID:          terminal.UserID,
		Name:            terminal.Name,
		Status:          terminal.Status,
		ExpiresAt:       terminal.ExpiresAt,
		InstanceType:    terminal.InstanceType,
		MachineSize:     terminal.MachineSize,
		Backend:         terminal.Backend,
		OrganizationID:  terminal.OrganizationID,
		IsHiddenByOwner: terminal.IsHiddenByOwner,
		HiddenByOwnerAt: terminal.HiddenByOwnerAt,
		CreatedAt:       terminal.CreatedAt,
	}

	// Si l'utilisateur est le propriétaire, récupérer tous les partages
	shares := make([]dto.TerminalShareOutput, 0, 8)
	if terminal.UserID == userID {
		terminalShares, err := tts.repository.GetTerminalSharesByTerminalID(terminal.ID.String())
		if err == nil {
			for _, share := range *terminalShares {
				shares = append(shares, dto.TerminalShareOutput{
					ID:               share.ID,
					TerminalID:       share.TerminalID,
					SharedWithUserID: share.SharedWithUserID,
					SharedByUserID:   share.SharedByUserID,
					AccessLevel:      share.AccessLevel,
					ExpiresAt:        share.ExpiresAt,
					IsActive:         share.IsActive,
					CreatedAt:        share.CreatedAt,
				})
			}
		}
	}

	// Déterminer le niveau d'accès de l'utilisateur
	var accessLevel string
	var sharedAt time.Time
	if terminal.UserID == userID {
		accessLevel = models.AccessLevelOwner
		sharedAt = terminal.CreatedAt
	} else {
		// Récupérer le partage pour cet utilisateur
		share, err := tts.repository.GetTerminalShare(terminal.ID.String(), userID)
		if err != nil || share == nil {
			return nil, fmt.Errorf("access information not found")
		}
		accessLevel = share.AccessLevel
		sharedAt = share.CreatedAt
	}

	// Récupérer le display name de l'utilisateur qui a partagé le terminal
	var sharedByDisplayName string
	if ownerUser, err := casdoorsdk.GetUserByUserId(terminal.UserID); err == nil && ownerUser != nil {
		sharedByDisplayName = ownerUser.DisplayName
	} else {
		// Fallback au UserID si on ne peut pas récupérer le display name
		sharedByDisplayName = terminal.UserID
	}

	return &dto.SharedTerminalInfo{
		Terminal:            terminalOutput,
		SharedBy:            terminal.UserID,
		SharedByDisplayName: sharedByDisplayName,
		AccessLevel:         accessLevel,
		SharedAt:            sharedAt,
		Shares:              shares,
	}, nil
}

func (tts *terminalTrainerService) GetSharedTerminalsWithHidden(userID string, includeHidden bool) (*[]models.Terminal, error) {
	return tts.repository.GetSharedTerminalsForUserWithHidden(userID, includeHidden)
}

func (tts *terminalTrainerService) HideTerminal(terminalID, userID string) error {
	// Get terminal info to check ownership and status
	terminal, err := tts.repository.GetTerminalByUUID(terminalID)
	if err != nil {
		return fmt.Errorf("terminal not found")
	}

	// Only allow hiding inactive terminals
	if !terminal.CanBeHidden() {
		return fmt.Errorf("cannot hide active terminals")
	}

	// Check if user is the owner
	if terminal.UserID == userID {
		// Hide owned terminal
		return tts.repository.HideOwnedTerminal(terminalID, userID)
	}

	// Check if user has shared access to this terminal
	hasAccess, err := tts.repository.HasTerminalAccess(terminalID, userID, models.AccessLevelRead)
	if err != nil {
		return fmt.Errorf("failed to check access: %w", err)
	}
	if !hasAccess {
		return fmt.Errorf("access denied")
	}

	// Hide shared terminal
	return tts.repository.HideTerminalForUser(terminalID, userID)
}

func (tts *terminalTrainerService) UnhideTerminal(terminalID, userID string) error {
	// Get terminal info to check ownership
	terminal, err := tts.repository.GetTerminalByUUID(terminalID)
	if err != nil {
		return fmt.Errorf("terminal not found")
	}

	// Check if user is the owner
	if terminal.UserID == userID {
		// Unhide owned terminal
		return tts.repository.UnhideOwnedTerminal(terminalID, userID)
	}

	// Check if user has shared access to this terminal
	hasAccess, err := tts.repository.HasTerminalAccess(terminalID, userID, models.AccessLevelRead)
	if err != nil {
		return fmt.Errorf("failed to check access: %w", err)
	}
	if !hasAccess {
		return fmt.Errorf("access denied")
	}

	// Unhide shared terminal
	return tts.repository.UnhideTerminalForUser(terminalID, userID)
}

// addTerminalHidePermissions ajoute les permissions Casbin pour qu'un utilisateur puisse masquer des terminaux
func (tts *terminalTrainerService) addTerminalHidePermissions(userID string) error {
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true

	// Ajouter les permissions pour les routes de masquage avec le pattern de route générique
	// Cela permet au middleware d'authentification de faire correspondre ctx.FullPath() avec les permissions stockées
	hideRoute := "/api/v1/terminals/:id/hide"

	// Permissions pour POST et DELETE /terminals/{id}/hide
	err := utils.AddPolicy(casdoor.Enforcer, userID, hideRoute, "POST|DELETE", opts)
	if err != nil {
		return err
	}

	return nil
}

// addTerminalConsolePermissions ajoute les permissions Casbin pour qu'un utilisateur puisse accéder à la console WebSocket
func (tts *terminalTrainerService) addTerminalConsolePermissions(userID string) error {
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true

	// Route de la console WebSocket
	consoleRoute := "/api/v1/terminals/:id/console"

	// Permission pour GET /terminals/{id}/console (connexion WebSocket)
	err := utils.AddPolicy(casdoor.Enforcer, userID, consoleRoute, "GET", opts)
	if err != nil {
		return err
	}

	return nil
}

// addTerminalSharePermissions ajoute les permissions Casbin pour qu'un utilisateur puisse accéder à un terminal partagé
// Note: Uses generic route pattern (/api/v1/terminals/:id) as per two-layer security model.
// Actual resource-level access is validated by HasTerminalAccess() checks in route handlers.
func (tts *terminalTrainerService) addTerminalSharePermissions(userID, accessLevel string) error {
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true

	// Route générique pour les terminaux (matches all terminal IDs)
	terminalRoute := "/api/v1/terminals/:id"

	// Build methods based on access level
	var methods string
	switch accessLevel {
	case models.AccessLevelRead:
		methods = "GET"
	case models.AccessLevelWrite:
		methods = "GET|PATCH"
	case models.AccessLevelOwner:
		methods = "GET|PATCH|DELETE"
	default:
		methods = "GET" // Fallback to read-only
	}

	// Add permissions based on access level
	err := utils.AddPolicy(casdoor.Enforcer, userID, terminalRoute, methods, opts)
	if err != nil {
		return err
	}

	return nil
}

// removeTerminalSharePermissions révoque les permissions Casbin d'un utilisateur pour un terminal partagé
// Note: Removes generic route permissions. The user will lose access to ALL shared terminals at the route level,
// but actual access is still controlled by terminal_shares table entries checked by HasTerminalAccess().
// This function should only be called when revoking the LAST share for a user.
func (tts *terminalTrainerService) removeTerminalSharePermissions(userID, accessLevel string) error {
	opts := utils.DefaultPermissionOptions()
	opts.LoadPolicyFirst = true
	opts.WarnOnError = true // Non-critical permission removals

	// Route générique pour les terminaux (matches all terminal IDs)
	terminalRoute := "/api/v1/terminals/:id"
	hideRoute := "/api/v1/terminals/:id/hide"
	consoleRoute := "/api/v1/terminals/:id/console"

	// Build methods to remove based on access level
	var methods string
	switch accessLevel {
	case models.AccessLevelRead:
		methods = "GET"
	case models.AccessLevelWrite:
		methods = "GET|PATCH"
	case models.AccessLevelOwner:
		methods = "GET|PATCH|DELETE"
	default:
		methods = "GET" // Fallback to read-only
	}

	// Supprimer les permissions du terminal
	err := utils.RemovePolicy(casdoor.Enforcer, userID, terminalRoute, methods, opts)
	if err != nil {
		utils.Warn("Failed to remove terminal permissions: %v", err)
	}

	// Supprimer les permissions de masquage (POST et DELETE)
	err = utils.RemovePolicy(casdoor.Enforcer, userID, hideRoute, "POST|DELETE", opts)
	if err != nil {
		utils.Warn("Failed to remove hide permissions: %v", err)
	}

	// Supprimer les permissions console WebSocket (GET)
	err = utils.RemovePolicy(casdoor.Enforcer, userID, consoleRoute, "GET", opts)
	if err != nil {
		utils.Warn("Failed to remove console permissions: %v", err)
	}

	return nil
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
	utils.ApplyOptions(&opts, utils.WithHeader("X-Admin-Key", tts.adminKey))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &backends, opts)
	if err != nil {
		return nil, err
	}

	for i := range backends {
		// Default Name to ID if upstream doesn't provide one
		if backends[i].Name == "" {
			backends[i].Name = backends[i].ID
		}
		// Mark the system default backend
		if tts.systemDefaultBackend != "" && backends[i].ID == tts.systemDefaultBackend {
			backends[i].IsDefault = true
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
			defaultID = tts.systemDefaultBackend
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
		defaultID = tts.systemDefaultBackend
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
		effectiveDefault = tts.systemDefaultBackend
	}

	// If no backend requested, use the effective default
	if requestedBackend == "" {
		return effectiveDefault, nil
	}

	// If AllowedBackends is empty, only the default backend is allowed
	if len(org.AllowedBackends) == 0 {
		if effectiveDefault != "" && requestedBackend == effectiveDefault {
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

// FixTerminalHidePermissions corrige les permissions de masquage pour tous les terminaux d'un utilisateur
// et tous les terminaux partagés avec lui
func (tts *terminalTrainerService) FixTerminalHidePermissions(userID string) (*dto.FixPermissionsResponse, error) {
	response := &dto.FixPermissionsResponse{
		UserID:             userID,
		ProcessedTerminals: 0,
		ProcessedShares:    0,
		Errors:             make([]string, 0, 4),
	}

	// 1. Ajouter les permissions générales de masquage pour cet utilisateur
	err := tts.addTerminalHidePermissions(userID)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to add general hide permissions: %v", err))
	}

	// Ajouter également les permissions console WebSocket
	err = tts.addTerminalConsolePermissions(userID)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to add general console permissions: %v", err))
	}

	// 2. Récupérer tous les terminaux appartenant à l'utilisateur
	ownedTerminals, err := tts.repository.GetTerminalSessionsByUserID(userID, false)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to get owned terminals: %v", err))
	} else {
		response.ProcessedTerminals = len(*ownedTerminals)
	}

	// 3. Récupérer tous les partages où l'utilisateur est destinataire et ajouter les permissions appropriées
	shares, err := tts.repository.GetTerminalSharesByUserID(userID)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to get shared terminals: %v", err))
	} else {
		response.ProcessedShares = len(*shares)
		// Ajouter les permissions pour chaque partage selon le niveau d'accès
		for _, share := range *shares {
			if share.IsActive {
				err := tts.addTerminalSharePermissions(userID, share.AccessLevel)
				if err != nil {
					response.Errors = append(response.Errors, fmt.Sprintf("Failed to add permissions for terminal %s: %v", share.TerminalID.String(), err))
				}
			}
		}
	}

	response.Success = len(response.Errors) == 0
	response.Message = fmt.Sprintf("Processed %d owned terminals and %d shared terminals",
		response.ProcessedTerminals, response.ProcessedShares)

	if !response.Success {
		response.Message += fmt.Sprintf(" with %d errors", len(response.Errors))
	}

	return response, nil
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
				if member.UserID == requestingUserID && (member.Role == groupModels.GroupMemberRoleAdmin || member.Role == groupModels.GroupMemberRoleOwner) {
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

		// Create session input for this user
		sessionInput := dto.CreateTerminalSessionInput{
			Terms:          request.Terms,
			Name:           terminalName,
			Expiry:         request.Expiry,
			InstanceType:   request.InstanceType,
			Backend:        request.Backend,
			OrganizationID: request.OrganizationID,
		}

		// Try to create terminal
		sessionResp, err := tts.StartSessionWithPlan(member.UserID, sessionInput, planInterface)

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

	// 2. Check local state
	if terminal.Status != "active" {
		return false, terminal.Status, nil // "stopped" or "expired"
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

	// 4. Check expiration time
	if time.Now().After(terminal.ExpiresAt) {
		terminal.Status = "expired"
		err := tts.repository.UpdateTerminalSession(terminal)
		if err != nil {
			// Log error but continue - we know the session is expired
			utils.Warn("failed to update expired session %s status: %v", sessionID, err)
		}
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

		// Convert numeric status to semantic name
		apiStatusName := tts.enumService.GetEnumName("session_status", int(apiInfo.Status))

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

// loadSystemDefaultBackend reads the system default backend from the features table
func loadSystemDefaultBackend(repo configRepositories.FeatureRepository) string {
	feature, err := repo.GetFeatureByKey("terminal_default_backend")
	if err != nil {
		return ""
	}
	return feature.Value
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

// SetSystemDefaultBackend sets the system-wide default backend
func (tts *terminalTrainerService) SetSystemDefaultBackend(backendID string) (*dto.BackendInfo, error) {
	backends, err := tts.getBackendsCached()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch backends: %w", err)
	}

	// Find the backend
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

	// Persist to features table
	if err := tts.featureRepo.UpdateFeatureValue("terminal_default_backend", backendID); err != nil {
		return nil, fmt.Errorf("failed to persist default backend: %w", err)
	}

	// Update in-memory cache
	tts.systemDefaultBackend = backendID

	target.IsDefault = true
	return target, nil
}
