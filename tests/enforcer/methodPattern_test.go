package enforcer_tests

import (
	"regexp"
	"testing"

	"soli/formations/src/auth/casdoor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var httpMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// Entity registrations write "()" for a role that gets no method on an entity
// (members on invoices, usage metrics, organization role plans). Matched as an
// unanchored regex, "()" matches every method, so those rows opened the
// routes they were meant to close.
func TestMethodPattern_EmptyGroupGrantsNoMethod(t *testing.T) {
	enforcer, err := casdoor.NewEnforcer("../../src/configuration/keymatch_model.conf")
	require.NoError(t, err)
	_, err = enforcer.AddPolicy("member", "/api/v1/invoices", "()")
	require.NoError(t, err)

	for _, method := range httpMethods {
		allowed, err := enforcer.Enforce("member", "/api/v1/invoices", method)
		require.NoError(t, err)
		assert.False(t, allowed, "() must not grant %s", method)
	}
}

// Every method-pattern shape stored in production (casbin_rule, 2026-11-11)
// grants exactly the methods it names, no more, no fewer.
func TestMethodPattern_ProductionShapesGrantExactlyTheirMethods(t *testing.T) {
	shapes := []string{
		"GET", "POST", "PUT", "PATCH", "DELETE",
		"(GET)", "(GET|POST)", "(GET|PATCH)", "(POST|PATCH)",
		"(GET|POST|PATCH)", "(GET|POST|DELETE)", "(GET|PATCH|DELETE)", "(POST|DELETE|PATCH)",
		"(GET|POST|PATCH|DELETE)", "(GET|POST|DELETE|PATCH)", "(GET|PATCH|POST|DELETE)",
		"(GET|DELETE|PATCH|PUT)", "(GET|PATCH|DELETE|PUT)", "(GET|POST|PUT|DELETE)",
		"(GET|POST|PUT|PATCH|DELETE)",
		"POST|DELETE", "GET|PATCH|DELETE", "GET|POST|PATCH", "GET|POST|PATCH|DELETE",
	}
	named := regexp.MustCompile(`[A-Z]+`)

	for _, shape := range shapes {
		enforcer, err := casdoor.NewEnforcer("../../src/configuration/keymatch_model.conf")
		require.NoError(t, err)
		_, err = enforcer.AddPolicy("member", "/api/v1/things", shape)
		require.NoError(t, err)

		grants := map[string]bool{}
		for _, m := range named.FindAllString(shape, -1) {
			grants[m] = true
		}
		for _, method := range httpMethods {
			allowed, err := enforcer.Enforce("member", "/api/v1/things", method)
			require.NoError(t, err)
			assert.Equal(t, grants[method], allowed, "pattern %q, method %s", shape, method)
		}
	}
}
