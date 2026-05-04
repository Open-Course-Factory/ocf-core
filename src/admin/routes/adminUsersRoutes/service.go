package adminUsersRoutes

import (
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

// BuildUserListings turns a slice of Casdoor users into UserListing rows
// by joining each user's id against the active organization_members and
// group_members tables. It is a pure function with no global state — the
// admin-flag computation is delegated to isAdminFn.
//
// The returned slice always has Organizations and Groups initialised to
// empty (non-nil) slices so the JSON serialises to [] rather than null.
func BuildUserListings(users []*casdoorsdk.User, db *gorm.DB, isAdminFn func(string) bool) ([]UserListing, error) {
	out := make([]UserListing, 0, len(users))
	for _, u := range users {
		listing := UserListing{
			ID:            u.Id,
			Username:      u.Name,
			DisplayName:   u.DisplayName,
			Email:         u.Email,
			Avatar:        u.Avatar,
			IsActive:      !u.IsForbidden && !u.IsDeleted,
			IsAdmin:       isAdminFn(u.Id),
			Organizations: []OrgMembership{},
			Groups:        []GroupMembership{},
		}

		// Org memberships — only active rows.
		var orgRows []struct {
			OrgID string
			Name  string
			Role  string
		}
		if err := db.Table("organization_members AS om").
			Select("om.organization_id AS org_id, o.name AS name, om.role AS role").
			Joins("JOIN organizations o ON o.id = om.organization_id").
			Where("om.user_id = ? AND om.is_active = ?", u.Id, true).
			Scan(&orgRows).Error; err != nil {
			return nil, err
		}
		for _, r := range orgRows {
			listing.Organizations = append(listing.Organizations, OrgMembership{
				ID:   r.OrgID,
				Name: r.Name,
				Role: r.Role,
			})
		}

		// Group memberships — only active rows.
		var groupRows []struct {
			GroupID string
			Name    string
			Role    string
		}
		if err := db.Table("group_members AS gm").
			Select("gm.group_id AS group_id, cg.name AS name, gm.role AS role").
			Joins("JOIN class_groups cg ON cg.id = gm.group_id").
			Where("gm.user_id = ? AND gm.is_active = ?", u.Id, true).
			Scan(&groupRows).Error; err != nil {
			return nil, err
		}
		for _, r := range groupRows {
			listing.Groups = append(listing.Groups, GroupMembership{
				ID:   r.GroupID,
				Name: r.Name,
				Role: r.Role,
			})
		}

		out = append(out, listing)
	}
	return out, nil
}
