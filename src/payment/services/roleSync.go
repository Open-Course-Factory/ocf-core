// src/payment/services/roleSync.go
package services

import (
	"fmt"
	"soli/formations/src/auth/casdoor"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

func (ss *subscriptionService) UpdateUserRoleBasedOnSubscription(userID string) error {
	// 1. Récupérer l'abonnement actuel
	subscription, err := ss.repository.GetActiveUserSubscription(userID)
	if err != nil {
		// Pas d'abonnement actif, assigner le rôle de base
		return ss.assignDefaultRole(userID)
	}

	sPlan, errSPlan := ss.GetSubscriptionPlan(subscription.SubscriptionPlanID)
	if errSPlan != nil {
		return errSPlan
	}

	requiredRole := sPlan.RequiredRole
	if requiredRole == "" {
		return nil
	}

	// 2. Charger les politiques
	if err := casdoor.Enforcer.LoadPolicy(); err != nil {
		return fmt.Errorf("failed to load policy: %v", err)
	}

	// 3. Supprimer les anciens rôles d'abonnement
	paymentRoles := []string{"member_pro", "trainer", "organization"}
	for _, role := range paymentRoles {
		casdoor.Enforcer.RemoveGroupingPolicy(userID, role)
	}

	// 4. Ajouter le nouveau rôle
	_, err = casdoor.Enforcer.AddGroupingPolicy(userID, requiredRole)
	if err != nil {
		return fmt.Errorf("failed to update user role: %v", err)
	}

	// 5. Mettre à jour dans Casdoor également
	user, err := casdoorsdk.GetUserByUserId(userID)
	if err != nil {
		return fmt.Errorf("failed to get user from Casdoor: %v", err)
	}

	// Ajouter le rôle dans Casdoor
	role, err := casdoorsdk.GetRole(requiredRole)
	if err != nil {
		return fmt.Errorf("failed to get role from Casdoor: %v", err)
	}

	// Ajouter l'utilisateur au rôle s'il n'y est pas déjà
	if !contains(role.Users, user.GetId()) {
		role.Users = append(role.Users, user.GetId())
		_, err = casdoorsdk.UpdateRole(role)
		if err != nil {
			return fmt.Errorf("failed to update role in Casdoor: %v", err)
		}
	}

	return nil
}

func (ss *subscriptionService) assignDefaultRole(userID string) error {
	// Assigner le rôle "member" par défaut
	_, err := casdoor.Enforcer.AddGroupingPolicy(userID, "member")
	return err
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
