// src/payment/entityRegistration/subscriptionRegistration.go
package registration

import (
	"net/http"
	authModels "soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/payment/dto"
	"soli/formations/src/payment/models"
)

// SubscriptionPlanRegistration
type SubscriptionPlanRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s SubscriptionPlanRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "subscription-plans",
		EntityName: "SubscriptionPlan",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les plans d'abonnement",
			Description: "Retourne la liste de tous les plans d'abonnement disponibles avec leurs tarifs, fonctionnalités et limites d'usage",
			Tags:        []string{"subscription-plans"},
			Security:    false, // Accessible publiquement pour que les utilisateurs voient les plans
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un plan d'abonnement",
			Description: "Retourne les détails complets d'un plan d'abonnement spécifique, incluant les prix Stripe et les fonctionnalités",
			Tags:        []string{"subscription-plans"},
			Security:    false, // Accessible publiquement
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer un plan d'abonnement",
			Description: "Crée un nouveau plan d'abonnement et génère automatiquement les produits et prix associés dans Stripe (Administrateurs seulement)",
			Tags:        []string{"subscription-plans"},
			Security:    true, // Accès admin uniquement
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour un plan d'abonnement",
			Description: "Modifie un plan d'abonnement existant et synchronise les changements avec Stripe (Administrateurs seulement)",
			Tags:        []string{"subscription-plans"},
			Security:    true, // Accès admin uniquement
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Désactiver un plan d'abonnement",
			Description: "Désactive un plan d'abonnement (les abonnements existants continuent mais plus de nouveaux abonnements possibles)",
			Tags:        []string{"subscription-plans"},
			Security:    true, // Accès admin uniquement
		},
	}
}

func (s SubscriptionPlanRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		plan := ptr.(*models.SubscriptionPlan)

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

		return &dto.SubscriptionPlanOutput{
			ID:                 plan.ID,
			Name:               plan.Name,
			Description:        plan.Description,
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

			// Planned features
			PlannedFeatures: plan.PlannedFeatures,

			// Tiered pricing
			UseTieredPricing: plan.UseTieredPricing,
			PricingTiers:     pricingTiers,
		}, nil
	})
}

func (s SubscriptionPlanRegistration) EntityInputDtoToEntityModel(input any) any {
	planInput := input.(dto.CreateSubscriptionPlanInput)
	return &models.SubscriptionPlan{
		Name:               planInput.Name,
		Description:        planInput.Description,
		PriceAmount:        planInput.PriceAmount,
		Currency:           planInput.Currency,
		BillingInterval:    planInput.BillingInterval,
		TrialDays:          planInput.TrialDays,
		Features:           planInput.Features,
		MaxConcurrentUsers: planInput.MaxConcurrentUsers,
		MaxCourses:         planInput.MaxCourses,
		RequiredRole:       planInput.RequiredRole,
		IsActive:           true,
	}
}

func (s SubscriptionPlanRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.SubscriptionPlan{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CreateSubscriptionPlanInput{},
			OutputDto:      dto.SubscriptionPlanOutput{},
			InputEditDto:   dto.UpdateSubscriptionPlanInput{},
		},
	}
}

func (s SubscriptionPlanRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)
	// Seuls les admins peuvent gérer les plans d'abonnement
	roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")" // Pour voir les plans disponibles

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
