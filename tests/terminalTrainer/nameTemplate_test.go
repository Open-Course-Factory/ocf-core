package terminalTrainer_tests

import (
	"strings"
	"testing"

	"soli/formations/src/terminalTrainer/services"
)

// TestApplyNameTemplate_MachineSizePlaceholder verifies that the {machine_size}
// placeholder is rendered in uppercase, that the default template appends the
// size, and that all placeholders compose correctly. Drives the bulk session
// admin UX where session names like "Cohort 1 - alice@x - L" make the
// allocated size visible at a glance.
func TestApplyNameTemplate(t *testing.T) {
	const (
		groupName = "Cohort 1"
		userEmail = "alice@example.com"
		userID    = "user-42"
	)

	testCases := []struct {
		name        string
		template    string
		machineSize string
		// expectedContains: substrings that MUST appear in the rendered name.
		expectedContains []string
		// expectedExact: when non-empty, the rendered name must equal this string exactly.
		expectedExact string
	}{
		{
			name:             "default template with size appends uppercase size",
			template:         "",
			machineSize:      "m",
			expectedContains: []string{groupName, userEmail, "M"},
		},
		{
			name:          "explicit {machine_size} placeholder renders uppercase",
			template:      "{group_name} - {machine_size}",
			machineSize:   "l",
			expectedExact: "Cohort 1 - L",
		},
		{
			name:          "all placeholders together compose correctly",
			template:      "{group_name}/{user_email}/{user_id}/{machine_size}",
			machineSize:   "xl",
			expectedExact: "Cohort 1/alice@example.com/user-42/XL",
		},
		{
			name:          "lowercase xs size renders as XS",
			template:      "{machine_size}",
			machineSize:   "xs",
			expectedExact: "XS",
		},
		{
			name:          "already-uppercase size stays uppercase",
			template:      "{machine_size}",
			machineSize:   "S",
			expectedExact: "S",
		},
		{
			name:          "empty machine size leaves placeholder empty",
			template:      "{group_name}-{machine_size}",
			machineSize:   "",
			expectedExact: "Cohort 1-",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := services.ApplyNameTemplate(tc.template, groupName, userEmail, userID, tc.machineSize)

			if tc.expectedExact != "" {
				if got != tc.expectedExact {
					t.Fatalf("ApplyNameTemplate() = %q, want exactly %q", got, tc.expectedExact)
				}
				return
			}

			for _, want := range tc.expectedContains {
				if !strings.Contains(got, want) {
					t.Errorf("ApplyNameTemplate() = %q, expected to contain %q", got, want)
				}
			}
		})
	}
}
