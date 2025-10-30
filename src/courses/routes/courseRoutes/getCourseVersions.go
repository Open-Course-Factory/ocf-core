package courseController

import (
	"net/http"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/courses/dto"

	"github.com/gin-gonic/gin"
)

// GetCourseVersions godoc
// @Summary Get all versions of a course
// @Description Retrieves all versions of a course by its name for the authenticated user
// @Tags courses
// @Accept json
// @Produce json
// @Param name query string true "Course name"
// @Success 200 {array} dto.CourseOutput "List of course versions (ordered newest to oldest)"
// @Failure 400 {object} map[string]string "Missing or invalid parameters"
// @Failure 404 {object} map[string]string "Course not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Security BearerAuth
// @Router /courses/versions [get]
func (cc *courseController) GetCourseVersions(ctx *gin.Context) {
	// Get course name from query params
	courseName := ctx.Query("name")
	if courseName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Course name is required",
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

	userIdStr, ok := userId.(string)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "Invalid user ID format",
		})
		return
	}

	// Retrieve all versions from service
	courses, err := cc.service.GetAllVersionsOfCourse(userIdStr, courseName)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to retrieve course versions",
			"details": err.Error(),
		})
		return
	}

	// Convert to DTOs
	var courseDtos []*dto.CourseOutput
	for _, course := range courses {
		courseDto := dto.CourseModelToCourseOutputDto(*course)
		courseDtos = append(courseDtos, courseDto)
	}

	// Filter courses based on permissions
	var filteredDtos []*dto.CourseOutput
	for _, courseDto := range courseDtos {
		hasAccess, err := casdoor.Enforcer.Enforce(userIdStr, "/api/v1/courses/"+courseDto.ID, "GET")
		if err != nil || !hasAccess {
			// Skip courses the user doesn't have access to
			continue
		}
		filteredDtos = append(filteredDtos, courseDto)
	}

	ctx.JSON(http.StatusOK, filteredDtos)
}
