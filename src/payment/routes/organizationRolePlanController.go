package paymentController

import (
	"net/http"
	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ==========================================
// Organization Role Plan Controller
// ==========================================

type OrganizationRolePlanController interface {
	GetOrganizationRolePlans(ctx *gin.Context)
}

type organizationRolePlanController struct {
	controller.GenericController
	orgSubRepo        repositories.OrganizationSubscriptionRepository
	conversionService services.ConversionService
}

func NewOrganizationRolePlanController(db *gorm.DB) OrganizationRolePlanController {
	return &organizationRolePlanController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		orgSubRepo:        repositories.NewOrganizationSubscriptionRepository(db),
		conversionService: services.NewConversionService(),
	}
}

// Get Organization Role Plans godoc
//
//	@Summary		Récupérer les plans par rôle d'une organisation
//	@Description	Retourne les mappings rôle→plan de l'organisation (réservé aux managers et propriétaires)
//	@Tags			organization-role-plans
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"Organization ID"
//	@Security		Bearer
//	@Success		200	{array}		dto.OrganizationRolePlanOutput
//	@Failure		400	{object}	errors.APIError	"Invalid organization ID"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/organizations/{id}/role-plans [get]
func (c *organizationRolePlanController) GetOrganizationRolePlans(ctx *gin.Context) {
	// Layer 2 (OrgRole, manager+) has already authorized the caller for this
	// organization before the handler runs, so the :id param is trusted here.
	orgID := ctx.Param("id")

	parsedID, parseErr := uuid.Parse(orgID)
	if parseErr != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID format",
		})
		return
	}

	rolePlans, err := c.orgSubRepo.GetOrganizationRolePlans(parsedID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	rolePlansDTO, err := c.conversionService.OrganizationRolePlansToDTO(rolePlans)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to convert organization role plans",
		})
		return
	}

	ctx.JSON(http.StatusOK, rolePlansDTO)
}
