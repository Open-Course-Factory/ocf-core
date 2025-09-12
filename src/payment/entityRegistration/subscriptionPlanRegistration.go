// src/payment/entityRegistration/subscriptionRegistration.go
package registration

import (
	"net/http"
	"reflect"
	authModels "soli/formations/src/auth/models"
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
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return subscriptionPlanPtrModelToOutput(input.(*models.SubscriptionPlan))
	} else {
		return subscriptionPlanValueModelToOutput(input.(models.SubscriptionPlan))
	}
}

func subscriptionPlanPtrModelToOutput(plan *models.SubscriptionPlan) (*dto.SubscriptionPlanOutput, error) {
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
		MaxLabSessions:     plan.MaxLabSessions,
		IsActive:           plan.IsActive,
		RequiredRole:       plan.RequiredRole,
		CreatedAt:          plan.CreatedAt,
		UpdatedAt:          plan.UpdatedAt,
	}, nil
}

func subscriptionPlanValueModelToOutput(plan models.SubscriptionPlan) (*dto.SubscriptionPlanOutput, error) {
	return subscriptionPlanPtrModelToOutput(&plan)
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
		MaxLabSessions:     planInput.MaxLabSessions,
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
	roleMap[string(authModels.GroupManager)] = "(" + http.MethodGet + ")"
	roleMap[string(authModels.Member)] = "(" + http.MethodGet + ")" // Pour voir les plans disponibles

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
