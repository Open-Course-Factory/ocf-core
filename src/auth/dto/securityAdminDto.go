package dto

// PolicyOverview

type PolicySubject struct {
	Subject  string       `json:"subject"`
	Policies []PolicyRule `json:"policies"`
}

type PolicyRule struct {
	Resource string   `json:"resource"`
	Methods  []string `json:"methods"`
}

type PolicyOverviewOutput struct {
	RolePolicies  []PolicySubject `json:"role_policies"`
	UserPolicies  []PolicySubject `json:"user_policies"`
	TotalPolicies int             `json:"total_policies"`
}

// Entity Role Matrix

type EntityRoleEntry struct {
	EntityName  string              `json:"entity_name"`
	RoleMethods map[string][]string `json:"role_methods"`
}

type EntityRoleMatrixOutput struct {
	Entities []EntityRoleEntry `json:"entities"`
}

// Policy Health Checks

type HealthFinding struct {
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Details     string `json:"details,omitempty"`
}

type HealthSummary struct {
	HighCount   int `json:"high_count"`
	MediumCount int `json:"medium_count"`
	LowCount    int `json:"low_count"`
	InfoCount   int `json:"info_count"`
}

type PolicyHealthCheckOutput struct {
	Findings []HealthFinding `json:"findings"`
	Summary  HealthSummary   `json:"summary"`
}
