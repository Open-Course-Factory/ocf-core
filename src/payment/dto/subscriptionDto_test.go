package dto

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSubscriptionPlanOutput_MapstructureTag_CommandHistoryRetentionDays verifies that
// the CommandHistoryRetentionDays field on SubscriptionPlanOutput has a `mapstructure`
// struct tag. Without this tag, mapstructure-based decoding (used by the generic entity
// framework) will silently skip this field, resulting in the value always being 0 in
// API responses even when the database has a non-zero value.
//
// The Create and Edit DTOs already have the mapstructure tag; this test ensures the
// Output DTO is consistent.
func TestSubscriptionPlanOutput_MapstructureTag_CommandHistoryRetentionDays(t *testing.T) {
	outputType := reflect.TypeOf(SubscriptionPlanOutput{})

	field, found := outputType.FieldByName("CommandHistoryRetentionDays")
	assert.True(t, found, "SubscriptionPlanOutput should have a CommandHistoryRetentionDays field")

	// Verify json tag exists (sanity check - this should already pass)
	jsonTag := field.Tag.Get("json")
	assert.Equal(t, "command_history_retention_days", jsonTag,
		"json tag should be 'command_history_retention_days'")

	// Verify mapstructure tag exists - this should FAIL because the tag is missing
	mapstructureTag := field.Tag.Get("mapstructure")
	assert.Equal(t, "command_history_retention_days", mapstructureTag,
		"SubscriptionPlanOutput.CommandHistoryRetentionDays must have mapstructure tag "+
			"'command_history_retention_days' for generic entity framework decoding; "+
			"CreateSubscriptionPlanInput and UpdateSubscriptionPlanInput already have it")
}

// TestSubscriptionPlanOutput_MapstructureTag_ConsistencyWithCreateDTO verifies that
// CommandHistoryRetentionDays in the Output DTO has the same mapstructure tag value
// as the Create DTO. This catches the inconsistency where Create/Edit DTOs have the
// tag but Output does not.
func TestSubscriptionPlanOutput_MapstructureTag_ConsistencyWithCreateDTO(t *testing.T) {
	createType := reflect.TypeOf(CreateSubscriptionPlanInput{})
	outputType := reflect.TypeOf(SubscriptionPlanOutput{})

	fieldName := "CommandHistoryRetentionDays"

	createField, found := createType.FieldByName(fieldName)
	assert.True(t, found, "CreateSubscriptionPlanInput should have %s field", fieldName)

	outputField, found := outputType.FieldByName(fieldName)
	assert.True(t, found, "SubscriptionPlanOutput should have %s field", fieldName)

	createMapTag := createField.Tag.Get("mapstructure")
	assert.Equal(t, "command_history_retention_days", createMapTag,
		"CreateSubscriptionPlanInput.%s should have mapstructure tag (sanity check)", fieldName)

	outputMapTag := outputField.Tag.Get("mapstructure")
	assert.Equal(t, createMapTag, outputMapTag,
		"SubscriptionPlanOutput.%s mapstructure tag should match CreateSubscriptionPlanInput.%s tag '%s'",
		fieldName, fieldName, createMapTag)
}
