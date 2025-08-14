// src/courses/routes/courseRoutes/getCourseUrl.go
package courseController

// GetCourseUrl godoc
//
//	@Summary		Récupération de l'URL du cours généré
//	@Description	Récupère l'URL d'accès au cours généré dans le stockage
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID du cours"
//
//	@Security		Bearer
//
//	@Success		200	{object}	map[string]string{"url": "http://..."}
//
//	@Failure		404	{object}	errors.APIError	"Cours non trouvé ou non généré"
//	@Failure		500	{object}	errors.APIError	"Erreur lors de la récupération de l'URL"
//
//	@Router			/courses/{id}/url [get]
// func (c courseController) GetCourseUrl(ctx *gin.Context) {
// 	courseID := ctx.Param("id")

// 	url, err := c.service.GetCourseURL(courseID)
// 	if err != nil {
// 		ctx.JSON(http.StatusNotFound, &errors.APIError{
// 			ErrorCode:    http.StatusNotFound,
// 			ErrorMessage: "Course not found or not generated: " + err.Error(),
// 		})
// 		return
// 	}

// 	ctx.JSON(http.StatusOK, gin.H{
// 		"url": url,
// 	})
// }
