package scenarioController

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	auditModels "soli/formations/src/audit/models"
	auditServices "soli/formations/src/audit/services"
	"soli/formations/src/scenarios/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ParticipationArchiveController lets an organization's managers read its copies.
type ParticipationArchiveController struct {
	db    *gorm.DB
	audit auditServices.AuditService
}

func NewParticipationArchiveController(db *gorm.DB) *ParticipationArchiveController {
	return &ParticipationArchiveController{db: db, audit: auditServices.NewAuditService(db)}
}

// ParticipationArchiveCopy is a copy with its report.
type ParticipationArchiveCopy struct {
	models.ParticipationArchive
	Report json.RawMessage `json:"report" swaggertype:"object"`
}

// List godoc
// @Summary List an organization's archived participation reports
// @Description Which classes and days have a frozen copy, and until when. It leaves out the reports themselves, so listing is not logged as a read.
// @Tags scenario-teacher
// @Produce json
// @Param id path string true "Organization ID (UUID)"
// @Success 200 {array} models.ParticipationArchive
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /organizations/{id}/participation-archives [get]
// @Security BearerAuth
func (pc *ParticipationArchiveController) List(c *gin.Context) {
	organizationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization ID"})
		return
	}
	archives := []models.ParticipationArchive{}
	if err := pc.db.Omit("report").Where("organization_id = ?", organizationID).Order("created_at DESC").Find(&archives).Error; err != nil {
		slog.Error("failed to list participation archives", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list the archives"})
		return
	}
	c.JSON(http.StatusOK, archives)
}

// Get godoc
// @Summary Read an archived participation report
// @Description Returns the frozen copy. Each read is logged first; a read that cannot be logged is refused.
// @Tags scenario-teacher
// @Produce json
// @Param id path string true "Organization ID (UUID)"
// @Param archiveId path string true "Archive ID (UUID)"
// @Success 200 {object} ParticipationArchiveCopy
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /organizations/{id}/participation-archives/{archiveId} [get]
// @Security BearerAuth
func (pc *ParticipationArchiveController) Get(c *gin.Context) {
	organizationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid organization ID"})
		return
	}
	var archive models.ParticipationArchive
	err = pc.db.First(&archive, "id = ? AND organization_id = ?", c.Param("archiveId"), organizationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "archive not found"})
		return
	}
	if err != nil {
		slog.Error("failed to load participation archive", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load the archive"})
		return
	}
	if err := pc.audit.Log(archiveReadEntry(archive, c.GetString("userId"))); err != nil {
		slog.Error("failed to log participation archive read", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load the archive"})
		return
	}
	c.JSON(http.StatusOK, ParticipationArchiveCopy{ParticipationArchive: archive, Report: json.RawMessage(archive.Report)})
}

func archiveReadEntry(archive models.ParticipationArchive, readerID string) auditModels.AuditLogCreate {
	entry := auditModels.AuditLogCreate{
		EventType:      auditModels.AuditEventParticipationArchiveRead,
		Severity:       auditModels.AuditSeverityInfo,
		TargetID:       &archive.ID,
		TargetType:     "participation_archive",
		TargetName:     archive.Title,
		OrganizationID: &archive.OrganizationID,
		GroupID:        &archive.GroupID,
		Action:         string(auditModels.AuditEventParticipationArchiveRead),
		Status:         "success",
	}
	if actorID, err := uuid.Parse(readerID); err == nil {
		entry.ActorID = &actorID
	}
	return entry
}
