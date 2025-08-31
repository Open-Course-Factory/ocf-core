package models

type RoleName string

var (
	Guest        RoleName = "guest"
	Member       RoleName = "member" // Inscrit
	GroupManager RoleName = "group_manager"
	Admin        RoleName = "administrator"

	MemberPro    RoleName = "member_pro"   // Membre payant
	Trainer      RoleName = "trainer"      // Membre payant qui a payé pour partager des machines
	Organization RoleName = "organization" // Compte entreprise/organisation peut payer pour des comptes
)

var RoleHierarchy = map[RoleName][]RoleName{
	Guest:        {},
	Member:       {Guest},
	MemberPro:    {Member, Guest},
	GroupManager: {Member, Guest},
	Trainer:      {GroupManager, MemberPro, Member, Guest},
	Organization: {Trainer, GroupManager, MemberPro, Member, Guest},
	Admin:        {Organization, Trainer, GroupManager, MemberPro, Member, Guest},
}

type RoleFeatures struct {
	MaxCourses            int // -1 = illimité
	MaxLabSessions        int // -1 = illimité
	MaxConcurrentUsers    int // Pour les comptes multi-utilisateurs
	CanCreateAdvancedLabs bool
	CanUseNetwork         bool
	CanExportCourses      bool
	CanUseAPI             bool
	HasPrioritySupport    bool
	CanCustomizeThemes    bool
	HasAnalytics          bool
	StorageLimit          int64 // En MB, -1 = illimité
}

// GetRoleFeatures retourne les fonctionnalités d'un rôle
func GetRoleFeatures(role RoleName) RoleFeatures {
	switch role {
	case Guest:
		return RoleFeatures{
			MaxCourses:            0,
			MaxLabSessions:        0,
			MaxConcurrentUsers:    1,
			CanUseNetwork:         false,
			CanCreateAdvancedLabs: false,
			CanExportCourses:      false,
			CanUseAPI:             false,
			HasPrioritySupport:    false,
			CanCustomizeThemes:    false,
			HasAnalytics:          false,
			StorageLimit:          0,
		}

	case Member:
		return RoleFeatures{
			MaxCourses:            3,
			MaxLabSessions:        10,
			MaxConcurrentUsers:    1,
			CanCreateAdvancedLabs: false,
			CanUseNetwork:         false,
			CanExportCourses:      false,
			CanUseAPI:             false,
			HasPrioritySupport:    false,
			CanCustomizeThemes:    false,
			HasAnalytics:          false,
			StorageLimit:          100, // 100 MB
		}

	case MemberPro:
		return RoleFeatures{
			MaxCourses:            -1, // Illimité
			MaxLabSessions:        100,
			MaxConcurrentUsers:    1,
			CanCreateAdvancedLabs: true,
			CanUseNetwork:         true,
			CanExportCourses:      true,
			CanUseAPI:             true,
			HasPrioritySupport:    false,
			CanCustomizeThemes:    true,
			HasAnalytics:          true,
			StorageLimit:          1000, // 1 GB
		}

	case GroupManager:
		return RoleFeatures{
			MaxCourses:            10,
			MaxLabSessions:        50,
			MaxConcurrentUsers:    5,
			CanCreateAdvancedLabs: true,
			CanUseNetwork:         true,
			CanExportCourses:      true,
			CanUseAPI:             false,
			HasPrioritySupport:    false,
			CanCustomizeThemes:    false,
			HasAnalytics:          true,
			StorageLimit:          500, // 500 MB
		}

	case Trainer:
		return RoleFeatures{
			MaxCourses:            -1, // Illimité
			MaxLabSessions:        -1, // Illimité
			MaxConcurrentUsers:    25,
			CanCreateAdvancedLabs: true,
			CanUseNetwork:         true,
			CanExportCourses:      true,
			CanUseAPI:             true,
			HasPrioritySupport:    true,
			CanCustomizeThemes:    true,
			HasAnalytics:          true,
			StorageLimit:          5000, // 5 GB
		}

	case Organization:
		return RoleFeatures{
			MaxCourses:            -1, // Illimité
			MaxLabSessions:        -1, // Illimité
			MaxConcurrentUsers:    100,
			CanCreateAdvancedLabs: true,
			CanUseNetwork:         true,
			CanExportCourses:      true,
			CanUseAPI:             true,
			HasPrioritySupport:    true,
			CanCustomizeThemes:    true,
			HasAnalytics:          true,
			StorageLimit:          20000, // 20 GB
		}

	case Admin:
		return RoleFeatures{
			MaxCourses:            -1, // Illimité
			MaxLabSessions:        -1, // Illimité
			MaxConcurrentUsers:    -1, // Illimité
			CanCreateAdvancedLabs: true,
			CanUseNetwork:         true,
			CanExportCourses:      true,
			CanUseAPI:             true,
			HasPrioritySupport:    true,
			CanCustomizeThemes:    true,
			HasAnalytics:          true,
			StorageLimit:          -1, // Illimité
		}

	default:
		return GetRoleFeatures(Guest) // Par défaut
	}
}

// IsRolePayingUser vérifie si un rôle correspond à un utilisateur payant
func IsRolePayingUser(role RoleName) bool {
	payingRoles := []RoleName{MemberPro, Trainer, Organization}
	for _, payingRole := range payingRoles {
		if role == payingRole {
			return true
		}
	}
	return false
}

// GetUpgradeRecommendations retourne les suggestions de mise à niveau
func GetUpgradeRecommendations(currentRole RoleName) []RoleName {
	switch currentRole {
	case Guest:
		return []RoleName{Member, MemberPro}
	case Member:
		return []RoleName{MemberPro, Trainer}
	case Trainer:
		return []RoleName{Organization}
	case Organization:
		return []RoleName{}
	default:
		return []RoleName{}
	}
}

// Permission helpers for middleware
func HasPermission(userRole RoleName, requiredRole RoleName) bool {
	if userRole == requiredRole {
		return true
	}

	// Vérifier la hiérarchie
	inheritedRoles, exists := RoleHierarchy[userRole]
	if !exists {
		return false
	}

	for _, inherited := range inheritedRoles {
		if inherited == requiredRole {
			return true
		}
	}

	return false
}

// GetMaximumRole retourne le rôle le plus élevé parmi une liste
func GetMaximumRole(roles []RoleName) RoleName {
	if len(roles) == 0 {
		return Guest
	}

	maxRole := Guest
	maxHierarchySize := 0

	for _, role := range roles {
		hierarchySize := len(RoleHierarchy[role])
		if hierarchySize > maxHierarchySize {
			maxHierarchySize = hierarchySize
			maxRole = role
		}
	}

	return maxRole
}
