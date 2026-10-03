package scenarioHooks

import "soli/formations/src/entityManagement/hooks"

// InlineContentEditHook makes an edited script or text the one that runs.
//
// An imported or seeded scenario stores each of them twice: inline on the row
// and in a ProjectFile the row points at, and the runtime and the export read
// the file when there is one (services.ResolveScriptContent). The API writes
// the inline field, so without this a teacher's edit changed nothing a learner
// saw. Clearing the pointer leaves the inline value as the content that runs;
// the startup migration (migrateInlineContentToProjectFiles) may later copy it
// into a new file, which is harmless because the copy is identical.
//
// A PATCH that names the file column itself is choosing the file and keeps it.
// It runs as a hook because the generic PATCH narrows its update map to the
// fields the request sent, which drops anything a DtoToMap converter adds.
type InlineContentEditHook struct {
	inlineToFile map[string]string
	hooks.BaseHook
}

func NewScenarioStepInlineContentEditHook() *InlineContentEditHook {
	return newInlineContentEditHook("ScenarioStep", map[string]string{
		"text_content":      "text_file_id",
		"hint_content":      "hint_file_id",
		"verify_script":     "verify_script_id",
		"background_script": "background_script_id",
		"foreground_script": "foreground_script_id",
	})
}

func NewScenarioInlineContentEditHook() *InlineContentEditHook {
	return newInlineContentEditHook("Scenario", map[string]string{
		"intro_text":   "intro_file_id",
		"finish_text":  "finish_file_id",
		"setup_script": "setup_script_id",
	})
}

func newInlineContentEditHook(entity string, inlineToFile map[string]string) *InlineContentEditHook {
	return &InlineContentEditHook{
		inlineToFile: inlineToFile,
		BaseHook: hooks.BaseHook{
			Name:       "inline_content_edit_" + entity,
			EntityName: entity,
			HookTypes:  []hooks.HookType{hooks.BeforeUpdate},
			Enabled:    true,
			Priority:   20,
		},
	}
}

func (h *InlineContentEditHook) Execute(ctx *hooks.HookContext) error {
	updates, ok := ctx.NewEntity.(map[string]any)
	if !ok {
		return nil
	}
	for inline, file := range h.inlineToFile {
		if _, edited := updates[inline]; !edited {
			continue
		}
		if _, chosen := updates[file]; !chosen {
			updates[file] = nil
		}
	}
	return nil
}
