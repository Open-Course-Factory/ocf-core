package scenarioController

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/errors"
	groupServices "soli/formations/src/groups/services"
	scenarioRegistration "soli/formations/src/scenarios/entityRegistration"
	scenarioHooks "soli/formations/src/scenarios/hooks"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
	"soli/formations/src/scenarios/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// scenarioControllerBase holds dependencies and helpers shared by the four
// scenario controllers (main, management, launch, progress). They embed it so
// they can reach helpers such as getSessionIfOwned and canManageScenarioByID
// without duplication.
type scenarioControllerBase struct {
	db               *gorm.DB
	groupService     groupServices.GroupService
	seedService      *services.ScenarioSeedService
	importerService  *services.ScenarioImporterService
	exportService    *services.ScenarioExportService
	duplicateService *services.ScenarioDuplicateService
}

func newScenarioControllerBase(db *gorm.DB) scenarioControllerBase {
	return scenarioControllerBase{
		db:               db,
		groupService:     groupServices.NewGroupService(db),
		seedService:      services.NewScenarioSeedService(db),
		importerService:  services.NewScenarioImporterService(db),
		exportService:    services.NewScenarioExportService(db),
		duplicateService: services.NewScenarioDuplicateService(db),
	}
}

// canManageScenarioByID loads the scenario and answers whether the caller may
// manage it: a platform admin always may, anyone else through
// CanManageScenario (creator, org manager, manager of an assigned group). It
// is the one rule behind PATCH, DELETE, archive and every export, so a handler
// that must refuse on its own (defense in depth behind Layer 2) calls this
// rather than restating a narrower check. A missing scenario comes back as
// gorm.ErrRecordNotFound.
func (b *scenarioControllerBase) canManageScenarioByID(ctx *gin.Context, scenarioID uuid.UUID) (*models.Scenario, bool, error) {
	var scenario models.Scenario
	if err := b.db.Where("id = ?", scenarioID).First(&scenario).Error; err != nil {
		return nil, false, err
	}

	userRoles, _ := ctx.Get("userRoles")
	roles, _ := userRoles.([]string)
	if access.IsAdmin(roles) {
		return &scenario, true, nil
	}

	allowed, err := scenarioHooks.CanManageScenario(b.db, b.groupService, &scenario, ctx.GetString("userId"))
	if err != nil {
		return nil, false, err
	}
	return &scenario, allowed, nil
}

// getSessionIfOwned loads a session by ID and checks that the authenticated user owns it.
func (b *scenarioControllerBase) getSessionIfOwned(ctx *gin.Context) (*models.ScenarioSession, error) {
	sessionID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Invalid session ID")
		return nil, err
	}

	userID := ctx.GetString("userId")

	var session models.ScenarioSession
	if err := b.db.First(&session, "id = ?", sessionID).Error; err != nil {
		errors.Respond(ctx, http.StatusNotFound, "Session not found")
		return nil, err
	}

	if session.UserID != userID {
		errors.Respond(ctx, http.StatusForbidden, "You do not own this session")
		return nil, fmt.Errorf("forbidden")
	}

	return &session, nil
}

// rejectIfArchived answers the request and reports true when the scenario has
// been retired. Archiving stops new runs only — sessions already in flight are
// left to finish, so this belongs on the launch entry points and not on the
// session-progress routes.
func (b *scenarioControllerBase) rejectIfArchived(ctx *gin.Context, scenario *models.Scenario) bool {
	if !scenario.IsArchived() {
		return false
	}
	errors.Respond(ctx, http.StatusConflict, models.ErrScenarioArchived.Error())
	return true
}

// importUploadedArchive imports the scenario archive sent in the "file" form
// field under the given owner, optionally assigning it to a group, and answers
// the request. The caller has already run its authorization gate.
func (b *scenarioControllerBase) importUploadedArchive(ctx *gin.Context, orgID *uuid.UUID, assignToGroup *uuid.UUID) {
	userID := ctx.GetString("userId")

	file, err := ctx.FormFile("file")
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "File is required")
		return
	}

	if file.Size > 10*1024*1024 {
		errors.Respond(ctx, http.StatusBadRequest, "File size exceeds 10MB limit")
		return
	}

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
		errors.Respond(ctx, http.StatusBadRequest, "File must be .zip, .tar.gz, or .tgz")
		return
	}

	tmpFile, err := os.CreateTemp("", "scenario-upload-*"+ext)
	if err != nil {
		slog.Error("failed to create temp file", "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to process upload")
		return
	}
	defer os.Remove(tmpFile.Name())

	src, err := file.Open()
	if err != nil {
		tmpFile.Close()
		slog.Error("failed to open uploaded file", "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to read uploaded file")
		return
	}

	_, err = io.Copy(tmpFile, src)
	src.Close()
	tmpFile.Close()
	if err != nil {
		slog.Error("failed to save uploaded file", "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to save uploaded file")
		return
	}

	tmpDir, err := os.MkdirTemp("", "scenario-extract-*")
	if err != nil {
		slog.Error("failed to create temp dir", "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to process upload")
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := utils.ExtractArchive(tmpFile.Name(), tmpDir); err != nil {
		slog.Error("failed to extract archive", "err", err)
		errors.Respond(ctx, http.StatusBadRequest, fmt.Sprintf("Failed to extract archive: %s", err.Error()))
		return
	}

	scenarioDir, err := utils.FindIndexJSON(tmpDir)
	if err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Archive must contain an index.json file")
		return
	}

	scenario, err := b.importerService.ImportFromDirectory(scenarioDir, userID, orgID, "upload")
	if err != nil {
		slog.Error("failed to import scenario from upload", "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, fmt.Sprintf("Failed to import scenario: %s", err.Error()))
		return
	}

	if assignToGroup != nil {
		b.ensureGroupAssignment(scenario.ID, *assignToGroup, userID)
	}

	var loaded models.Scenario
	if err := b.db.Preload("Steps", func(db *gorm.DB) *gorm.DB {
		return db.Order("\"order\" ASC")
	}).First(&loaded, "id = ?", scenario.ID).Error; err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to reload scenario")
		return
	}

	ctx.JSON(http.StatusOK, scenarioRegistration.ScenarioToOutput(&loaded))
}

// ensureGroupAssignment links a scenario to a group unless it already is. A
// failure is logged, not answered: the scenario itself was already written.
func (b *scenarioControllerBase) ensureGroupAssignment(scenarioID, groupID uuid.UUID, userID string) {
	var existing models.ScenarioAssignment
	if err := b.db.Where("scenario_id = ? AND group_id = ?", scenarioID, groupID).First(&existing).Error; err == nil {
		return
	}
	assignment := models.ScenarioAssignment{
		ScenarioID:  scenarioID,
		GroupID:     &groupID,
		Scope:       "group",
		CreatedByID: userID,
		IsActive:    true,
	}
	if err := b.db.Create(&assignment).Error; err != nil {
		slog.Error("failed to create scenario assignment", "err", err)
	}
}
