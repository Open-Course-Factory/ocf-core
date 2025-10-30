---
description: Debug permission system - check what policies exist for users/roles
tags: [permissions, casbin, debug, auth]
---

# Permission Debugger

Analyze and debug Casbin permissions for users, roles, or routes.

## What to Check

1. **Ask what to investigate:**
   - A specific user ID
   - A role (member, admin, etc.)
   - A route path
   - All permissions for an entity type

2. **Search permission-related code:**
   - Use Grep to find `utils.AddPolicy` calls
   - Find `GetEntityRoles()` implementations
   - Look for permission setup in services

3. **Analyze permission patterns:**
   - Entity-level permissions (from registration)
   - User-specific permissions (sharing, ownership)
   - Role-based permissions (admin, member, etc.)
   - Custom route permissions

4. **Common issues to check:**
   - Missing `LoadPolicy` before operations
   - Permissions not added during entity creation
   - Wrong route path format (must match `ctx.FullPath()`)
   - Permissions not removed on entity deletion
   - Role mappings (student → member)

5. **Show findings:**
   - Where permissions are granted
   - What HTTP methods are allowed
   - Which middleware applies
   - Potential permission gaps

6. **Suggest fixes:**
   - Use `utils.AddPolicy()` helpers
   - Add LoadPolicyFirst option
   - Check permission cleanup in delete handlers

Reference: See `.claude/docs/REFACTORING_COMPLETE_SUMMARY.md` for permission patterns.
