package scenarioController

import (
	"io"
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// LocalMachineBootController serves the platform's boot defaults for
// machines in the learner's browser to its administrators.
type LocalMachineBootController struct {
	db *gorm.DB
}

func NewLocalMachineBootController(db *gorm.DB) *LocalMachineBootController {
	return &LocalMachineBootController{db: db}
}

// GetBootDefaults godoc
//
//	@Summary		Boot defaults of machines in the browser
//	@Description	How unkernel boots a machine in the learner's browser unless its scenario says otherwise: every key set, the built-in values where the administrator stored none (platform administrators).
//	@Tags			local-machines
//	@Produce		json
//	@Success		200	{object}	models.LocalMachineBoot
//	@Failure		403	{object}	errors.APIError
//	@Router			/local-machines/boot-defaults [get]
func (c *LocalMachineBootController) GetBootDefaults(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, services.LocalMachineBootDefaults(c.db))
}

// PutBootDefaults godoc
//
//	@Summary		Replace the boot defaults of machines in the browser
//	@Description	Stores the keys given; a key left out follows the built-in value. An unknown key, or an append longer than 512 characters or holding a control character, is refused (platform administrators).
//	@Tags			local-machines
//	@Accept			json
//	@Produce		json
//	@Param			boot	body		models.LocalMachineBoot	true	"Boot defaults"
//	@Success		200		{object}	models.LocalMachineBoot
//	@Failure		400		{object}	errors.APIError
//	@Failure		403		{object}	errors.APIError
//	@Router			/local-machines/boot-defaults [put]
func (c *LocalMachineBootController) PutBootDefaults(ctx *gin.Context) {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, err.Error())
		return
	}
	boot, err := models.ParseLocalMachineBoot(string(body))
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := services.SaveLocalMachineBootDefaults(c.db, boot); err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, services.LocalMachineBootDefaults(c.db))
}
