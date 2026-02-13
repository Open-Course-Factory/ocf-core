package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

func RegisterUsageMetrics(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.UsageMetrics, dto.UsageMetricsOutput, dto.UsageMetricsOutput, dto.UsageMetricsOutput](
		service,
		"UsageMetrics",
		entityManagementInterfaces.TypedEntityRegistration[models.UsageMetrics, dto.UsageMetricsOutput, dto.UsageMetricsOutput, dto.UsageMetricsOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.UsageMetrics, dto.UsageMetricsOutput, dto.UsageMetricsOutput, dto.UsageMetricsOutput]{
				ModelToDto: func(metrics *models.UsageMetrics) (dto.UsageMetricsOutput, error) {
					var usagePercent float64
					if metrics.LimitValue > 0 {
						usagePercent = (float64(metrics.CurrentValue) / float64(metrics.LimitValue)) * 100
					} else {
						usagePercent = 0
					}

					return dto.UsageMetricsOutput{
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
				},
				DtoToModel: func(input dto.UsageMetricsOutput) *models.UsageMetrics {
					// Metrics are created automatically
					return &models.UsageMetrics{}
				},
				DtoToMap: nil,
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
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
			},
		},
	)
}
