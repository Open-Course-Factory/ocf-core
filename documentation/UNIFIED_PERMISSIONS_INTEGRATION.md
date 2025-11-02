# Unified Permissions System - Frontend Integration Guide

**Date**: 2025-11-02
**Status**: Backend Complete, Frontend Integration Required

## Overview

A unified permission system has been implemented to replace fragmented permission checks across the codebase. This system consolidates:
- Role-based checks (administrator, supervisor, student)
- Organization ownership and membership
- Group membership and roles
- Subscription-based features
- Quick access flags

## Backend API

### Endpoint

```
GET /api/v1/auth/permissions
```

**Authentication**: Required (Bearer token)

### Response Structure

```typescript
interface UserPermissionsOutput {
  user_id: string;
  permissions: PermissionRule[];
  roles: string[];
  is_system_admin: boolean;
  organization_memberships: OrganizationMembershipContext[];
  group_memberships: GroupMembershipContext[];
  aggregated_features: string[];
  can_create_organization: boolean;
  can_create_group: boolean;
  has_any_subscription: boolean;
}

interface PermissionRule {
  resource: string;
  methods: string[];
}

interface OrganizationMembershipContext {
  organization_id: string;
  organization_name: string;
  role: string;  // "owner" | "manager" | "member"
  is_owner: boolean;
  features: string[];
  has_subscription: boolean;
}

interface GroupMembershipContext {
  group_id: string;
  group_name: string;
  role: string;  // "owner" | "admin" | "assistant" | "member"
  is_owner: boolean;
}
```

### Example Response

```json
{
  "user_id": "027ee7be-9843-486e-93f3-60ce9f8dd10b",
  "permissions": [
    {
      "resource": "/api/v1/courses",
      "methods": ["GET", "POST"]
    },
    {
      "resource": "/api/v1/groups",
      "methods": ["GET", "POST", "PUT", "DELETE"]
    }
  ],
  "roles": ["student"],
  "is_system_admin": false,
  "organization_memberships": [
    {
      "organization_id": "019a3f3f-a5ac-7e38-96e2-08d9722e31c8",
      "organization_name": "My Team Org !",
      "role": "owner",
      "is_owner": true,
      "features": [],
      "has_subscription": false
    }
  ],
  "group_memberships": [],
  "aggregated_features": [],
  "can_create_organization": true,
  "can_create_group": true,
  "has_any_subscription": false
}
```

## Frontend Implementation

### 1. Fetch Permissions on App Load

```typescript
// services/permissionsService.ts
export async function fetchUserPermissions(token: string): Promise<UserPermissionsOutput> {
  const response = await fetch('http://localhost:8080/api/v1/auth/permissions', {
    headers: {
      'Authorization': `Bearer ${token}`
    }
  });

  if (!response.ok) {
    throw new Error('Failed to fetch user permissions');
  }

  return response.json();
}
```

### 2. Store Permissions in State

```typescript
// Example with React Context
import { createContext, useContext, useEffect, useState } from 'react';

interface PermissionsContextType {
  permissions: UserPermissionsOutput | null;
  loading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

const PermissionsContext = createContext<PermissionsContextType | undefined>(undefined);

export function PermissionsProvider({ children, token }) {
  const [permissions, setPermissions] = useState<UserPermissionsOutput | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchPermissions = async () => {
    try {
      setLoading(true);
      const data = await fetchUserPermissions(token);
      setPermissions(data);
      setError(null);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (token) {
      fetchPermissions();
    }
  }, [token]);

  return (
    <PermissionsContext.Provider value={{ permissions, loading, error, refetch: fetchPermissions }}>
      {children}
    </PermissionsContext.Provider>
  );
}

export function usePermissions() {
  const context = useContext(PermissionsContext);
  if (!context) {
    throw new Error('usePermissions must be used within PermissionsProvider');
  }
  return context;
}
```

### 3. Navigation Menu - Show/Hide Based on Permissions

**CRITICAL**: The group menu should be shown when `can_create_group === true`

```typescript
// components/Navigation.tsx
import { usePermissions } from '../contexts/PermissionsContext';

export function Navigation() {
  const { permissions, loading } = usePermissions();

  if (loading) {
    return <LoadingSpinner />;
  }

  return (
    <nav>
      {/* Always show for authenticated users */}
      <NavItem label="Dashboard" path="/dashboard" />

      {/* Show courses if user has permission */}
      {permissions?.permissions.some(p => p.resource === '/api/v1/courses') && (
        <NavItem label="Courses" path="/courses" />
      )}

      {/* Show organizations if user can create or is member of any */}
      {(permissions?.can_create_organization || permissions?.organization_memberships.length > 0) && (
        <NavItem label="Organizations" path="/organizations" />
      )}

      {/* CRITICAL: Show groups menu if user can create groups */}
      {permissions?.can_create_group && (
        <NavItem label="Groups" path="/groups" />
      )}

      {/* Show admin panel for system admins */}
      {permissions?.is_system_admin && (
        <NavItem label="Admin" path="/admin" />
      )}
    </nav>
  );
}
```

### 4. Check Specific Organization/Group Permissions

```typescript
// hooks/useOrganizationPermissions.ts
import { usePermissions } from '../contexts/PermissionsContext';

export function useOrganizationPermissions(organizationId: string) {
  const { permissions } = usePermissions();

  const orgMembership = permissions?.organization_memberships.find(
    m => m.organization_id === organizationId
  );

  return {
    isMember: !!orgMembership,
    isOwner: orgMembership?.is_owner ?? false,
    isManager: orgMembership?.role === 'manager',
    role: orgMembership?.role,
    features: orgMembership?.features ?? [],
    hasSubscription: orgMembership?.has_subscription ?? false,
    canManage: orgMembership?.is_owner || orgMembership?.role === 'manager'
  };
}

// hooks/useGroupPermissions.ts
export function useGroupPermissions(groupId: string) {
  const { permissions } = usePermissions();

  const groupMembership = permissions?.group_memberships.find(
    m => m.group_id === groupId
  );

  return {
    isMember: !!groupMembership,
    isOwner: groupMembership?.is_owner ?? false,
    isAdmin: groupMembership?.role === 'admin',
    role: groupMembership?.role,
    canManage: groupMembership?.is_owner || groupMembership?.role === 'admin'
  };
}
```

### 5. Feature-Based UI

```typescript
// components/OrganizationFeatures.tsx
import { useOrganizationPermissions } from '../hooks/useOrganizationPermissions';

export function OrganizationFeatures({ organizationId }) {
  const { features } = useOrganizationPermissions(organizationId);

  return (
    <div>
      {features.includes('bulk_purchase') && (
        <Button>Purchase Bulk Licenses</Button>
      )}

      {features.includes('advanced_labs') && (
        <Button>Create Advanced Lab</Button>
      )}

      {features.includes('custom_themes') && (
        <Button>Customize Theme</Button>
      )}
    </div>
  );
}
```

## Permission Check Logic

### Backend Logic Summary

From `src/auth/services/userPermissionsService.go:95`:

```go
canCreateGroup := isSystemAdmin || len(orgMemberships) > 0
```

**This means:**
- System admins can ALWAYS create groups
- ANY user who is a member of at least one organization can create groups
- Users with zero organization memberships CANNOT create groups

### Frontend Checks

```typescript
// Simple permission checks
function canUserCreateGroup(permissions: UserPermissionsOutput): boolean {
  return permissions.can_create_group;
}

function canUserCreateOrganization(permissions: UserPermissionsOutput): boolean {
  return permissions.can_create_organization;
}

function canUserManageOrganization(
  permissions: UserPermissionsOutput,
  organizationId: string
): boolean {
  const org = permissions.organization_memberships.find(
    m => m.organization_id === organizationId
  );
  return org?.is_owner || org?.role === 'manager';
}

function hasFeature(
  permissions: UserPermissionsOutput,
  featureKey: string
): boolean {
  return permissions.aggregated_features.includes(featureKey);
}
```

## Current Issue: User Cannot See Group Menu

### Problem
User `tsaquet+7@gmail.com` (tsaquet7) owns a team organization but cannot see the group menu.

### Root Cause
The frontend is **not yet integrated** with the new permissions endpoint. It's likely using old permission checks that don't account for organization membership.

### Solution
1. Integrate the frontend with `GET /api/v1/auth/permissions`
2. Update navigation menu to check `permissions.can_create_group`
3. Remove old role-based checks for group menu visibility

### Expected Behavior After Integration
- User `tsaquet7` should see `can_create_group: true` in the API response
- The group menu should appear in the navigation
- User should be able to create groups within their team organization

## Migration Checklist

### Backend (✅ Complete)
- [x] Created `GET /api/v1/auth/permissions` endpoint
- [x] Implemented permission aggregation logic
- [x] Added Casbin permissions for the endpoint
- [x] Wrote comprehensive tests
- [x] Integrated with organization subscriptions
- [x] Calculated `can_create_group` flag correctly

### Frontend (❌ Required)
- [ ] Create permissions service to fetch from API
- [ ] Set up permissions context/store
- [ ] Update navigation menu to use `can_create_group` flag
- [ ] Update organization pages to use membership data
- [ ] Update group pages to use membership data
- [ ] Remove old role-based permission checks
- [ ] Add loading states during permission fetch
- [ ] Handle permission fetch errors gracefully
- [ ] Test with various user types (admin, owner, member, non-member)

## Testing

### Test Cases
1. **System Admin**
   - Should see all menus
   - `can_create_group: true`
   - `can_create_organization: true`

2. **Organization Owner** (like tsaquet7)
   - Should see group menu
   - `can_create_group: true`
   - Has organization in `organization_memberships` with `is_owner: true`

3. **Organization Member**
   - Should see group menu
   - `can_create_group: true`
   - Has organization in `organization_memberships` with `is_owner: false`

4. **User with No Organizations**
   - Should NOT see group menu
   - `can_create_group: false`
   - Empty `organization_memberships` array

### Testing the API Manually

```bash
# Login to get token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "tsaquet+7@gmail.com", "password": "Test123!"}'

# Fetch permissions (replace TOKEN)
curl -H "Authorization: Bearer TOKEN" \
  http://localhost:8080/api/v1/auth/permissions | jq
```

## Next Steps

1. **Immediate**: Update frontend navigation to call permissions endpoint
2. **Short-term**: Replace all fragmented permission checks with unified system
3. **Long-term**: Add permission caching and real-time updates on subscription changes

## Contact

For questions about this integration, see:
- Backend code: `src/auth/services/userPermissionsService.go`
- Endpoint handler: `src/auth/routes/usersRoutes/getUserPermissions.go`
- Tests: `src/auth/services/userPermissionsService_test.go`
