package access

import (
	"fmt"
	"log"
	"sort"

	"github.com/gin-gonic/gin"
)

// ValidatePermissionSetup checks that:
// 1. Every AccessRuleType used in RouteRegistry has a registered enforcer
// 2. Logs Gin routes not covered by the RouteRegistry (informational)
func ValidatePermissionSetup(router *gin.Engine) {
	log.Println("=== Validating permission setup ===")

	// Check 1: All access rule types have enforcers
	ref := RouteRegistry.GetReference()
	missingEnforcers := make(map[AccessRuleType]bool)

	for _, cat := range ref.Categories {
		for _, route := range cat.Routes {
			if getAccessEnforcer(route.Access.Type) == nil {
				missingEnforcers[route.Access.Type] = true
			}
		}
	}

	if len(missingEnforcers) > 0 {
		for ruleType := range missingEnforcers {
			log.Printf("⚠️  WARNING: AccessRuleType %q used in route declarations but no enforcer registered", ruleType)
		}
	}

	// Check 2: Compare Gin routes against RouteRegistry
	ginRoutes := router.Routes()
	registeredCount := 0
	unregisteredCount := 0

	for _, route := range ginRoutes {
		if route.Path == "" || route.Method == "" {
			continue
		}
		_, found := RouteRegistry.Lookup(route.Method, route.Path)
		if found {
			registeredCount++
		} else {
			unregisteredCount++
		}
	}

	log.Printf("✅ Permission validation: %d routes with declarations, %d routes without (entity CRUD, public, webhooks, etc.)",
		registeredCount, unregisteredCount)

	if len(missingEnforcers) == 0 {
		log.Println("✅ All access rule types have registered enforcers")
	}

	log.Println("=== Permission validation complete ===")
}

// ValidatePermissionSetupStrict is the fail-fast counterpart to
// ValidatePermissionSetup: it returns a non-nil error when any AccessRuleType
// used in a RouteRegistry route declaration has no registered enforcer. Such a
// route runs fail-open (Layer2Enforcement passes it through), so this is always
// a bug. Wire it into CI (a test) and optionally an env-gated startup check so
// a route shipped with an unregistered/typo'd rule type fails the build.
//
// The router param mirrors ValidatePermissionSetup's signature for symmetry and
// future Gin-route cross-checks; the strict guarantee is about the RouteRegistry.
func ValidatePermissionSetupStrict(_ *gin.Engine) error {
	ref := RouteRegistry.GetReference()
	missing := map[AccessRuleType]bool{}
	for _, cat := range ref.Categories {
		for _, route := range cat.Routes {
			if getAccessEnforcer(route.Access.Type) == nil {
				missing[route.Access.Type] = true
			}
		}
	}
	if len(missing) > 0 {
		types := make([]string, 0, len(missing))
		for t := range missing {
			types = append(types, string(t))
		}
		sort.Strings(types) // deterministic message
		return fmt.Errorf("permission validation failed: %d access rule type(s) used in route declarations have no registered enforcer: %v", len(types), types)
	}
	return nil
}
