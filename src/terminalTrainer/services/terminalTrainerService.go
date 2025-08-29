package services

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	// Cleanup
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
func (tts *terminalTrainerService) CreateUserKey(userID, userName string) error {
	// Vérifier si l'utilisateur a déjà une clé
	existingKey, err := tts.repository.GetUserTerminalKeyByUserID(userID)
	if err == nil && existingKey != nil {
		return fmt.Errorf("user already has a terminal trainer key")
	}

	// Appel à l'API Terminal Trainer
	payload := map[string]any{
		"name":                    fmt.Sprintf("user-%s", userName),
		"is_admin":                false,
		"max_concurrent_sessions": 5,
	}

	jsonPayload, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/admin/api-keys?key=%s", tts.baseURL, tts.adminKey),
		bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
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
		APIKey:      apiResponse.Data.Key,
		KeyName:     apiResponse.Data.Name,
		IsActive:    true,
		MaxSessions: apiResponse.Data.MaxConcurrentSessions,
	}

	return tts.repository.CreateUserTerminalKey(userTerminalKey)
}

// GetUserKey récupère la clé Terminal Trainer d'un utilisateur
func (tts *terminalTrainerService) GetUserKey(userID string) (*models.UserTerminalKey, error) {
	return tts.repository.GetUserTerminalKeyByUserID(userID)
}

// DisableUserKey désactive la clé d'un utilisateur
func (tts *terminalTrainerService) DisableUserKey(userID string) error {
	key, err := tts.repository.GetUserTerminalKeyByUserID(userID)
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

	client := &http.Client{}
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
	userKey, err := tts.repository.GetUserTerminalKeyByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("no terminal trainer key found for user: %v", err)
	}

	if !userKey.IsActive {
		return nil, fmt.Errorf("user terminal trainer key is disabled")
	}

	// Vérifier le nombre de sessions actives
	activeSessions, err := tts.repository.GetActiveTerminalSessionsByUserID(userID)
	if err == nil && len(*activeSessions) >= userKey.MaxSessions {
		return nil, fmt.Errorf("maximum number of concurrent sessions reached")
	}

	// Appel à l'API Terminal Trainer pour démarrer la session
	req, _ := http.NewRequest("GET",
		fmt.Sprintf("%s/1.0/start?terms=%s", tts.baseURL, sessionInput.Terms), nil)

	if sessionInput.Expiry > 0 {
		req.URL.RawQuery += fmt.Sprintf("&expiry=%d", sessionInput.Expiry)
	}

	req.Header.Set("X-API-Key", userKey.APIKey)

	client := &http.Client{}
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

	// Créer l'enregistrement local
	expiresAt := time.Unix(sessionResp.ExpiresAt, 0)
	terminal := &models.Terminal{
		SessionID:         sessionResp.SessionID,
		UserID:            userID,
		Status:            "active",
		ExpiresAt:         expiresAt,
		UserTerminalKeyID: userKey.ID,
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
	return tts.repository.GetActiveTerminalSessionsByUserID(userID)
}

// StopSession arrête une session
func (tts *terminalTrainerService) StopSession(sessionID string) error {
	terminal, err := tts.repository.GetTerminalSessionByID(sessionID)
	if err != nil {
		return err
	}

	// Marquer comme arrêtée en base
	terminal.Status = "stopped"
	return tts.repository.UpdateTerminalSession(terminal)
}

// CleanupExpiredSessions nettoie les sessions expirées
func (tts *terminalTrainerService) CleanupExpiredSessions() error {
	return tts.repository.CleanupExpiredSessions()
}
