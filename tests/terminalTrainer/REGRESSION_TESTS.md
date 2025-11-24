# Terminal PATCH Permission Regression Tests

## Purpose

These tests were added to prevent the security bug where member role had universal PATCH access to all terminals from happening again.

## The Bug That Was Missed

**Original tests (Tests 1-9)** tested user-specific permissions but **never assigned users to roles**, so they didn't replicate the production environment where:
- Real users have role assignments (via Casdoor)
- Role-level permissions are checked BEFORE user-specific permissions
- A role-level PATCH permission would bypass user-specific restrictions

## New Regression Tests

### Test 10: `TestMemberRoleDoesNotGrantUniversalPatch` 🔒 **CRITICAL**

**Purpose**: Verify that having the "member" role does NOT grant PATCH access to terminals you don't own.

**What it tests**:
1. Creates a terminal owned by user A
2. Assigns user B to "member" role (like production does)
3. Verifies user B cannot PATCH the terminal
4. Verifies owner (user A) still can PATCH

**Why this would have caught the bug**:
```go
// BEFORE the fix (with bug):
roleMap["member"] = "(GET|POST|PATCH)"  // ❌ Bug!

// Test would FAIL because:
// - User B has "member" role
// - "member" role has PATCH at role-level
// - Role permission checked first → Access granted ❌

// AFTER the fix (secure):
roleMap["member"] = "(GET|POST)"  // ✅ No PATCH at role-level

// Test PASSES because:
// - User B has "member" role
// - "member" role has NO PATCH at role-level
// - Falls back to user-specific permissions
// - User B has no user-specific PATCH policy → Access denied ✅
```

**Code Location**: Lines 406-434

### Test 11: `TestMemberRoleCanPerformBasicOperations` ✅

**Purpose**: Verify that member role still allows normal operations (GET, POST).

**What it tests**:
1. User with "member" role can create terminals (POST)
2. User with "member" role can view terminals (GET)
3. User who creates a terminal gets PATCH via ownership hook (not role)

**Why this is important**: Ensures our fix didn't break legitimate member operations.

**Code Location**: Lines 437-462

### Test 12: `TestMemberRoleCannotPatchViaKeymatch` 🔍

**Purpose**: Verify PATCH is blocked at multiple levels.

**What it tests**:
1. Member cannot PATCH via the actual UUID route (`/api/v1/terminals/019ab...`)
2. Member cannot PATCH via the pattern route (`/api/v1/terminals/:id`)
3. Tests both Casbin keymatch scenarios

**Why this is important**: Ensures the security isn't accidentally bypassed through route pattern matching.

**Code Location**: Lines 465-494

## Test Coverage Summary

| Test # | Name | Purpose | Key Assertion |
|--------|------|---------|---------------|
| 1 | TestTerminalOwnerCanPatch | Owner can PATCH | ✅ Owner has permission |
| 2 | TestNonOwnerMemberCannotPatch | Non-owner blocked | ❌ Non-owner has no permission |
| 3 | TestSharedUserReadAccessCannotPatch | "read" cannot PATCH | ❌ Read access blocked |
| 4 | TestSharedUserWriteAccessCanPatch | "write" can PATCH | ✅ Write access allowed |
| 5 | TestSharedUserAdminAccessCanPatch | "admin" can PATCH | ✅ Admin access allowed |
| 6 | TestRevokeShareRemovesPermission | Revoke works | ❌ Permission removed |
| 7 | TestDeleteTerminalRemovesAllPolicies | Cleanup works | ❌ All policies removed |
| 8 | TestAdministratorCanPatchAnyTerminal | Admin bypasses | ✅ Admin has permission |
| 9 | TestMultipleShares | Multiple shares | Mixed assertions |
| **10** | **TestMemberRoleDoesNotGrantUniversalPatch** | **🔒 Role isolation** | **❌ Member role blocked** |
| **11** | **TestMemberRoleCanPerformBasicOperations** | **✅ Basic ops work** | **✅ GET/POST allowed** |
| **12** | **TestMemberRoleCannotPatchViaKeymatch** | **🔍 Pattern matching** | **❌ All routes blocked** |

## How to Verify the Tests Catch the Bug

1. **Revert the fix** temporarily:
```bash
git diff src/terminalTrainer/entityRegistration/terminalRegistration.go
```

2. **Change back to the buggy version**:
```go
roleMap[string(authModels.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + ")"
```

3. **Run Test 10**:
```bash
go test -v -run TestMemberRoleDoesNotGrantUniversalPatch ./tests/terminalTrainer/
```

**Expected**: Test should FAIL with:
```
Error: Should be false
Test: TestMemberRoleDoesNotGrantUniversalPatch
Messages: Member role should NOT grant PATCH to terminals they don't own
```

4. **Restore the fix** and test passes ✅

## Lessons Learned

### What Was Missing in Original Tests

1. ❌ No role assignments to test users
2. ❌ Didn't replicate production permission check flow
3. ❌ Only tested user-specific permissions in isolation

### What New Tests Provide

1. ✅ Role assignments to test users (like production)
2. ✅ Full permission check flow (role → user-specific)
3. ✅ Tests both role-level and user-specific permissions

### Best Practices for Security Tests

1. **Replicate production environment** - Include role assignments, not just policies
2. **Test negative cases** - Verify unauthorized access is blocked
3. **Test permission precedence** - Understand role vs user-specific checks
4. **Test all access paths** - Pattern routes, UUID routes, keymatch scenarios
5. **Document regression tests** - Explain what bug they prevent

## Running the Tests

```bash
# Run all terminal permission tests
go test -v ./tests/terminalTrainer/terminal_patch_permissions_test.go

# Run only regression tests
go test -v -run "TestMemberRole" ./tests/terminalTrainer/terminal_patch_permissions_test.go

# Run the critical regression test
go test -v -run TestMemberRoleDoesNotGrantUniversalPatch ./tests/terminalTrainer/
```

## Related Documentation

- Security fix details: `/workspaces/ocf-core/TERMINAL_SECURITY_FIX.md`
- Hook implementations: `/workspaces/ocf-core/src/terminalTrainer/hooks/terminalHooks.go`
- Entity registration: `/workspaces/ocf-core/src/terminalTrainer/entityRegistration/terminalRegistration.go`
