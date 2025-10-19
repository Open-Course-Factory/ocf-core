package courseController

import (
	"net/http"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

// GetCourseByVersion godoc
// @Summary Get a specific version of a course
// @Description Retrieves a specific version of a course by name and version for the authenticated user
// @Tags courses
// @Accept json
// @Produce json
// @Param name query string true "Course name"
// @Param version query string true "Course version"
// @Success 200 {object} dto.CourseOutput "Course details"
// @Failure 400 {object} map[string]string "Missing or invalid parameters"
// @Failure 404 {object} map[string]string "Course version not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /courses/by-version [get]
func (cc *courseController) GetCourseByVersion(ctx *gin.Context) {
	// Get course name and version from query params
	courseName := ctx.Query("name")
	version := ctx.Query("version")

	if courseName == "" || version == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Both course name and version are required",
		})
		return
	}

	// Get authenticated user ID
	userId, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not authenticated",
		})
		return
	}

	userIdStr := userId.(string)

	// Retrieve specific version from service
	course, err := cc.service.GetCourseByVersion(userIdStr, courseName, version)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": "Course version not found",
			"details": err.Error(),
		})
		return
	}

	// Check permissions
	hasAccess, err := casdoor.Enforcer.Enforce(userIdStr, "/api/v1/courses/"+course.ID.String(), "GET")
	if err != nil || !hasAccess {
		ctx.JSON(http.StatusForbidden, gin.H{
			"error": "You don't have access to this course",
		})
		return
	}

	// Convert to DTO
	courseDto := dto.CourseModelToCourseOutputDto(*course)

	ctx.JSON(http.StatusOK, courseDto)
}
