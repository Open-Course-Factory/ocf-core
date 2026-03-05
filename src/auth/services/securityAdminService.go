package services

import (
	"fmt"
	"strings"

	authDto "soli/formations/src/auth/dto"
	"soli/formations/src/auth/interfaces"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SecurityAdminService struct {
	enforcer           interfaces.EnforcerInterface
	permissionsService UserPermissionsService
}

func NewSecurityAdminService(enforcer interfaces.EnforcerInterface, db *gorm.DB) *SecurityAdminService {
	return &SecurityAdminService{
		enforcer:           enforcer,
		permissionsService: NewUserPermissionsService(db),
	}
}

// GetPolicyOverview returns all Casbin policies grouped by subject type (role vs user)
func (s *SecurityAdminService) GetPolicyOverview() (*authDto.PolicyOverviewOutput, error) {
	policies, err := s.enforcer.GetPolicy()
	if err != nil {
		return nil, fmt.Errorf("failed to get policies: %w", err)
	}

	roleMap := make(map[string][]authDto.PolicyRule)
	userMap := make(map[string][]authDto.PolicyRule)

	for _, policy := range policies {
		if len(policy) < 3 {
			continue
		}

		subject := policy[0]
		resource := policy[1]
		methodStr := policy[2]

		methods := authDto.ParseMethods(methodStr)

		rule := authDto.PolicyRule{
			Resource: resource,
			Methods:  methods,
		}

		// Classify: if it parses as UUID, it's a user policy; otherwise it's a role
		_, uuidErr := uuid.Parse(subject)
		if uuidErr == nil {
			userMap[subject] = append(userMap[subject], rule)
		} else {
			roleMap[subject] = append(roleMap[subject], rule)
		}
	}

	rolePolicies := make([]authDto.PolicySubject, 0, len(roleMap))
	for subject, rules := range roleMap {
		rolePolicies = append(rolePolicies, authDto.PolicySubject{
			Subject:  subject,
			Policies: rules,
		})
	}

	userPolicies := make([]authDto.PolicySubject, 0, len(userMap))
	for subject, rules := range userMap {
		userPolicies = append(userPolicies, authDto.PolicySubject{
			Subject:  subject,
			Policies: rules,
		})
	}

	return &authDto.PolicyOverviewOutput{
		RolePolicies:  rolePolicies,
		UserPolicies:  userPolicies,
		TotalPolicies: len(policies),
	}, nil
}

// GetUserPermissionLookup returns the full permission set for a specific user
func (s *SecurityAdminService) GetUserPermissionLookup(userID string) (*authDto.UserPermissionsOutput, error) {
	return s.permissionsService.GetUserPermissions(userID)
}

// GetEntityRoleMatrix returns the role-to-method mapping for all registered entities
func (s *SecurityAdminService) GetEntityRoleMatrix() (*authDto.EntityRoleMatrixOutput, error) {
	allRoles := ems.GlobalEntityRegistrationService.GetAllEntityRoles()

	entities := make([]authDto.EntityRoleEntry, 0, len(allRoles))
	for entityName, entityRoles := range allRoles {
		roleMethods := make(map[string][]string)
		for role, methodStr := range entityRoles.Roles {
			roleMethods[role] = authDto.ParseMethods(methodStr)
		}

		entities = append(entities, authDto.EntityRoleEntry{
			EntityName:  entityName,
			RoleMethods: roleMethods,
		})
	}

	return &authDto.EntityRoleMatrixOutput{
		Entities: entities,
	}, nil
}

// GetPolicyHealthChecks analyzes policies for potential issues
func (s *SecurityAdminService) GetPolicyHealthChecks() (*authDto.PolicyHealthCheckOutput, error) {
	findings := make([]authDto.HealthFinding, 0)

	policies, err := s.enforcer.GetPolicy()
	if err != nil {
		return nil, fmt.Errorf("failed to get policies: %w", err)
	}

	// Check 1: Overly permissive policies
	for _, policy := range policies {
		if len(policy) < 3 {
			continue
		}

		subject := policy[0]
		resource := policy[1]
		methodStr := policy[2]

		// Skip admin subjects
		if subject == "administrator" {
			continue
		}

		if methodStr == "*" || methodStr == "(GET|POST|PATCH|DELETE)" {
			findings = append(findings, authDto.HealthFinding{
				Severity:    "medium",
				Category:    "overly_permissive",
				Description: fmt.Sprintf("Subject '%s' has broad access to '%s'", subject, resource),
				Details:     fmt.Sprintf("Methods: %s", methodStr),
			})
		}
	}

	// Check 2: Admin user count
	adminUsers, err := s.enforcer.GetUsersForRole("administrator")
	if err == nil && len(adminUsers) > 0 {
		severity := "info"
		if len(adminUsers) > 5 {
			severity = "medium"
		}
		findings = append(findings, authDto.HealthFinding{
			Severity:    severity,
			Category:    "admin_users",
			Description: fmt.Sprintf("%d users have administrator role", len(adminUsers)),
			Details:     strings.Join(adminUsers, ", "),
		})
	}

	// Check 3: Orphaned user policies (UUID subjects with direct policies)
	userPolicyCount := 0
	for _, policy := range policies {
		if len(policy) < 3 {
			continue
		}
		_, uuidErr := uuid.Parse(policy[0])
		if uuidErr == nil {
			userPolicyCount++
		}
	}
	if userPolicyCount > 0 {
		findings = append(findings, authDto.HealthFinding{
			Severity:    "info",
			Category:    "user_specific_policies",
			Description: fmt.Sprintf("%d user-specific policies found (may be intentional)", userPolicyCount),
		})
	}

	// Check 4: Entities without admin DELETE protection
	allRoles := ems.GlobalEntityRegistrationService.GetAllEntityRoles()
	for entityName, entityRoles := range allRoles {
		hasAdminDelete := false
		for role, methodStr := range entityRoles.Roles {
			if role == "administrator" && strings.Contains(methodStr, "DELETE") {
				hasAdminDelete = true
				break
			}
		}
		if !hasAdminDelete {
			findings = append(findings, authDto.HealthFinding{
				Severity:    "low",
				Category:    "missing_admin_delete",
				Description: fmt.Sprintf("Entity '%s' has no explicit administrator DELETE permission", entityName),
			})
		}
	}

	// Build summary
	summary := authDto.HealthSummary{}
	for _, f := range findings {
		switch f.Severity {
		case "high":
			summary.HighCount++
		case "medium":
			summary.MediumCount++
		case "low":
			summary.LowCount++
		case "info":
			summary.InfoCount++
		}
	}

	return &authDto.PolicyHealthCheckOutput{
		Findings: findings,
		Summary:  summary,
	}, nil
}

