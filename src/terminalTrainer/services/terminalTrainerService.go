package services

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"soli/formations/src/auth/casdoor"
	groupModels "soli/formations/src/groups/models"
	paymentModels "soli/formations/src/payment/models"
	paymentServices "soli/formations/src/payment/services"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TerminalTrainerService interface {
	// User key management
	CreateUserKey(userID, userName string) error
	GetUserKey(userID string) (*models.UserTerminalKey, error)
	DisableUserKey(userID string) error

	// Session management
	StartSession(userID string, sessionInput dto.CreateTerminalSessionInput) (*dto.TerminalSessionResponse, error)
	StartSessionWithPlan(userID string, sessionInput dto.CreateTerminalSessionInput, planInterface interface{}) (*dto.TerminalSessionResponse, error)
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
	GetInstanceTypes() ([]dto.InstanceType, error)

	// Metrics
	GetServerMetrics(nocache bool) (*dto.ServerMetricsResponse, error)

	// Correction des permissions
	FixTerminalHidePermissions(userID string) (*dto.FixPermissionsResponse, error)

	// Bulk operations
	BulkCreateTerminalsForGroup(groupID string, requestingUserID string, request dto.BulkCreateTerminalsRequest, planInterface interface{}) (*dto.BulkCreateTerminalsResponse, error)
}

type terminalTrainerService struct {
	adminKey            string
	baseURL             string
	apiVersion          string
	terminalType        string
	repository          repositories.TerminalRepository
	subscriptionService paymentServices.UserSubscriptionService
	db                  *gorm.DB
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

	return &terminalTrainerService{
		adminKey:            os.Getenv("TERMINAL_TRAINER_ADMIN_KEY"),
		baseURL:             os.Getenv("TERMINAL_TRAINER_URL"), // http://localhost:8090
		apiVersion:          apiVersion,
		terminalType:        terminalType,
		repository:          repositories.NewTerminalRepository(db),
		subscriptionService: paymentServices.NewSubscriptionService(db),
		db:                  db,
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

	url := fmt.Sprintf("%s/%s/admin/api-keys?key=%s", tts.baseURL, tts.apiVersion, tts.adminKey)
	var apiResponse dto.TerminalTrainerAPIKeyResponse

	opts := utils.DefaultHTTPClientOptions()
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
func (tts *terminalTrainerService) DisableUserKey(userID string) error {
	key, err := tts.repository.GetUserTerminalKeyByUserID(userID, true)
	if err != nil {
		return err
	}

	// Désactiver côté Terminal Trainer
	payload := map[string]interface{}{
		"is_active": false,
	}

	url := fmt.Sprintf("%s/%s/admin/api-keys/%s?key=%s", tts.baseURL, tts.apiVersion, key.APIKey, tts.adminKey)
	opts := utils.DefaultHTTPClientOptions()

	_, err = utils.MakeExternalAPIRequest("Terminal Trainer", "PUT", url, payload, opts)
	if err != nil {
		return err
	}

	// Désactiver en base locale
	key.IsActive = false
	return tts.repository.UpdateUserTerminalKey(key)
}

// StartSession démarre une nouvelle session
func (tts *terminalTrainerService) StartSession(userID string, sessionInput dto.CreateTerminalSessionInput) (*dto.TerminalSessionResponse, error) {
	// Récupérer la clé utilisateur
	userKey, err := tts.repository.GetUserTerminalKeyByUserID(userID, true)
	if err != nil {
		return nil, fmt.Errorf("no terminal trainer key found for user: %v", err)
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

	// Parser la réponse du Terminal Trainer
	var sessionResp dto.TerminalTrainerSessionResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithAPIKey(userKey.APIKey))

	err = utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &sessionResp, opts)
	if err != nil {
		return nil, err
	}

	if sessionResp.Status != 0 {
		return nil, fmt.Errorf("failed to start session response status: %d", sessionResp.Status)
	}

	// Créer l'enregistrement local
	expiresAt := time.Unix(sessionResp.ExpiresAt, 0)
	terminal := &models.Terminal{
		SessionID:         sessionResp.SessionID,
		UserID:            userID,
		Name:              sessionInput.Name,
		Status:            "active",
		ExpiresAt:         expiresAt,
		InstanceType:      sessionInput.InstanceType,
		MachineSize:       sessionResp.MachineSize, // Taille réelle retournée par Terminal Trainer
		UserTerminalKeyID: userKey.ID,
		UserTerminalKey:   *userKey,
	}

	if err := tts.repository.CreateTerminalSession(terminal); err != nil {
		return nil, fmt.Errorf("failed to save terminal session: %v", err)
	}

	// Ajouter les permissions Casbin pour que le propriétaire puisse masquer des terminaux
	err = tts.addTerminalHidePermissions(userID)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer la création du terminal
		fmt.Printf("Warning: failed to add hide permissions for terminal %s: %v\n", terminal.ID.String(), err)
	}

	// Ajouter les permissions Casbin pour que le propriétaire puisse accéder à la console WebSocket
	err = tts.addTerminalConsolePermissions(userID)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer la création du terminal
		fmt.Printf("Warning: failed to add console permissions for terminal %s: %v\n", terminal.ID.String(), err)
	}

	// Construire la réponse
	consolePath := tts.buildAPIPath("/console", sessionInput.InstanceType)
	response := &dto.TerminalSessionResponse{
		SessionID:  sessionResp.SessionID,
		ExpiresAt:  expiresAt,
		ConsoleURL: fmt.Sprintf("%s%s?id=%s", tts.baseURL, consolePath, sessionResp.SessionID),
		Status:     "active",
	}

	return response, nil
}

// StartSessionWithPlan démarre une nouvelle session avec validation du plan d'abonnement
func (tts *terminalTrainerService) StartSessionWithPlan(userID string, sessionInput dto.CreateTerminalSessionInput, planInterface interface{}) (*dto.TerminalSessionResponse, error) {
	// Convertir l'interface en SubscriptionPlan
	plan, ok := planInterface.(*paymentModels.SubscriptionPlan)
	if !ok {
		return nil, fmt.Errorf("invalid subscription plan type")
	}

	// Valider la taille de la machine
	if sessionInput.InstanceType != "" {
		// Récupérer les types d'instances disponibles depuis l'API Terminal Trainer
		instanceTypes, err := tts.GetInstanceTypes()
		if err != nil {
			return nil, fmt.Errorf("failed to get instance types: %v", err)
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

	// Appliquer la durée maximale de session depuis le plan
	maxDurationSeconds := plan.MaxSessionDurationMinutes * 60
	if sessionInput.Expiry == 0 || sessionInput.Expiry > maxDurationSeconds {
		sessionInput.Expiry = maxDurationSeconds
	}

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
	log.Printf("[DEBUG] StopSession called for session %s\n", sessionID)

	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %v", err)
	}

	log.Printf("[DEBUG] Session %s current status: %s\n", sessionID, terminal.Status)

	// 1. Appeler l'API Terminal Trainer pour expirer la session
	log.Printf("[DEBUG] Calling expireSessionInAPI for session %s\n", sessionID)
	err = tts.expireSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey, terminal.InstanceType)
	if err != nil {
		// Log l'erreur complète pour debugging
		fmt.Printf("Warning: failed to expire session in Terminal Trainer API: %v\n", err)
	} else {
		log.Printf("[DEBUG] Successfully called expireSessionInAPI for session %s\n", sessionID)
	}

	// 2. Marquer la session comme arrêtée localement
	// L'utilisateur pourra la masquer s'il le souhaite
	log.Printf("[DEBUG] Updating session %s status to 'stopped'\n", sessionID)
	terminal.Status = "stopped"
	err = tts.repository.UpdateTerminalSession(terminal)
	if err != nil {
		fmt.Printf("[ERROR] Failed to update session %s status: %v\n", sessionID, err)
		return err
	}

	// 3. Décrémenter la métrique concurrent_terminals
	log.Printf("[DEBUG] Decrementing concurrent_terminals for user %s\n", terminal.UserID)
	decrementErr := tts.subscriptionService.IncrementUsage(terminal.UserID, "concurrent_terminals", -1)
	if decrementErr != nil {
		// Log l'erreur mais ne pas faire échouer l'arrêt du terminal
		fmt.Printf("Warning: failed to decrement concurrent_terminals for user %s: %v\n", terminal.UserID, decrementErr)
	}

	log.Printf("[DEBUG] Successfully updated session %s status to 'stopped'\n", sessionID)
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
		return nil, fmt.Errorf("session not found locally: %v", err)
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
		return nil, fmt.Errorf("no terminal trainer key found for user: %v", err)
	}

	if !userKey.IsActive {
		return nil, fmt.Errorf("user terminal trainer key is disabled")
	}

	// 2. Récupérer TOUTES les sessions depuis l'API Terminal Trainer pour tous les types d'instances
	apiSessions, err := tts.getAllSessionsFromAllInstanceTypes(userKey.APIKey, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions from Terminal Trainer API: %v", err)
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
	var sessionResults []dto.SyncSessionResponse
	var errors []string
	syncedCount := 0
	updatedCount := 0
	createdCount := 0

	// 5a. Traiter les sessions qui existent côté API (source de vérité)
	for sessionID, apiSession := range apiSessionsMap {
		localSession := localSessionsMap[sessionID]

		if localSession == nil {
			// Session existe côté API mais pas côté local
			// Ne recréer que les sessions actives, pas les expirées/arrêtées
			if apiSession.Status == "active" {
				log.Printf("[DEBUG] SyncUserSessions - Creating missing active session %s\n", sessionID)
				err := tts.createMissingLocalSession(userID, userKey, apiSession)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to create missing session %s: %v", sessionID, err))
				} else {
					sessionResults = append(sessionResults, dto.SyncSessionResponse{
						SessionID:      sessionID,
						PreviousStatus: "missing",
						CurrentStatus:  apiSession.Status,
						Updated:        true,
						LastSyncAt:     time.Now(),
					})
					syncedCount++
					updatedCount++
					createdCount++
				}
			} else {
				log.Printf("[DEBUG] SyncUserSessions - Ignoring non-active session %s (status: %s) from API\n", sessionID, apiSession.Status)
				// Ajouter quand même aux résultats pour le suivi
				sessionResults = append(sessionResults, dto.SyncSessionResponse{
					SessionID:      sessionID,
					PreviousStatus: "missing",
					CurrentStatus:  fmt.Sprintf("ignored-%s", apiSession.Status),
					Updated:        false,
					LastSyncAt:     time.Now(),
				})
				syncedCount++
			}
		} else {
			// Session existe des deux côtés -> synchroniser le statut
			previousStatus := localSession.Status
			needsUpdate := false

			log.Printf("[DEBUG] SyncUserSessions - Session %s: local='%s', api='%s'\n", sessionID, localSession.Status, apiSession.Status)

			// Vérifier si le statut a changé
			// Ne pas écraser les sessions arrêtées manuellement (status "stopped")
			if localSession.Status != apiSession.Status && localSession.Status != "stopped" {
				log.Printf("[DEBUG] SyncUserSessions - Status mismatch for session %s: changing '%s' -> '%s'\n", sessionID, localSession.Status, apiSession.Status)
				localSession.Status = apiSession.Status
				needsUpdate = true
			} else if localSession.Status == "stopped" {
				log.Printf("[DEBUG] SyncUserSessions - Session %s is manually stopped, keeping local status\n", sessionID)
			}

			// Vérifier si la session a expiré selon la date
			expiryTime := time.Unix(apiSession.ExpiresAt, 0)
			if time.Now().After(expiryTime) && apiSession.Status == "active" {
				log.Printf("[DEBUG] SyncUserSessions - Session %s expired by date, marking as expired\n", sessionID)
				localSession.Status = "expired"
				needsUpdate = true
			}

			if needsUpdate {
				log.Printf("[DEBUG] SyncUserSessions - Updating session %s status to '%s'\n", sessionID, localSession.Status)
				err := tts.repository.UpdateTerminalSession(localSession)
				if err != nil {
					fmt.Printf("[ERROR] SyncUserSessions - Failed to update session %s: %v\n", sessionID, err)
					errors = append(errors, fmt.Sprintf("Failed to update session %s: %v", sessionID, err))
				} else {
					log.Printf("[DEBUG] SyncUserSessions - Successfully updated session %s\n", sessionID)
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

	terminal := &models.Terminal{
		SessionID:         apiSession.SessionID,
		UserID:            userID,
		Status:            apiSession.Status,
		ExpiresAt:         expiresAt,
		MachineSize:       apiSession.MachineSize, // Taille réelle depuis l'API
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
		return fmt.Errorf("failed to get active user keys: %v", err)
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
func (tts *terminalTrainerService) GetInstanceTypes() ([]dto.InstanceType, error) {
	// Utiliser le type par défaut pour récupérer la liste des instances disponibles
	path := tts.buildAPIPath("/instances", "")
	url := fmt.Sprintf("%s%s", tts.baseURL, path)

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
	var allSessions []dto.TerminalTrainerSession
	totalCount := 0

	for instanceType := range instanceTypesUsed {
		apiResponse, err := tts.getSessionsFromInstanceType(userAPIKey, instanceType)
		if err != nil {
			// Log l'erreur mais continuer avec les autres types d'instances
			fmt.Printf("Warning: failed to get sessions from instance type '%s': %v\n", instanceType, err)
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
	// Vérifier que le terminal existe
	terminal, err := tts.repository.GetTerminalSessionBySessionID(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get terminal: %v", err)
	}
	if terminal == nil {
		return fmt.Errorf("terminal not found")
	}

	// Vérifier que l'utilisateur qui partage est le propriétaire du terminal
	if terminal.UserID != sharedByUserID {
		return fmt.Errorf("only terminal owner can share access")
	}

	// Vérifier que l'utilisateur ne partage pas avec lui-même
	if sharedByUserID == sharedWithUserID {
		return fmt.Errorf("cannot share terminal with yourself")
	}

	// Valider le niveau d'accès
	validLevels := map[string]bool{"read": true, "write": true, "admin": true}
	if !validLevels[accessLevel] {
		return fmt.Errorf("invalid access level: %s", accessLevel)
	}

	// Vérifier si un partage existe déjà
	existingShare, err := tts.repository.GetTerminalShare(terminal.ID.String(), sharedWithUserID)
	if err != nil {
		return fmt.Errorf("failed to check existing share: %v", err)
	}

	if existingShare != nil {
		// Si le niveau d'accès change, supprimer d'abord les anciennes permissions
		if existingShare.AccessLevel != accessLevel {
			err := tts.removeTerminalSharePermissions(sharedWithUserID, existingShare.AccessLevel)
			if err != nil {
				fmt.Printf("Warning: failed to remove old permissions for terminal %s from user %s: %v\n", terminal.ID.String(), sharedWithUserID, err)
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
			fmt.Printf("Warning: failed to add hide permissions for updated shared terminal %s to user %s: %v\n", terminal.ID.String(), sharedWithUserID, err)
		}

		// Ajouter les permissions console WebSocket
		err = tts.addTerminalConsolePermissions(sharedWithUserID)
		if err != nil {
			fmt.Printf("Warning: failed to add console permissions for updated shared terminal %s to user %s: %v\n", terminal.ID.String(), sharedWithUserID, err)
		}

		// Ajouter les nouvelles permissions d'édition selon le niveau d'accès
		err = tts.addTerminalSharePermissions(sharedWithUserID, accessLevel)
		if err != nil {
			fmt.Printf("Warning: failed to add share permissions for updated shared terminal %s to user %s: %v\n", terminal.ID.String(), sharedWithUserID, err)
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
		fmt.Printf("Warning: failed to add hide permissions for shared terminal %s to user %s: %v\n", terminal.ID.String(), sharedWithUserID, err)
	}

	// Ajouter les permissions console WebSocket
	err = tts.addTerminalConsolePermissions(sharedWithUserID)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer le partage
		fmt.Printf("Warning: failed to add console permissions for shared terminal %s to user %s: %v\n", terminal.ID.String(), sharedWithUserID, err)
	}

	// Ajouter les permissions d'édition pour les utilisateurs avec accès "admin"
	err = tts.addTerminalSharePermissions(sharedWithUserID, accessLevel)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer le partage
		fmt.Printf("Warning: failed to add share permissions for shared terminal %s to user %s: %v\n", terminal.ID.String(), sharedWithUserID, err)
	}

	return nil
}

// RevokeTerminalAccess révoque l'accès d'un utilisateur à un terminal
func (tts *terminalTrainerService) RevokeTerminalAccess(sessionID, sharedWithUserID, requestingUserID string) error {
	// Vérifier que le terminal existe
	terminal, err := tts.repository.GetTerminalSessionBySessionID(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get terminal: %v", err)
	}
	if terminal == nil {
		return fmt.Errorf("terminal not found")
	}

	// Vérifier que l'utilisateur qui révoque est le propriétaire du terminal
	if terminal.UserID != requestingUserID {
		return fmt.Errorf("only terminal owner can revoke access")
	}

	// Récupérer le partage
	share, err := tts.repository.GetTerminalShare(terminal.ID.String(), sharedWithUserID)
	if err != nil {
		return fmt.Errorf("failed to get share: %v", err)
	}
	if share == nil {
		return fmt.Errorf("no active share found")
	}

	// Révoquer les permissions Casbin avant de désactiver le partage
	err = tts.removeTerminalSharePermissions(sharedWithUserID, share.AccessLevel)
	if err != nil {
		// Log l'erreur mais ne pas faire échouer la révocation
		fmt.Printf("Warning: failed to remove permissions for terminal %s from user %s: %v\n", terminal.ID.String(), sharedWithUserID, err)
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
		return nil, fmt.Errorf("failed to get terminal: %v", err)
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
func (tts *terminalTrainerService) HasTerminalAccess(sessionID, userID string, requiredLevel string) (bool, error) {
	// D'abord vérifier si l'utilisateur est le propriétaire
	terminal, err := tts.repository.GetTerminalSessionBySessionID(sessionID)
	if err != nil {
		return false, fmt.Errorf("failed to get terminal: %v", err)
	}
	if terminal == nil {
		return false, fmt.Errorf("terminal not found")
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
		return nil, fmt.Errorf("failed to get terminal: %v", err)
	}
	if terminal == nil {
		return nil, fmt.Errorf("terminal not found")
	}

	// Vérifier si l'utilisateur a accès
	hasAccess, err := tts.HasTerminalAccess(sessionID, userID, "read")
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
		IsHiddenByOwner: terminal.IsHiddenByOwner,
		HiddenByOwnerAt: terminal.HiddenByOwnerAt,
		CreatedAt:       terminal.CreatedAt,
	}

	// Si l'utilisateur est le propriétaire, récupérer tous les partages
	var shares []dto.TerminalShareOutput
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
		accessLevel = "owner"
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
	hasAccess, err := tts.repository.HasTerminalAccess(terminalID, userID, "read")
	if err != nil {
		return fmt.Errorf("failed to check access: %v", err)
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
	hasAccess, err := tts.repository.HasTerminalAccess(terminalID, userID, "read")
	if err != nil {
		return fmt.Errorf("failed to check access: %v", err)
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
	case "read":
		methods = "GET"
	case "write":
		methods = "GET|PATCH"
	case "admin":
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
	case "read":
		methods = "GET"
	case "write":
		methods = "GET|PATCH"
	case "admin":
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
func (tts *terminalTrainerService) GetServerMetrics(nocache bool) (*dto.ServerMetricsResponse, error) {
	// Skip if Terminal Trainer is not configured
	if tts.baseURL == "" {
		return nil, fmt.Errorf("terminal trainer not configured")
	}

	// Construire l'URL des métriques
	path := fmt.Sprintf("/%s/metrics", tts.apiVersion)
	url := fmt.Sprintf("%s%s", tts.baseURL, path)

	// Ajouter le paramètre nocache si demandé
	if nocache {
		url += "?nocache=true"
	}

	// Exécuter la requête (pas besoin d'authentification selon les specs)
	var metrics dto.ServerMetricsResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithTimeout(10*time.Second))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &metrics, opts)
	if err != nil {
		return nil, err
	}

	return &metrics, nil
}

// FixTerminalHidePermissions corrige les permissions de masquage pour tous les terminaux d'un utilisateur
// et tous les terminaux partagés avec lui
func (tts *terminalTrainerService) FixTerminalHidePermissions(userID string) (*dto.FixPermissionsResponse, error) {
	response := &dto.FixPermissionsResponse{
		UserID:             userID,
		ProcessedTerminals: 0,
		ProcessedShares:    0,
		Errors:             []string{},
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
	request dto.BulkCreateTerminalsRequest,
	planInterface interface{},
) (*dto.BulkCreateTerminalsResponse, error) {
	// Parse groupID
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, fmt.Errorf("invalid group ID: %v", err)
	}

	// Get group details
	var group groupModels.ClassGroup
	if err := tts.db.Preload("Members").Where("id = ?", groupUUID).First(&group).Error; err != nil {
		return nil, fmt.Errorf("group not found: %v", err)
	}

	// Check permissions - only owner or admin can bulk create terminals
	canManage := false
	if group.OwnerUserID == requestingUserID {
		canManage = true
	} else {
		// Check if user is an admin of the group
		for _, member := range group.Members {
			if member.UserID == requestingUserID && (member.Role == groupModels.GroupMemberRoleAdmin || member.Role == groupModels.GroupMemberRoleOwner) {
				canManage = true
				break
			}
		}
	}

	if !canManage {
		return nil, fmt.Errorf("only group owner or admin can create bulk terminals")
	}

	// Filter active members only
	var activeMembers []groupModels.GroupMember
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
		Terminals:    []dto.BulkTerminalCreationResult{},
		Errors:       []string{},
	}

	// Get user details from Casdoor for email addresses
	userEmails := make(map[string]string) // userID -> email
	for _, member := range activeMembers {
		user, err := casdoorsdk.GetUserByUserId(member.UserID)
		if err != nil {
			log.Printf("Warning: failed to get user details for %s: %v", member.UserID, err)
			userEmails[member.UserID] = member.UserID // Fallback to userID
		} else {
			userEmails[member.UserID] = user.Email
		}
	}

	// Create terminals for each member
	for _, member := range activeMembers {
		userEmail := userEmails[member.UserID]

		// Generate terminal name using template
		terminalName := tts.applyNameTemplate(request.NameTemplate, group.DisplayName, userEmail, member.UserID)

		// Create session input for this user
		sessionInput := dto.CreateTerminalSessionInput{
			Terms:        request.Terms,
			Name:         terminalName,
			Expiry:       request.Expiry,
			InstanceType: request.InstanceType,
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
