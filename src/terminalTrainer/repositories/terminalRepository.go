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
	GetTerminalSessionsByOrganizationID(orgID uuid.UUID) (*[]models.Terminal, error)
	UpdateTerminalSession(terminal *models.Terminal) error
	DeleteTerminalSession(sessionID string) error
	HideOwnedTerminal(terminalID, userID string) error
	UnhideOwnedTerminal(terminalID, userID string) error

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
	// Check if a record (active or soft-deleted) exists with the same session_id
	var existing models.Terminal
	err := r.db.Unscoped().Where("session_id = ?", terminal.SessionID).First(&existing).Error
	if err == nil {
		if existing.DeletedAt.Valid {
			// Soft-deleted record found: restore it with updated fields
			return r.db.Unscoped().Model(&existing).Updates(map[string]any{
				"deleted_at":            nil,
				"status":               terminal.Status,
				"user_id":              terminal.UserID,
				"name":                 terminal.Name,
				"expires_at":           terminal.ExpiresAt,
				"instance_type":        terminal.InstanceType,
				"machine_size":         terminal.MachineSize,
				"backend":              terminal.Backend,
				"user_terminal_key_id": terminal.UserTerminalKeyID,
			}).Error
		}
		// Active record found (reinit case): update it with new fields
		return r.db.Model(&existing).Updates(map[string]any{
			"status":               terminal.Status,
			"user_id":              terminal.UserID,
			"name":                 terminal.Name,
			"expires_at":           terminal.ExpiresAt,
			"instance_type":        terminal.InstanceType,
			"machine_size":         terminal.MachineSize,
			"backend":              terminal.Backend,
			"user_terminal_key_id": terminal.UserTerminalKeyID,
		}).Error
	}
	// No existing record: create normally
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
	// Check with Unscoped to find soft-deleted records too
	var existing models.Terminal
	err := tr.db.Unscoped().Where("session_id = ?", terminal.SessionID).First(&existing).Error
	if err == nil {
		// Record exists
		if existing.DeletedAt.Valid {
			// Soft-deleted: restore it with updated fields
			return tr.db.Unscoped().Model(&existing).Updates(map[string]any{
				"deleted_at":            nil,
				"status":               terminal.Status,
				"user_id":              terminal.UserID,
				"name":                 terminal.Name,
				"expires_at":           terminal.ExpiresAt,
				"instance_type":        terminal.InstanceType,
				"machine_size":         terminal.MachineSize,
				"backend":              terminal.Backend,
				"user_terminal_key_id": terminal.UserTerminalKeyID,
			}).Error
		}
		// Not soft-deleted: already exists, idempotent
		return nil
	}
	// Doesn't exist at all: create
	return tr.db.Create(terminal).Error
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

