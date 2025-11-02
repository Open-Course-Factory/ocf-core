# Organizations & Groups System - Complete Guide

**Status**: ✅ Phase 1 Complete (Organizations & Groups)
**Date**: 2025-01-27
**Architecture**: GitLab-style Multi-tenant System

---

## Overview

OCF Core implements a GitLab-style organization and groups system that supports multi-tenancy, hierarchical groups, and context-based permissions. This replaces the previous role-based access control system with a more flexible organization-based approach.

---

## Architecture

### System Hierarchy

```
User (system role: member or administrator)
  │
  ├─ Personal Organization (auto-created)
  │    ├─ Full owner access
  │    └─ Personal groups
  │
  ├─ Organization Membership (role: owner/manager/member)
  │    ├─ Organization A (Company)
  │    │    ├─ Group 1 (accessible via org membership)
  │    │    ├─ Group 2
  │    │    └─ Group 3
  │    │
  │    └─ Organization B (School)
  │         ├─ Department Group (parent)
  │         │    ├─ Class A (child)
  │         │    └─ Class B (child)
  │         └─ Admin Group
  │
  └─ Direct Group Membership (role: owner/admin/assistant/member)
       └─ Specific Group (not via org)
```

### Key Concepts

1. **Personal Organizations**
   - Every user gets a personal organization (like GitHub)
   - Auto-created on user registration/first login
   - User is the owner
   - Used for personal groups and resources

2. **Multi-tenancy**
   - Users can belong to multiple organizations
   - Different roles in different organizations
   - Same user can be owner in Org A, member in Org B

3. **Cascading Permissions**
   - Organization owners/managers automatically access all org groups
   - No need for explicit group membership
   - Group-level roles still apply for fine-grained control

4. **Hierarchical Groups**
   - Groups can have parent-child relationships
   - Nested structure (Department → Classes)
   - Permissions don't cascade through group hierarchy (only org → groups)

---

## Models

### Organization

**File**: `src/organizations/models/organization.go`

```go
type Organization struct {
    BaseModel
    Name               string                 // Unique identifier
    DisplayName        string                 // Human-readable name
    Description        string                 // Optional description
    OwnerUserID        string                 // Primary owner (creator)
    SubscriptionPlanID *uuid.UUID             // Organization subscription
    IsPersonal         bool                   // Auto-created personal org
    MaxGroups          int                    // Limit for groups in org
    MaxMembers         int                    // Limit for total org members
    IsActive           bool                   // Organization status
    Metadata           map[string]interface{} // Custom fields

    // Relations
    Groups  []ClassGroup         // All groups in organization
    Members []OrganizationMember // All members
}
```

**Key Methods**:
- `GetMemberCount()` - Current member count
- `GetGroupCount()` - Current group count
- `HasMember(userID)` - Check if user is member
- `IsOwner(userID)` - Check if user is owner
- `GetMemberRole(userID)` - Get user's role in org
- `CanUserManageOrganization(userID)` - Check management permission
- `IsFull()` - Check if at member limit
- `HasReachedGroupLimit()` - Check if at group limit

### OrganizationMember

**File**: `src/organizations/models/organizationMember.go`

```go
type OrganizationMemberRole string

const (
    OrgRoleOwner   OrganizationMemberRole = "owner"   // Full control
    OrgRoleManager OrganizationMemberRole = "manager" // Manage groups/members
    OrgRoleMember  OrganizationMemberRole = "member"  // Basic access
)

type OrganizationMember struct {
    BaseModel
    OrganizationID uuid.UUID              // Parent organization
    UserID         string                 // User ID
    Role           OrganizationMemberRole // Role in organization
    InvitedBy      string                 // Who added this member
    JoinedAt       time.Time              // When joined
    IsActive       bool                   // Membership status
    Metadata       map[string]interface{} // Custom fields

    // Relations
    Organization Organization
}
```

**Key Methods**:
- `IsOwner()` - Check if owner role
- `IsManager()` - Check if manager or owner
- `CanManageOrganization()` - Check management permission
- `CanManageMembers()` - Check member management permission
- `CanManageGroups()` - Check group management permission
- `GetRolePriority()` - Get numeric priority (100=owner, 50=manager, 10=member)
- `HasHigherRoleThan(role)` - Compare roles

### ClassGroup (Enhanced)

**File**: `src/groups/models/group.go`

```go
type ClassGroup struct {
    BaseModel
    // Basic fields
    Name           string     // Unique identifier
    DisplayName    string     // Human-readable name
    Description    string     // Optional description

    // Ownership & Organization
    OwnerUserID    string     // Group creator/owner
    OrganizationID *uuid.UUID // Parent organization (optional)

    // Hierarchy
    ParentGroupID  *uuid.UUID // Parent group for nesting

    // Limits
    MaxMembers     int        // Member capacity

    // Lifecycle
    ExpiresAt      *time.Time // Optional expiration
    IsActive       bool       // Group status

    // Metadata
    Metadata       map[string]interface{}
    ExternalID     string     // For external system integration

    // Relations
    Members       []GroupMember // All members
    ParentGroup   *ClassGroup   // Parent (for nested groups)
    SubGroups     []ClassGroup  // Children
    Organization  *Organization // Parent organization
}
```

**Key Methods**:
- `GetMemberCount()` - Current member count
- `HasMember(userID)` - Check if user is member
- `IsOwner(userID)` - Check if user is owner
- `GetMemberRole(userID)` - Get user's role in group
- `IsExpired()` - Check if past expiration date
- `IsFull()` - Check if at capacity
- `IsNested()` - Check if has parent group

### GroupMember

**File**: `src/groups/models/groupMember.go`

```go
type GroupMemberRole string

const (
    GroupRoleOwner     GroupMemberRole = "owner"     // Full control
    GroupRoleAdmin     GroupMemberRole = "admin"     // Manage members
    GroupRoleAssistant GroupMemberRole = "assistant" // Helper (e.g., TA)
    GroupRoleMember    GroupMemberRole = "member"    // Regular member
)

type GroupMember struct {
    BaseModel
    GroupID   uuid.UUID       // Parent group
    UserID    string          // User ID
    Role      GroupMemberRole // Role in group
    InvitedBy string          // Who added this member
    JoinedAt  time.Time       // When joined
    IsActive  bool            // Membership status
    Metadata  map[string]interface{}

    // Relations
    Group ClassGroup
}
```

**Key Methods**:
- `IsOwner()` - Check if owner role
- `IsAdmin()` - Check if admin or owner
- `CanManageGroup()` - Check management permission
- `CanManageMembers()` - Check member management permission

---

## Permission System

### Access Control Flow

1. **System Admin** → Full access to everything
2. **Organization Owner/Manager** → Access all groups in organization
3. **Direct Group Membership** → Access specific group

```go
// Check if user can access a group
func CanUserAccessGroup(groupID, userID) bool {
    // 1. System administrator
    if IsSystemAdmin(userID) {
        return true
    }

    // 2. Direct group membership
    if IsGroupMember(groupID, userID) {
        return true
    }

    // 3. Organization membership (cascading)
    group := GetGroup(groupID)
    if group.OrganizationID != nil {
        return IsOrganizationMember(userID, group.OrganizationID, OrgRoleManager)
    }

    return false
}
```

### Casbin Policies

**Organization Permissions**:

```go
// Organization members can view org
"group:org:{orgID}", "/api/v1/organizations/{orgID}", "GET"

// Organization managers can manage org
"group:org_manager:{orgID}", "/api/v1/organizations/{orgID}", "GET|PATCH|DELETE"
"group:org_manager:{orgID}", "/api/v1/organizations/{orgID}/groups", "GET|POST"
"group:org_manager:{orgID}", "/api/v1/organizations/{orgID}/members", "GET|POST|PATCH|DELETE"

// Cascading group access for org managers
"group:org_manager:{orgID}", "/api/v1/class-groups/{groupID}", "GET|PATCH|DELETE"
```

**Group Permissions**:

```go
// Group members can view group
"group:group_member:{groupID}", "/api/v1/class-groups/{groupID}", "GET"

// Group admins can manage group
"group:group_admin:{groupID}", "/api/v1/class-groups/{groupID}", "GET|PATCH|DELETE"
"group:group_admin:{groupID}", "/api/v1/class-groups/{groupID}/members", "GET|POST|PATCH|DELETE"
```

---

## API Endpoints

### Organizations

```
POST   /api/v1/organizations                     - Create organization
GET    /api/v1/organizations/:id                 - Get organization details
GET    /api/v1/organizations/:id/groups          - List all groups in org
PATCH  /api/v1/organizations/:id                 - Update organization
DELETE /api/v1/organizations/:id                 - Delete organization

POST   /api/v1/organizations/:id/members         - Add member to org
DELETE /api/v1/organizations/:id/members/:userID - Remove member
PATCH  /api/v1/organizations/:id/members/:userID - Update member role
GET    /api/v1/organizations/:id/members         - List org members

GET    /api/v1/users/me/organizations            - List user's organizations
POST   /api/v1/organizations/:id/import          - Bulk import (CSV)
```

### Groups

```
POST   /api/v1/class-groups                - Create group
GET    /api/v1/class-groups/:id            - Get group details
GET    /api/v1/class-groups                - List groups (filtered by access)
PATCH  /api/v1/class-groups/:id            - Update group
DELETE /api/v1/class-groups/:id            - Delete group

POST   /api/v1/class-groups/:id/members    - Add member to group
DELETE /api/v1/class-groups/:id/members/:userID - Remove member
PATCH  /api/v1/class-groups/:id/members/:userID - Update member role
GET    /api/v1/class-groups/:id/members    - List group members

GET    /api/v1/users/me/groups              - List user's groups
```

---

## Services

### OrganizationService

**File**: `src/organizations/services/organizationService.go`

**Key Methods**:

```go
// Organization management
CreateOrganization(userID, input) (*Organization, error)
CreatePersonalOrganization(userID) (*Organization, error)
GetOrganization(orgID) (*Organization, error)
GetUserOrganizations(userID) ([]Organization, error)
UpdateOrganization(orgID, updates) (*Organization, error)
DeleteOrganization(orgID) error

// Member management
AddMemberToOrganization(orgID, userID, role) error
RemoveMemberFromOrganization(orgID, userID) error
UpdateMemberRole(orgID, userID, newRole) error
GetOrganizationMembers(orgID) ([]OrganizationMember, error)
IsUserInOrganization(orgID, userID) (bool, error)

// Permissions
CanUserManageOrganization(orgID, userID) (bool, error)
GrantOrganizationPermissions(userID, orgID) error
RevokeOrganizationPermissions(userID, orgID) error

// Group access through org
GetOrganizationGroups(orgID) ([]ClassGroup, error)
CanUserAccessGroupViaOrg(groupID, userID) (bool, error)
```

### GroupService (Enhanced)

**File**: `src/groups/services/groupService.go`

**New Features**:
- Organization-aware group creation
- Cascading permission checks (org → group)
- Parent-child group relationships
- External ID support for integration

**Key Methods**:

```go
// Group management
CreateGroup(input, ownerID) (*ClassGroup, error)
GetGroup(groupID) (*ClassGroup, error)
UpdateGroup(groupID, updates) (*ClassGroup, error)
DeleteGroup(groupID) error

// Member management
AddMember(groupID, userID, role) error
RemoveMember(groupID, userID) error
UpdateMemberRole(groupID, userID, newRole) error

// Access control
CanUserAccessGroup(groupID, userID) (bool, error)
CanUserManageGroup(groupID, userID) (bool, error)

// Hierarchy
GetSubGroups(groupID) ([]ClassGroup, error)
GetGroupAncestors(groupID) ([]ClassGroup, error)
```

---

## Hooks

### Organization Hooks

**File**: `src/organizations/hooks/organizationHooks.go`

**Lifecycle Hooks**:

1. **BeforeCreate**
   - Set OwnerUserID
   - Set default values (MaxGroups, MaxMembers)
   - Set IsPersonal flag

2. **AfterCreate**
   - Add owner as organization member (role: owner)
   - Grant organization permissions to owner
   - Log organization creation

3. **BeforeDelete**
   - Check if personal organization (cannot delete)
   - Verify user has permission to delete
   - Check if organization has active groups

4. **AfterDelete**
   - Revoke all organization permissions
   - Clean up orphaned data

### Group Hooks

**File**: `src/groups/hooks/groupHooks.go`

**Enhanced Hooks**:

1. **BeforeCreate**
   - Validate organization membership (if OrganizationID set)
   - Validate parent group exists (if ParentGroupID set)
   - Check organization group limit
   - Set default values

2. **AfterCreate**
   - Add owner as group member (role: owner)
   - Grant group permissions to owner
   - If organization group: grant access to org managers

3. **BeforeUpdate**
   - Prevent orphaning groups (changing OrganizationID)
   - Validate parent group changes (prevent circular references)

---

## Personal Organization Creation

**When**: User registration or first login

```go
func CreatePersonalOrganization(userID string) (*Organization, error) {
    // Check if already exists
    existing := GetPersonalOrganization(userID)
    if existing != nil {
        return existing, nil
    }

    // Create personal organization
    org := &Organization{
        Name:        fmt.Sprintf("personal_%s", userID),
        DisplayName: "Personal Organization",
        OwnerUserID: userID,
        IsPersonal:  true,
        IsActive:    true,
        MaxGroups:   10,
        MaxMembers:  50,
    }

    created, err := CreateOrganization(org)
    if err != nil {
        return nil, err
    }

    return created, nil
}
```

---

## Common Use Cases

### 1. Company Organization

```go
// Create company organization
org := CreateOrganization(ownerID, CreateOrganizationInput{
    Name:        "acme-corp",
    DisplayName: "ACME Corporation",
    MaxGroups:   50,
    MaxMembers:  200,
})

// Add employees as members
AddMemberToOrganization(org.ID, employeeID, OrgRoleMember)

// Add managers
AddMemberToOrganization(org.ID, managerID, OrgRoleManager)

// Create department groups
deptGroup := CreateGroup(CreateGroupInput{
    Name:           "engineering",
    DisplayName:    "Engineering Department",
    OrganizationID: org.ID,
})

// Managers automatically have access to department group
// No need to add them explicitly
```

### 2. School Organization with Nested Groups

```go
// Create school organization
school := CreateOrganization(adminID, CreateOrganizationInput{
    Name:        "school-paris",
    DisplayName: "School of Paris",
    MaxGroups:   100,
    MaxMembers:  1000,
})

// Create master program (parent group)
m1Program := CreateGroup(CreateGroupInput{
    Name:           "m1_devops",
    DisplayName:    "Master 1 DevOps",
    OrganizationID: school.ID,
    MaxMembers:     150,
})

// Create class groups (children)
classA := CreateGroup(CreateGroupInput{
    Name:           "m1_devops_a",
    DisplayName:    "Master 1 DevOps - Class A",
    OrganizationID: school.ID,
    ParentGroupID:  m1Program.ID,  // Nested under program
    MaxMembers:     50,
})

// Add students to specific class
AddMember(classA.ID, studentID, GroupRoleMember)

// Add teaching assistants
AddMember(classA.ID, taID, GroupRoleAssistant)
```

### 3. User with Multiple Organization Roles

```go
user := GetUser(userID)

// Personal organization (auto-created)
personalOrg := GetPersonalOrganization(user.ID)
// Role: owner

// Company organization
companyOrg := GetOrganization(companyOrgID)
companyMember := GetOrganizationMember(companyOrg.ID, user.ID)
// Role: manager → can manage all company groups

// School organization
schoolOrg := GetOrganization(schoolOrgID)
schoolMember := GetOrganizationMember(schoolOrg.ID, user.ID)
// Role: member → basic access, need explicit group membership

// Specific training group (no org affiliation)
trainingGroup := GetGroup(trainingGroupID)
trainingMember := GetGroupMember(trainingGroup.ID, user.ID)
// Role: member → access only this group
```

---

## Migration from Old System

### Phase 1 Status: ✅ Complete

- Organizations entity created
- OrganizationMember entity created
- Groups linked to organizations (optional OrganizationID)
- Cascading permissions implemented
- Personal organization auto-creation
- Backward compatibility maintained

### Phase 2: Subscription Migration (Planned)

- Move subscriptions from users to organizations
- Organization-level feature limits
- Org owners manage billing
- Members inherit features from org

### Phase 3: Role Simplification (Planned)

- Remove business roles (keep only member + administrator)
- All permissions through org/group membership
- Context-based access control only

---

## Backward Compatibility

**Phase 1 maintains full compatibility**:

- ✅ Existing groups without OrganizationID still work
- ✅ Direct group membership still functional
- ✅ All existing roles still function
- ✅ No breaking API changes

**Migration Pattern**:

```go
// Groups can exist without organizations (legacy)
if group.OrganizationID == nil {
    // Use old permission system (direct group membership)
    return IsGroupMember(groupID, userID)
} else {
    // Use new permission system (org + group membership)
    return IsGroupMember(groupID, userID) ||
           IsOrganizationManager(group.OrganizationID, userID)
}
```

---

## Testing

### Organization Tests

- Create organization
- Add/remove members
- Update member roles
- Delete organization (with constraints)
- Personal organization creation
- Permission checks

### Group Tests

- Create group in organization
- Create nested group
- Cascading permission checks (org → group)
- Group without organization (legacy)
- Member management
- Hierarchy navigation

### Integration Tests

- Create org → Create group → Add member → Access group
- Org manager can access all groups without membership
- Group permissions still work for non-org groups
- Personal org auto-creation on registration

---

## Related Documentation

- **`BULK_IMPORT_FRONTEND_SPEC.md`** - CSV import for orgs/groups/memberships
- **`ORGANIZATION_ARCHITECTURE_MIGRATION.md`** (root) - Original migration plan
- **`REFACTORING_COMPLETE_SUMMARY.md`** - Related refactoring work
- **`CLAUDE.md`** - Main project guidance

---

## References

**Models**: `src/organizations/models/`, `src/groups/models/`
**Services**: `src/organizations/services/`, `src/groups/services/`
**Hooks**: `src/organizations/hooks/`, `src/groups/hooks/`
**Repositories**: `src/organizations/repositories/`, `src/groups/repositories/`
**DTOs**: `src/organizations/dto/`, `src/groups/dto/`

---

**Document Version**: 1.0
**Last Updated**: 2025-01-27
**Status**: ✅ Phase 1 Complete
