package scenarioController

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"
	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/scenarios/dto"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	"soli/formations/src/scenarios/utils"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScenarioController defines the handler methods for scenario-related endpoints
type ScenarioController interface {
	ImportScenario(ctx *gin.Context)
	SeedScenario(ctx *gin.Context)
	UploadScenario(ctx *gin.Context)
	GetSessionByTerminal(ctx *gin.Context)
	GetSessionInfo(ctx *gin.Context)
	ExportScenario(ctx *gin.Context)
	ExportScenarios(ctx *gin.Context)
	ImportJSON(ctx *gin.Context)
	GroupExportScenario(ctx *gin.Context)
	GroupImportJSON(ctx *gin.Context)
	GroupUploadScenario(ctx *gin.Context)
	GroupCreateScenario(ctx *gin.Context)
	OrgListScenarios(ctx *gin.Context)
	OrgImportJSON(ctx *gin.Context)
	OrgUploadScenario(ctx *gin.Context)
	OrgExportScenario(ctx *gin.Context)
	OrgCreateScenario(ctx *gin.Context)
	OrgDeleteScenario(ctx *gin.Context)
	ListGroupAvailableScenarios(ctx *gin.Context)
	DuplicateScenario(ctx *gin.Context)
	OrgDuplicateScenario(ctx *gin.Context)
}

type scenarioController struct {
	scenarioControllerBase
	sessionService   *services.ScenarioSessionService
	importerService  *services.ScenarioImporterService
	exportService    *services.ScenarioExportService
	seedService      *services.ScenarioSeedService
	duplicateService *services.ScenarioDuplicateService
	terminalService  terminalServices.TerminalTrainerService
	groupService     groupServices.GroupService
}

// NewScenarioController creates a new scenario controller with its service dependencies
func NewScenarioController(db *gorm.DB) ScenarioController {
	flagService := services.NewFlagService()
	verificationService := services.NewVerificationService()
	sessionService := services.NewScenarioSessionService(db, flagService, verificationService)
	importerService := services.NewScenarioImporterService(db)
	exportService := services.NewScenarioExportService(db)
	seedService := services.NewScenarioSeedService(db)
	duplicateService := services.NewScenarioDuplicateService(db)
	terminalService := terminalServices.NewTerminalTrainerService(db)

	// Wire terminal stop callback so the session service can stop terminals on setup failure
	sessionService.SetTerminalStopFunc(func(terminalSessionID string) error {
		return terminalService.StopSession(terminalSessionID)
	})

	return &scenarioController{
		scenarioControllerBase: scenarioControllerBase{db: db},
		sessionService:         sessionService,
		importerService:        importerService,
		exportService:          exportService,
		seedService:            seedService,
		duplicateService:       duplicateService,
		terminalService:        terminalService,
		groupService:           groupServices.NewGroupService(db),
	}
}

// ImportScenario godoc
// @Summary Import a scenario from git
// @Description Import a KillerCoda-compatible scenario from a git repository (not yet implemented)
// @Tags scenarios
// @Accept json
// @Produce json
// @Param body body dto.ImportScenarioInput true "Import request"
// @Success 201 {object} models.Scenario
// @Failure 400 {object} errors.APIError
// @Failure 501 {object} errors.APIError
// @Router /scenarios/import [post]
// @Security BearerAuth
func (sc *scenarioController) ImportScenario(ctx *gin.Context) {
	var input dto.ImportScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusNotImplemented, &errors.APIError{
		ErrorCode:    http.StatusNotImplemented,
		ErrorMessage: "Git import not yet implemented. Use directory import via admin API.",
	})
}

// GetSessionByTerminal godoc
// @Summary Get scenario session by terminal
// @Description Find the most recent scenario session linked to a terminal session
// @Tags scenario-sessions
// @Produce json
// @Param terminalId path string true "Terminal session ID"
// @Success 200 {object} dto.ScenarioSessionOutput
// @Failure 400 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenario-sessions/by-terminal/{terminalId} [get]
// @Security BearerAuth
func (sc *scenarioController) GetSessionByTerminal(ctx *gin.Context) {
	terminalID := ctx.Param("terminalId")
	if terminalID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Terminal session ID is required",
		})
		return
	}

	var session models.ScenarioSession
	err := sc.db.Where("terminal_session_id = ?", terminalID).Order("created_at DESC").First(&session).Error
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No scenario session for this terminal",
		})
		return
	}

	userID := ctx.GetString("userId")
	if session.UserID != userID {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You do not own this session",
		})
		return
	}

	terminalSessionID := ""
	if session.TerminalSessionID != nil {
		terminalSessionID = *session.TerminalSessionID
	}
	ctx.JSON(http.StatusOK, dto.SessionResponse{
		ID:                session.ID.String(),
		ScenarioID:        session.ScenarioID.String(),
		UserID:            session.UserID,
		TrainerID:         session.TrainerID,
		TerminalSessionID: terminalSessionID,
		CurrentStep:       session.CurrentStep,
		Status:            session.Status,
		StartedAt:         session.StartedAt,
	})
}

// GetSessionInfo godoc
// @Summary Get session info
// @Description Get session info for the authenticated user (ownership check)
// @Tags scenario-sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} dto.SessionResponse
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenario-sessions/{id}/info [get]
// @Security BearerAuth
func (sc *scenarioController) GetSessionInfo(ctx *gin.Context) {
	session, err := sc.getSessionIfOwned(ctx)
	if err != nil {
		return
	}

	terminalSessionID := ""
	if session.TerminalSessionID != nil {
		terminalSessionID = *session.TerminalSessionID
	}
	ctx.JSON(http.StatusOK, dto.SessionResponse{
		ID:                session.ID.String(),
		ScenarioID:        session.ScenarioID.String(),
		UserID:            session.UserID,
		TrainerID:         session.TrainerID,
		TerminalSessionID: terminalSessionID,
		CurrentStep:       session.CurrentStep,
		Status:            session.Status,
		ProvisioningPhase: session.ProvisioningPhase,
		Grade:             session.Grade,
		StartedAt:         session.StartedAt,
	})
}

// SeedScenario godoc
// @Summary Seed a scenario with steps
// @Description Create a scenario with all steps from a single JSON payload (admin/testing)
// @Tags scenarios
// @Accept json
// @Produce json
// @Param body body dto.SeedScenarioInput true "Seed request"
// @Success 201 {object} models.Scenario
// @Failure 400 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/seed [post]
// @Security BearerAuth
func (sc *scenarioController) SeedScenario(ctx *gin.Context) {
	var input dto.SeedScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	userID := ctx.GetString("userId")

	scenario, isUpdate, err := sc.seedService.SeedScenario(input, userID, nil)
	if err != nil {
		slog.Error("failed to seed scenario", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to seed scenario",
		})
		return
	}

	statusCode := http.StatusCreated
	if isUpdate {
		statusCode = http.StatusOK
	}

	ctx.JSON(statusCode, sc.buildScenarioOutput(scenario))
}

// UploadScenario godoc
// @Summary Upload a scenario archive
// @Description Upload a .zip or .tar.gz archive containing a KillerCoda-compatible scenario directory and import it
// @Tags scenarios
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Scenario archive (.zip, .tar.gz, or .tgz)"
// @Success 200 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/upload [post]
// @Security BearerAuth
func (sc *scenarioController) UploadScenario(ctx *gin.Context) {
	userID := ctx.GetString("userId")

	// Get file from multipart form
	file, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File is required",
		})
		return
	}

	// Validate file size (10MB max)
	if file.Size > 10*1024*1024 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File size exceeds 10MB limit",
		})
		return
	}

	// Validate extension
	filename := strings.ToLower(file.Filename)
	var ext string
	switch {
	case strings.HasSuffix(filename, ".tar.gz"):
		ext = ".tar.gz"
	case strings.HasSuffix(filename, ".tgz"):
		ext = ".tgz"
	case strings.HasSuffix(filename, ".zip"):
		ext = ".zip"
	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File must be .zip, .tar.gz, or .tgz",
		})
		return
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "scenario-upload-*"+ext)
	if err != nil {
		slog.Error("failed to create temp file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.Remove(tmpFile.Name())

	src, err := file.Open()
	if err != nil {
		tmpFile.Close()
		slog.Error("failed to open uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to read uploaded file",
		})
		return
	}

	_, err = io.Copy(tmpFile, src)
	src.Close()
	tmpFile.Close()
	if err != nil {
		slog.Error("failed to save uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to save uploaded file",
		})
		return
	}

	// Extract archive
	tmpDir, err := os.MkdirTemp("", "scenario-extract-*")
	if err != nil {
		slog.Error("failed to create temp dir", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := utils.ExtractArchive(tmpFile.Name(), tmpDir); err != nil {
		slog.Error("failed to extract archive", "err", err)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Failed to extract archive: %s", err.Error()),
		})
		return
	}

	// Find index.json
	scenarioDir, err := utils.FindIndexJSON(tmpDir)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Archive must contain an index.json file",
		})
		return
	}

	// Import scenario
	scenario, err := sc.importerService.ImportFromDirectory(scenarioDir, userID, nil, "upload")
	if err != nil {
		slog.Error("failed to import scenario from upload", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to import scenario: %s", err.Error()),
		})
		return
	}

	// Reload with steps
	var loaded models.Scenario
	if err := sc.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to reload scenario",
		})
		return
	}

	ctx.JSON(http.StatusOK, sc.buildScenarioOutput(&loaded))
}

// ExportScenario godoc
// @Summary Export a scenario
// @Description Export a scenario as JSON or KillerCoda archive
// @Tags scenarios
// @Produce json
// @Produce application/zip
// @Param id path string true "Scenario ID"
// @Param format query string false "Export format: json (default) or killerkoda"
// @Success 200 {object} dto.ScenarioExportOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenarios/{id}/export [get]
// @Security BearerAuth
func (sc *scenarioController) ExportScenario(ctx *gin.Context) {
	scenarioID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	// Authorization: admin always allowed; otherwise the caller must be able
	// to manage this scenario (creator, org manager, or group manager of any
	// group it's assigned to). Aligns export with the PATCH/DELETE rule.
	userID := ctx.GetString("userId")
	userRoles, _ := ctx.Get("userRoles")
	roles, _ := userRoles.([]string)
	if !access.IsAdmin(roles) {
		var scenario models.Scenario
		if err := sc.db.Where("id = ?", scenarioID).First(&scenario).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				ctx.JSON(http.StatusNotFound, &errors.APIError{
					ErrorCode:    http.StatusNotFound,
					ErrorMessage: "Scenario not found",
				})
				return
			}
			slog.Error("failed to load scenario for export auth check", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Internal error",
			})
			return
		}
		allowed, err := scenarioHooks.CanManageScenario(sc.db, sc.groupService, &scenario, userID)
		if err != nil {
			slog.Error("failed to check scenario manage permission", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Internal error",
			})
			return
		}
		if !allowed {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied",
			})
			return
		}
	}

	format := ctx.DefaultQuery("format", "json")

	switch format {
	case "json":
		export, err := sc.exportService.ExportAsJSON(scenarioID)
		if err != nil {
			slog.Error("failed to export scenario as JSON", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.JSON(http.StatusOK, export)

	case "killerkoda":
		zipBytes, filename, err := sc.exportService.ExportAsArchive(scenarioID)
		if err != nil {
			slog.Error("failed to export scenario as archive", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		ctx.Data(http.StatusOK, "application/zip", zipBytes)

	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid format. Use 'json' or 'killerkoda'",
		})
	}
}

// ExportScenarios godoc
// @Summary Export multiple scenarios
// @Description Export multiple scenarios as JSON array
// @Tags scenarios
// @Accept json
// @Produce json
// @Param body body dto.ExportScenariosInput true "Scenario IDs to export"
// @Success 200 {array} dto.ScenarioExportOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Router /scenarios/export [post]
// @Security BearerAuth
func (sc *scenarioController) ExportScenarios(ctx *gin.Context) {
	var input dto.ExportScenariosInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Authorization: admin always allowed; otherwise every scenario in the
	// list must be manageable by the caller. If ANY id is unauthorized the
	// whole request is rejected — avoids partial exports leaking data.
	userID := ctx.GetString("userId")
	userRoles, _ := ctx.Get("userRoles")
	roles, _ := userRoles.([]string)
	if !access.IsAdmin(roles) {
		for _, id := range input.IDs {
			var scenario models.Scenario
			if err := sc.db.Where("id = ?", id).First(&scenario).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					ctx.JSON(http.StatusForbidden, &errors.APIError{
						ErrorCode:    http.StatusForbidden,
						ErrorMessage: "Access denied: not authorized to export one or more scenarios",
					})
					return
				}
				slog.Error("failed to load scenario for bulk export auth check", "err", err)
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Internal error",
				})
				return
			}
			allowed, err := scenarioHooks.CanManageScenario(sc.db, sc.groupService, &scenario, userID)
			if err != nil {
				slog.Error("failed to check scenario manage permission", "err", err)
				ctx.JSON(http.StatusInternalServerError, &errors.APIError{
					ErrorCode:    http.StatusInternalServerError,
					ErrorMessage: "Internal error",
				})
				return
			}
			if !allowed {
				ctx.JSON(http.StatusForbidden, &errors.APIError{
					ErrorCode:    http.StatusForbidden,
					ErrorMessage: "Access denied: not authorized to export one or more scenarios",
				})
				return
			}
		}
	}

	exports, err := sc.exportService.ExportMultipleAsJSON(input.IDs)
	if err != nil {
		slog.Error("failed to export scenarios", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to export scenarios",
		})
		return
	}

	ctx.JSON(http.StatusOK, exports)
}

// ImportJSON godoc
// @Summary Import a scenario from JSON
// @Description Create or update a scenario from a JSON payload (admin)
// @Tags scenarios
// @Accept json
// @Produce json
// @Param body body dto.SeedScenarioInput true "Scenario data"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/import-json [post]
// @Security BearerAuth
func (sc *scenarioController) ImportJSON(ctx *gin.Context) {
	var input dto.SeedScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	userID := ctx.GetString("userId")

	scenario, isUpdate, err := sc.seedService.SeedScenario(input, userID, nil)
	if err != nil {
		slog.Error("failed to import scenario from JSON", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to import scenario",
		})
		return
	}

	statusCode := http.StatusCreated
	if isUpdate {
		statusCode = http.StatusOK
	}
	ctx.JSON(statusCode, sc.buildScenarioOutput(scenario))
}

// GroupExportScenario godoc
// @Summary Export a group scenario
// @Description Export a scenario assigned to a group as JSON or KillerCoda archive
// @Tags scenarios
// @Produce json
// @Produce application/zip
// @Param groupId path string true "Group ID"
// @Param scenarioId path string true "Scenario ID"
// @Param format query string false "Export format: json (default) or killerkoda"
// @Success 200 {object} dto.ScenarioExportOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /groups/{groupId}/scenarios/{scenarioId}/export [get]
// @Security BearerAuth
func (sc *scenarioController) GroupExportScenario(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid group ID",
		})
		return
	}

	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	// Verify scenario is assigned to the group
	var assignment models.ScenarioAssignment
	if err := sc.db.Where("scenario_id = ? AND group_id = ? AND is_active = true",
		scenarioID, groupID).First(&assignment).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario not assigned to this group",
		})
		return
	}

	format := ctx.DefaultQuery("format", "json")

	switch format {
	case "json":
		export, err := sc.exportService.ExportAsJSON(scenarioID)
		if err != nil {
			slog.Error("failed to export group scenario as JSON", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.JSON(http.StatusOK, export)

	case "killerkoda":
		zipBytes, filename, err := sc.exportService.ExportAsArchive(scenarioID)
		if err != nil {
			slog.Error("failed to export group scenario as archive", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		ctx.Data(http.StatusOK, "application/zip", zipBytes)

	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid format. Use 'json' or 'killerkoda'",
		})
	}
}

// GroupImportJSON godoc
// @Summary Import a scenario into a group from JSON
// @Description Create or update a scenario from JSON and assign it to a group
// @Tags scenarios
// @Accept json
// @Produce json
// @Param groupId path string true "Group ID"
// @Param body body dto.SeedScenarioInput true "Scenario data"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /groups/{groupId}/scenarios/import-json [post]
// @Security BearerAuth
func (sc *scenarioController) GroupImportJSON(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid group ID",
		})
		return
	}

	var input dto.SeedScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	userID := ctx.GetString("userId")

	// Get the group's organization ID
	var group groupModels.ClassGroup
	if err := sc.db.First(&group, "id = ?", groupID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		return
	}

	scenario, isUpdate, err := sc.seedService.SeedScenario(input, userID, group.OrganizationID)
	if err != nil {
		slog.Error("failed to import scenario for group", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to import scenario",
		})
		return
	}

	// Auto-create ScenarioAssignment for the group (if not already assigned)
	var existingAssignment models.ScenarioAssignment
	if err := sc.db.Where("scenario_id = ? AND group_id = ?",
		scenario.ID, groupID).First(&existingAssignment).Error; err != nil {
		// No existing assignment, create one
		assignment := models.ScenarioAssignment{
			ScenarioID:  scenario.ID,
			GroupID:     &groupID,
			Scope:       "group",
			CreatedByID: userID,
			IsActive:    true,
		}
		if err := sc.db.Create(&assignment).Error; err != nil {
			slog.Error("failed to create scenario assignment", "err", err)
			// Don't fail the whole request, scenario was already created
		}
	}

	statusCode := http.StatusCreated
	if isUpdate {
		statusCode = http.StatusOK
	}
	ctx.JSON(statusCode, sc.buildScenarioOutput(scenario))
}

// GroupUploadScenario godoc
// @Summary Upload a scenario archive for a group
// @Description Upload a .zip or .tar.gz archive containing a KillerCoda-compatible scenario and assign it to a group
// @Tags scenarios
// @Accept multipart/form-data
// @Produce json
// @Param groupId path string true "Group ID"
// @Param file formData file true "Scenario archive (.zip, .tar.gz, or .tgz)"
// @Success 200 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /groups/{groupId}/scenarios/upload [post]
// @Security BearerAuth
func (sc *scenarioController) GroupUploadScenario(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid group ID",
		})
		return
	}

	userID := ctx.GetString("userId")

	// Get the group's organization ID
	var group groupModels.ClassGroup
	if err := sc.db.First(&group, "id = ?", groupID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		return
	}

	// Get file from multipart form
	file, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File is required",
		})
		return
	}

	// Validate file size (10MB max)
	if file.Size > 10*1024*1024 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File size exceeds 10MB limit",
		})
		return
	}

	// Validate extension
	filename := strings.ToLower(file.Filename)
	var ext string
	switch {
	case strings.HasSuffix(filename, ".tar.gz"):
		ext = ".tar.gz"
	case strings.HasSuffix(filename, ".tgz"):
		ext = ".tgz"
	case strings.HasSuffix(filename, ".zip"):
		ext = ".zip"
	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File must be .zip, .tar.gz, or .tgz",
		})
		return
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "scenario-upload-*"+ext)
	if err != nil {
		slog.Error("failed to create temp file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.Remove(tmpFile.Name())

	src, err := file.Open()
	if err != nil {
		tmpFile.Close()
		slog.Error("failed to open uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to read uploaded file",
		})
		return
	}

	_, err = io.Copy(tmpFile, src)
	src.Close()
	tmpFile.Close()
	if err != nil {
		slog.Error("failed to save uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to save uploaded file",
		})
		return
	}

	// Extract archive
	tmpDir, err := os.MkdirTemp("", "scenario-extract-*")
	if err != nil {
		slog.Error("failed to create temp dir", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := utils.ExtractArchive(tmpFile.Name(), tmpDir); err != nil {
		slog.Error("failed to extract archive", "err", err)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Failed to extract archive: %s", err.Error()),
		})
		return
	}

	// Find index.json
	scenarioDir, err := utils.FindIndexJSON(tmpDir)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Archive must contain an index.json file",
		})
		return
	}

	// Import scenario with org ID from group
	scenario, err := sc.importerService.ImportFromDirectory(scenarioDir, userID, group.OrganizationID, "upload")
	if err != nil {
		slog.Error("failed to import scenario from upload", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to import scenario: %s", err.Error()),
		})
		return
	}

	// Auto-create ScenarioAssignment for the group (if not already assigned)
	var existingAssignment models.ScenarioAssignment
	if err := sc.db.Where("scenario_id = ? AND group_id = ?",
		scenario.ID, groupID).First(&existingAssignment).Error; err != nil {
		assignment := models.ScenarioAssignment{
			ScenarioID:  scenario.ID,
			GroupID:     &groupID,
			Scope:       "group",
			CreatedByID: userID,
			IsActive:    true,
		}
		if err := sc.db.Create(&assignment).Error; err != nil {
			slog.Error("failed to create scenario assignment", "err", err)
		}
	}

	// Reload with steps
	var loaded models.Scenario
	if err := sc.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to reload scenario",
		})
		return
	}

	ctx.JSON(http.StatusOK, sc.buildScenarioOutput(&loaded))
}

// GetAvailableScenarios godoc
// OrgListScenarios godoc
// @Summary List organization scenarios
// @Description List all scenarios belonging to an organization
// @Tags scenarios
// @Produce json
// @Param id path string true "Organization ID"
// @Success 200 {array} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Router /organizations/{id}/scenarios [get]
// @Security BearerAuth
func (sc *scenarioController) OrgListScenarios(ctx *gin.Context) {
	orgID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	// In-handler membership check (defense in depth — do not rely solely on Layer 2).
	// Admins bypass; all other users must be an active member of this org.
	userID := ctx.GetString("userId")
	userRoles, _ := ctx.Get("userRoles")
	roles, _ := userRoles.([]string)
	if !access.IsAdmin(roles) {
		var orgMember orgModels.OrganizationMember
		result := sc.db.Where("organization_id = ? AND user_id = ? AND is_active = ?", orgID, userID, true).First(&orgMember)
		if result.Error != nil {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied",
			})
			return
		}
	}

	var scenarios []models.Scenario
	if err := sc.db.Where("organization_id = ?", orgID).Preload("Steps").Find(&scenarios).Error; err != nil {
		slog.Error("failed to list org scenarios", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to list scenarios",
		})
		return
	}

	output := make([]dto.ScenarioOutput, 0, len(scenarios))
	for i := range scenarios {
		output = append(output, sc.buildScenarioOutput(&scenarios[i]))
	}
	ctx.JSON(http.StatusOK, output)
}

// OrgImportJSON godoc
// @Summary Import a scenario into an organization from JSON
// @Description Create or update a scenario from JSON and assign it to an organization
// @Tags scenarios
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param body body dto.SeedScenarioInput true "Scenario data"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /organizations/{id}/scenarios/import-json [post]
// @Security BearerAuth
func (sc *scenarioController) OrgImportJSON(ctx *gin.Context) {
	orgID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	var input dto.SeedScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	userID := ctx.GetString("userId")

	scenario, isUpdate, err := sc.seedService.SeedScenario(input, userID, &orgID)
	if err != nil {
		slog.Error("failed to import scenario for org", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to import scenario",
		})
		return
	}

	// Do NOT create ScenarioAssignment (unlike GroupImportJSON)

	statusCode := http.StatusCreated
	if isUpdate {
		statusCode = http.StatusOK
	}
	ctx.JSON(statusCode, sc.buildScenarioOutput(scenario))
}

// OrgUploadScenario godoc
// @Summary Upload a scenario archive for an organization
// @Description Upload a .zip or .tar.gz archive containing a KillerCoda-compatible scenario for an organization
// @Tags scenarios
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Organization ID"
// @Param file formData file true "Scenario archive (.zip, .tar.gz, or .tgz)"
// @Success 200 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// OrgCreateScenario creates a blank scenario inside an organization.
//
// Org managers can't POST to the platform-wide /scenarios endpoint
// (admin-only). They use this org-scoped variant; organization_id is
// taken from the path so the body cannot retarget another org.
//
// @Summary Create a scenario in an organization
// @Description Org-scoped scenario creation for managers/owners. organization_id comes from the path.
// @Tags scenarios
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param input body dto.CreateScenarioInput true "Scenario fields (organization_id is overridden by the path)"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /organizations/{id}/scenarios [post]
// @Security BearerAuth
func (sc *scenarioController) OrgCreateScenario(ctx *gin.Context) {
	orgID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	var input dto.CreateScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Path is the source of truth for org scoping — never trust the body.
	input.OrganizationID = &orgID

	userID := ctx.GetString("userId")

	scenario := &models.Scenario{
		Name:             input.Name,
		Title:            input.Title,
		Description:      input.Description,
		Difficulty:       input.Difficulty,
		EstimatedTime:    input.EstimatedTime,
		InstanceType:     input.InstanceType,
		Hostname:         input.Hostname,
		OsType:           input.OsType,
		RequiredFeatures: input.RequiredFeatures,
		SourceType:       input.SourceType,
		GitRepository:    input.GitRepository,
		GitBranch:        input.GitBranch,
		SourcePath:       input.SourcePath,
		FlagsEnabled:     input.FlagsEnabled,
		AllowedFlagPaths: input.AllowedFlagPaths,
		GshEnabled:       input.GshEnabled,
		CrashTraps:       input.CrashTraps,
		Objectives:       input.Objectives,
		Prerequisites:    input.Prerequisites,
		IntroText:        input.IntroText,
		FinishText:       input.FinishText,
		OrganizationID:   &orgID,
		IsPublic:         input.IsPublic,
		SetupScript:      input.SetupScript,
		SetupScriptID:    input.SetupScriptID,
		IntroFileID:      input.IntroFileID,
		FinishFileID:     input.FinishFileID,
		CreatedByID:      userID,
	}

	if err := sc.db.Create(scenario).Error; err != nil {
		slog.Error("failed to create org scenario", "err", err, "org_id", orgID)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to create scenario",
		})
		return
	}

	ctx.JSON(http.StatusCreated, sc.buildScenarioOutput(scenario))
}

// GroupCreateScenario creates a blank scenario inside a group's organization
// and auto-assigns it to the group.
//
// Group managers (typically teachers) can't POST to the platform-wide
// /scenarios endpoint and may not have org-manager role on the parent org.
// This endpoint mirrors OrgCreateScenario but takes a groupId path param,
// derives the organization_id from the group record, and additionally
// creates a ScenarioAssignment so the new scenario is immediately visible
// to the group (mirrors the auto-assignment block from GroupImportJSON).
//
// @Summary Create a scenario in a group
// @Description Group-scoped scenario creation for managers/owners. organization_id is derived from the group; auto-creates a group ScenarioAssignment.
// @Tags scenarios
// @Accept json
// @Produce json
// @Param groupId path string true "Group ID"
// @Param input body dto.CreateScenarioInput true "Scenario fields (organization_id is overridden by the group's org)"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /groups/{groupId}/scenarios [post]
// @Security BearerAuth
func (sc *scenarioController) GroupCreateScenario(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid group ID",
		})
		return
	}

	var input dto.CreateScenarioInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Look up the group to derive the organization scope.
	var group groupModels.ClassGroup
	if err := sc.db.First(&group, "id = ?", groupID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		return
	}

	// Path-derived org — never trust the body.
	input.OrganizationID = group.OrganizationID

	userID := ctx.GetString("userId")

	scenario := &models.Scenario{
		Name:             input.Name,
		Title:            input.Title,
		Description:      input.Description,
		Difficulty:       input.Difficulty,
		EstimatedTime:    input.EstimatedTime,
		InstanceType:     input.InstanceType,
		Hostname:         input.Hostname,
		OsType:           input.OsType,
		RequiredFeatures: input.RequiredFeatures,
		SourceType:       input.SourceType,
		GitRepository:    input.GitRepository,
		GitBranch:        input.GitBranch,
		SourcePath:       input.SourcePath,
		FlagsEnabled:     input.FlagsEnabled,
		AllowedFlagPaths: input.AllowedFlagPaths,
		GshEnabled:       input.GshEnabled,
		CrashTraps:       input.CrashTraps,
		Objectives:       input.Objectives,
		Prerequisites:    input.Prerequisites,
		IntroText:        input.IntroText,
		FinishText:       input.FinishText,
		OrganizationID:   group.OrganizationID,
		IsPublic:         input.IsPublic,
		SetupScript:      input.SetupScript,
		SetupScriptID:    input.SetupScriptID,
		IntroFileID:      input.IntroFileID,
		FinishFileID:     input.FinishFileID,
		CreatedByID:      userID,
	}

	if err := sc.db.Create(scenario).Error; err != nil {
		slog.Error("failed to create group scenario", "err", err, "group_id", groupID)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to create scenario",
		})
		return
	}

	// Auto-create a ScenarioAssignment so the new scenario is immediately
	// visible to the group (mirrors GroupImportJSON's behaviour).
	assignment := models.ScenarioAssignment{
		ScenarioID:  scenario.ID,
		GroupID:     &groupID,
		Scope:       "group",
		CreatedByID: userID,
		IsActive:    true,
	}
	if err := sc.db.Create(&assignment).Error; err != nil {
		slog.Error("failed to create scenario assignment for group", "err", err, "group_id", groupID, "scenario_id", scenario.ID)
		// Don't fail the whole request — the scenario was already created.
	}

	ctx.JSON(http.StatusCreated, sc.buildScenarioOutput(scenario))
}

// @Router /organizations/{id}/scenarios/upload [post]
// @Security BearerAuth
func (sc *scenarioController) OrgUploadScenario(ctx *gin.Context) {
	orgID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	userID := ctx.GetString("userId")

	// Get file from multipart form
	file, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File is required",
		})
		return
	}

	// Validate file size (10MB max)
	if file.Size > 10*1024*1024 {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File size exceeds 10MB limit",
		})
		return
	}

	// Validate extension
	filename := strings.ToLower(file.Filename)
	var ext string
	switch {
	case strings.HasSuffix(filename, ".tar.gz"):
		ext = ".tar.gz"
	case strings.HasSuffix(filename, ".tgz"):
		ext = ".tgz"
	case strings.HasSuffix(filename, ".zip"):
		ext = ".zip"
	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "File must be .zip, .tar.gz, or .tgz",
		})
		return
	}

	// Save to temp file
	tmpFile, err := os.CreateTemp("", "scenario-upload-*"+ext)
	if err != nil {
		slog.Error("failed to create temp file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.Remove(tmpFile.Name())

	src, err := file.Open()
	if err != nil {
		tmpFile.Close()
		slog.Error("failed to open uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to read uploaded file",
		})
		return
	}

	_, err = io.Copy(tmpFile, src)
	src.Close()
	tmpFile.Close()
	if err != nil {
		slog.Error("failed to save uploaded file", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to save uploaded file",
		})
		return
	}

	// Extract archive
	tmpDir, err := os.MkdirTemp("", "scenario-extract-*")
	if err != nil {
		slog.Error("failed to create temp dir", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to process upload",
		})
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := utils.ExtractArchive(tmpFile.Name(), tmpDir); err != nil {
		slog.Error("failed to extract archive", "err", err)
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: fmt.Sprintf("Failed to extract archive: %s", err.Error()),
		})
		return
	}

	// Find index.json
	scenarioDir, err := utils.FindIndexJSON(tmpDir)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Archive must contain an index.json file",
		})
		return
	}

	// Import scenario with org ID directly
	scenario, err := sc.importerService.ImportFromDirectory(scenarioDir, userID, &orgID, "upload")
	if err != nil {
		slog.Error("failed to import scenario from upload", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: fmt.Sprintf("Failed to import scenario: %s", err.Error()),
		})
		return
	}

	// Do NOT create ScenarioAssignment (unlike GroupUploadScenario)

	// Reload with steps
	var loaded models.Scenario
	if err := sc.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to reload scenario",
		})
		return
	}

	ctx.JSON(http.StatusOK, sc.buildScenarioOutput(&loaded))
}

// OrgExportScenario godoc
// @Summary Export an organization scenario
// @Description Export a scenario belonging to an organization as JSON or KillerCoda archive
// @Tags scenarios
// @Produce json
// @Produce application/zip
// @Param id path string true "Organization ID"
// @Param scenarioId path string true "Scenario ID"
// @Param format query string false "Export format: json (default) or killerkoda"
// @Success 200 {object} dto.ScenarioExportOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /organizations/{id}/scenarios/{scenarioId}/export [get]
// @Security BearerAuth
func (sc *scenarioController) OrgExportScenario(ctx *gin.Context) {
	orgID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	// Verify scenario belongs to this organization
	var scenario models.Scenario
	if err := sc.db.Where("id = ? AND organization_id = ?", scenarioID, orgID).First(&scenario).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario not found in this organization",
		})
		return
	}

	format := ctx.DefaultQuery("format", "json")

	switch format {
	case "json":
		export, err := sc.exportService.ExportAsJSON(scenarioID)
		if err != nil {
			slog.Error("failed to export org scenario as JSON", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.JSON(http.StatusOK, export)

	case "killerkoda":
		zipBytes, filename, err := sc.exportService.ExportAsArchive(scenarioID)
		if err != nil {
			slog.Error("failed to export org scenario as archive", "err", err)
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		ctx.Data(http.StatusOK, "application/zip", zipBytes)

	default:
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid format. Use 'json' or 'killerkoda'",
		})
	}
}

// OrgDeleteScenario godoc
// @Summary Delete an organization scenario
// @Description Delete a scenario belonging to an organization and clean up its assignments
// @Tags scenarios
// @Param id path string true "Organization ID"
// @Param scenarioId path string true "Scenario ID"
// @Success 200 {object} gin.H
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /organizations/{id}/scenarios/{scenarioId} [delete]
// @Security BearerAuth
func (sc *scenarioController) OrgDeleteScenario(ctx *gin.Context) {
	orgID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	// Verify scenario belongs to this organization
	var scenario models.Scenario
	if err := sc.db.Where("id = ? AND organization_id = ?", scenarioID, orgID).First(&scenario).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario not found in this organization",
		})
		return
	}

	// Delete in a transaction: abandon active sessions, delete assignments, then scenario
	if err := sc.db.Transaction(func(tx *gorm.DB) error {
		// Auto-abandon all active/provisioning sessions before deletion
		if err := tx.Model(&models.ScenarioSession{}).
			Where("scenario_id = ? AND status IN ?", scenarioID, []string{"active", "provisioning", "setup_failed"}).
			Updates(map[string]any{"status": "abandoned"}).Error; err != nil {
			return fmt.Errorf("abandon sessions: %w", err)
		}
		if err := tx.Where("scenario_id = ?", scenarioID).Delete(&models.ScenarioAssignment{}).Error; err != nil {
			return fmt.Errorf("delete assignments: %w", err)
		}
		if err := tx.Delete(&scenario).Error; err != nil {
			return fmt.Errorf("delete scenario: %w", err)
		}
		return nil
	}); err != nil {
		slog.Error("failed to delete scenario", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to delete scenario",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Scenario deleted successfully"})
}

// ListGroupAvailableScenarios godoc
// @Summary List scenarios available for a group
// @Description List all scenarios available to a group, including org-level and group-level assignments
// @Tags scenarios
// @Produce json
// @Param groupId path string true "Group ID"
// @Success 200 {array} gin.H
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Router /groups/{groupId}/scenarios [get]
// @Security BearerAuth
func (sc *scenarioController) ListGroupAvailableScenarios(ctx *gin.Context) {
	groupID, err := uuid.Parse(ctx.Param("groupId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid group ID",
		})
		return
	}

	// Get the group to find its organization ID
	var group groupModels.ClassGroup
	if err := sc.db.First(&group, "id = ?", groupID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Group not found",
		})
		return
	}

	// Collect scenarios with their source info
	type scenarioWithSource struct {
		Scenario models.Scenario
		Source   string
	}

	scenarioMap := make(map[uuid.UUID]*scenarioWithSource)

	// 1. Org-level scenarios: all scenarios belonging to the group's organization
	// These are available for assignment whether or not they have a ScenarioAssignment
	if group.OrganizationID != nil {
		var orgScenarios []models.Scenario
		if err := sc.db.Where("organization_id = ?", group.OrganizationID).
			Preload("Steps").
			Find(&orgScenarios).Error; err != nil {
			slog.Error("failed to fetch org scenarios", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to fetch scenarios",
			})
			return
		}
		for _, s := range orgScenarios {
			scenarioMap[s.ID] = &scenarioWithSource{
				Scenario: s,
				Source:   "org",
			}
		}
	}

	// 2. Group-level scenarios (via ScenarioAssignment with scope="group")
	var groupAssignments []models.ScenarioAssignment
	if err := sc.db.Where("group_id = ? AND scope = ? AND is_active = true",
		groupID, "group").
		Preload("Scenario").Preload("Scenario.Steps").
		Find(&groupAssignments).Error; err != nil {
		slog.Error("failed to fetch group scenario assignments", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to fetch scenarios",
		})
		return
	}
	for _, a := range groupAssignments {
		if a.Scenario.ID != uuid.Nil {
			// Group assignment takes precedence over org assignment
			scenarioMap[a.Scenario.ID] = &scenarioWithSource{
				Scenario: a.Scenario,
				Source:   "group",
			}
		}
	}

	// Build output with source field
	output := make([]gin.H, 0, len(scenarioMap))
	for _, sw := range scenarioMap {
		scenarioOutput := sc.buildScenarioOutput(&sw.Scenario)
		output = append(output, gin.H{
			"id":              scenarioOutput.ID,
			"name":            scenarioOutput.Name,
			"title":           scenarioOutput.Title,
			"description":     scenarioOutput.Description,
			"difficulty":      scenarioOutput.Difficulty,
			"estimated_time":  scenarioOutput.EstimatedTime,
			"instance_type":   scenarioOutput.InstanceType,
			"os_type":         scenarioOutput.OsType,
			"source_type":     scenarioOutput.SourceType,
			"created_by_id":   scenarioOutput.CreatedByID,
			"organization_id": scenarioOutput.OrganizationID,
			"created_at":      scenarioOutput.CreatedAt,
			"updated_at":      scenarioOutput.UpdatedAt,
			"steps":           scenarioOutput.Steps,
			"source":          sw.Source,
		})
	}

	ctx.JSON(http.StatusOK, output)
}

// DuplicateScenario godoc
// @Summary Duplicate a scenario
// @Description Create a deep copy of a scenario including steps, hints, instance types, and project files
// @Tags scenarios
// @Produce json
// @Param id path string true "Source Scenario ID"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/{id}/duplicate [post]
// @Security BearerAuth
func (sc *scenarioController) DuplicateScenario(ctx *gin.Context) {
	scenarioID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	userID := ctx.GetString("userId")

	newScenario, err := sc.duplicateService.DuplicateScenario(scenarioID, userID, nil)
	if err != nil {
		slog.Error("failed to duplicate scenario", "err", err)
		if strings.Contains(err.Error(), "not found") {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
		} else {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to duplicate scenario",
			})
		}
		return
	}

	ctx.JSON(http.StatusCreated, sc.buildScenarioOutput(newScenario))
}

// OrgDuplicateScenario godoc
// @Summary Duplicate an organization scenario
// @Description Create a deep copy of a scenario within an organization
// @Tags scenarios
// @Produce json
// @Param id path string true "Organization ID"
// @Param scenarioId path string true "Source Scenario ID"
// @Success 201 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /organizations/{id}/scenarios/{scenarioId}/duplicate [post]
// @Security BearerAuth
func (sc *scenarioController) OrgDuplicateScenario(ctx *gin.Context) {
	orgID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	scenarioID, err := uuid.Parse(ctx.Param("scenarioId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return
	}

	// Verify scenario belongs to this organization
	var scenario models.Scenario
	if err := sc.db.Where("id = ? AND organization_id = ?", scenarioID, orgID).First(&scenario).Error; err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Scenario not found in this organization",
		})
		return
	}

	userID := ctx.GetString("userId")

	newScenario, err := sc.duplicateService.DuplicateScenario(scenarioID, userID, &orgID)
	if err != nil {
		slog.Error("failed to duplicate org scenario", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to duplicate scenario",
		})
		return
	}

	ctx.JSON(http.StatusCreated, sc.buildScenarioOutput(newScenario))
}

// PreviewScenario starts a preview session for testing a scenario without group assignment.
