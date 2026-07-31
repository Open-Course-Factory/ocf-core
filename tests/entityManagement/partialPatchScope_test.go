// tests/entityManagement/partialPatchScope_test.go
//
// #443: a partial PATCH blanked every column the request did not mention.
//
// Observed on a real database: PATCH {"is_catalog": false} on a subscription plan
// returned 204 and wiped name, is_active, priority, both budget fields, the
// session duration and every capability boolean. Only price_amount and the field
// sent survived.
//
// The cause is generic, not plan-specific. EditEntity decodes the sparse request
// into a FULL edit-DTO struct, then converts that whole struct to an update map —
// so fields nobody sent arrive as zero values (nil pointers, empty strings) and
// GORM's Updates(map) writes every key. Every entity goes through this handler.
//
// It stayed invisible because ocf-front's plan editor always sends the complete
// object. Any narrow edit UI, script, or other client destroys the row.
package entityManagement_tests

import (
	"testing"

	controller "soli/formations/src/entityManagement/routes"

	"github.com/stretchr/testify/assert"
)

// patchProbeDto mirrors the shape of a real edit DTO: pointers with omitempty for
// optionals, a plain string, and a differing mapstructure key.
type patchProbeDto struct {
	Name        string `json:"name,omitempty" mapstructure:"name"`
	Priority    *int   `json:"priority,omitempty" mapstructure:"priority"`
	IsActive    *bool  `json:"is_active,omitempty" mapstructure:"is_active"`
	Description string `json:"description,omitempty" mapstructure:"description"`
	MaxCPU      *int   `json:"max_cpu,omitempty" mapstructure:"max_cpu"`
}

// TestRetainRequestedFields_DropsWhatTheCallerNeverSent is the fix: only the
// fields present in the request may reach the update map.
func TestRetainRequestedFields_DropsWhatTheCallerNeverSent(t *testing.T) {
	inbound := map[string]any{"is_active": false}

	// What the current code produces: the whole struct, zero values and all.
	updateMap := map[string]any{
		"name":        "",
		"priority":    nil,
		"is_active":   false,
		"description": "",
		"max_cpu":     nil,
	}

	controller.RetainRequestedFields(patchProbeDto{}, inbound, updateMap)

	assert.Equal(t, map[string]any{"is_active": false}, updateMap,
		"a one-field PATCH must update one column — everything else the caller did not send "+
			"must never reach the UPDATE, or the row is blanked")
}

// TestRetainRequestedFields_KeepsEveryRequestedField guards the other direction:
// filtering must not silently drop a change the caller did make.
func TestRetainRequestedFields_KeepsEveryRequestedField(t *testing.T) {
	inbound := map[string]any{"name": "Renamed", "priority": 20, "max_cpu": 6000}
	updateMap := map[string]any{
		"name":        "Renamed",
		"priority":    20,
		"is_active":   false,
		"description": "",
		"max_cpu":     6000,
	}

	controller.RetainRequestedFields(patchProbeDto{}, inbound, updateMap)

	assert.Equal(t, map[string]any{"name": "Renamed", "priority": 20, "max_cpu": 6000}, updateMap)
}

// TestRetainRequestedFields_PreservesAnExplicitEmptyString — clearing an optional
// text field is a legitimate PATCH, and the value is indistinguishable from the
// zero value, so presence in the request is what decides.
func TestRetainRequestedFields_PreservesAnExplicitEmptyString(t *testing.T) {
	inbound := map[string]any{"description": ""}
	updateMap := map[string]any{"name": "", "description": "", "priority": nil}

	controller.RetainRequestedFields(patchProbeDto{}, inbound, updateMap)

	assert.Equal(t, map[string]any{"description": ""}, updateMap,
		"an explicitly sent empty string must still clear the field")
}

// TestRetainRequestedFields_PreservesAnExplicitFalse — the same argument for
// booleans, which is precisely the case that blanked the plans: is_catalog=false
// is a real change, not an absent field.
func TestRetainRequestedFields_PreservesAnExplicitFalse(t *testing.T) {
	inbound := map[string]any{"is_active": false}
	updateMap := map[string]any{"is_active": false, "name": ""}

	controller.RetainRequestedFields(patchProbeDto{}, inbound, updateMap)

	assert.Equal(t, map[string]any{"is_active": false}, updateMap)
}

// TestRetainRequestedFields_LeavesUnattributableKeysAlone keeps the filter
// conservative: a converter may inject a key that belongs to no DTO field, and
// dropping it would break whatever it was for.
func TestRetainRequestedFields_LeavesUnattributableKeysAlone(t *testing.T) {
	inbound := map[string]any{"name": "X"}
	updateMap := map[string]any{"name": "X", "computed_by_converter": 42, "priority": nil}

	controller.RetainRequestedFields(patchProbeDto{}, inbound, updateMap)

	assert.Equal(t, 42, updateMap["computed_by_converter"],
		"a key that maps to no DTO field is not ours to remove")
	assert.NotContains(t, updateMap, "priority")
}

// TestRetainRequestedFields_EmptyRequestChangesNothing — the empty-input case
// this codebase keeps getting wrong. An empty PATCH must be a no-op, not a wipe.
func TestRetainRequestedFields_EmptyRequestChangesNothing(t *testing.T) {
	updateMap := map[string]any{"name": "", "priority": nil, "is_active": false}

	controller.RetainRequestedFields(patchProbeDto{}, map[string]any{}, updateMap)

	assert.Empty(t, updateMap, "a PATCH that asks for nothing must update nothing")
}
