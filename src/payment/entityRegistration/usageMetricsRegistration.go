package registration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// UsageMetricsRegistration
type UsageMetricsRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (u UsageMetricsRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "usage-metrics",
		EntityName: "Session",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer toutes les métriques d'utilisation",
			Description: "Retourne la liste de toutes les métriques d'utilisation disponibles",
			Tags:        []string{"usage-metrics"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer une métrique d'utilisation",
			Description: "Retourne les détails complets d'une métrique d'utilisation spécifique",
			Tags:        []string{"usage-metrics"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour une métrique d'utilisation",
			Description: "Modifie une métrique d'utilisation existante",
			Tags:        []string{"usage-metrics"},
			Security:    true,
		},
	}
}

func (u UsageMetricsRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		metrics := ptr.(*models.UsageMetrics)

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
	})
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
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
