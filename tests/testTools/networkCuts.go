package test_tools

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"soli/formations/src/terminalTrainer/models"
)

// CreateParticipantNetworkCutsTable creates participant_network_cuts, the
// sticky internet cut a class manager puts on a participant (#580), with the
// production schema: the model's AutoMigrate (one row per user_id, group_id).
func CreateParticipantNetworkCutsTable(db *gorm.DB) error {
	return db.AutoMigrate(&models.ParticipantNetworkCut{})
}

// SeedNetworkCut records a sticky internet cut on userID in the class groupID
// of the organization orgID (nil: a class without organization).
func SeedNetworkCut(db *gorm.DB, userID string, groupID uuid.UUID, orgID *uuid.UUID, cutBy string) error {
	var org any
	if orgID != nil {
		org = orgID.String()
	}
	return db.Exec(
		`INSERT INTO participant_network_cuts (id, user_id, group_id, organization_id, cut_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), userID, groupID.String(), org, cutBy, time.Now(),
	).Error
}

// NetworkCutRow is one participant_network_cuts row, read with raw SQL.
type NetworkCutRow struct {
	UserID         string
	GroupID        string
	OrganizationID *string
	CutBy          string
}

// NetworkCutsOf returns userID's sticky cuts, whatever the class.
func NetworkCutsOf(db *gorm.DB, userID string) ([]NetworkCutRow, error) {
	var rows []NetworkCutRow
	err := db.Raw(`SELECT user_id, group_id, organization_id, cut_by FROM participant_network_cuts WHERE user_id = ?`, userID).
		Scan(&rows).Error
	return rows, err
}

// CountNetworkCuts returns how many sticky cuts userID has, whatever the class.
func CountNetworkCuts(db *gorm.DB, userID string) int64 {
	var n int64
	db.Table("participant_network_cuts").Where("user_id = ?", userID).Count(&n)
	return n
}
