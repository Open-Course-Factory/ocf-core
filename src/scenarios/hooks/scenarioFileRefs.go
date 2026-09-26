package scenarioHooks

import (
	"fmt"
	"strings"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// fileRef is one file-id column of M and the getter of its field.
type fileRef[M any] struct {
	column string
	get    func(*M) *uuid.UUID
}

// The file-id columns of a step and of a scenario. Each id is a read of the
// file behind it: the session engine runs it and the export returns it.
var (
	stepFileRefs = []fileRef[models.ScenarioStep]{
		{"verify_script_id", func(s *models.ScenarioStep) *uuid.UUID { return s.VerifyScriptID }},
		{"background_script_id", func(s *models.ScenarioStep) *uuid.UUID { return s.BackgroundScriptID }},
		{"foreground_script_id", func(s *models.ScenarioStep) *uuid.UUID { return s.ForegroundScriptID }},
		{"text_file_id", func(s *models.ScenarioStep) *uuid.UUID { return s.TextFileID }},
		{"hint_file_id", func(s *models.ScenarioStep) *uuid.UUID { return s.HintFileID }},
	}
	scenarioFileRefs = []fileRef[models.Scenario]{
		{"setup_script_id", func(s *models.Scenario) *uuid.UUID { return s.SetupScriptID }},
		{"intro_file_id", func(s *models.Scenario) *uuid.UUID { return s.IntroFileID }},
		{"finish_file_id", func(s *models.Scenario) *uuid.UUID { return s.FinishFileID }},
	}
	stepFileColumns     = columnList(stepFileRefs)
	scenarioFileColumns = columnList(scenarioFileRefs)
)

func columnList[M any](refs []fileRef[M]) string {
	columns := make([]string, len(refs))
	for i, ref := range refs {
		columns[i] = ref.column
	}
	return strings.Join(columns, ", ")
}

// refuseForeignFileRefs enforces that a scenario only references its own
// files (#516). Every file id the write sets must be unchanged from old, or
// already referenced by the scenario row or one of its steps, or linked to the
// scenario by ProjectFile.ScenarioID. newEntity is the created model or the
// PATCH map; old is nil on create. Importer, seed and duplicate write ids
// without the entity hooks and are not subject to this.
func refuseForeignFileRefs[M any](db *gorm.DB, scenarioID uuid.UUID, refs []fileRef[M], newEntity any, old *M) error {
	for _, ref := range refs {
		id, set, err := fileRefOf(newEntity, ref)
		if err != nil {
			return err
		}
		if !set || old != nil && ref.get(old) != nil && *ref.get(old) == id {
			continue
		}
		var owned int64
		err = db.Model(&models.ProjectFile{}).
			Where("id = ?", id).
			Where(db.Where("scenario_id = ?", scenarioID).
				Or("EXISTS (?)", db.Model(&models.Scenario{}).Select("1").
					Where("id = ? AND ? IN ("+scenarioFileColumns+")", scenarioID, id)).
				Or("EXISTS (?)", db.Model(&models.ScenarioStep{}).Select("1").
					Where("scenario_id = ? AND ? IN ("+stepFileColumns+")", scenarioID, id))).
			Count(&owned).Error
		if err != nil {
			return fmt.Errorf("check %s: %w", ref.column, err)
		}
		if owned == 0 {
			return utils.PermissionDeniedError("set "+ref.column+" to a file outside", "scenario")
		}
	}
	return nil
}

// fileRefOf reads one file id from the created model or the PATCH map. The
// DtoToMap converters put uuid.UUID values in the map; any other type is
// refused rather than skipped.
func fileRefOf[M any](newEntity any, ref fileRef[M]) (uuid.UUID, bool, error) {
	switch e := newEntity.(type) {
	case *M:
		if id := ref.get(e); id != nil {
			return *id, true, nil
		}
	case map[string]any:
		switch v := e[ref.column].(type) {
		case uuid.UUID:
			return v, true, nil
		case nil:
		default:
			return uuid.Nil, false, fmt.Errorf("%s: unexpected %T", ref.column, v)
		}
	default:
		return uuid.Nil, false, fmt.Errorf("unexpected %T for a file reference", newEntity)
	}
	return uuid.Nil, false, nil
}
