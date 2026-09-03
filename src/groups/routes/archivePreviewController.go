package groupRoutes

import (
	"net/http"
	"time"

	access "soli/formations/src/auth/access"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/groups/dto"
	"soli/formations/src/groups/models"
	"soli/formations/src/groups/services"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListCasdoorUsers is the one Casdoor call the preview makes: a single bulk
// load, indexed by id, so a roster of N members never costs N round-trips.
// A variable so tests can stand in for Casdoor.
var ListCasdoorUsers = casdoorsdk.GetUsers

// ArchivePreviewController answers GET /class-groups/:id/archive-preview: the
// roster of a class about to be archived, each member with the count of their
// OTHER open classes in the same organization and their organization standing.
type ArchivePreviewController struct {
	db           *gorm.DB
	groupService services.GroupService
}

func NewArchivePreviewController(db *gorm.DB) *ArchivePreviewController {
	return &ArchivePreviewController{db: db, groupService: services.NewGroupService(db)}
}

// previewRow is one line of the preview query; the identity fields are filled
// from Casdoor afterwards.
type previewRow struct {
	UserID                  string
	Role                    string
	OtherActiveClassesInOrg int
	OrgMemberActive         *bool
	OrgMemberLeftAt         *time.Time
}

// Preview godoc
// @Summary Preview archiving a class
// @Description Lists the members of a class with, for each, how many other open classes of the same organization they belong to and their organization membership state
// @Tags class-groups
// @Produce json
// @Param id path string true "Class group ID"
// @Success 200 {object} dto.ArchivePreviewOutput
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /class-groups/{id}/archive-preview [get]
// @Security BearerAuth
func (c *ArchivePreviewController) Preview(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid class id"})
		return
	}
	var group models.ClassGroup
	if err := c.db.First(&group, "id = ?", groupID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "class not found"})
		return
	}

	// The route is SelfScoped at Layer 2; the authority is the same one that
	// gates the archive action itself.
	if !c.callerMayArchive(ctx, groupID) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "you may not archive this class"})
		return
	}

	organization, err := c.organizationOf(group)
	if err != nil {
		utils.Error("archive preview of class %s failed: %v", groupID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build the archive preview"})
		return
	}
	rows, err := c.rosterWithContinuation(group)
	if err != nil {
		utils.Error("archive preview of class %s failed: %v", groupID, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build the archive preview"})
		return
	}
	ctx.JSON(http.StatusOK, dto.ArchivePreviewOutput{
		RetentionDays: organization.EffectiveRetentionDays(),
		Members:       c.identify(rows, group.OrganizationID != nil),
	})
}

// organizationOf loads the class's organization; a personal class yields a zero
// Organization, whose EffectiveRetentionDays is the platform default.
func (c *ArchivePreviewController) organizationOf(group models.ClassGroup) (orgModels.Organization, error) {
	var organization orgModels.Organization
	if group.OrganizationID == nil {
		return organization, nil
	}
	return organization, c.db.First(&organization, "id = ?", *group.OrganizationID).Error
}

func (c *ArchivePreviewController) callerMayArchive(ctx *gin.Context, groupID uuid.UUID) bool {
	if access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		return true
	}
	canManage, err := c.groupService.CanUserManageGroup(groupID, ctx.GetString("userId"))
	return err == nil && canManage
}

// rosterWithContinuation is ONE query: the active roster, a correlated count of
// each member's other open classes in the class's organization, and their
// organization membership row when the class has one.
func (c *ArchivePreviewController) rosterWithContinuation(group models.ClassGroup) ([]previewRow, error) {
	otherClasses := c.db.Table("group_members AS other").
		Select("COUNT(*)").
		Joins("JOIN class_groups other_class ON other_class.id = other.group_id AND other_class.deleted_at IS NULL").
		Where("other.user_id = gm.user_id AND other.group_id <> gm.group_id AND other.is_active = ? AND other.deleted_at IS NULL", true).
		Scopes(entityManagementModels.NotArchived("other_class"))
	if group.OrganizationID != nil {
		otherClasses = otherClasses.Where("other_class.organization_id = ?", *group.OrganizationID)
	} else {
		otherClasses = otherClasses.Where("other_class.organization_id IS NULL")
	}

	query := c.db.Table("group_members AS gm").
		Select("gm.user_id, gm.role, (?) AS other_active_classes_in_org, om.is_active AS org_member_active, om.left_at AS org_member_left_at", otherClasses).
		Where("gm.group_id = ? AND gm.is_active = ? AND gm.deleted_at IS NULL", group.ID, true).
		Order("gm.role, gm.user_id")
	if group.OrganizationID != nil {
		query = query.Joins("LEFT JOIN organization_members om ON om.user_id = gm.user_id AND om.organization_id = ? AND om.deleted_at IS NULL", *group.OrganizationID)
	} else {
		query = query.Joins("LEFT JOIN organization_members om ON 1 = 0")
	}

	var rows []previewRow
	return rows, query.Scan(&rows).Error
}

// identify attaches email and display name from Casdoor. A Casdoor failure
// degrades the preview to ids rather than refusing it: the continuation counts
// are the decision-making part.
func (c *ArchivePreviewController) identify(rows []previewRow, hasOrganization bool) []dto.ArchivePreviewMember {
	users := map[string]*casdoorsdk.User{}
	if all, err := ListCasdoorUsers(); err != nil {
		utils.Warn("archive preview: could not load users from Casdoor: %v", err)
	} else {
		for _, u := range all {
			users[u.Id] = u
		}
	}

	members := make([]dto.ArchivePreviewMember, 0, len(rows))
	for _, row := range rows {
		member := dto.ArchivePreviewMember{
			UserID:                  row.UserID,
			Role:                    row.Role,
			OtherActiveClassesInOrg: row.OtherActiveClassesInOrg,
			OrgMemberState:          orgMemberState(row, hasOrganization),
		}
		if u := users[row.UserID]; u != nil {
			member.Email = u.Email
			member.DisplayName = u.DisplayName
			if member.DisplayName == "" {
				member.DisplayName = u.Name
			}
		}
		members = append(members, member)
	}
	return members
}

// orgMemberState names the three standings a roster member can have in the
// class's organization. The offboarding work (#492) adds "offboarded" here.
// orgMemberState mirrors OrganizationMember.IsOffboarded on the scanned row: an
// offboarded membership is also stood down, so left_at is checked before is_active.
func orgMemberState(row previewRow, hasOrganization bool) string {
	if !hasOrganization || row.OrgMemberActive == nil {
		return "none"
	}
	if row.OrgMemberLeftAt != nil {
		return "offboarded"
	}
	if *row.OrgMemberActive {
		return "active"
	}
	return "removed"
}
