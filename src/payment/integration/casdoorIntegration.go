// src/payment/integration/casdoorIntegration.go
package integration

import (
	"fmt"
	"log"
	"strings"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/models"
	"soli/formations/src/payment/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"gorm.io/gorm"
)

// CasdoorPaymentIntegration gère l'intégration entre le système de paiement et Casdoor
type CasdoorPaymentIntegration interface {
	// Gestion des rôles basés sur les abonnements
	UpdateUserRoleBasedOnSubscription(userID string) error
	SyncUserRoleWithSubscription(userID string, planRequiredRole string) error

	// Gestion des politiques d'accès
	GrantSubscriptionPermissions(userID string, features []string) error
	RevokeSubscriptionPermissions(userID string) error

	// Gestion des groupes
	AddUserToSubscriptionGroup(userID string, planName string) error
	RemoveUserFromSubscriptionGroups(userID string) error

	// Vérifications
	UserHasSubscriptionRole(userID string, requiredRole string) (bool, error)
	GetUserSubscriptionRole(userID string) (string, error)
}

type casdoorPaymentIntegration struct {
	subscriptionService services.UserSubscriptionService
	db                  *gorm.DB
}

func NewCasdoorPaymentIntegration(db *gorm.DB) CasdoorPaymentIntegration {
	return &casdoorPaymentIntegration{
		subscriptionService: services.NewSubscriptionService(db),
		db:                  db,
	}
}

// UpdateUserRoleBasedOnSubscription met à jour le rôle de l'utilisateur selon son abonnement actif
func (cpi *casdoorPaymentIntegration) UpdateUserRoleBasedOnSubscription(userID string) error {
	// Récupérer l'abonnement actif
	subscription, err := cpi.subscriptionService.GetActiveUserSubscription(userID)
	if err != nil {
		// Pas d'abonnement actif, remettre le rôle de base
		return cpi.setUserToDefaultRole(userID)
	}

	sPlan, errSPlan := cpi.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
	if errSPlan != nil {
		return cpi.setUserToDefaultRole(userID)
	}

	requiredRole := sPlan.RequiredRole
	if requiredRole == "" {
		requiredRole = string(models.Member) // Rôle par défaut
	}

	// Synchroniser le rôle
	return cpi.SyncUserRoleWithSubscription(userID, requiredRole)
}

// SyncUserRoleWithSubscription synchronise le rôle d'un utilisateur avec un plan d'abonnement
func (cpi *casdoorPaymentIntegration) SyncUserRoleWithSubscription(userID string, planRequiredRole string) error {
	// Récupérer l'utilisateur depuis Casdoor
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return fmt.Errorf("failed to get user from Casdoor: %v", err)
	}

	if user == nil {
		return fmt.Errorf("user not found in Casdoor: %s", userID)
	}

	// Supprimer tous les anciens rôles liés aux abonnements
	err = cpi.removeSubscriptionRoles(userID)
	if err != nil {
		log.Printf("Warning: failed to remove old subscription roles for user %s: %v", userID, err)
	}

	// Ajouter le nouveau rôle
	_, err = casdoor.Enforcer.AddGroupingPolicy(userID, planRequiredRole)
	if err != nil {
		return fmt.Errorf("failed to add subscription role %s to user %s: %v", planRequiredRole, userID, err)
	}

	// Mettre à jour le rôle dans Casdoor également
	err = cpi.updateCasdoorUserRole(user, planRequiredRole)
	if err != nil {
		log.Printf("Warning: failed to update Casdoor user role: %v", err)
		// Ne pas faire échouer l'opération pour ça
	}

	log.Printf("Successfully updated user %s to role %s", userID, planRequiredRole)
	return nil
}

// GrantSubscriptionPermissions accorde des permissions basées sur les fonctionnalités de l'abonnement
func (cpi *casdoorPaymentIntegration) GrantSubscriptionPermissions(userID string, features []string) error {
	// Mapper les fonctionnalités vers des permissions spécifiques
	permissions := cpi.mapFeaturesToPermissions(features)

	for resource, actions := range permissions {
		// Supprimer les anciennes permissions pour cette ressource
		_, err := casdoor.Enforcer.RemoveFilteredPolicy(0, userID, resource)
		if err != nil {
			log.Printf("Warning: failed to remove old permissions for %s on %s: %v", userID, resource, err)
		}

		// Ajouter les nouvelles permissions
		_, err = casdoor.Enforcer.AddPolicy(userID, resource, actions)
		if err != nil {
			log.Printf("Warning: failed to add permission for %s on %s: %v", userID, resource, err)
			continue
		}
	}

	// Sauvegarder les politiques
	err := casdoor.Enforcer.LoadPolicy()
	if err != nil {
		return fmt.Errorf("failed to save policies: %v", err)
	}

	return nil
}

// RevokeSubscriptionPermissions révoque toutes les permissions liées aux abonnements
func (cpi *casdoorPaymentIntegration) RevokeSubscriptionPermissions(userID string) error {
	// Liste des ressources liées aux fonctionnalités payantes
	premiumResources := []string{
		"/api/v1/labs/advanced/*",
		"/api/v1/courses/export/*",
		"/api/v1/themes/custom/*",
		"/api/v1/analytics/*",
		"/api/v1/api-access/*",
	}

	for _, resource := range premiumResources {
		_, err := casdoor.Enforcer.RemoveFilteredPolicy(0, userID, resource)
		if err != nil {
			log.Printf("Warning: failed to revoke permission for %s on %s: %v", userID, resource, err)
		}
	}

	return nil
}

// AddUserToSubscriptionGroup ajoute un utilisateur au groupe correspondant à son plan
func (cpi *casdoorPaymentIntegration) AddUserToSubscriptionGroup(userID string, planName string) error {
	groupName := fmt.Sprintf("subscription_%s", strings.ToLower(planName))

	// Créer le groupe s'il n'existe pas
	err := cpi.ensureSubscriptionGroupExists(groupName, planName)
	if err != nil {
		return fmt.Errorf("failed to ensure group exists: %v", err)
	}

	// Ajouter l'utilisateur au groupe
	_, err = casdoor.Enforcer.AddGroupingPolicy(userID, groupName)
	if err != nil {
		return fmt.Errorf("failed to add user to subscription group: %v", err)
	}

	return nil
}

// RemoveUserFromSubscriptionGroups supprime un utilisateur de tous les groupes d'abonnement
func (cpi *casdoorPaymentIntegration) RemoveUserFromSubscriptionGroups(userID string) error {
	// Récupérer tous les groupes de l'utilisateur
	groups, err := casdoor.Enforcer.GetRolesForUser(userID)
	if err != nil {
		return fmt.Errorf("failed to get user groups: %v", err)
	}

	// Supprimer uniquement les groupes d'abonnement
	for _, group := range groups {
		if strings.HasPrefix(group, "subscription_") {
			_, err = casdoor.Enforcer.RemoveGroupingPolicy(userID, group)
			if err != nil {
				log.Printf("Warning: failed to remove user from group %s: %v", group, err)
			}
		}
	}

	return nil
}

// UserHasSubscriptionRole vérifie si un utilisateur a un rôle d'abonnement spécifique
func (cpi *casdoorPaymentIntegration) UserHasSubscriptionRole(userID string, requiredRole string) (bool, error) {
	roles, err := casdoor.Enforcer.GetRolesForUser(userID)
	if err != nil {
		return false, err
	}

	for _, role := range roles {
		if role == requiredRole {
			return true, nil
		}

		// Vérifier la hiérarchie des rôles
		if cpi.roleHasPermission(role, requiredRole) {
			return true, nil
		}
	}

	return false, nil
}

// GetUserSubscriptionRole récupère le rôle d'abonnement le plus élevé de l'utilisateur
func (cpi *casdoorPaymentIntegration) GetUserSubscriptionRole(userID string) (string, error) {
	roles, err := casdoor.Enforcer.GetRolesForUser(userID)
	if err != nil {
		return "", err
	}

	// Trouver le rôle d'abonnement le plus élevé
	subscriptionRoles := []models.RoleName{
		models.Admin,
		models.Organization,
		models.Trainer,
		models.GroupManager,
		models.MemberPro,
		models.Member,
		models.Guest,
	}

	for _, subscriptionRole := range subscriptionRoles {
		for _, userRole := range roles {
			if userRole == string(subscriptionRole) {
				return userRole, nil
			}
		}
	}

	return string(models.Guest), nil
}

// Méthodes privées

// setUserToDefaultRole remet l'utilisateur au rôle par défaut (membre de base)
func (cpi *casdoorPaymentIntegration) setUserToDefaultRole(userID string) error {
	// Supprimer tous les rôles d'abonnement
	err := cpi.removeSubscriptionRoles(userID)
	if err != nil {
		return err
	}

	// Ajouter le rôle de membre de base
	_, err = casdoor.Enforcer.AddGroupingPolicy(userID, string(models.Member))
	if err != nil {
		return fmt.Errorf("failed to set default role: %v", err)
	}

	return nil
}

// removeSubscriptionRoles supprime tous les rôles liés aux abonnements
func (cpi *casdoorPaymentIntegration) removeSubscriptionRoles(userID string) error {
	subscriptionRoles := []string{
		string(models.MemberPro),
		string(models.Trainer),
		string(models.GroupManager),
		string(models.Organization),
		// Ne pas supprimer Admin car il peut être accordé manuellement
	}

	for _, role := range subscriptionRoles {
		_, err := casdoor.Enforcer.RemoveGroupingPolicy(userID, role)
		if err != nil {
			log.Printf("Warning: failed to remove role %s from user %s: %v", role, userID, err)
		}
	}

	return nil
}

// updateCasdoorUserRole met à jour le rôle dans l'objet utilisateur Casdoor
func (cpi *casdoorPaymentIntegration) updateCasdoorUserRole(user *casdoorsdk.User, newRole string) error {
	// Ajouter le nouveau rôle s'il n'est pas déjà présent
	roleExists := false
	for _, role := range user.Roles {
		if role.Name == newRole {
			roleExists = true
			break
		}
	}

	if !roleExists {
		// Récupérer le rôle depuis Casdoor
		role, err := casdoorsdk.GetRole(newRole)
		if err != nil {
			return fmt.Errorf("failed to get role %s: %v", newRole, err)
		}

		if role != nil {
			// Ajouter l'utilisateur au rôle
			role.Users = append(role.Users, user.GetId())
			_, err = casdoorsdk.UpdateRole(role)
			if err != nil {
				return fmt.Errorf("failed to update role in Casdoor: %v", err)
			}
		}
	}

	return nil
}

// mapFeaturesToPermissions mappe les fonctionnalités vers des permissions Casbin
func (cpi *casdoorPaymentIntegration) mapFeaturesToPermissions(features []string) map[string]string {
	permissions := make(map[string]string)

	for _, feature := range features {
		switch feature {
		case "advanced_labs":
			permissions["/api/v1/labs/advanced/*"] = "(GET|POST|PUT|DELETE)"
		case "api_access":
			permissions["/api/v1/api/*"] = "(GET|POST|PUT|DELETE)"
		case "custom_themes":
			permissions["/api/v1/themes/custom/*"] = "(GET|POST|PUT|DELETE)"
		case "export":
			permissions["/api/v1/courses/export/*"] = "(GET|POST)"
		case "analytics":
			permissions["/api/v1/analytics/*"] = "(GET)"
		case "priority_support":
			permissions["/api/v1/support/priority"] = "(GET|POST)"
		}
	}

	return permissions
}

// ensureSubscriptionGroupExists s'assure qu'un groupe d'abonnement existe
func (cpi *casdoorPaymentIntegration) ensureSubscriptionGroupExists(groupName, planName string) error {
	// Vérifier si le groupe existe
	group, err := casdoorsdk.GetGroup(groupName)
	if err != nil || group == nil {
		// Créer le groupe
		newGroup := &casdoorsdk.Group{
			Name:        groupName,
			DisplayName: fmt.Sprintf("Subscription %s Users", planName),
		}

		_, err = casdoorsdk.AddGroup(newGroup)
		if err != nil {
			return fmt.Errorf("failed to create subscription group: %v", err)
		}
	}

	return nil
}

// roleHasPermission vérifie si un rôle a les permissions d'un autre selon la hiérarchie
func (cpi *casdoorPaymentIntegration) roleHasPermission(userRole, requiredRole string) bool {
	return models.HasPermission(models.RoleName(userRole), models.RoleName(requiredRole))
}

// WebhookHandler gère les événements webhook pour synchroniser les rôles
type SubscriptionWebhookHandler struct {
	casdoorIntegration CasdoorPaymentIntegration
}

func NewSubscriptionWebhookHandler(db *gorm.DB) *SubscriptionWebhookHandler {
	return &SubscriptionWebhookHandler{
		casdoorIntegration: NewCasdoorPaymentIntegration(db),
	}
}

// HandleSubscriptionCreated gère la création d'un abonnement
func (swh *SubscriptionWebhookHandler) HandleSubscriptionCreated(userID string, planRequiredRole string) error {
	err := swh.casdoorIntegration.UpdateUserRoleBasedOnSubscription(userID)
	if err != nil {
		return fmt.Errorf("failed to update user role after subscription creation: %v", err)
	}

	log.Printf("Successfully handled subscription creation for user %s", userID)
	return nil
}

// HandleSubscriptionUpdated gère la mise à jour d'un abonnement
func (swh *SubscriptionWebhookHandler) HandleSubscriptionUpdated(userID string) error {
	err := swh.casdoorIntegration.UpdateUserRoleBasedOnSubscription(userID)
	if err != nil {
		return fmt.Errorf("failed to update user role after subscription update: %v", err)
	}

	log.Printf("Successfully handled subscription update for user %s", userID)
	return nil
}

// HandleSubscriptionCancelled gère l'annulation d'un abonnement
func (swh *SubscriptionWebhookHandler) HandleSubscriptionCancelled(userID string) error {
	// Révoquer les permissions premium
	err := swh.casdoorIntegration.RevokeSubscriptionPermissions(userID)
	if err != nil {
		log.Printf("Warning: failed to revoke subscription permissions for user %s: %v", userID, err)
	}

	// Supprimer des groupes d'abonnement
	err = swh.casdoorIntegration.RemoveUserFromSubscriptionGroups(userID)
	if err != nil {
		log.Printf("Warning: failed to remove user from subscription groups: %v", err)
	}

	// Mettre à jour le rôle (remettre au rôle de base)
	err = swh.casdoorIntegration.UpdateUserRoleBasedOnSubscription(userID)
	if err != nil {
		return fmt.Errorf("failed to update user role after subscription cancellation: %v", err)
	}

	log.Printf("Successfully handled subscription cancellation for user %s", userID)
	return nil
}
