package routes

import (
	"net/http"
	config "soli/formations/src/configuration"
	"soli/formations/src/organizations/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type migrationController struct {
	migrationService services.OrganizationMigrationService
}

func newMigrationController(db *gorm.DB) *migrationController {
	return &migrationController{
		migrationService: services.NewOrganizationMigrationService(db),
	}
}

// MigratePersonalOrganizations godoc
//
//	@Summary		Migrate existing users to personal organizations
//	@Description	Creates personal organizations for all users who don't have one. This is a one-time migration endpoint.
//	@Tags			migrations
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	services.MigrationResult
//	@Failure		500	{object}	map[string]any
//	@Router			/migrations/personal-organizations [post]
//	@Security		Bearer
func (mc *migrationController) MigratePersonalOrganizations(ctx *gin.Context) {
	result, err := mc.migrationService.MigrateExistingUsersToPersonalOrganizations()

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Migration failed",
			"message": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Migration completed successfully",
		"result":  result,
	})
}

// OrganizationMigrationRoutes sets up the migration routes
func OrganizationMigrationRoutes(rg *gin.RouterGroup, conf *config.Configuration, db *gorm.DB) {
	migrationCtrl := newMigrationController(db)

	migrations := rg.Group("/migrations")
	{
		migrations.POST("/personal-organizations", migrationCtrl.MigratePersonalOrganizations)
	}
}
