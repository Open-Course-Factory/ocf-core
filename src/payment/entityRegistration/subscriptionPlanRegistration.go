package registration

import (
	"net/http"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

func RegisterSubscriptionPlan(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.SubscriptionPlan, dto.CreateSubscriptionPlanInput, dto.UpdateSubscriptionPlanInput, dto.SubscriptionPlanOutput](
		service,
		"SubscriptionPlan",
		entityManagementInterfaces.TypedEntityRegistration[models.SubscriptionPlan, dto.CreateSubscriptionPlanInput, dto.UpdateSubscriptionPlanInput, dto.SubscriptionPlanOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.SubscriptionPlan, dto.CreateSubscriptionPlanInput, dto.UpdateSubscriptionPlanInput, dto.SubscriptionPlanOutput]{
				ModelToDto: func(plan *models.SubscriptionPlan) (dto.SubscriptionPlanOutput, error) {
					// Convert model PricingTiers to DTO PricingTiers
					pricingTiers := make([]dto.PricingTier, len(plan.PricingTiers))
					for i, tier := range plan.PricingTiers {
						pricingTiers[i] = dto.PricingTier{
							MinQuantity: tier.MinQuantity,
							MaxQuantity: tier.MaxQuantity,
							UnitAmount:  tier.UnitAmount,
							Description: tier.Description,
						}
					}

					return dto.SubscriptionPlanOutput{
						ID:                 plan.ID,
						Name:               plan.Name,
						Description:        plan.Description,
						Priority:           plan.Priority,
						StripeProductID:    plan.StripeProductID,
						StripePriceID:      plan.StripePriceID,
						PriceAmount:        plan.PriceAmount,
						Currency:           plan.Currency,
						BillingInterval:    plan.BillingInterval,
						TrialDays:          plan.TrialDays,
						Features:           plan.Features,
						MaxConcurrentUsers: plan.MaxConcurrentUsers,
						MaxCourses:         plan.MaxCourses,
						IsActive:           plan.IsActive,
						RequiredRole:       plan.RequiredRole,
						CreatedAt:          plan.CreatedAt,
						UpdatedAt:          plan.UpdatedAt,

						// Terminal-specific limits
						MaxSessionDurationMinutes: plan.MaxSessionDurationMinutes,
						MaxConcurrentTerminals:    plan.MaxConcurrentTerminals,
						AllowedMachineSizes:       plan.AllowedMachineSizes,
						NetworkAccessEnabled:      plan.NetworkAccessEnabled,
						DataPersistenceEnabled:    plan.DataPersistenceEnabled,
						DataPersistenceGB:         plan.DataPersistenceGB,
						AllowedTemplates:          plan.AllowedTemplates,
						AllowedBackends:           plan.AllowedBackends,
						DefaultBackend:            plan.DefaultBackend,

						// Command history
						CommandHistoryRetentionDays: plan.CommandHistoryRetentionDays,

						// Planned features
						PlannedFeatures: plan.PlannedFeatures,

						// Tiered pricing
						UseTieredPricing: plan.UseTieredPricing,
						PricingTiers:     pricingTiers,
					}, nil
				},
				DtoToModel: func(input dto.CreateSubscriptionPlanInput) *models.SubscriptionPlan {
					isActive := true
					if input.IsActive != nil {
						isActive = *input.IsActive
					}
					return &models.SubscriptionPlan{
						Name:                       input.Name,
						Description:                input.Description,
						PriceAmount:                input.PriceAmount,
						Currency:                   input.Currency,
						BillingInterval:            input.BillingInterval,
						TrialDays:                  input.TrialDays,
						Features:                   input.Features,
						MaxConcurrentUsers:         input.MaxConcurrentUsers,
						MaxCourses:                 input.MaxCourses,
						RequiredRole:               input.RequiredRole,
						MaxSessionDurationMinutes:  input.MaxSessionDurationMinutes,
						MaxConcurrentTerminals:     input.MaxConcurrentTerminals,
						AllowedMachineSizes:        input.AllowedMachineSizes,
						NetworkAccessEnabled:       input.NetworkAccessEnabled,
						DataPersistenceEnabled:     input.DataPersistenceEnabled,
						DataPersistenceGB:          input.DataPersistenceGB,
						AllowedTemplates:           input.AllowedTemplates,
						AllowedBackends:            input.AllowedBackends,
						DefaultBackend:             input.DefaultBackend,
						CommandHistoryRetentionDays: input.CommandHistoryRetentionDays,
						Priority:                   input.Priority,
						IsActive:                   isActive,
					}
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
				Tag:        "subscription-plans",
				EntityName: "SubscriptionPlan",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Récupérer tous les plans d'abonnement",
					Description: "Retourne la liste de tous les plans d'abonnement disponibles avec leurs tarifs, fonctionnalités et limites d'usage",
					Tags:        []string{"subscription-plans"},
					Security:    false,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Récupérer un plan d'abonnement",
					Description: "Retourne les détails complets d'un plan d'abonnement spécifique, incluant les prix Stripe et les fonctionnalités",
					Tags:        []string{"subscription-plans"},
					Security:    false,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Créer un plan d'abonnement",
					Description: "Crée un nouveau plan d'abonnement et génère automatiquement les produits et prix associés dans Stripe (Administrateurs seulement)",
					Tags:        []string{"subscription-plans"},
					Security:    true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Mettre à jour un plan d'abonnement",
					Description: "Modifie un plan d'abonnement existant et synchronise les changements avec Stripe (Administrateurs seulement)",
					Tags:        []string{"subscription-plans"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Désactiver un plan d'abonnement",
					Description: "Désactive un plan d'abonnement (les abonnements existants continuent mais plus de nouveaux abonnements possibles)",
					Tags:        []string{"subscription-plans"},
					Security:    true,
				},
			},
		},
	)
}
