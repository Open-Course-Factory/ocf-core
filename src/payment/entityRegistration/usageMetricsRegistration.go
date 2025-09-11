package registration

import (
	"net/http"
	"reflect"
	authModels "soli/formations/src/auth/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// UsageMetricsRegistration
type UsageMetricsRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (u UsageMetricsRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return usageMetricsPtrModelToOutput(input.(*models.UsageMetrics))
	} else {
		return usageMetricsValueModelToOutput(input.(models.UsageMetrics))
	}
}

func usageMetricsPtrModelToOutput(metrics *models.UsageMetrics) (*dto.UsageMetricsOutput, error) {
	var usagePercent float64
	if metrics.LimitValue > 0 {
		usagePercent = (float64(metrics.CurrentValue) / float64(metrics.LimitValue)) * 100
	} else {
		usagePercent = 0 // Unlimited
	}

	return &dto.UsageMetricsOutput{
		ID:           metrics.ID,
		UserID:       metrics.UserID,
		MetricType:   metrics.MetricType,
		CurrentValue: metrics.CurrentValue,
		LimitValue:   metrics.LimitValue,
		PeriodStart:  metrics.PeriodStart,
		PeriodEnd:    metrics.PeriodEnd,
		LastUpdated:  metrics.LastUpdated,
		UsagePercent: usagePercent,
	}, nil
}

func usageMetricsValueModelToOutput(metrics models.UsageMetrics) (*dto.UsageMetricsOutput, error) {
	return usageMetricsPtrModelToOutput(&metrics)
}

func (u UsageMetricsRegistration) EntityInputDtoToEntityModel(input any) any {
	// Les métriques sont généralement créées automatiquement
	return &models.UsageMetrics{}
}

func (u UsageMetricsRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.UsageMetrics{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: u.EntityModelToEntityOutput,
			DtoToModel: u.EntityInputDtoToEntityModel,
			DtoToMap:   u.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.UsageMetricsOutput{},
			OutputDto:      dto.UsageMetricsOutput{},
			InputEditDto:   dto.UsageMetricsOutput{},
		},
	}
}

func (u UsageMetricsRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	// Les utilisateurs peuvent voir leurs propres métriques
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
