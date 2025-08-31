package repositories

import (
	"errors"
	"soli/formations/src/terminalTrainer/models"
	"time"

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
	GetTerminalSessionsByUserID(userID string, isActive bool) (*[]models.Terminal, error)
	UpdateTerminalSession(terminal *models.Terminal) error
	DeleteTerminalSession(sessionID string) error

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
	err := r.db.Preload("UserTerminalKey").Where("session_id = ?", sessionID).Where("status = ?", "active").First(&terminal).Error
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
		query.Where("status = ?", "active")
	}

	err := query.
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
	return r.db.Where("session_id = ?", sessionID).Delete(&models.Terminal{}).Error
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

// Méthode utilitaire pour nettoyer les sessions expirées depuis longtemps
func (tr *terminalRepository) CleanupOldExpiredSessions(daysOld int) error {
	cutoffTime := time.Now().AddDate(0, 0, -daysOld)

	result := tr.db.Where(
		"status IN (?) AND expires_at < ?",
		[]string{"expired", "stopped"},
		cutoffTime,
	).Delete(&models.Terminal{})

	return result.Error
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
