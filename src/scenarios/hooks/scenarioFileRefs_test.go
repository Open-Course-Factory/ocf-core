package scenarioHooks

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"

	"soli/formations/src/scenarios/models"
)

// A file-id column missing from stepFileRefs / scenarioFileRefs is a column
// the #516 rule never checks. Every *uuid.UUID field named …ScriptID or
// …FileID must be listed under its gorm column, with a getter that reads that
// very field.
func TestFileRefColumns_ListEveryFileIdField(t *testing.T) {
	t.Run("ScenarioStep", func(t *testing.T) {
		assertListsEveryFileRef(t, &models.ScenarioStep{}, stepFileRefs)
	})
	t.Run("Scenario", func(t *testing.T) {
		assertListsEveryFileRef(t, &models.Scenario{}, scenarioFileRefs)
	})
}

func assertListsEveryFileRef[M any](t *testing.T, model *M, listed map[string]func(*M) *uuid.UUID) {
	t.Helper()
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)

	uuidPtr := reflect.TypeOf((*uuid.UUID)(nil))
	typ := reflect.TypeOf(model).Elem()
	found := 0
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type != uuidPtr || !(strings.HasSuffix(f.Name, "ScriptID") || strings.HasSuffix(f.Name, "FileID")) {
			continue
		}
		found++
		field := s.LookUpField(f.Name)
		require.NotNil(t, field, "%s has no gorm column", f.Name)

		get, ok := listed[field.DBName]
		if !assert.True(t, ok, "%s (column %s) is a file reference the #516 rule does not check", f.Name, field.DBName) {
			continue
		}
		var m M
		id := uuid.New()
		reflect.ValueOf(&m).Elem().Field(i).Set(reflect.ValueOf(&id))
		got := get(&m)
		if assert.NotNil(t, got, "the getter for %s does not read %s", field.DBName, f.Name) {
			assert.Equal(t, id, *got, "the getter for %s does not read %s", field.DBName, f.Name)
		}
	}
	assert.Equal(t, len(listed), found, "every listed column must match a file-id field")
}
