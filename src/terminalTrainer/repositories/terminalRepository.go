package repositories

import (
	"soli/formations/src/terminalTrainer/models"

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
	GetActiveTerminalSessionsByUserID(userID string) (*[]models.Terminal, error)
	UpdateTerminalSession(terminal *models.Terminal) error
	DeleteTerminalSession(sessionID string) error

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

func (r *terminalRepository) GetActiveTerminalSessionsByUserID(userID string) (*[]models.Terminal, error) {
	var terminals []models.Terminal
	err := r.db.Preload("UserTerminalKey").
		Where("user_id = ? AND status = ?", userID, "active").
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
