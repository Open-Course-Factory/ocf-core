// tests/entityManagement/json_serializer_test.go
package entityManagement_tests

import (
	"encoding/json"
	"testing"

	controller "soli/formations/src/entityManagement/routes"
	paymentModels "soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
)

// --- Test structs used only in this file ---

type simpleSerializedModel struct {
	Name     string   `gorm:"type:varchar(100)" json:"name"`
	Features []string `gorm:"serializer:json" json:"features"`
}

type modelWithoutSerializer struct {
	Name string   `gorm:"type:varchar(100)" json:"name"`
	Tags []string `gorm:"type:text[]" json:"tags"`
}

type modelWithMapField struct {
	Config map[string]any `gorm:"serializer:json" json:"config"`
}

type embeddedBase struct {
	Metadata []string `gorm:"serializer:json" json:"metadata"`
}

type modelWithEmbeddedStruct struct {
	embeddedBase
	Title string `gorm:"type:varchar(100)" json:"title"`
}

type modelWithColumnTag struct {
	Data []string `gorm:"column:custom_name;serializer:json"`
}

type modelWithNoJsonTag struct {
	FrontMatterContent []string `gorm:"serializer:json"`
}

// TestJsonEncodeSerializedFields_SliceField tests that a []string value
// for a field with gorm:"serializer:json" is JSON-encoded to a string.
func TestJsonEncodeSerializedFields_SliceField(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := simpleSerializedModel{}
	updateMap := map[string]any{
		"features": []string{"terminal", "ssh", "vnc"},
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	// The slice should have been JSON-encoded to a string
	encoded, ok := updateMap["features"].(string)
	assert.True(t, ok, "features should be a string after encoding")

	var decoded []string
	err := json.Unmarshal([]byte(encoded), &decoded)
	assert.NoError(t, err, "encoded value should be valid JSON")
	assert.Equal(t, []string{"terminal", "ssh", "vnc"}, decoded)
}

// TestJsonEncodeSerializedFields_NilValue tests that nil values in the map
// are skipped and not JSON-encoded.
func TestJsonEncodeSerializedFields_NilValue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := simpleSerializedModel{}
	updateMap := map[string]any{
		"features": nil,
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	// nil should remain nil — not encoded
	assert.Nil(t, updateMap["features"], "nil value should remain nil")
}

// TestJsonEncodeSerializedFields_NonSerializedField tests that fields WITHOUT
// gorm:"serializer:json" are left as-is in the map, even if they are slices.
func TestJsonEncodeSerializedFields_NonSerializedField(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := modelWithoutSerializer{}
	originalTags := []string{"linux", "devops"}
	updateMap := map[string]any{
		"tags": originalTags,
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	// tags should NOT be encoded — it has no serializer:json tag
	result, ok := updateMap["tags"].([]string)
	assert.True(t, ok, "tags should still be a []string, not encoded to a string")
	assert.Equal(t, originalTags, result)
}

// TestJsonEncodeSerializedFields_MapValue tests that a map[string]any value
// for a serializer:json field is JSON-encoded.
func TestJsonEncodeSerializedFields_MapValue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := modelWithMapField{}
	updateMap := map[string]any{
		"config": map[string]any{
			"timeout": 30,
			"enabled": true,
		},
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	encoded, ok := updateMap["config"].(string)
	assert.True(t, ok, "config should be a string after encoding")

	var decoded map[string]any
	err := json.Unmarshal([]byte(encoded), &decoded)
	assert.NoError(t, err, "encoded value should be valid JSON")
	assert.Equal(t, float64(30), decoded["timeout"])
	assert.Equal(t, true, decoded["enabled"])
}

// TestJsonEncodeSerializedFields_EmbeddedStruct tests that serializer:json fields
// inside embedded (anonymous) structs are found and encoded.
func TestJsonEncodeSerializedFields_EmbeddedStruct(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := modelWithEmbeddedStruct{}
	updateMap := map[string]any{
		"metadata": []string{"key1", "key2"},
		"title":    "Test Title",
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	// metadata (from embedded struct) should be encoded
	encoded, ok := updateMap["metadata"].(string)
	assert.True(t, ok, "metadata should be a string after encoding")

	var decoded []string
	err := json.Unmarshal([]byte(encoded), &decoded)
	assert.NoError(t, err)
	assert.Equal(t, []string{"key1", "key2"}, decoded)

	// title should remain unchanged (no serializer:json tag)
	assert.Equal(t, "Test Title", updateMap["title"])
}

// TestJsonEncodeSerializedFields_PointerModel tests that passing a pointer
// to the model struct works the same as passing a value.
func TestJsonEncodeSerializedFields_PointerModel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := &simpleSerializedModel{}
	updateMap := map[string]any{
		"features": []string{"terminal", "ssh"},
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	encoded, ok := updateMap["features"].(string)
	assert.True(t, ok, "features should be a string after encoding (pointer model)")

	var decoded []string
	err := json.Unmarshal([]byte(encoded), &decoded)
	assert.NoError(t, err)
	assert.Equal(t, []string{"terminal", "ssh"}, decoded)
}

// TestJsonEncodeSerializedFields_ColumnTag tests that a field with
// gorm:"column:custom_name;serializer:json" uses the custom column name
// as the map key.
func TestJsonEncodeSerializedFields_ColumnTag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := modelWithColumnTag{}
	updateMap := map[string]any{
		"custom_name": []string{"a", "b", "c"},
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	encoded, ok := updateMap["custom_name"].(string)
	assert.True(t, ok, "custom_name should be a string after encoding")

	var decoded []string
	err := json.Unmarshal([]byte(encoded), &decoded)
	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, decoded)
}

// TestJsonEncodeSerializedFields_NoJsonTag tests that a field with
// gorm:"serializer:json" but no json tag falls back to snake_case
// of the Go field name.
func TestJsonEncodeSerializedFields_NoJsonTag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := modelWithNoJsonTag{}
	updateMap := map[string]any{
		"front_matter_content": []string{"intro", "summary"},
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	encoded, ok := updateMap["front_matter_content"].(string)
	assert.True(t, ok, "front_matter_content should be a string after encoding")

	var decoded []string
	err := json.Unmarshal([]byte(encoded), &decoded)
	assert.NoError(t, err)
	assert.Equal(t, []string{"intro", "summary"}, decoded)
}

// TestCamelToSnake tests the CamelToSnake helper for various inputs.
func TestCamelToSnake(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"FrontMatterContent", "front_matter_content"},
		{"Toc", "toc"},
		{"ID", "i_d"},
		{"MaxConcurrentUsers", "max_concurrent_users"},
		{"", ""},
		{"name", "name"},
		{"A", "a"},
		{"ABCDef", "a_b_c_def"},
		{"IsActive", "is_active"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := controller.CamelToSnake(tt.input)
			assert.Equal(t, tt.expected, result, "CamelToSnake(%q)", tt.input)
		})
	}
}

// TestJsonEncodeSerializedFields_RealSubscriptionPlan tests with the actual
// SubscriptionPlan model from the payment module, ensuring real-world fields
// like features, allowed_machine_sizes, allowed_templates, etc. are all encoded.
func TestJsonEncodeSerializedFields_RealSubscriptionPlan(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := paymentModels.SubscriptionPlan{}
	updateMap := map[string]any{
		"name":                  "Pro Plan",
		"features":              []string{"terminal", "ssh", "vnc"},
		"allowed_machine_sizes": []string{"XS", "S", "M"},
		"allowed_templates":     []string{"tmpl-1", "tmpl-2"},
		"allowed_backends":      []string{"backend-a"},
		"planned_features":      []string{"gpu-support"},
		"pricing_tiers": []map[string]any{
			{"min_quantity": 1, "max_quantity": 5, "unit_amount": 1000},
			{"min_quantity": 6, "max_quantity": 0, "unit_amount": 800},
		},
		"max_concurrent_users": 10,
		"is_active":            true,
	}

	controller.JsonEncodeSerializedFields(model, updateMap)

	// All serializer:json fields should be encoded to strings
	serializedFields := []string{
		"features",
		"allowed_machine_sizes",
		"allowed_templates",
		"allowed_backends",
		"planned_features",
		"pricing_tiers",
	}
	for _, fieldName := range serializedFields {
		val, ok := updateMap[fieldName].(string)
		assert.True(t, ok, "%s should be a string after encoding, got %T", fieldName, updateMap[fieldName])
		assert.True(t, json.Valid([]byte(val)), "%s should contain valid JSON", fieldName)
	}

	// Non-serialized fields should remain unchanged
	assert.Equal(t, "Pro Plan", updateMap["name"], "name should remain a string")
	assert.Equal(t, 10, updateMap["max_concurrent_users"], "max_concurrent_users should remain an int")
	assert.Equal(t, true, updateMap["is_active"], "is_active should remain a bool")

	// Verify specific decoded content
	var features []string
	err := json.Unmarshal([]byte(updateMap["features"].(string)), &features)
	assert.NoError(t, err)
	assert.Equal(t, []string{"terminal", "ssh", "vnc"}, features)

	var sizes []string
	err = json.Unmarshal([]byte(updateMap["allowed_machine_sizes"].(string)), &sizes)
	assert.NoError(t, err)
	assert.Equal(t, []string{"XS", "S", "M"}, sizes)
}

// TestJsonEncodeSerializedFields_NilModel tests that a nil model is handled
// gracefully without panicking.
func TestJsonEncodeSerializedFields_NilModel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	updateMap := map[string]any{
		"features": []string{"a", "b"},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		controller.JsonEncodeSerializedFields(nil, updateMap)
	})

	// Map should be unchanged
	_, ok := updateMap["features"].([]string)
	assert.True(t, ok, "features should remain []string when model is nil")
}

// TestJsonEncodeSerializedFields_NilMap tests that a nil updateMap is handled
// gracefully without panicking.
func TestJsonEncodeSerializedFields_NilMap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	model := simpleSerializedModel{}

	// Should not panic
	assert.NotPanics(t, func() {
		controller.JsonEncodeSerializedFields(model, nil)
	})
}
