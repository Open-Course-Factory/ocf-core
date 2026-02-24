package repositories

import (
	"errors"
	"fmt"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TerminalRepository interface {
	// UserTerminalKey methods
	CreateUserTerminalKey(key *models.UserTerminalKey) error
	GetUserTerminalKeyByUserID(userID string, lookForActive bool) (*models.UserTerminalKey, error)
	UpdateUserTerminalKey(key *models.UserTerminalKey) error
	DeleteUserTerminalKey(userID string) error

	// Terminal session methods
	CreateTerminalSession(terminal *models.Terminal) error
	GetTerminalSessionByID(sessionID string) (*models.Terminal, error)
	GetTerminalByUUID(terminalUUID string) (*models.Terminal, error)
	GetTerminalSessionsByUserID(userID string, isActive bool) (*[]models.Terminal, error)
	GetTerminalSessionsByUserIDAndOrg(userID string, organizationID *uuid.UUID, isActive bool) (*[]models.Terminal, error)
	GetTerminalSessionsByUserIDWithHidden(userID string, isActive bool, includeHidden bool) (*[]models.Terminal, error)
	GetTerminalSessionsSharedWithGroup(groupID string, includeHidden bool) (*[]models.Terminal, error)
	GetTerminalSessionsByOrganizationID(orgID uuid.UUID) (*[]models.Terminal, error)
	UpdateTerminalSession(terminal *models.Terminal) error
	DeleteTerminalSession(sessionID string) error
	HideOwnedTerminal(terminalID, userID string) error
	UnhideOwnedTerminal(terminalID, userID string) error

	// TerminalShare methods
	CreateTerminalShare(share *models.TerminalShare) error
	GetTerminalSharesByTerminalID(terminalID string) (*[]models.TerminalShare, error)
	GetTerminalSharesByUserID(userID string) (*[]models.TerminalShare, error)
	GetTerminalShare(terminalID, userID string) (*models.TerminalShare, error)
	UpdateTerminalShare(share *models.TerminalShare) error
	GetSharedTerminalsForUser(userID string) (*[]models.Terminal, error)
	GetSharedTerminalsForUserWithHidden(userID string, includeHidden bool) (*[]models.Terminal, error)
	HasTerminalAccess(terminalID, userID string, requiredLevel string) (bool, error)
	HideTerminalForUser(terminalID, userID string) error
	UnhideTerminalForUser(terminalID, userID string) error

	// Synchronisation
	GetAllActiveUserKeys() (*[]models.UserTerminalKey, error)
	GetTerminalSessionBySessionID(sessionID string) (*models.Terminal, error)
	CreateTerminalSessionFromAPI(terminal *models.Terminal) error
	GetOrphanedLocalSessions(apiSessionIDs []string) (*[]models.Terminal, error)
	GetSyncStatistics(userID string) (map[string]int, error)

	// Cleanup methods
	CleanupExpiredSessions() error
}

type terminalRepository struct {
	db *gorm.DB
}

func NewTerminalRepository(db *gorm.DB) TerminalRepository {
	return &terminalRepository{
		db: db,
	}
}

// UserTerminalKey methods
func (r *terminalRepository) CreateUserTerminalKey(key *models.UserTerminalKey) error {
	return r.db.Create(key).Error
}

func (r *terminalRepository) GetUserTerminalKeyByUserID(userID string, lookForActive bool) (*models.UserTerminalKey, error) {
	var key models.UserTerminalKey
	query := r.db.Where("user_id = ?", userID)
	if lookForActive {
		query = query.Where("is_active = ?", lookForActive)
	}
	err := query.First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (r *terminalRepository) UpdateUserTerminalKey(key *models.UserTerminalKey) error {
	return r.db.Save(key).Error
}

func (r *terminalRepository) DeleteUserTerminalKey(userID string) error {
	return r.db.Where("user_id = ?", userID).Delete(&models.UserTerminalKey{}).Error
}

// Terminal session methods
func (r *terminalRepository) CreateTerminalSession(terminal *models.Terminal) error {
	return r.db.Create(terminal).Error
}

func (r *terminalRepository) GetTerminalSessionByID(sessionID string) (*models.Terminal, error) {
	var terminal models.Terminal
	err := r.db.Preload("UserTerminalKey").Where("session_id = ?", sessionID).First(&terminal).Error
	if err != nil {
		return nil, err
	}
	return &terminal, nil
}

func (r *terminalRepository) GetTerminalByUUID(terminalUUID string) (*models.Terminal, error) {
	var terminal models.Terminal
	uuid, err := uuid.Parse(terminalUUID)
	if err != nil {
		return nil, fmt.Errorf("invalid terminal UUID format: %w", err)
	}

	err = r.db.Preload("UserTerminalKey").Where("id = ?", uuid).First(&terminal).Error
	if err != nil {
		return nil, err
	}
	return &terminal, nil
}

func (r *terminalRepository) GetTerminalSessionsByUserID(userID string, isActive bool) (*[]models.Terminal, error) {
	var terminals []models.Terminal

	query := r.db.Preload("UserTerminalKey").
		Where("user_id = ?", userID)

	if isActive {
		query = query.Where("status = ?", "active")
	}

	err := query.
		Find(&terminals).Error
	if err != nil {
		return nil, err
	}

	return &terminals, nil
}

func (r *terminalRepository) GetTerminalSessionsByUserIDAndOrg(userID string, organizationID *uuid.UUID, isActive bool) (*[]models.Terminal, error) {
	var terminals []models.Terminal

	query := r.db.Preload("UserTerminalKey").Where("user_id = ?", userID)

	if organizationID != nil {
		query = query.Where("organization_id = ?", *organizationID)
	}

	if isActive {
		query = query.Where("status = ?", "active")
	}

	err := query.Find(&terminals).Error
	if err != nil {
		return nil, err
	}
	return &terminals, nil
}

func (r *terminalRepository) GetTerminalSessionsByOrganizationID(orgID uuid.UUID) (*[]models.Terminal, error) {
	var terminals []models.Terminal
	err := r.db.Preload("UserTerminalKey").
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Find(&terminals).Error
	if err != nil {
		return nil, err
	}
	return &terminals, nil
}

func (r *terminalRepository) UpdateTerminalSession(terminal *models.Terminal) error {
	return r.db.Save(terminal).Error
}

func (r *terminalRepository) DeleteTerminalSession(sessionID string) error {
	utils.Debug("DeleteTerminalSession called for session %s", sessionID)
	result := r.db.Where("session_id = ?", sessionID).Delete(&models.Terminal{})
	utils.Debug("DeleteTerminalSession - rows affected: %d, error: %v", result.RowsAffected, result.Error)
	return result.Error
}

// Cleanup methods
func (r *terminalRepository) CleanupExpiredSessions() error {
	return r.db.Model(&models.Terminal{}).
		Where("expires_at < NOW() AND status != ?", "expired").
		Update("status", "expired").Error
}

func (tr *terminalRepository) GetAllActiveUserKeys() (*[]models.UserTerminalKey, error) {
	var keys []models.UserTerminalKey

	result := tr.db.Where("is_active = ?", true).Find(&keys)
	if result.Error != nil {
		return nil, result.Error
	}

	return &keys, nil
}

func (tr *terminalRepository) GetTerminalSessionBySessionID(sessionID string) (*models.Terminal, error) {
	var terminal models.Terminal

	result := tr.db.Preload("UserTerminalKey").Where("session_id = ?", sessionID).First(&terminal)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil // Retourner nil si pas trouvé (pas une erreur)
		}
		return nil, result.Error
	}

	return &terminal, nil
}

func (tr *terminalRepository) CreateTerminalSessionFromAPI(terminal *models.Terminal) error {
	// Vérifier d'abord si la session existe déjà
	existing, err := tr.GetTerminalSessionBySessionID(terminal.SessionID)
	if err != nil {
		return err
	}
	if existing != nil {
		// Session existe déjà, ne pas la créer
		return nil
	}

	result := tr.db.Create(terminal)
	return result.Error
}

func (tr *terminalRepository) GetOrphanedLocalSessions(apiSessionIDs []string) (*[]models.Terminal, error) {
	var orphanedSessions []models.Terminal

	if len(apiSessionIDs) == 0 {
		// Si aucune session côté API, toutes les sessions locales actives sont orphelines
		result := tr.db.Preload("UserTerminalKey").Where(
			"status IN (?)",
			[]string{"active", "pending", "connecting", "waiting"},
		).Find(&orphanedSessions)

		if result.Error != nil {
			return nil, result.Error
		}
	} else {
		// Sessions actives qui ne sont pas dans la liste API
		result := tr.db.Preload("UserTerminalKey").Where(
			"status IN (?) AND session_id NOT IN (?)",
			[]string{"active", "pending", "connecting", "waiting"},
			apiSessionIDs,
		).Find(&orphanedSessions)

		if result.Error != nil {
			return nil, result.Error
		}
	}

	return &orphanedSessions, nil
}

// Méthode pour obtenir des statistiques de synchronisation
func (tr *terminalRepository) GetSyncStatistics(userID string) (map[string]int, error) {
	stats := make(map[string]int)

	// Compter par statut
	var counts []struct {
		Status string
		Count  int
	}

	query := tr.db.Model(&models.Terminal{}).
		Select("status, COUNT(*) as count").
		Group("status")

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	result := query.Scan(&counts)
	if result.Error != nil {
		return nil, result.Error
	}

	for _, count := range counts {
		stats[count.Status] = count.Count
	}

	// Ajouter le total
	var total int64
	countQuery := tr.db.Model(&models.Terminal{})
	if userID != "" {
		countQuery = countQuery.Where("user_id = ?", userID)
	}
	countQuery.Count(&total)
	stats["total"] = int(total)

	return stats, nil
}

// TerminalShare methods
func (r *terminalRepository) CreateTerminalShare(share *models.TerminalShare) error {
	return r.db.Create(share).Error
}

func (r *terminalRepository) GetTerminalSharesByTerminalID(terminalID string) (*[]models.TerminalShare, error) {
	var shares []models.TerminalShare
	// Convert string to UUID for proper comparison
	terminalUUID, err := uuid.Parse(terminalID)
	if err != nil {
		return nil, fmt.Errorf("invalid terminal ID format: %w", err)
	}
	err = r.db.Preload("Terminal").Where("terminal_id = ? AND is_active = ?", terminalUUID, true).Find(&shares).Error
	if err != nil {
		return nil, err
	}
	return &shares, nil
}

func (r *terminalRepository) GetTerminalSharesByUserID(userID string) (*[]models.TerminalShare, error) {
	var shares []models.TerminalShare
	err := r.db.Preload("Terminal").Where("shared_with_user_id = ? AND is_active = ?", userID, true).Find(&shares).Error
	if err != nil {
		return nil, err
	}
	return &shares, nil
}

func (r *terminalRepository) GetTerminalShare(terminalID, userID string) (*models.TerminalShare, error) {
	var share models.TerminalShare
	// Convert string to UUID for proper comparison
	terminalUUID, err := uuid.Parse(terminalID)
	if err != nil {
		return nil, fmt.Errorf("invalid terminal ID format: %w", err)
	}
	err = r.db.Preload("Terminal").Where("terminal_id = ? AND shared_with_user_id = ? AND is_active = ?", terminalUUID, userID, true).First(&share).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &share, nil
}

func (r *terminalRepository) UpdateTerminalShare(share *models.TerminalShare) error {
	return r.db.Save(share).Error
}

func (r *terminalRepository) GetSharedTerminalsForUser(userID string) (*[]models.Terminal, error) {
	var terminals []models.Terminal

	// Récupérer tous les terminaux partagés avec cet utilisateur
	err := r.db.Joins("JOIN terminal_shares ON terminals.id = terminal_shares.terminal_id").
		Preload("UserTerminalKey").
		Where("terminal_shares.shared_with_user_id = ? AND terminal_shares.is_active = ? AND terminals.status = ?", userID, true, "active").
		Find(&terminals).Error

	if err != nil {
		return nil, err
	}
	return &terminals, nil
}

func (r *terminalRepository) HasTerminalAccess(terminalID, userID string, requiredLevel string) (bool, error) {
	var count int64

	// Convert string to UUID for proper comparison
	terminalUUID, err := uuid.Parse(terminalID)
	if err != nil {
		return false, fmt.Errorf("invalid terminal ID format: %w", err)
	}

	// Vérifier s'il y a un partage actif avec le niveau d'accès requis ou supérieur
	requiredLevelInt, exists := models.AccessLevelHierarchy[requiredLevel]
	if !exists {
		return false, errors.New("invalid access level")
	}

	// Construire la liste des niveaux d'accès suffisants
	var allowedLevels []string
	for level, value := range models.AccessLevelHierarchy {
		if value >= requiredLevelInt {
			allowedLevels = append(allowedLevels, level)
		}
	}

	err = r.db.Model(&models.TerminalShare{}).
		Where("terminal_id = ? AND shared_with_user_id = ? AND is_active = ? AND access_level IN ?",
			terminalUUID, userID, true, allowedLevels).
		Where("(expires_at IS NULL OR expires_at > ?)", time.Now()).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (r *terminalRepository) GetSharedTerminalsForUserWithHidden(userID string, includeHidden bool) (*[]models.Terminal, error) {
	var terminals []models.Terminal

	query := r.db.Joins("JOIN terminal_shares ON terminals.id = terminal_shares.terminal_id").
		Preload("UserTerminalKey").
		Where("terminal_shares.shared_with_user_id = ? AND terminal_shares.is_active = ?", userID, true)

	if !includeHidden {
		query = query.Where("terminal_shares.is_hidden_by_recipient = ?", false)
	}

	err := query.Find(&terminals).Error
	if err != nil {
		return nil, err
	}
	return &terminals, nil
}

func (r *terminalRepository) HideTerminalForUser(terminalID, userID string) error {
	terminalUUID, err := uuid.Parse(terminalID)
	if err != nil {
		return fmt.Errorf("invalid terminal ID format: %w", err)
	}

	now := time.Now()
	return r.db.Model(&models.TerminalShare{}).
		Where("terminal_id = ? AND shared_with_user_id = ?", terminalUUID, userID).
		Updates(map[string]any{
			"is_hidden_by_recipient": true,
			"hidden_at":              now,
		}).Error
}

func (r *terminalRepository) UnhideTerminalForUser(terminalID, userID string) error {
	terminalUUID, err := uuid.Parse(terminalID)
	if err != nil {
		return fmt.Errorf("invalid terminal ID format: %w", err)
	}

	// Create an empty share model to update with
	share := models.TerminalShare{
		IsHiddenByRecipient: false,
		HiddenAt:            nil,
	}

	return r.db.Model(&models.TerminalShare{}).
		Where("terminal_id = ? AND shared_with_user_id = ?", terminalUUID, userID).
		Select("is_hidden_by_recipient", "hidden_at").
		Updates(share).Error
}

func (r *terminalRepository) GetTerminalSessionsByUserIDWithHidden(userID string, isActive bool, includeHidden bool) (*[]models.Terminal, error) {
	var terminals []models.Terminal

	query := r.db.Preload("UserTerminalKey").Where("user_id = ?", userID)

	if isActive {
		query = query.Where("status = ?", "active")
	}

	if !includeHidden {
		query = query.Where("is_hidden_by_owner = ?", false)
	}

	err := query.Find(&terminals).Error
	if err != nil {
		return nil, err
	}
	return &terminals, nil
}

func (r *terminalRepository) HideOwnedTerminal(terminalID, userID string) error {
	terminalUUID, err := uuid.Parse(terminalID)
	if err != nil {
		return fmt.Errorf("invalid terminal ID format: %w", err)
	}

	now := time.Now()
	return r.db.Model(&models.Terminal{}).
		Where("id = ? AND user_id = ?", terminalUUID, userID).
		Updates(map[string]any{
			"is_hidden_by_owner": true,
			"hidden_by_owner_at": now,
		}).Error
}

func (r *terminalRepository) UnhideOwnedTerminal(terminalID, userID string) error {
	terminalUUID, err := uuid.Parse(terminalID)
	if err != nil {
		return fmt.Errorf("invalid terminal ID format: %w", err)
	}

	// Create an empty terminal model to update with
	terminal := models.Terminal{
		IsHiddenByOwner: false,
		HiddenByOwnerAt: nil,
	}

	return r.db.Model(&models.Terminal{}).
		Where("id = ? AND user_id = ?", terminalUUID, userID).
		Select("is_hidden_by_owner", "hidden_by_owner_at").
		Updates(terminal).Error
}

func (r *terminalRepository) GetTerminalSessionsSharedWithGroup(groupID string, includeHidden bool) (*[]models.Terminal, error) {
	var terminals []models.Terminal

	// Parse group ID as UUID
	groupUUID, err := uuid.Parse(groupID)
	if err != nil {
		return nil, fmt.Errorf("invalid group ID format: %w", err)
	}

	// Query terminals that are shared with the specified group
	query := r.db.Joins("JOIN terminal_shares ON terminals.id = terminal_shares.terminal_id").
		Preload("UserTerminalKey").
		Where("terminal_shares.shared_with_group_id = ? AND terminal_shares.is_active = ?", groupUUID, true)

	// Filter by hidden status if requested
	if !includeHidden {
		query = query.Where("terminal_shares.is_hidden_by_recipient = ?", false)
	}

	err = query.Find(&terminals).Error
	if err != nil {
		return nil, err
	}
	return &terminals, nil
}
