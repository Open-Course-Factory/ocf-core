// tests/payment/planFeatureDto_test.go
// Tests for PlanFeature DTO validation.
// Regression tests: verify that Category and ValueType fields have proper enum validation
// to prevent arbitrary strings from being submitted.
package payment_tests

import (
	"reflect"
	"strings"
	"testing"

	"soli/formations/src/payment/dto"

	"github.com/stretchr/testify/assert"
)

// getBindingTag extracts the "binding" struct tag from a field by name.
func getBindingTag(structType reflect.Type, fieldName string) string {
	field, found := structType.FieldByName(fieldName)
	if !found {
		return ""
	}
	return field.Tag.Get("binding")
}

// ==========================================
// Regression tests — enum validation on Category and ValueType
// ==========================================

// TestCreatePlanFeatureInput_InvalidCategory_ReturnsError tests that submitting
// a PlanFeature with an arbitrary category value is rejected by the oneof validation.
func TestCreatePlanFeatureInput_InvalidCategory_ReturnsError(t *testing.T) {
	dtoType := reflect.TypeOf(dto.CreatePlanFeatureInput{})
	bindingTag := getBindingTag(dtoType, "Category")

	// The Category field should have a binding tag that includes "oneof" validation
	// with the allowed categories.
	assert.True(t, strings.Contains(bindingTag, "oneof"),
		"Category field binding tag should include 'oneof' validation to restrict "+
			"values to valid categories. Current tag: %q", bindingTag)

	// If oneof is present, verify it includes all expected categories
	if strings.Contains(bindingTag, "oneof") {
		assert.Contains(t, bindingTag, "capabilities",
			"Category oneof should include 'capabilities'")
		assert.Contains(t, bindingTag, "machine_sizes",
			"Category oneof should include 'machine_sizes'")
		assert.Contains(t, bindingTag, "terminal_limits",
			"Category oneof should include 'terminal_limits'")
		assert.Contains(t, bindingTag, "course_limits",
			"Category oneof should include 'course_limits'")
	}
}

// TestCreatePlanFeatureInput_InvalidValueType_ReturnsError tests that submitting
// a PlanFeature with an arbitrary value_type is rejected by the oneof validation.
func TestCreatePlanFeatureInput_InvalidValueType_ReturnsError(t *testing.T) {
	dtoType := reflect.TypeOf(dto.CreatePlanFeatureInput{})
	bindingTag := getBindingTag(dtoType, "ValueType")

	// The ValueType field should have a binding tag that includes "oneof" validation
	// with the allowed value types (boolean, number, string).
	assert.True(t, strings.Contains(bindingTag, "oneof"),
		"ValueType field binding tag should include 'oneof' validation to restrict "+
			"values to valid types. Current tag: %q", bindingTag)

	// If oneof is present, verify it includes all expected types
	if strings.Contains(bindingTag, "oneof") {
		assert.Contains(t, bindingTag, "boolean",
			"ValueType oneof should include 'boolean'")
		assert.Contains(t, bindingTag, "number",
			"ValueType oneof should include 'number'")
		assert.Contains(t, bindingTag, "string",
			"ValueType oneof should include 'string'")
	}
}
