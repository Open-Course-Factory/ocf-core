package scenarioController

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/services"
)

// TeacherController handles teacher-facing dashboard endpoints
type TeacherController struct {
	dashboardService *services.TeacherDashboardService
	sessionService   *services.ScenarioSessionService
	db               *gorm.DB
}

// NewTeacherController creates a new teacher controller
func NewTeacherController(db *gorm.DB) *TeacherController {
	flagService := services.NewFlagService()
	verificationService := services.NewVerificationService()
	return &TeacherController{
		dashboardService: services.NewTeacherDashboardService(db),
		sessionService:   services.NewScenarioSessionService(db, flagService, verificationService),
		db:               db,
	}
}

// validateTeacherAccess checks that the current user is an admin or a group owner/admin
func (tc *TeacherController) validateTeacherAccess(c *gin.Context, groupID uuid.UUID) bool {
	userID := c.GetString("userId")
	userRoles, _ := c.Get("userRoles")

	// Platform admins have access
	if roles, ok := userRoles.([]string); ok {
		for _, role := range roles {
			if role == "admin" || role == "administrator" {
				return true
			}
		}
	}

	// Check group-level ownership/admin
	var member groupModels.GroupMember
	err := tc.db.Where("group_id = ? AND user_id = ? AND is_active = true AND role IN ?",
		groupID, userID, []string{"owner", "admin"}).First(&member).Error
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not authorized as group teacher"})
		return false
	}
	return true
}

// GetGroupActivity returns active sessions for all members of a group
func (tc *TeacherController) GetGroupActivity(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	results, err := tc.dashboardService.GetGroupActivity(groupID)
	if err != nil {
		slog.Error("failed to get group activity", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get group activity"})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetScenarioResults returns all sessions for a specific scenario within a group
func (tc *TeacherController) GetScenarioResults(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	scenarioID, err := uuid.Parse(c.Param("scenarioId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	results, err := tc.dashboardService.GetScenarioResults(groupID, scenarioID)
	if err != nil {
		slog.Error("failed to get scenario results", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get scenario results"})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetScenarioAnalytics returns aggregate analytics for a scenario within a group
func (tc *TeacherController) GetScenarioAnalytics(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	scenarioID, err := uuid.Parse(c.Param("scenarioId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	analytics, err := tc.dashboardService.GetScenarioAnalytics(groupID, scenarioID)
	if err != nil {
		slog.Error("failed to get scenario analytics", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get scenario analytics"})
		return
	}

	c.JSON(http.StatusOK, analytics)
}

// BulkStartScenario starts a scenario for all active group members who don't already have an active session
func (tc *TeacherController) BulkStartScenario(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group ID"})
		return
	}

	scenarioID, err := uuid.Parse(c.Param("scenarioId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scenario ID"})
		return
	}

	if !tc.validateTeacherAccess(c, groupID) {
		return
	}

	// Load active group members
	var members []groupModels.GroupMember
	if err := tc.db.Where("group_id = ? AND is_active = true", groupID).Find(&members).Error; err != nil {
		slog.Error("failed to load group members", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load group members"})
		return
	}

	started := 0
	skipped := 0
	failed := 0

	for _, member := range members {
		_, err := tc.sessionService.StartScenario(member.UserID, scenarioID, "")
		if err != nil {
			// If active session already exists, count as skipped
			if err.Error() == "active session already exists for this scenario" {
				skipped++
			} else {
				failed++
				slog.Error("failed to start scenario for member", "userID", member.UserID, "err", err)
			}
		} else {
			started++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"started": started,
		"skipped": skipped,
		"failed":  failed,
	})
}
