# Phase 3: Role Simplification - Implementation Guide

## Overview

Phase 3 simplifies the OCF Core role system from 7 system roles to just 2 (`member` and `administrator`). Business roles (trainer, group manager, etc.) are now determined by organization and group membership.

**Status**: ✅ Implementation Complete (Backward Compatible)

## What Changed

### Before Phase 3 (7 System Roles)
```go
- guest           // No access
- member          // Basic user
- member_pro      // Paying user
- group_manager   // Can manage groups
- trainer         // Can share machines
- organization    // Enterprise account
- administrator   // Full system access
```

### After Phase 3 (2 System Roles)
```go
- member          // Default authenticated user
- administrator   // System administrator (platform management)
```

### Business Roles (Context-Based)

Business capabilities are now determined by **organization** and **group** membership:

#### Organization Roles
```go
- owner    // Full organization control, manages billing
- manager  // Full access to all org groups and resources
- member   // Basic org access
```

#### Group Roles
```go
- owner      // Full group control
- admin      // Manages group settings and members
- assistant  // Helper role (teaching assistant)
- member     // Regular group member (student)
```

## Migration Status

### ✅ Completed

1. **Simplified roles.go**:
   - Only `Member` and `Administrator` as active roles
   - Old roles kept as DEPRECATED (backward compatible)
   - Updated Casdoor role mapping

2. **Feature Access**:
   - Features now come from organization subscriptions
   - `payment/utils.GetUserEffectiveFeatures()` aggregates across all orgs
   - `payment/utils.CanUserAccessFeature()` checks feature access

3. **Permission System**:
   - Casbin handles context-based permissions
   - Organization membership grants cascading group access
   - Direct user permissions for specific resources

4. **Migration Script**:
   - `scripts/migrate_roles_phase3.go` handles user migration
   - Converts old roles to org/group membership
   - Maintains administrator privileges

### 🔄 Backward Compatibility

Old role constants and functions are **deprecated but functional**:
- Old code continues to work
- Deprecated functions return safe defaults
- Clear migration path provided

## How to Use the New System

### Check if User is Administrator

**Old Way** (deprecated):
```go
if user.Role == models.Admin {
    // System admin
}
```

**New Way**:
```go
if models.IsSystemAdmin(user.Role) {
    // System admin
}
```

### Get User's Effective Features

**Old Way** (deprecated):
```go
features := models.GetRoleFeatures(user.Role)
if features.MaxCourses > count {
    // Allow
}
```

**New Way**:
```go
import "soli/formations/src/payment/utils"

plan, err := utils.GetUserEffectiveFeatures(db, userID)
if err != nil {
    // Handle error
}
if plan.MaxCourses == -1 || plan.MaxCourses > count {
    // Allow
}
```

### Check Feature Access

**New Way**:
```go
import "soli/formations/src/payment/utils"

canExport, err := utils.CanUserAccessFeature(db, userID, "can_export_courses")
if err != nil {
    // Handle error
}
if canExport {
    // Allow export
}
```

### Check Organization Membership

```go
import "soli/formations/src/organizations/services"

orgService := services.NewOrganizationService(db)
isMember, err := orgService.IsUserInOrganization(orgID, userID)
if err != nil {
    // Handle error
}
if isMember {
    // User belongs to organization
}
```

### Check if User Can Manage Organization

```go
import "soli/formations/src/organizations/services"

orgService := services.NewOrganizationService(db)
canManage, err := orgService.CanUserManageOrganization(orgID, userID)
if err != nil {
    // Handle error
}
if canManage {
    // User is owner or manager
}
```

## Running the Migration

### Prerequisites

1. **Backup your database** before running migration
2. Ensure all 3 phases are deployed:
   - Phase 1: Organizations ✅
   - Phase 2: Organization subscriptions ✅
   - Phase 3: Role simplification ✅

### Execute Migration

```bash
# Run the migration script
go run scripts/migrate_roles_phase3.go
```

### What the Migration Does

1. **Identifies user roles** from Casdoor
2. **Keeps administrators** as-is
3. **Converts all others to "member"**
4. **Ensures organization membership**:
   - Users with trainer/supervisor roles → Organization managers
   - Regular users → Organization members
5. **Reports summary** of changes

### Post-Migration Verification

```bash
# Check user roles in Casdoor
# All non-admins should now be "member"

# Verify organization memberships
# Users should have appropriate org roles

# Test permissions
# Ensure users can access expected resources
```

## API Changes (None - Backward Compatible)

**No breaking API changes.** All existing endpoints work as before:
- ✅ Authentication endpoints unchanged
- ✅ Authorization via Casbin unchanged
- ✅ Feature limits now come from org subscriptions
- ✅ Permission checks work via org/group membership

## Feature Access Flow

### Old System (Role-Based)
```
User → System Role → GetRoleFeatures() → Feature Limits
```

### New System (Organization-Based)
```
User → Organization Memberships → Subscription Plans → Aggregate Features → Max Limits
```

**Example**: User belongs to 3 organizations
- Personal Org: Free plan (3 courses max)
- Company A: Pro plan (unlimited courses)
- Company B: Team plan (50 courses max)

**Result**: User gets **unlimited courses** (highest limit across all orgs)

## Code Examples

### Frontend: Check User Role

```typescript
// Get current user
const response = await fetch('http://localhost:8080/api/v1/users/me', {
  headers: { 'Authorization': `Bearer ${token}` }
});
const user = await response.json();

// Check if admin (only 2 possible system roles now)
const isAdmin = user.roles?.some(role =>
  role.name === 'administrator' || role.name === 'admin'
);

// For business capabilities, check organization membership
const isOrgManager = user.organization_memberships?.some(membership =>
  membership.role === 'manager' || membership.role === 'owner'
);
```

### Backend: Feature Gate

```go
// Check if user can access advanced features
func (c *courseController) CreateAdvancedCourse(ctx *gin.Context) {
    userID := ctx.GetString("userId")

    // Get user's effective features from all organizations
    plan, err := utils.GetUserEffectiveFeatures(sqldb.DB, userID)
    if err != nil {
        ctx.JSON(500, gin.H{"error": "Failed to check features"})
        return
    }

    // Check specific feature
    if !plan.CanCreateAdvancedLabs {
        ctx.JSON(403, gin.H{"error": "Advanced labs not available in your subscription"})
        return
    }

    // Proceed with creation
    // ...
}
```

## Troubleshooting

### Issue: User Can't Access Resources After Migration

**Cause**: User doesn't have organization membership

**Solution**:
```bash
# Check user's organizations
curl -X GET http://localhost:8080/api/v1/users/me/organizations \
  -H "Authorization: Bearer $TOKEN"

# If no organizations, create personal org
# (Should be auto-created, but if missing)
curl -X POST http://localhost:8080/api/v1/organizations \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"personal","is_personal":true}'
```

### Issue: Features Not Working

**Cause**: Organization doesn't have subscription

**Solution**:
```bash
# Subscribe organization to a plan
curl -X POST http://localhost:8080/api/v1/organizations/{orgId}/subscribe \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"subscription_plan_id":"PLAN_ID"}'
```

### Issue: Permission Denied Errors

**Cause**: Casbin policies not updated

**Solution**: Restart the server to reload Casbin policies

## Testing Checklist

- [ ] All users migrated successfully
- [ ] Administrators can still access admin features
- [ ] Regular users can access their organizations
- [ ] Organization managers can manage their orgs
- [ ] Group admins can manage their groups
- [ ] Feature limits work correctly
- [ ] Subscriptions apply features to orgs
- [ ] Feature aggregation works across multiple orgs
- [ ] No permission errors in logs
- [ ] Frontend displays correct role information

## Rollback Plan

If issues occur, rollback steps:

1. **Restore database** from backup
2. **Revert roles.go** to old version
3. **Restart application**
4. **Investigate issue** before re-attempting

## Next Steps (Optional Cleanup)

Once Phase 3 is stable and verified:

1. **Remove deprecated code** in roles.go:
   - Delete old role constants
   - Delete deprecated functions
   - Remove backward compatibility code

2. **Update documentation**:
   - Remove references to old roles
   - Update all role examples
   - Clarify organization-based access

3. **Clean up tests**:
   - Update tests to use new role system
   - Remove old role-based test cases
   - Add org membership test cases

## References

- Architecture Document: `ORGANIZATION_ARCHITECTURE_MIGRATION.md`
- Frontend Guide: `FRONTEND_SUBSCRIPTION_INTEGRATION_GUIDE.md`
- Feature Access Utils: `src/payment/utils/featureAccess.go`
- Organization Service: `src/organizations/services/organizationService.go`
- Simplified Roles: `src/auth/models/roles.go`

## Support

For issues or questions:
1. Check logs for specific errors
2. Verify organization memberships
3. Ensure subscriptions are active
4. Test with fresh user account
5. Review Casbin policies in database
