# Terminal PATCH Permission Security Fix

## Issue Report

**Reporter**: User (tsaquet+7@gmail.com)
**Date**: 2025-11-24
**Severity**: HIGH - Unauthorized Access

### Problem Description

User was able to update (PATCH) terminal `019ab6d0-cb61-7a16-a9f2-9fbac3a132fe` that they:
- Did NOT create/own
- Was NOT shared with them

This violated the intended security model where only:
- Terminal owners
- Users with "write" or "admin" share access
- System administrators

...should be able to modify terminals.

## Root Cause Analysis

### The Casbin Permission Check Flow

The `AuthManagement` middleware checks permissions in this order:

1. **First**: Check role-based permissions (for each user role)
2. **Fallback**: Check user-specific permissions (only if role check fails)

Code from `/workspaces/ocf-core/src/auth/authMiddleware.go` (lines 82-105):

```go
// Check authorization for each role - if any role has permission, allow access
authorized := false
for _, role := range userRoles {
    ok, errEnforce := am.permissionService.HasPermission(role, ctx.Request.URL.Path, ctx.Request.Method)
    if ok {
        authorized = true
        break  // ⚠️ Stops here if role has permission
    }
}

// Also check direct user permissions (fallback for specific user permissions)
if !authorized {  // ⚠️ Only checked if role permissions FAILED
    ok, errEnforce := am.permissionService.HasPermission(fmt.Sprint(userId), ctx.Request.URL.Path, ctx.Request.Method)
    authorized = ok
}
```

### The Bug

In `/workspaces/ocf-core/src/terminalTrainer/entityRegistration/terminalRegistration.go`:

```go
// BEFORE (VULNERABLE):
roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + ")"
```

This granted **ALL members** PATCH permission at the role level, meaning:
- User attempts PATCH `/api/v1/terminals/019ab6d0-cb61-7a16-a9f2-9fbac3a132fe`
- Auth middleware checks: Does "member" role have PATCH on `/api/v1/terminals/:id`?
- Answer: YES ✅ → Access granted immediately
- User-specific permissions (from hooks) **never checked**

### Why Hooks Weren't Protecting Us

The hooks we implemented DO work correctly:
- `TerminalOwnerPermissionHook` - Grants PATCH to terminal owner
- `TerminalSharePermissionHook` - Grants PATCH to shared users (write/admin)
- `TerminalShareRevokeHook` - Removes PATCH when shares are revoked
- `TerminalCleanupHook` - Removes all permissions on terminal deletion

**BUT**: These create user-specific Casbin policies that are only checked as a **fallback** after role-level permissions fail.

## The Fix

### Changed File: `/workspaces/ocf-core/src/terminalTrainer/entityRegistration/terminalRegistration.go`

```go
// AFTER (SECURE):
// NOTE: Member role does NOT have PATCH at role-level - PATCH is granted via user-specific
// permissions through hooks (owner or shared with write/admin access)
roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"
roleMap[string(authModels.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + "|" + http.MethodPatch + ")"
```

### How It Works Now

1. User attempts PATCH terminal
2. Auth middleware checks role permissions
   - Member role: GET, POST only (no PATCH) → ❌ Denied
   - Admin role: GET, POST, PATCH, DELETE → ✅ Allowed (admins can modify any terminal)
3. If role check fails, fallback to user-specific permissions
   - Check if user has user-specific PATCH policy for this terminal
   - These are created by hooks:
     - When user creates a terminal (owner)
     - When terminal is shared with user (write/admin access)

### Permission Flow Diagram

```
User tries to PATCH terminal
│
├─> Has Admin role?
│   └─> YES → ✅ Allow (role-level permission)
│
└─> Has Member role?
    ├─> YES → Check user-specific permissions
    │   ├─> Is terminal owner? → ✅ Allow (hook granted)
    │   ├─> Shared with write access? → ✅ Allow (hook granted)
    │   ├─> Shared with admin access? → ✅ Allow (hook granted)
    │   └─> None of above? → ❌ Deny (403 Forbidden)
    │
    └─> NO → ❌ Deny (not authenticated)
```

## Test Coverage

All 9 test cases pass with the fix:

1. ✅ `TestTerminalOwnerCanPatch` - Owner can PATCH their terminal
2. ✅ `TestNonOwnerMemberCannotPatch` - Non-owner cannot PATCH
3. ✅ `TestSharedUserReadAccessCannotPatch` - "read" access cannot PATCH
4. ✅ `TestSharedUserWriteAccessCanPatch` - "write" access can PATCH
5. ✅ `TestSharedUserAdminAccessCanPatch` - "admin" access can PATCH
6. ✅ `TestRevokeShareRemovesPermission` - Revoking share removes PATCH
7. ✅ `TestDeleteTerminalRemovesAllPolicies` - Deleting terminal cleans up
8. ✅ `TestAdministratorCanPatchAnyTerminal` - Admin bypasses restrictions
9. ✅ `TestMultipleShares` - Multiple shares work correctly

## Files Changed

1. `/workspaces/ocf-core/src/terminalTrainer/entityRegistration/terminalRegistration.go`
   - Removed PATCH from member role-level permissions

2. `/workspaces/ocf-core/tests/terminalTrainer/terminal_patch_permissions_test.go`
   - Updated test setup to match new permission model

## Impact

- ✅ Terminal owners can still rename/update their terminals
- ✅ Users with write/admin share access can still update
- ✅ Administrators can still update any terminal
- ❌ Regular members can NO LONGER update terminals they don't own/have access to

## Verification Steps

1. Restart the server (to reload role permissions)
2. Try to PATCH a terminal you don't own → Should get 403 Forbidden
3. Create a terminal → Should be able to PATCH it (owner)
4. Share terminal with "write" access → Shared user can PATCH
5. Share terminal with "read" access → Shared user cannot PATCH
6. Revoke share → User loses PATCH permission

## Lessons Learned

1. **Role-level permissions override user-specific permissions** in the current Casbin model
2. **Fine-grained access control** should use user-specific policies, NOT role-level policies
3. **Always test with actual unauthorized users** - our unit tests caught this, but only after the issue was reported
4. **Security tests should include negative cases** - testing that unauthorized access is properly denied

## Related Files

- Hook implementations: `/workspaces/ocf-core/src/terminalTrainer/hooks/terminalHooks.go`
- Auth middleware: `/workspaces/ocf-core/src/auth/authMiddleware.go`
- Test suite: `/workspaces/ocf-core/tests/terminalTrainer/terminal_patch_permissions_test.go`
- Entity registration: `/workspaces/ocf-core/src/terminalTrainer/entityRegistration/terminalRegistration.go`
