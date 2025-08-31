package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"

	"gorm.io/gorm"
)

type TerminalTrainerService interface {
	// User key management
	CreateUserKey(userID, userName string) error
	GetUserKey(userID string) (*models.UserTerminalKey, error)
	DisableUserKey(userID string) error

	// Session management
	StartSession(userID string, sessionInput dto.CreateTerminalSessionInput) (*dto.TerminalSessionResponse, error)
	GetSessionInfo(sessionID string) (*models.Terminal, error)
	GetActiveUserSessions(userID string) (*[]models.Terminal, error)
	StopSession(sessionID string) error

	// Synchronization methods (nouvelle approche avec API comme source de vérité)
	GetAllSessionsFromAPI(userAPIKey string) (*dto.TerminalTrainerSessionsResponse, error)
	SyncUserSessions(userID string) (*dto.SyncAllSessionsResponse, error)
	SyncAllActiveSessions() error
	GetSessionInfoFromAPI(sessionID string) (*dto.TerminalTrainerSessionInfo, error)

	// Utilities
	GetRepository() repositories.TerminalRepository
	CleanupExpiredSessions() error
}

type terminalTrainerService struct {
	adminKey   string
	baseURL    string
	repository repositories.TerminalRepository
}

func NewTerminalTrainerService(db *gorm.DB) TerminalTrainerService {
	return &terminalTrainerService{
		adminKey:   os.Getenv("TERMINAL_TRAINER_ADMIN_KEY"),
		baseURL:    os.Getenv("TERMINAL_TRAINER_URL"), // http://localhost:8090
		repository: repositories.NewTerminalRepository(db),
	}
}

// CreateUserKey crée une clé Terminal Trainer et la stocke en DB
func (tts *terminalTrainerService) CreateUserKey(userID, keyName string) error {
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

	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/admin/api-keys?key=%s", tts.baseURL, tts.adminKey),
		bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("terminal trainer API call failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("terminal trainer API error: %d", resp.StatusCode)
	}

	// Parser la réponse
	var apiResponse dto.TerminalTrainerAPIKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return fmt.Errorf("failed to parse terminal trainer response: %v", err)
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
	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PUT",
		fmt.Sprintf("%s/admin/api-keys/%s?key=%s", tts.baseURL, key.APIKey, tts.adminKey),
		bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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

	// Vérifier le nombre de sessions actives
	activeSessions, err := tts.repository.GetTerminalSessionsByUserID(userID, true)
	if err == nil && len(*activeSessions) >= userKey.MaxSessions {
		return nil, fmt.Errorf("maximum number of concurrent sessions reached")
	}

	// Appel à l'API Terminal Trainer pour démarrer la session
	hash := sha256.New()
	io.WriteString(hash, sessionInput.Terms)
	req, _ := http.NewRequest("GET",
		fmt.Sprintf("%s/1.0/start?terms=%s", tts.baseURL, fmt.Sprintf("%x", hash.Sum(nil))), nil)

	if sessionInput.Expiry > 0 {
		req.URL.RawQuery += fmt.Sprintf("&expiry=%d", sessionInput.Expiry)
	}

	req.Header.Set("X-API-Key", userKey.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to start terminal session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("terminal trainer session start error: %d", resp.StatusCode)
	}

	// Parser la réponse du Terminal Trainer
	var sessionResp dto.TerminalTrainerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return nil, fmt.Errorf("failed to parse session response: %v", err)
	}

	if sessionResp.Status != "0" {
		return nil, fmt.Errorf("failed to start session response status: %s", sessionResp.Status)
	}

	// Créer l'enregistrement local
	expiresAt := time.Unix(sessionResp.ExpiresAt, 0)
	terminal := &models.Terminal{
		SessionID:         sessionResp.SessionID,
		UserID:            userID,
		Status:            "active",
		ExpiresAt:         expiresAt,
		UserTerminalKeyID: userKey.ID,
		UserTerminalKey:   *userKey,
	}

	if err := tts.repository.CreateTerminalSession(terminal); err != nil {
		return nil, fmt.Errorf("failed to save terminal session: %v", err)
	}

	// Construire la réponse
	response := &dto.TerminalSessionResponse{
		SessionID:  sessionResp.SessionID,
		ExpiresAt:  expiresAt,
		ConsoleURL: fmt.Sprintf("%s/1.0/console?id=%s", tts.baseURL, sessionResp.SessionID),
		Status:     "active",
	}

	return response, nil
}

// GetSessionInfo récupère les informations d'une session
func (tts *terminalTrainerService) GetSessionInfo(sessionID string) (*models.Terminal, error) {
	return tts.repository.GetTerminalSessionByID(sessionID)
}

// GetActiveUserSessions récupère toutes les sessions actives d'un utilisateur
func (tts *terminalTrainerService) GetActiveUserSessions(userID string) (*[]models.Terminal, error) {
	return tts.repository.GetTerminalSessionsByUserID(userID, true)
}

// StopSession arrête une session ET appelle l'API externe pour expirer
func (tts *terminalTrainerService) StopSession(sessionID string) error {
	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %v", err)
	}

	// 1. Appeler l'API Terminal Trainer pour expirer la session
	err = tts.expireSessionInAPI(sessionID, terminal.UserTerminalKey.APIKey)
	if err != nil {
		// Log l'erreur mais ne pas bloquer la mise à jour locale
		fmt.Printf("Warning: failed to expire session in Terminal Trainer API: %v\n", err)
	}

	// 2. Marquer comme arrêtée en base locale
	terminal.Status = "stopped"
	return tts.repository.UpdateTerminalSession(terminal)
}

// expireSessionInAPI appelle l'endpoint /1.0/expire de l'API Terminal Trainer
func (tts *terminalTrainerService) expireSessionInAPI(sessionID, userAPIKey string) error {
	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/1.0/expire?id=%s", tts.baseURL, sessionID), nil)
	if err != nil {
		return fmt.Errorf("failed to create expire request: %v", err)
	}

	req.Header.Set("X-API-Key", userAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call expire endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("expire API returned error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetAllSessionsFromAPI récupère toutes les sessions depuis l'API Terminal Trainer
func (tts *terminalTrainerService) GetAllSessionsFromAPI(userAPIKey string) (*dto.TerminalTrainerSessionsResponse, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/1.0/sessions?include_expired=true&limit=1000", tts.baseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create sessions request: %v", err)
	}

	req.Header.Set("X-API-Key", userAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call sessions endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sessions API returned error %d: %s", resp.StatusCode, string(body))
	}

	var sessionsResp dto.TerminalTrainerSessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionsResp); err != nil {
		return nil, fmt.Errorf("failed to parse sessions response: %v", err)
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

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/1.0/info?id=%s", tts.baseURL, sessionID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create info request: %v", err)
	}

	req.Header.Set("X-API-Key", terminal.UserTerminalKey.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call info endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("session not found on Terminal Trainer")
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("info API returned error %d: %s", resp.StatusCode, string(body))
	}

	var sessionInfo dto.TerminalTrainerSessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&sessionInfo); err != nil {
		return nil, fmt.Errorf("failed to parse session info response: %v", err)
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

	// 2. Récupérer TOUTES les sessions depuis l'API Terminal Trainer (source de vérité)
	apiSessions, err := tts.GetAllSessionsFromAPI(userKey.APIKey)
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
			// Session existe côté API mais pas côté local -> la créer
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
			// Session existe des deux côtés -> synchroniser le statut
			previousStatus := localSession.Status
			needsUpdate := false

			// Vérifier si le statut a changé
			if localSession.Status != apiSession.Status {
				localSession.Status = apiSession.Status
				needsUpdate = true
			}

			// Vérifier si la session a expiré selon la date
			expiryTime := time.Unix(apiSession.ExpiresAt, 0)
			if time.Now().After(expiryTime) && apiSession.Status == "active" {
				localSession.Status = "expired"
				needsUpdate = true
			}

			if needsUpdate {
				err := tts.repository.UpdateTerminalSession(localSession)
				if err != nil {
					errors = append(errors, fmt.Sprintf("Failed to update session %s: %v", sessionID, err))
				} else {
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
