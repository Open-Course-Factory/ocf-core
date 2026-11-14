package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ParticipantNetworkCut is a sticky internet cut a class manager puts on a
// participant (#580): while it stands, every session composed for the
// participant in OrganizationID (the class's organization at the time of the
// cut) starts without internet. One row per user and class; it leaves with
// the class membership.
type ParticipantNetworkCut struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id" mapstructure:"id"`
	UserID         string     `gorm:"not null;uniqueIndex:idx_participant_network_cuts_user_group,priority:1" json:"user_id" mapstructure:"user_id"`
	GroupID        uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_participant_network_cuts_user_group,priority:2" json:"group_id" mapstructure:"group_id"`
	OrganizationID *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty" mapstructure:"organization_id"`
	CutBy          string     `json:"cut_by" mapstructure:"cut_by"`
	CreatedAt      time.Time  `json:"created_at" mapstructure:"created_at"`
}

// BeforeCreate generates the ID when the caller did not set one.
func (c *ParticipantNetworkCut) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// UpsertNetworkCut records (or renews) userID's sticky cut in the class
// groupID of the organization orgID, by cutBy. One row per user and class.
func UpsertNetworkCut(db *gorm.DB, userID string, groupID uuid.UUID, orgID *uuid.UUID, cutBy string) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "group_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"organization_id", "cut_by"}),
	}).Create(&ParticipantNetworkCut{UserID: userID, GroupID: groupID, OrganizationID: orgID, CutBy: cutBy, CreatedAt: time.Now()}).Error
}

// DeleteNetworkCut lifts userID's sticky cut in the class groupID.
func DeleteNetworkCut(db *gorm.DB, userID string, groupID uuid.UUID) error {
	return db.Where("user_id = ? AND group_id = ?", userID, groupID).Delete(&ParticipantNetworkCut{}).Error
}

// liveNetworkCuts scopes a query on participant_network_cuts (aliased nc) to
// userID's cuts that still stand: in a live class (not deleted, not archived)
// the user is an active member of. A cut nobody can lift any more cuts
// nothing.
func liveNetworkCuts(db *gorm.DB, userID string) *gorm.DB {
	return db.Table("participant_network_cuts nc").
		Joins(`JOIN class_groups ncg ON ncg.id = nc.group_id
			AND ncg.deleted_at IS NULL AND ncg.archived_at IS NULL`).
		Joins(`JOIN group_members ngm ON ngm.group_id = nc.group_id AND ngm.user_id = nc.user_id
			AND ngm.is_active = true AND ngm.deleted_at IS NULL`).
		Where("nc.user_id = ?", userID)
}

// IsNetworkCutIn reports whether userID's internet is cut in the organization
// orgID: a standing cut (liveNetworkCuts) in any of its classes. A nil
// organization is never cut.
func IsNetworkCutIn(db *gorm.DB, userID string, orgID *uuid.UUID) (bool, error) {
	if orgID == nil {
		return false, nil
	}
	var n int64
	err := liveNetworkCuts(db, userID).
		Where("nc.organization_id = ?", *orgID).
		Count(&n).Error
	return n > 0, err
}

// NetworkCutInClass returns which of userIDs are cut in the class groupID.
// One query.
func NetworkCutInClass(db *gorm.DB, userIDs []string, groupID uuid.UUID) (map[string]bool, error) {
	cut := make(map[string]bool)
	if len(userIDs) == 0 {
		return cut, nil
	}
	var ids []string
	if err := db.Model(&ParticipantNetworkCut{}).
		Where("user_id IN ? AND group_id = ?", userIDs, groupID).
		Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		cut[id] = true
	}
	return cut, nil
}

// ScenarioNetworkCut reports whether a standing cut (liveNetworkCuts) of
// userID's internet applies where the scenario scenarioID is assigned to
// them, with the reach of ScenarioBlockRefusal(openOnly=false): a cut in an
// assigning class, or in
// the organization that owns it, among the classes the user is an active
// member of; or a cut in the organization that assigned the scenario
// org-wide, when the user is an active member of it.
//
// It reads the scenarios tables by name: this package cannot import theirs.
func ScenarioNetworkCut(db *gorm.DB, userID string, scenarioID uuid.UUID) (bool, error) {
	var n int64
	err := liveNetworkCuts(db, userID).
		Joins(`JOIN scenario_assignments sa ON sa.scenario_id = ? AND sa.deleted_at IS NULL AND sa.is_active = true`, scenarioID).
		Joins(`LEFT JOIN class_groups cg ON sa.scope = 'group' AND cg.id = sa.group_id
			AND cg.deleted_at IS NULL AND cg.archived_at IS NULL`).
		Joins(`LEFT JOIN group_members gm ON gm.group_id = cg.id AND gm.user_id = nc.user_id
			AND gm.is_active = true AND gm.deleted_at IS NULL`).
		Joins(`LEFT JOIN organization_members om ON sa.scope = 'org' AND om.organization_id = sa.organization_id
			AND om.user_id = nc.user_id AND om.is_active = true AND om.deleted_at IS NULL`).
		Where(`(gm.id IS NOT NULL AND (nc.group_id = sa.group_id OR nc.organization_id = cg.organization_id))
			OR (om.id IS NOT NULL AND nc.organization_id = sa.organization_id)`).
		Count(&n).Error
	return n > 0, err
}
