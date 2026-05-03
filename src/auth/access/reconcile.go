package access

import (
	"log"
	"soli/formations/src/auth/interfaces"
)

// ReconcilePolicy idempotently registers a Casbin policy for the exact triple
// (role, path, method). It is the single helper modules should use from their
// permissions.go files.
//
// Lives in its own package to avoid pulling in initialization's swagger
// dependency from module permission files.
//
// Behavior:
//   - If a row matching the exact (role, path, method) triple already exists,
//     this is a no-op.
//   - Otherwise the row is added via AddPolicy. Casbin's AddPolicy is itself
//     idempotent on exact duplicates, so concurrent / duplicate calls are safe.
//
// History:
// This function previously filtered existing rows by (role, path) only and
// called RemoveFilteredPolicy(0, role, path) before re-adding the new method,
// which silently wiped every other method registered on that path. When two
// methods were registered for the same path during startup (e.g. GET then POST
// on /api/v1/groups/:groupId/scenarios), only the last one survived. See
// issue #297 for the production audit (6 wiped policies in prod).
//
// Trade-off: if a method is later removed from a module's permissions.go, the
// corresponding casbin_rule row will linger in the DB until cleaned up
// explicitly. That is an over-grant condition, detectable via
// ValidatePermissionSetup, and strictly safer than the previous silent
// lockout bug. Cleanup is out of scope here.
func ReconcilePolicy(enforcer interfaces.EnforcerInterface, role, path, method string) {
	existing, err := enforcer.GetFilteredPolicy(0, role, path, method)
	if err != nil {
		log.Printf("Error reading policy for %s %s %s: %v — falling back to add", role, path, method, err)
	} else if len(existing) > 0 {
		// Exact triple is already present — nothing to do.
		return
	}

	if _, err := enforcer.AddPolicy(role, path, method); err != nil {
		log.Printf("Error adding permission for %s %s %s: %v", role, path, method, err)
	} else {
		log.Printf("✅ Added %s permission for %s %s", role, method, path)
	}
}
