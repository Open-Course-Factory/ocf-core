package repositories

import (
	"errors"
	"fmt"
	"time"

	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/utils"

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
	// Reservation methods back the composed-session reserve-first flow:
	// a row is created in StateStarting inside a locked tx (so it counts
	// toward the budget before provisioning), then either finalized to a
	// real running session or deleted if provisioning fails.
	CreateReservation(tx *gorm.DB, terminal *models.Terminal) error
	FinalizeReservation(terminal *models.Terminal) error
	DeleteReservation(reservationID uuid.UUID) error
	GetTerminalSessionByID(sessionID string) (*models.Terminal, error)
	GetTerminalByUUID(terminalUUID string) (*models.Terminal, error)
	GetTerminalSessionsByUserID(userID string, isActive bool) (*[]models.Terminal, error)
	GetTerminalSessionsByUserIDAndOrg(userID string, organizationID *uuid.UUID, isActive bool) (*[]models.Terminal, error)
	GetTerminalSessionsByOrganizationID(orgID uuid.UUID) (*[]models.Terminal, error)
	// GetTerminalSessionsForOrgUsageExport returns every terminal attributed to
	// the org (terminals.organization_id) whose created_at falls in [from, to),
	// ordered created_at ASC. Billing attribution: scoped by the row's
	// organization_id (the budget the session actually consumed), NOT the
	// organization_members join the live dashboard uses — a member who has
	// since left the org still consumed its budget during the period. No state
	// filter: running, stopped, and deleted tombstones (deleted_at NULL) all
	// count if created_at is in range.
	GetTerminalSessionsForOrgUsageExport(orgID uuid.UUID, from, to time.Time) (*[]models.Terminal, error)
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
	// Check if a record (active or soft-deleted) exists with the same session_id
	var existing models.Terminal
	err := r.db.Unscoped().Where("session_id = ?", terminal.SessionID).First(&existing).Error
	if err == nil {
		if existing.DeletedAt.Valid {
			// Soft-deleted record found: restore it with updated fields.
			// size_cpu / size_memory_mb MUST be carried in the Updates map —
			// otherwise a reinit at a different size silently keeps the old
			// denorm and the budget aggregate undercharges the new footprint.
			return r.db.Unscoped().Model(&existing).Updates(map[string]any{
				"deleted_at":           nil,
				"state":                terminal.State,
				"user_id":              terminal.UserID,
				"name":                 terminal.Name,
				"expires_at":           terminal.ExpiresAt,
				"instance_type":        terminal.InstanceType,
				"machine_size":         terminal.MachineSize,
				"backend":              terminal.Backend,
				"user_terminal_key_id": terminal.UserTerminalKeyID,
				"size_cpu":             terminal.SizeCPU,
				"size_memory_mb":       terminal.SizeMemoryMB,
			}).Error
		}
		// Active record found (reinit case): update it with new fields.
		// size_cpu / size_memory_mb MUST be carried — same reason as above.
		return r.db.Model(&existing).Updates(map[string]any{
			"state":                terminal.State,
			"user_id":              terminal.UserID,
			"name":                 terminal.Name,
			"expires_at":           terminal.ExpiresAt,
			"instance_type":        terminal.InstanceType,
			"machine_size":         terminal.MachineSize,
			"backend":              terminal.Backend,
			"user_terminal_key_id": terminal.UserTerminalKeyID,
			"size_cpu":             terminal.SizeCPU,
			"size_memory_mb":       terminal.SizeMemoryMB,
		}).Error
	}
	// No existing record: create normally
	return r.db.Create(terminal).Error
}

// CreateReservation inserts a budget reservation row on the caller's
// transaction. It always takes the clean tx.Create path (never the
// session_id-keyed reinit/restore branch of CreateTerminalSession): the
// reservation carries a unique placeholder session_id and must persist as
// a fresh row so it counts toward the budget under OccupiesSlotScope while
// the container provisions.
func (r *terminalRepository) CreateReservation(tx *gorm.DB, terminal *models.Terminal) error {
	return tx.Create(terminal).Error
}

// FinalizeReservation promotes a committed reservation to a live session
// once tt-backend has provisioned the container. It overwrites the
// placeholder session_id with the real one, flips the state to running,
// and refreshes the resource snapshot — size_cpu/size_memory_mb are
// rewritten so a finalize at a different size can never desync the budget
// aggregate (mirrors CreateTerminalSession's reinit branch).
//
// tt-backend can hand back the id of a prior (soft-deleted) session, and
// session_id carries a plain unique index: a blind UPDATE of the placeholder
// to that id would violate the constraint. So finalize restores the existing
// row into the live session and drops the placeholder, preserving the
// upsert-by-session_id behavior the old CreateTerminalSession provided before
// the reserve-first refactor (see composedSessionFinalizeCollision_test.go).
func (r *terminalRepository) FinalizeReservation(terminal *models.Terminal) error {
	fields := map[string]any{
		"session_id":            terminal.SessionID,
		"state":                 terminal.State,
		"name":                  terminal.Name,
		"expires_at":            terminal.ExpiresAt,
		"instance_type":         terminal.InstanceType,
		"machine_size":          terminal.MachineSize,
		"backend":               terminal.Backend,
		"organization_id":       terminal.OrganizationID,
		"subscription_plan_id":  terminal.SubscriptionPlanID,
		"user_terminal_key_id":  terminal.UserTerminalKeyID,
		"composed_distribution": terminal.ComposedDistribution,
		"composed_size":         terminal.ComposedSize,
		"composed_features":     terminal.ComposedFeatures,
		"persistence_mode":      terminal.PersistenceMode,
		"size_cpu":              terminal.SizeCPU,
		"size_memory_mb":        terminal.SizeMemoryMB,
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing models.Terminal
		err := tx.Unscoped().
			Where("session_id = ? AND id <> ?", terminal.SessionID, terminal.ID).
			First(&existing).Error
		switch {
		case err == nil:
			// Another row already owns this session_id (a recycled, soft-deleted
			// session). Restore it into the live session and hard-delete the
			// placeholder so exactly one row owns the id and the budget counts
			// the session once.
			fields["deleted_at"] = nil
			if err := tx.Unscoped().Model(&existing).Updates(fields).Error; err != nil {
				return err
			}
			return tx.Unscoped().Delete(&models.Terminal{}, "id = ?", terminal.ID).Error
		case errors.Is(err, gorm.ErrRecordNotFound):
			// No collision: promote the placeholder in place.
			return tx.Model(&models.Terminal{}).Where("id = ?", terminal.ID).Updates(fields).Error
		default:
			return err
		}
	})
}

// DeleteReservation hard-deletes a reservation row by primary key. Used
// when provisioning fails: the placeholder row never represented a real
// container, so removing it (rather than soft-deleting) immediately frees
// the reserved budget and leaves no zombie to reconcile.
func (r *terminalRepository) DeleteReservation(reservationID uuid.UUID) error {
	return r.db.Unscoped().Delete(&models.Terminal{}, "id = ?", reservationID).Error
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
		// SSOT: "alive RIGHT NOW" is defined once in models.RunningDisplayScope.
		// Inline `status = "active"` predicates drifted from the per-second-aware
		// UI by including past-expiry zombie rows.
		query = query.Scopes(models.RunningDisplayScope)
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
		// SSOT: route through models.RunningDisplayScope so this caller
		// agrees with every other "currently running terminals" query.
		query = query.Scopes(models.RunningDisplayScope)
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

func (r *terminalRepository) GetTerminalSessionsForOrgUsageExport(orgID uuid.UUID, from, to time.Time) (*[]models.Terminal, error) {
	var terminals []models.Terminal
	err := r.db.
		Where("organization_id = ? AND created_at >= ? AND created_at < ?", orgID, from, to).
		Order("created_at ASC").
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
	// State is the SSOT — past-expiry rows that aren't already marked as
	// terminated get flipped to StateDeleted so they stop appearing in active
	// queries and the slot accounting is correct.
	return r.db.Model(&models.Terminal{}).
		Where("expires_at < NOW() AND state NOT IN ?", []models.TerminalState{models.StateDeleted, models.StateStopped}).
		Update("state", models.StateDeleted).Error
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
				"deleted_at":           nil,
				"state":                terminal.State,
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

	// "Alive" sessions are those whose State indicates they're still consumable.
	// We don't include StateStopped because those are intentionally paused —
	// they shouldn't be treated as orphans even if tt-backend doesn't list
	// them on a /sessions sweep. The "paused" entry is an explicit TerminalState
	// cast on a raw string because the literal is not part of the canonical
	// TerminalState enum (legacy tt-backend value).
	aliveStates := []models.TerminalState{
		models.StateRunning,
		models.StateResuming,
		models.StateHibernating,
		models.TerminalState("paused"),
	}

	if len(apiSessionIDs) == 0 {
		// Si aucune session côté API, toutes les sessions locales vivantes sont orphelines
		result := tr.db.Preload("UserTerminalKey").Where(
			"state IN (?)", aliveStates,
		).Find(&orphanedSessions)

		if result.Error != nil {
			return nil, result.Error
		}
	} else {
		// Sessions vivantes qui ne sont pas dans la liste API
		result := tr.db.Preload("UserTerminalKey").Where(
			"state IN (?) AND session_id NOT IN (?)",
			aliveStates,
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

	// Compter par state (SSOT)
	var counts []struct {
		State string
		Count int
	}

	query := tr.db.Model(&models.Terminal{}).
		Select("state, COUNT(*) as count").
		Group("state")

	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	result := query.Scan(&counts)
	if result.Error != nil {
		return nil, result.Error
	}

	for _, count := range counts {
		stats[count.State] = count.Count
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


