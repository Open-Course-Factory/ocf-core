// tests/payment/planFeatureDto_test.go
// Tests for PlanFeature DTO validation.
// Bug-exposing tests: the DTOs currently lack enum validation on Category and ValueType,
// allowing arbitrary strings to be submitted.
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
// BUG-EXPOSING TESTS (should FAIL with current code -- item #12)
// ==========================================

// TestCreatePlanFeatureInput_InvalidCategory_ReturnsError tests that submitting
// a PlanFeature with an arbitrary category value is rejected.
// BUG: Currently, the Category field in CreatePlanFeatureInput has no enum validation
// (`binding:"required,oneof=capabilities machine_sizes terminal_limits course_limits"`),
// so any string is accepted.
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
// a PlanFeature with an arbitrary value_type is rejected.
// BUG: Currently, the ValueType field has no binding validation at all,
// so any string including "script" or "exec" is accepted.
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
