package services

import (
	"fmt"
	"regexp"
	"strings"

	authDto "soli/formations/src/auth/dto"
	"soli/formations/src/auth/interfaces"
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// entityTableLookup defines how to resolve a display name from each entity table.
// nameExpr is a SQL expression that produces the best display name for the entity.
type entityTableLookup struct {
	table    string
	nameExpr string
}

// entityTableMap maps kebab-case entity route segments to their table lookup config.
// Both singular and plural forms are included for flexible matching.
var entityTableMap = map[string]entityTableLookup{
	"organization":       {table: "organizations", nameExpr: "COALESCE(NULLIF(display_name, ''), name)"},
	"organizations":      {table: "organizations", nameExpr: "COALESCE(NULLIF(display_name, ''), name)"},
	"class-group":        {table: "class_groups", nameExpr: "COALESCE(NULLIF(display_name, ''), name)"},
	"class-groups":       {table: "class_groups", nameExpr: "COALESCE(NULLIF(display_name, ''), name)"},
	"course":             {table: "courses", nameExpr: "COALESCE(NULLIF(title, ''), name)"},
	"courses":            {table: "courses", nameExpr: "COALESCE(NULLIF(title, ''), name)"},
	"session":            {table: "sessions", nameExpr: "title"},
	"sessions":           {table: "sessions", nameExpr: "title"},
	"chapter":            {table: "chapters", nameExpr: "title"},
	"chapters":           {table: "chapters", nameExpr: "title"},
	"section":            {table: "sections", nameExpr: "title"},
	"sections":           {table: "sections", nameExpr: "title"},
	"theme":              {table: "themes", nameExpr: "name"},
	"themes":             {table: "themes", nameExpr: "name"},
	"generation":         {table: "generations", nameExpr: "id"},
	"generations":        {table: "generations", nameExpr: "id"},
	"terminal":           {table: "terminals", nameExpr: "name"},
	"terminals":          {table: "terminals", nameExpr: "name"},
	"subscription-plan":  {table: "subscription_plans", nameExpr: "name"},
	"subscription-plans": {table: "subscription_plans", nameExpr: "name"},
	"feature":            {table: "features", nameExpr: "name"},
	"features":           {table: "features", nameExpr: "name"},
	"invoice":            {table: "invoices", nameExpr: "id"},
	"invoices":           {table: "invoices", nameExpr: "id"},
}

// resourcePathRegex matches API paths containing entity UUIDs:
// e.g., /api/v1/organizations/019c6d26-135f-7518-8a6d-12720a15bf4b
var resourcePathRegex = regexp.MustCompile(`/api/v1/([\w-]+)/([\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12})`)

type SecurityAdminService struct {
	db                 *gorm.DB
	enforcer           interfaces.EnforcerInterface
	permissionsService UserPermissionsService
	resolveNames       func(uuids []string) map[string]string
}

func NewSecurityAdminService(enforcer interfaces.EnforcerInterface, db *gorm.DB) *SecurityAdminService {
	svc := &SecurityAdminService{
		db:                 db,
		enforcer:           enforcer,
		permissionsService: NewUserPermissionsService(db),
	}
	svc.resolveNames = svc.defaultResolveUserNames
	return svc
}

// SetNameResolver allows overriding the name resolution function (for testing)
func (s *SecurityAdminService) SetNameResolver(resolver func(uuids []string) map[string]string) {
	s.resolveNames = resolver
}

// defaultResolveUserNames resolves a list of user UUIDs to display names via the Casdoor SDK.
// Falls back to a truncated UUID (first 8 characters + "...") when resolution fails.
func (s *SecurityAdminService) defaultResolveUserNames(uuids []string) map[string]string {
	result := make(map[string]string, len(uuids))
	for _, uid := range uuids {
		user, err := casdoorsdk.GetUserByUserId(uid)
		if err == nil && user != nil {
			name := user.DisplayName
			if name == "" {
				name = user.Name
			}
			if name != "" {
				result[uid] = name
				continue
			}
		}
		// Fallback: truncated UUID
		if len(uid) >= 8 {
			result[uid] = uid[:8] + "..."
		} else {
			result[uid] = uid
		}
	}
	return result
}

// resolveEntityNames resolves entity-scoped identifiers (e.g., "organization:UUID")
// and resource path UUIDs to display names by querying the database.
// It accepts a list of "entityType:UUID" strings and returns a map from the full
// key to the resolved display name.
func (s *SecurityAdminService) resolveEntityNames(keys []string) map[string]string {
	if s.db == nil || len(keys) == 0 {
		return make(map[string]string)
	}

	// Group UUIDs by table+nameExpr key
	type lookupEntry struct {
		lookup entityTableLookup
		uuid   string
		key    string
	}
	// Use table name as grouping key (same table = same query)
	byTable := make(map[string][]lookupEntry)
	var lookupForTable = make(map[string]entityTableLookup)
	for _, key := range keys {
		entityType, uid := parseEntityKey(key)
		if entityType == "" || uid == "" {
			continue
		}
		lookup, ok := entityTableMap[entityType]
		if !ok {
			continue
		}
		byTable[lookup.table] = append(byTable[lookup.table], lookupEntry{lookup: lookup, uuid: uid, key: key})
		lookupForTable[lookup.table] = lookup
	}

	result := make(map[string]string, len(keys))

	// Batch query per table
	for table, entries := range byTable {
		uuids := make([]string, len(entries))
		for i, e := range entries {
			uuids[i] = e.uuid
		}

		lookup := lookupForTable[table]

		type nameRow struct {
			ID   string `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		var rows []nameRow
		s.db.Raw(
			fmt.Sprintf("SELECT id, %s AS name FROM %s WHERE id IN ?", lookup.nameExpr, table),
			uuids,
		).Scan(&rows)

		nameByID := make(map[string]string, len(rows))
		for _, row := range rows {
			nameByID[row.ID] = row.Name
		}

		for _, entry := range entries {
			if name, ok := nameByID[entry.uuid]; ok {
				result[entry.key] = name
			}
		}
	}

	return result
}

// parseEntityKey splits an entity-scoped key like "organization:UUID" into its parts.
// Returns empty strings if the key doesn't match the expected pattern.
func parseEntityKey(key string) (entityType string, uid string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	// Validate the UUID part
	if _, err := uuid.Parse(parts[1]); err != nil {
		return "", ""
	}
	return parts[0], parts[1]
}

// extractResourceEntityKey extracts an entity key from a resource path.
// For "/api/v1/organizations/019c6d26-...", returns "organizations:019c6d26-...".
// Returns empty string if no UUID is found in the path.
func extractResourceEntityKey(resource string) string {
	matches := resourcePathRegex.FindStringSubmatch(resource)
	if len(matches) < 3 {
		return ""
	}
	return matches[1] + ":" + matches[2]
}

// GetPolicyOverview returns all Casbin policies grouped by subject type (role vs user)
func (s *SecurityAdminService) GetPolicyOverview() (*authDto.PolicyOverviewOutput, error) {
	policies, err := s.enforcer.GetPolicy()
	if err != nil {
		return nil, fmt.Errorf("failed to get policies: %w", err)
	}

	roleMap := make(map[string][]authDto.PolicyRule)
	userMap := make(map[string][]authDto.PolicyRule)

	// Collect entity keys for batch resolution (from both subjects and resources)
	entityKeySet := make(map[string]bool)

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

		// Collect entity key from resource path (e.g., /api/v1/organizations/UUID)
		if rk := extractResourceEntityKey(resource); rk != "" {
			entityKeySet[rk] = true
		}

		// Classify: if it parses as UUID, it's a user policy; otherwise it's a role
		_, uuidErr := uuid.Parse(subject)
		if uuidErr == nil {
			userMap[subject] = append(userMap[subject], rule)
		} else {
			// Check if it's an entity-scoped subject (e.g., "organization:UUID")
			if et, uid := parseEntityKey(subject); et != "" && uid != "" {
				entityKeySet[subject] = true
			}
			roleMap[subject] = append(roleMap[subject], rule)
		}
	}

	// Batch resolve entity names from DB
	entityKeys := make([]string, 0, len(entityKeySet))
	for k := range entityKeySet {
		entityKeys = append(entityKeys, k)
	}
	entityNameMap := s.resolveEntityNames(entityKeys)

	// Build role policies with entity name resolution
	rolePolicies := make([]authDto.PolicySubject, 0, len(roleMap))
	for subject, rules := range roleMap {
		ps := authDto.PolicySubject{
			Subject:  subject,
			Policies: s.populateResourceNames(rules, entityNameMap),
		}
		// Resolve entity-scoped subject names (e.g., "organization:UUID" → "My Org")
		if name, ok := entityNameMap[subject]; ok {
			ps.SubjectName = name
		}
		rolePolicies = append(rolePolicies, ps)
	}

	// Resolve user UUID display names
	userUUIDs := make([]string, 0, len(userMap))
	for uid := range userMap {
		userUUIDs = append(userUUIDs, uid)
	}
	nameMap := s.resolveNames(userUUIDs)

	userPolicies := make([]authDto.PolicySubject, 0, len(userMap))
	for subject, rules := range userMap {
		userPolicies = append(userPolicies, authDto.PolicySubject{
			Subject:     subject,
			SubjectName: nameMap[subject],
			Policies:    s.populateResourceNames(rules, entityNameMap),
		})
	}

	return &authDto.PolicyOverviewOutput{
		RolePolicies:  rolePolicies,
		UserPolicies:  userPolicies,
		TotalPolicies: len(policies),
	}, nil
}

// populateResourceNames sets ResourceName on each PolicyRule whose resource path
// contains an entity UUID that was resolved.
func (s *SecurityAdminService) populateResourceNames(rules []authDto.PolicyRule, entityNameMap map[string]string) []authDto.PolicyRule {
	for i := range rules {
		rk := extractResourceEntityKey(rules[i].Resource)
		if rk != "" {
			if name, ok := entityNameMap[rk]; ok {
				rules[i].ResourceName = name
			}
		}
	}
	return rules
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

		// Resolve admin UUIDs to display names to avoid leaking raw UUIDs
		adminNameMap := s.resolveNames(adminUsers)
		adminNames := make([]string, 0, len(adminUsers))
		for _, uid := range adminUsers {
			adminNames = append(adminNames, adminNameMap[uid])
		}

		findings = append(findings, authDto.HealthFinding{
			Severity:    severity,
			Category:    "admin_users",
			Description: fmt.Sprintf("%d users have administrator role", len(adminUsers)),
			Details:     strings.Join(adminNames, ", "),
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

