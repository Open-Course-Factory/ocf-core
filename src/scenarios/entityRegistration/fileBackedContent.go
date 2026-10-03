package scenarioRegistration

import (
	"soli/formations/src/scenarios/dto"
	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// An imported or seeded scenario stores each script and text twice: inline on
// the row and in a ProjectFile the row points at, and the runtime reads the
// file (services.ResolveScriptContent). The API output carries the inline
// field, so it shows the file's content instead; the write side is
// scenarioHooks.InlineContentEditHook.

// fileBacked is an output field together with the file that overrides it.
type fileBacked struct {
	content *string
	fileID  *uuid.UUID
}

// showFileContent replaces each field with its file's content, so the editor
// shows (and saves back) what the runtime runs. A file that cannot be read
// leaves the inline copy, as ResolveScriptContent does.
func showFileContent(db *gorm.DB, fields []fileBacked) {
	var ids []uuid.UUID
	for _, f := range fields {
		if f.fileID != nil {
			ids = append(ids, *f.fileID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var files []models.ProjectFile
	if err := db.Select("id", "content").Where("id IN ?", ids).Find(&files).Error; err != nil {
		return
	}
	contents := make(map[uuid.UUID]string, len(files))
	for _, file := range files {
		contents[file.ID] = file.Content
	}
	for _, f := range fields {
		if f.fileID == nil {
			continue
		}
		if content, ok := contents[*f.fileID]; ok {
			*f.content = content
		}
	}
}

func stepOutputFiles(out *dto.ScenarioStepOutput) []fileBacked {
	return []fileBacked{
		{&out.TextContent, out.TextFileID},
		{&out.HintContent, out.HintFileID},
		{&out.VerifyScript, out.VerifyScriptID},
		{&out.BackgroundScript, out.BackgroundScriptID},
		{&out.ForegroundScript, out.ForegroundScriptID},
	}
}

func scenarioOutputFiles(out *dto.ScenarioOutput) []fileBacked {
	fields := []fileBacked{
		{&out.IntroText, out.IntroFileID},
		{&out.FinishText, out.FinishFileID},
		{&out.SetupScript, out.SetupScriptID},
	}
	// Nested steps carry their prose but not their scripts; filling the
	// scripts in here would add them to every scenario listing.
	for i := range out.Steps {
		step := &out.Steps[i]
		fields = append(fields,
			fileBacked{&step.TextContent, step.TextFileID},
			fileBacked{&step.HintContent, step.HintFileID})
	}
	return fields
}

// withFileContent applies showFileContent to the DTO a generic GET handler
// hands its redactor (an *any holding T), and stores the result back.
func withFileContent[T any](dtoPtr any, db *gorm.DB, fields func(*T) []fileBacked) {
	wrapper, ok := dtoPtr.(*any)
	if !ok || db == nil {
		return
	}
	output, ok := (*wrapper).(T)
	if !ok {
		return
	}
	showFileContent(db, fields(&output))
	*wrapper = output
}
