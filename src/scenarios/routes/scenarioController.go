package scenarioController

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/scenarios/dto"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	"soli/formations/src/scenarios/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScenarioController defines the handler methods for platform-wide scenario CRUD
// and session-read endpoints. Group-scoped and org-scoped scenario management
// lives on scenarioManagementController.
type ScenarioController interface {
	ImportScenario(ctx *gin.Context)
	SeedScenario(ctx *gin.Context)
	UploadScenario(ctx *gin.Context)
	GetSessionByTerminal(ctx *gin.Context)
	GetSessionInfo(ctx *gin.Context)
	ExportScenario(ctx *gin.Context)
	ExportScenarios(ctx *gin.Context)
	ImportJSON(ctx *gin.Context)
	DuplicateScenario(ctx *gin.Context)
	GetTranslationCoverage(ctx *gin.Context)
	GetScenarioHealth(ctx *gin.Context)
	GetLexicon(ctx *gin.Context)
	ReplaceLexicon(ctx *gin.Context)
	ArchiveScenario(ctx *gin.Context)
	UnarchiveScenario(ctx *gin.Context)
}

type scenarioController struct {
	scenarioControllerBase
	importerService  *services.ScenarioImporterService
	exportService    *services.ScenarioExportService
	seedService      *services.ScenarioSeedService
	duplicateService *services.ScenarioDuplicateService
	groupService     groupServices.GroupService
	sessionService   *services.ScenarioSessionService
}

// NewScenarioController creates a new scenario controller with its service dependencies
func NewScenarioController(db *gorm.DB) ScenarioController {
	return &scenarioController{
		scenarioControllerBase: scenarioControllerBase{db: db},
		importerService:        services.NewScenarioImporterService(db),
		exportService:          services.NewScenarioExportService(db),
		seedService:            services.NewScenarioSeedService(db),
		duplicateService:       services.NewScenarioDuplicateService(db),
		groupService:           groupServices.NewGroupService(db),
		sessionService:         services.NewScenarioSessionService(db, services.NewFlagService(), services.NewVerificationService()),
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

	session, err := sc.sessionService.FindSessionByTerminal(terminalID)
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

	ctx.JSON(http.StatusOK, sc.sessionResponse(session))
}

// sessionResponse describes a session the same way wherever it is asked for.
//
// Two handlers answer with a session — one found by its id, one by its terminal
// — and the player uses both. Built in two places, a field added for one is
// simply absent from the other, which is how the briefing came to be resolved
// on one path and not the other.
func (sc *scenarioController) sessionResponse(session *models.ScenarioSession) dto.SessionResponse {
	terminalSessionID := ""
	if session.TerminalSessionID != nil {
		terminalSessionID = *session.TerminalSessionID
	}

	response := dto.SessionResponse{
		ID:                         session.ID.String(),
		ScenarioID:                 session.ScenarioID.String(),
		UserID:                     session.UserID,
		TrainerID:                  session.TrainerID,
		TerminalSessionID:          terminalSessionID,
		CurrentStep:                session.CurrentStep,
		Status:                     session.Status,
		ProvisioningPhase:          session.ProvisioningPhase,
		ProvisioningTimeoutSeconds: sc.sessionService.CurrentStepProvisioningTimeout(session),
		Grade:                      session.Grade,
		StartedAt:                  session.StartedAt,
	}

	// The scenario's prose in the language this session was started in. The
	// steps have always resolved this way; the briefing was read straight off
	// the scenario, so a French session opened with an English welcome.
	var scenario models.Scenario
	if err := sc.db.First(&scenario, "id = ?", session.ScenarioID).Error; err == nil {
		prose := services.ResolveScenarioText(sc.db, scenario, session.Locale)
		response.ScenarioText = &dto.SessionScenarioText{
			Title:       prose.Title,
			Description: prose.Description,
			IntroText:   prose.Intro,
			FinishText:  prose.Finish,
		}
	}
	return response
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

	ctx.JSON(http.StatusOK, sc.sessionResponse(session))
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

	// Aligns export with the PATCH/DELETE rule — see canManageScenarioByID.
	_, allowed, err := sc.canManageScenarioByID(ctx, scenarioID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return
		}
		slog.Error("failed to check scenario manage permission for export", "err", err)
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
	for _, id := range input.IDs {
		_, allowed, err := sc.canManageScenarioByID(ctx, id)
		if err != nil && err != gorm.ErrRecordNotFound {
			slog.Error("failed to check scenario manage permission for bulk export", "err", err)
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Internal error",
			})
			return
		}
		// A missing id answers 403, not 404: this endpoint takes a caller-supplied
		// list, so distinguishing "does not exist" from "not yours" would turn it
		// into a probe for which scenario ids exist.
		if err == gorm.ErrRecordNotFound || !allowed {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: "Access denied: not authorized to export one or more scenarios",
			})
			return
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

// canManageScenarioByID loads a scenario and answers whether the caller may
// manage it — the one rule behind PATCH, DELETE, export and archive: platform
// admin, creator, org manager of the scenario's org, or manager of any group
// it is assigned to.
//
// It deliberately writes no response. The callers answer differently on
// purpose: the single-scenario routes report a missing id as 404, while the
// bulk export reports it as 403 so the endpoint cannot be used to probe which
// ids exist. Returning the decision keeps that difference visible at each call
// site instead of burying it in a shared responder.
//
// A missing scenario comes back as gorm.ErrRecordNotFound.
func (sc *scenarioController) canManageScenarioByID(ctx *gin.Context, scenarioID uuid.UUID) (*models.Scenario, bool, error) {
	var scenario models.Scenario
	if err := sc.db.Where("id = ?", scenarioID).First(&scenario).Error; err != nil {
		return nil, false, err
	}

	userRoles, _ := ctx.Get("userRoles")
	roles, _ := userRoles.([]string)
	if access.IsAdmin(roles) {
		return &scenario, true, nil
	}

	allowed, err := scenarioHooks.CanManageScenario(sc.db, sc.groupService, &scenario, ctx.GetString("userId"))
	if err != nil {
		return nil, false, err
	}
	return &scenario, allowed, nil
}

// loadManageableScenario loads the scenario named by the :id parameter and
// checks the caller may manage it — the same rule that guards PATCH, DELETE
// and export (creator, org manager, group manager, or platform admin). It
// answers the request and returns nil when the caller may not.
func (sc *scenarioController) loadManageableScenario(ctx *gin.Context) *models.Scenario {
	scenarioID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid scenario ID",
		})
		return nil
	}

	scenario, allowed, err := sc.canManageScenarioByID(ctx, scenarioID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, &errors.APIError{
				ErrorCode:    http.StatusNotFound,
				ErrorMessage: "Scenario not found",
			})
			return nil
		}
		slog.Error("failed to check scenario manage permission", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Internal error",
		})
		return nil
	}
	if !allowed {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "Access denied",
		})
		return nil
	}

	return scenario
}

// ArchiveScenario godoc
// @Summary Archive a scenario
// @Description Retires a scenario: it stops being listed, assignable and launchable, while past sessions, grades and flags keep referring to it. Sessions already running are left to finish.
// @Tags scenarios
// @Produce json
// @Param id path string true "Scenario ID"
// @Success 200 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/{id}/archive [post]
// @Security BearerAuth
func (sc *scenarioController) ArchiveScenario(ctx *gin.Context) {
	now := time.Now()
	sc.setScenarioArchivedAt(ctx, &now)
}

// GetTranslationCoverage godoc
// @Summary Report how completely each locale covers a scenario
// @Description Per declared locale: how many steps are translated, how many are
// @Description stale against the source they were written from, and whether the
// @Description locale is complete enough to be offered to a learner.
// @Tags scenarios
// @Produce json
// @Param id path string true "Scenario ID"
// @Success 200 {array} services.LocaleCoverage
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/{id}/translation-coverage [get]
// @Security BearerAuth
func (sc *scenarioController) GetTranslationCoverage(ctx *gin.Context) {
	scenario := sc.loadManageableScenario(ctx)
	if scenario == nil {
		return
	}

	coverage, err := services.TranslationCoverage(sc.db, scenario.ID)
	if err != nil {
		slog.Error("failed to compute translation coverage", "scenario_id", scenario.ID, "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to compute translation coverage",
		})
		return
	}

	// A scenario declaring no locales has nothing to report. Answer with an
	// empty array rather than null, so a client can render it without a
	// special case.
	if coverage == nil {
		coverage = []services.LocaleCoverage{}
	}
	ctx.JSON(http.StatusOK, coverage)
}

// GetLexicon godoc
// @Summary Read a scenario's vocabulary
// @Description Every object the scenario's world is built from, what each
// @Description language calls it, and anything wrong with the set.
// @Tags scenarios
// @Produce json
// @Param id path string true "Scenario ID"
// @Success 200 {object} dto.LexiconDocumentOutput
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Router /scenarios/{id}/lexicon [get]
// @Security BearerAuth
func (sc *scenarioController) GetLexicon(ctx *gin.Context) {
	scenario := sc.loadManageableScenario(ctx)
	if scenario == nil {
		return
	}

	document, err := services.LoadLexiconDocument(sc.db, scenario.ID)
	if err != nil {
		slog.Error("failed to load lexicon", "scenario_id", scenario.ID, "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to load the scenario's vocabulary",
		})
		return
	}
	ctx.JSON(http.StatusOK, document)
}

// ReplaceLexicon godoc
// @Summary Replace a scenario's vocabulary
// @Description Stores the whole vocabulary at once. A lexicon that is merely
// @Description unfinished is stored and its gaps reported; one that cannot
// @Description resolve — an unknown parent, a room inside itself, a key used
// @Description twice — is refused and nothing is changed.
// @Tags scenarios
// @Accept json
// @Produce json
// @Param id path string true "Scenario ID"
// @Param lexicon body dto.ReplaceLexiconInput true "The whole vocabulary"
// @Success 200 {object} dto.LexiconDocumentOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Router /scenarios/{id}/lexicon [put]
// @Security BearerAuth
func (sc *scenarioController) ReplaceLexicon(ctx *gin.Context) {
	scenario := sc.loadManageableScenario(ctx)
	if scenario == nil {
		return
	}

	var input dto.ReplaceLexiconInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid vocabulary: " + err.Error(),
		})
		return
	}

	if err := services.ReplaceLexicon(sc.db, scenario.ID, input.Entries); err != nil {
		// A vocabulary that cannot resolve is the caller's mistake, and the
		// message names which entry — an editor has to be able to point at it.
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	document, err := services.LoadLexiconDocument(sc.db, scenario.ID)
	if err != nil {
		slog.Error("failed to reload lexicon after save", "scenario_id", scenario.ID, "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Saved, but the vocabulary could not be read back",
		})
		return
	}
	ctx.JSON(http.StatusOK, document)
}

// UnarchiveScenario godoc
// @Summary Unarchive a scenario
// @Description Puts an archived scenario back in service.
// @Tags scenarios
// @Produce json
// @Param id path string true "Scenario ID"
// @Success 200 {object} dto.ScenarioOutput
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Failure 404 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/{id}/unarchive [post]
// @Security BearerAuth
func (sc *scenarioController) UnarchiveScenario(ctx *gin.Context) {
	sc.setScenarioArchivedAt(ctx, nil)
}

// setScenarioArchivedAt writes archived_at explicitly rather than through the
// loaded struct, so a nil value clears the column instead of being skipped as
// a zero value.
func (sc *scenarioController) setScenarioArchivedAt(ctx *gin.Context, at *time.Time) {
	scenario := sc.loadManageableScenario(ctx)
	if scenario == nil {
		return
	}

	if err := sc.db.Model(&models.Scenario{}).
		Where("id = ?", scenario.ID).
		Update("archived_at", at).Error; err != nil {
		slog.Error("failed to update scenario archived state", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to update scenario",
		})
		return
	}

	scenario.ArchivedAt = at
	ctx.JSON(http.StatusOK, sc.buildScenarioOutput(scenario))
}

// GetScenarioHealth godoc
// @Summary Report what every scenario claims but cannot deliver
// @Description One entry per scenario with something wrong: a language declared
// @Description and not offered, a vocabulary a language cannot build a world
// @Description from, a step with no way to pass it. Scenarios with nothing wrong
// @Description are absent — the report is a list of things to fix.
// @Tags scenarios
// @Produce json
// @Success 200 {array} services.ScenarioHealth
// @Failure 403 {object} errors.APIError
// @Failure 500 {object} errors.APIError
// @Router /scenarios/health [get]
// @Security BearerAuth
func (sc *scenarioController) GetScenarioHealth(ctx *gin.Context) {
	var scenarios []models.Scenario
	if err := sc.db.Order("name ASC").Find(&scenarios).Error; err != nil {
		slog.Error("failed to list scenarios for the health report", "err", err)
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to read the scenarios",
		})
		return
	}

	report := []services.ScenarioHealth{}
	for _, scenario := range scenarios {
		health, err := services.CheckScenarioHealth(sc.db, scenario)
		if err != nil {
			// One unreadable scenario must not hide the faults in every other
			// one: the report says so and carries on. A page that fails whole
			// is a page nobody opens twice.
			slog.Warn("could not check a scenario's health", "scenario", scenario.Name, "err", err)
			continue
		}
		if len(health.Findings) > 0 {
			report = append(report, health)
		}
	}

	ctx.JSON(http.StatusOK, report)
}
