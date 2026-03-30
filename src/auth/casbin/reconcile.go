package casbin

import (
	"log"
	"soli/formations/src/auth/interfaces"
)

// ReconcilePolicy compares the desired policy with what exists in the DB and only
// makes changes when they differ. This is safe for production — no blind deletes.
// Lives in its own package to avoid pulling in initialization's swagger dependency.
func ReconcilePolicy(enforcer interfaces.EnforcerInterface, role, path, method string) {
	existing, err := enforcer.GetFilteredPolicy(0, role, path)
	if err != nil {
		log.Printf("Error reading policy for %s %s: %v — falling back to add", role, path, err)
		enforcer.AddPolicy(role, path, method)
		return
	}

	// Check if exact policy already exists
	for _, policy := range existing {
		if len(policy) >= 3 && policy[2] == method {
			return // Already correct, nothing to do
		}
	}

	// Policy is missing or has a different method — fix it
	if len(existing) > 0 {
		oldMethod := existing[0][2]
		enforcer.RemoveFilteredPolicy(0, role, path)
		log.Printf("🔄 Updating permission %s %s: %s → %s", role, path, oldMethod, method)
	}

	_, err = enforcer.AddPolicy(role, path, method)
	if err != nil {
		log.Printf("Error adding permission for %s %s %s: %v", role, path, method, err)
	} else {
		log.Printf("✅ Added %s permission for %s %s", role, method, path)
	}
}
