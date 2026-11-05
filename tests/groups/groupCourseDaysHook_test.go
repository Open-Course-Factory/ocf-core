package groups_tests

import (
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"soli/formations/src/entityManagement/hooks"
	groupHooks "soli/formations/src/groups/hooks"
)

func TestGroupCourseDaysHook_RefusesWhatIsNotADate(t *testing.T) {
	hook := groupHooks.NewGroupCourseDaysValidationHook()
	execute := func(patch map[string]any) error {
		return hook.Execute(&hooks.HookContext{NewEntity: patch, HookType: hooks.BeforeUpdate})
	}

	assert.NoError(t, execute(map[string]any{"display_name": "Other field"}))
	assert.NoError(t, execute(map[string]any{"course_days": pq.StringArray{}}), "clearing the days is allowed")
	assert.NoError(t, execute(map[string]any{"course_days": pq.StringArray{"2026-03-02"}}))
	assert.Error(t, execute(map[string]any{"course_days": pq.StringArray{"monday"}}))
}
