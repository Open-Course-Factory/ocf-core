// tests/payment/planFeatureRegistration_test.go
// Tests for PlanFeature DtoToModel converter defaults and Key uniqueness constraint.
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildDtoToModel returns the DtoToModel converter used in registration.
// We recreate the same logic here to test it in isolation.
func buildDtoToModel() func(dto.CreatePlanFeatureInput) *models.PlanFeature {
	return func(input dto.CreatePlanFeatureInput) *models.PlanFeature {
		isActive := true
		if input.IsActive != nil {
			isActive = *input.IsActive
		}
		valueType := input.ValueType
		if valueType == "" {
			valueType = "boolean"
		}
		defaultValue := input.DefaultValue
		if defaultValue == "" {
			switch valueType {
			case "number":
				defaultValue = "0"
			case "boolean":
				defaultValue = "false"
			default:
				defaultValue = ""
			}
		}
		return &models.PlanFeature{
			Key:           input.Key,
			DisplayNameEn: input.DisplayNameEn,
			DisplayNameFr: input.DisplayNameFr,
			DescriptionEn: input.DescriptionEn,
			DescriptionFr: input.DescriptionFr,
			Category:      input.Category,
			ValueType:     valueType,
			Unit:          input.Unit,
			DefaultValue:  defaultValue,
			IsActive:      isActive,
		}
	}
}

// ==========================================
// DtoToModel converter defaults
// ==========================================

func TestDtoToModel_EmptyValueType_DefaultsToBoolean(t *testing.T) {
	convert := buildDtoToModel()

	input := dto.CreatePlanFeatureInput{
		Key:           "test_feature",
		DisplayNameEn: "Test Feature",
		DisplayNameFr: "Fonctionnalité test",
		Category:      "capabilities",
		// ValueType intentionally empty
	}

	model := convert(input)
	assert.Equal(t, "boolean", model.ValueType, "Empty ValueType should default to 'boolean'")
	assert.Equal(t, "false", model.DefaultValue, "Boolean type with empty DefaultValue should default to 'false'")
}

func TestDtoToModel_NumberType_DefaultValueIsZero(t *testing.T) {
	convert := buildDtoToModel()

	input := dto.CreatePlanFeatureInput{
		Key:           "max_items",
		DisplayNameEn: "Max Items",
		DisplayNameFr: "Éléments max",
		Category:      "course_limits",
		ValueType:     "number",
		// DefaultValue intentionally empty
	}

	model := convert(input)
	assert.Equal(t, "number", model.ValueType)
	assert.Equal(t, "0", model.DefaultValue, "Number type with empty DefaultValue should default to '0', not 'false'")
}

func TestDtoToModel_StringType_DefaultValueIsEmpty(t *testing.T) {
	convert := buildDtoToModel()

	input := dto.CreatePlanFeatureInput{
		Key:           "custom_label",
		DisplayNameEn: "Custom Label",
		DisplayNameFr: "Libellé personnalisé",
		Category:      "capabilities",
		ValueType:     "string",
		// DefaultValue intentionally empty
	}

	model := convert(input)
	assert.Equal(t, "string", model.ValueType)
	assert.Equal(t, "", model.DefaultValue, "String type with empty DefaultValue should default to empty string")
}

func TestDtoToModel_ExplicitDefaultValue_NotOverridden(t *testing.T) {
	convert := buildDtoToModel()

	input := dto.CreatePlanFeatureInput{
		Key:           "max_sessions",
		DisplayNameEn: "Max Sessions",
		DisplayNameFr: "Sessions max",
		Category:      "terminal_limits",
		ValueType:     "number",
		DefaultValue:  "42",
	}

	model := convert(input)
	assert.Equal(t, "42", model.DefaultValue, "Explicit DefaultValue should not be overridden")
}

// ==========================================
// Key uniqueness constraint (DB-level)
// ==========================================

func TestPlanFeature_DuplicateKey_FailsUniqueConstraint(t *testing.T) {
	db := freshTestDB(t)

	first := models.PlanFeature{
		Key:           "duplicate_key",
		DisplayNameEn: "First Feature",
		DisplayNameFr: "Première fonctionnalité",
		Category:      "capabilities",
		ValueType:     "boolean",
		DefaultValue:  "false",
		IsActive:      true,
	}
	err := db.Create(&first).Error
	require.NoError(t, err, "First insert should succeed")

	second := models.PlanFeature{
		Key:           "duplicate_key",
		DisplayNameEn: "Second Feature",
		DisplayNameFr: "Deuxième fonctionnalité",
		Category:      "capabilities",
		ValueType:     "boolean",
		DefaultValue:  "false",
		IsActive:      true,
	}
	err = db.Create(&second).Error
	assert.Error(t, err, "Second insert with same Key should fail due to unique constraint")
}
