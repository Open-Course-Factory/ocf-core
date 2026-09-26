package scenarioHooks

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"soli/formations/src/auth/access"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The file-id columns of a step and of a scenario. Each id is a read of the
// file behind it: the session engine runs it and the export returns it.
var (
	stepFileRefs = map[string]func(*models.ScenarioStep) *uuid.UUID{
		"verify_script_id":     func(s *models.ScenarioStep) *uuid.UUID { return s.VerifyScriptID },
		"background_script_id": func(s *models.ScenarioStep) *uuid.UUID { return s.BackgroundScriptID },
		"foreground_script_id": func(s *models.ScenarioStep) *uuid.UUID { return s.ForegroundScriptID },
		"text_file_id":         func(s *models.ScenarioStep) *uuid.UUID { return s.TextFileID },
		"hint_file_id":         func(s *models.ScenarioStep) *uuid.UUID { return s.HintFileID },
	}
	scenarioFileRefs = map[string]func(*models.Scenario) *uuid.UUID{
		"setup_script_id": func(s *models.Scenario) *uuid.UUID { return s.SetupScriptID },
		"intro_file_id":   func(s *models.Scenario) *uuid.UUID { return s.IntroFileID },
		"finish_file_id":  func(s *models.Scenario) *uuid.UUID { return s.FinishFileID },
	}
	stepFileColumns     = strings.Join(slices.Collect(maps.Keys(stepFileRefs)), ", ")
	scenarioFileColumns = strings.Join(slices.Collect(maps.Keys(scenarioFileRefs)), ", ")
)

// refuseForeignFileRefs enforces that a scenario only references its own
// files (#516). Every file id the write sets must be unchanged from old, or
// already referenced by the scenario row or one of its steps, or linked to the
// scenario by ProjectFile.ScenarioID. newEntity is the created model or the
// PATCH map; old is nil on create. Importer, seed and duplicate write ids
// without the entity hooks and are not subject to this.
func refuseForeignFileRefs[M any](db *gorm.DB, scenarioID uuid.UUID, fields map[string]func(*M) *uuid.UUID, newEntity any, old *M) error {
	for column, get := range fields {
		id, set, err := fileRefOf(newEntity, column, get)
		if err != nil {
			return err
		}
		if !set || old != nil && get(old) != nil && *get(old) == id {
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
			return fmt.Errorf("check %s: %w", column, err)
		}
		if owned == 0 {
			return foreignFileRefError(column)
		}
	}
	return nil
}

// fileRefOf reads one file id from the created model or the PATCH map.
func fileRefOf[M any](newEntity any, column string, get func(*M) *uuid.UUID) (uuid.UUID, bool, error) {
	switch e := newEntity.(type) {
	case *M:
		if id := get(e); id != nil {
			return *id, true, nil
		}
	case map[string]any:
		switch v := e[column].(type) {
		case uuid.UUID:
			return v, true, nil
		case string:
			id, err := uuid.Parse(v)
			return id, err == nil, err
		case nil:
		default:
			return uuid.Nil, false, fmt.Errorf("%s: unexpected %T", column, v)
		}
	default:
		return uuid.Nil, false, fmt.Errorf("unexpected %T for a file reference", newEntity)
	}
	return uuid.Nil, false, nil
}

// RefuseFileRefsOnNewScenario refuses any file id on a scenario a
// non-administrator creates: a scenario that does not exist yet owns no file,
// so the id can only be someone else's.
func RefuseFileRefsOnNewScenario(scenario *models.Scenario, roles []string) error {
	if access.IsAdmin(roles) {
		return nil
	}
	for column, get := range scenarioFileRefs {
		if get(scenario) != nil {
			return foreignFileRefError(column)
		}
	}
	return nil
}

func foreignFileRefError(column string) error {
	return utils.PermissionDeniedError("set "+column+" to a file outside", "scenario")
}
