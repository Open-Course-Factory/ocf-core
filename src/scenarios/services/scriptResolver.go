package services

import (
	"log"
	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ResolveScriptContent loads the content from a ProjectFile if fileID is non-nil,
// otherwise returns the inlineContent as a fallback.
func ResolveScriptContent(db *gorm.DB, fileID *uuid.UUID, inlineContent string) string {
	if fileID == nil {
		return inlineContent
	}

	var file models.ProjectFile
	if err := db.First(&file, "id = ?", *fileID).Error; err != nil {
		log.Printf("[ScriptResolver] Failed to load ProjectFile %s: %v", fileID, err)
		return inlineContent
	}

	return file.Content
}
