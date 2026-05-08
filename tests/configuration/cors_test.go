package configuration_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	config "soli/formations/src/configuration"
)

func TestInitAllowedOrigins_FrontendURL_ParsedAsCommaSeparatedList(t *testing.T) {
	cases := []struct {
		name           string
		frontendURL    string
		adminURL       string
		extraOrigins   string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "single value preserved (no comma)",
			frontendURL:  "https://ocf.solution-libre.fr",
			wantContains: []string{"https://ocf.solution-libre.fr"},
		},
		{
			name:         "two comma-separated values both included",
			frontendURL:  "https://ocf.solution-libre.fr,https://ocf.labinux.com",
			wantContains: []string{"https://ocf.solution-libre.fr", "https://ocf.labinux.com"},
		},
		{
			name:         "whitespace around values is trimmed",
			frontendURL:  "  https://a.com , https://b.com  ",
			wantContains: []string{"https://a.com", "https://b.com"},
		},
		{
			name:           "empty entries between commas are dropped",
			frontendURL:    "https://a.com,,https://b.com",
			wantContains:   []string{"https://a.com", "https://b.com"},
			wantNotContain: []string{""},
		},
		{
			name:         "ADMIN_FRONTEND_URL also accepts comma-separated",
			frontendURL:  "https://front.com",
			adminURL:     "https://admin1.com,https://admin2.com",
			wantContains: []string{"https://front.com", "https://admin1.com", "https://admin2.com"},
		},
		{
			name:         "EXTRA_FRONTEND_ORIGINS appends additional CORS-allowed origins",
			frontendURL:  "https://ocf.solution-libre.fr",
			extraOrigins: "https://ocf.labinux.com,https://staging.example.com",
			wantContains: []string{"https://ocf.solution-libre.fr", "https://ocf.labinux.com", "https://staging.example.com"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", "production") // skip dev-mode localhost fallback
			t.Setenv("FRONTEND_URL", tc.frontendURL)
			t.Setenv("ADMIN_FRONTEND_URL", tc.adminURL)
			t.Setenv("EXTRA_FRONTEND_ORIGINS", tc.extraOrigins)

			got := config.InitAllowedOrigins()

			for _, want := range tc.wantContains {
				assert.Contains(t, got, want, "expected origin %q to be in result %v", want, got)
			}
			for _, notWant := range tc.wantNotContain {
				assert.NotContains(t, got, notWant, "did not expect origin %q in result %v", notWant, got)
			}
		})
	}
}
