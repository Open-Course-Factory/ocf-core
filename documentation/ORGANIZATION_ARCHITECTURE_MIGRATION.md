# Organization Architecture Migration Plan

## Overview

This document outlines the complete migration from a **role-based access control system** to a **GitLab-style organization-based access control system**. The migration is divided into 3 phases to minimize risk and ensure each component can be independently tested and deployed.

## Current Architecture (Before Migration)

### System Roles (7 levels)
```go
- guest           // No access
- member          // Basic user (Inscrit)
- member_pro      // Paying user
- group_manager   // Can manage specific groups
- trainer         // Can share machines, manage groups
- organization    // Enterprise account
- administrator   // Full system access
```

### Access Control
- **Role-based**: User's system role determines features and limits
- **Subscription**: Individual users subscribe to plans
- **Features**: Defined per role in `GetRoleFeatures()`
- **Groups**: Flat structure, no parent organizations

### Key Files
- `src/auth/models/roles.go` - Role definitions and hierarchy
- `src/groups/models/group.go` - ClassGroup model
- `src/payment/` - User subscriptions and Stripe integration

---

## Target Architecture (After Migration)

### System Roles (2 only)
```go
- member          // Default authenticated user
- administrator   // System admin (platform management)
```

### Business Roles (Context-based)

#### Organization Level
```go
- owner    // Creates org, full control, manages billing
- manager  // Full access to all org groups and resources
- member   // Basic org access (view-only or limited)
```

#### Group Level
```go
- owner      // Creates group, full control within group
- admin      // Manages group settings and members
- assistant  // Helper role (e.g., teaching assistant)
- member     // Regular group member (e.g., student)
```

### Access Control Flow
```
User (system: member)
  ├─ Personal Organization (auto-created)
  │    └─ Full owner access to personal groups
  │
  ├─ OrganizationMember (role: manager) → "Company A"
  │    └─ Full access to all groups in Company A
  │    └─ Can create groups, manage members
  │
  └─ GroupMember (role: member) → "Training Group B"
       └─ Access only to this specific group
       └─ Limited by group permissions
```

### Key Concepts

1. **Personal Organizations**: Every user gets a personal org (like GitHub)
2. **Multi-tenancy**: Users can belong to multiple organizations with different roles
3. **Cascading Permissions**: Org owners/managers automatically access all org groups
4. **Organization Subscriptions**: Feature limits applied at org level, shared by members
5. **Flexible Collaboration**: Same user can be owner in Org A, member in Org B

---

## Phase 1: Add Organizations (Foundation)

**Goal**: Introduce organization entity and link groups to organizations WITHOUT breaking existing functionality.

### 1.1 New Models

#### Organization (`src/organizations/models/organization.go`)
```go
type Organization struct {
    BaseModel
    Name               string
    DisplayName        string
    Description        string
    OwnerUserID        string         // Primary owner
    SubscriptionPlanID *uuid.UUID     // MOVED from ClassGroup
    IsPersonal         bool           // Auto-created for users
    MaxGroups          int            // Limit for groups in org
    MaxMembers         int            // Limit for total org members
    IsActive           bool
    Metadata           map[string]interface{}

    // Relations
    Groups  []ClassGroup
    Members []OrganizationMember
}
```

#### OrganizationMember (`src/organizations/models/organizationMember.go`)
```go
type OrganizationMemberRole string

const (
    OrgRoleOwner   OrganizationMemberRole = "owner"
    OrgRoleManager OrganizationMemberRole = "manager"
    OrgRoleMember  OrganizationMemberRole = "member"
)

type OrganizationMember struct {
    BaseModel
    OrganizationID uuid.UUID
    UserID         string
    Role           OrganizationMemberRole
    InvitedBy      string
    JoinedAt       time.Time
    IsActive       bool
    Metadata       map[string]interface{}

    // Relations
    Organization Organization
}
```

### 1.2 Modified Models

#### ClassGroup - Add Organization Link
```go
type ClassGroup struct {
    // ... existing fields ...
    OrganizationID     *uuid.UUID     // NEW: Link to parent organization
    // SubscriptionPlanID - KEEP for now (remove in Phase 2)
    // ... rest unchanged ...
}
```

### 1.3 New Services & Repositories

**Files to Create**:
- `src/organizations/services/organizationService.go` - Business logic
- `src/organizations/repositories/organizationRepository.go` - Database operations
- `src/organizations/dto/organizationDto.go` - Input/Output DTOs
- `src/organizations/entityRegistration/organizationRegistration.go` - Entity management
- `src/organizations/hooks/organizationHooks.go` - Lifecycle hooks

**Key Service Methods**:
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

### 1.4 New API Endpoints

```
POST   /api/v1/organizations                    - Create organization
GET    /api/v1/organizations/:id                - Get organization details
GET    /api/v1/organizations/:id/groups         - List all groups in org
PATCH  /api/v1/organizations/:id                - Update organization
DELETE /api/v1/organizations/:id                - Delete organization

POST   /api/v1/organizations/:id/members        - Add member to org
DELETE /api/v1/organizations/:id/members/:userID - Remove member
PATCH  /api/v1/organizations/:id/members/:userID - Update member role
GET    /api/v1/organizations/:id/members        - List org members

GET    /api/v1/users/me/organizations           - List user's organizations
GET    /api/v1/users/me/groups                  - List user's groups (existing)
```

### 1.5 Updated Group Service

**Modify `CreateGroup()` to**:
- Accept optional `OrganizationID`
- Validate user has permission in organization
- Auto-assign org's subscription (if exists)

**Add Permission Checks**:
```go
// Check if user can access group via org membership
func (gs *groupService) CanUserAccessGroup(groupID, userID) (bool, error) {
    // 1. Direct group membership (existing)
    if isMember, _ := gs.IsUserInGroup(groupID, userID); isMember {
        return true, nil
    }

    // 2. NEW: Organization membership (cascading access)
    group, _ := gs.GetGroup(groupID, false)
    if group.OrganizationID != nil {
        return orgService.CanUserAccessGroupViaOrg(groupID, userID)
    }

    return false, nil
}
```

### 1.6 Casbin Policies

**New Organization Permissions**:
```go
// Organization members can view org
"group:org:{orgID}", "/api/v1/organizations/{orgID}", "GET"

// Organization managers can manage org
"group:org_manager:{orgID}", "/api/v1/organizations/{orgID}", "GET|PATCH|DELETE"
"group:org_manager:{orgID}", "/api/v1/organizations/{orgID}/groups", "GET|POST"
"group:org_manager:{orgID}", "/api/v1/organizations/{orgID}/members", "GET|POST|PATCH|DELETE"

// Cascading group access for org managers
"group:org_manager:{orgID}", "/api/v1/groups/{groupID}", "GET|PATCH|DELETE"
```

### 1.7 Automatic Personal Organization

**On User Registration/First Login**:
```go
func CreatePersonalOrganization(userID string) (*Organization, error) {
    // Check if personal org already exists
    existing := repo.GetPersonalOrganization(userID)
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
    }

    created, err := repo.CreateOrganization(org)
    if err != nil {
        return nil, err
    }

    // Add user as owner
    member := &OrganizationMember{
        OrganizationID: created.ID,
        UserID:         userID,
        Role:           OrgRoleOwner,
        JoinedAt:       time.Now(),
    }
    repo.AddOrganizationMember(member)

    return created, nil
}
```

### 1.8 Backward Compatibility

**Phase 1 maintains full backward compatibility**:
- ✅ Existing groups continue to work (OrganizationID = null)
- ✅ Group-level subscriptions still work (will migrate in Phase 2)
- ✅ All existing roles still function (will simplify in Phase 3)
- ✅ No breaking API changes

**Migration Pattern**:
```go
// Groups can exist without organizations (legacy)
if group.OrganizationID == nil {
    // Use old permission system (direct group membership)
} else {
    // Use new permission system (org + group membership)
}
```

### 1.9 Testing Requirements

**Unit Tests**:
- Organization CRUD operations
- OrganizationMember CRUD operations
- Personal organization creation
- Cascading permission checks
- Group-org relationship validation

**Integration Tests**:
- Create org → Create group in org → Add member to org → Access group
- Org manager can access all groups without direct membership
- Group permissions still work for non-org groups (legacy)
- Personal org auto-creation on user registration

---

## Phase 2: Migrate Subscriptions (Business Logic)

**Goal**: Move subscription management from users/groups to organizations.

### 2.1 Subscription Model Changes

#### Current (Group-level)
```go
type ClassGroup struct {
    SubscriptionPlanID *uuid.UUID  // Group has subscription
}
```

#### Target (Organization-level)
```go
type Organization struct {
    SubscriptionPlanID *uuid.UUID  // Organization has subscription
}

type ClassGroup struct {
    OrganizationID     *uuid.UUID  // Group inherits org subscription
    // SubscriptionPlanID REMOVED
}
```

### 2.2 Feature Limit Changes

#### Before
```go
// Check user's role-based features
features := models.GetRoleFeatures(user.Role)
if features.MaxCourses == -1 || count < features.MaxCourses {
    // Allow
}
```

#### After
```go
// Check organization's subscription features
org := GetUserPrimaryOrganization(userID)
plan := org.SubscriptionPlan
if plan.MaxCourses == -1 || count < plan.MaxCourses {
    // Allow
}
```

### 2.3 Payment System Changes

**Current**: `src/payment/services/userSubscriptionService.go`
- Stripe customer = User
- User subscribes to plan
- User manages payment methods

**Target**: `src/payment/services/organizationSubscriptionService.go`
- Stripe customer = Organization
- Organization owner subscribes
- Organization owner manages billing
- Members inherit features from org subscription

### 2.4 Stripe Webhook Updates

**Modified Webhooks**:
- `customer.subscription.created` → Update organization subscription
- `customer.subscription.updated` → Update organization features
- `customer.subscription.deleted` → Downgrade organization
- `invoice.payment_succeeded` → Record org payment

**New Fields**:
```go
// Stripe metadata
customer.metadata.organization_id = orgID
subscription.metadata.organization_id = orgID
```

### 2.5 Usage Metrics

**Before**: Per-user limits
```go
userMetrics.TerminalCount < user.SubscriptionPlan.MaxTerminals
```

**After**: Per-organization limits
```go
orgMetrics.TotalTerminalCount < org.SubscriptionPlan.MaxTerminals
orgMetrics.ActiveUsers < org.SubscriptionPlan.MaxConcurrentUsers
```

### 2.6 Migration Script

```go
// Migrate existing user subscriptions to personal organizations
func MigrateUserSubscriptionsToOrgs() error {
    users := GetAllUsersWithSubscriptions()

    for _, user := range users {
        // Get or create personal org
        org := GetOrCreatePersonalOrganization(user.ID)

        // Move subscription to org
        org.SubscriptionPlanID = user.SubscriptionPlanID
        UpdateOrganization(org)

        // Clear user subscription (deprecated)
        user.SubscriptionPlanID = nil
        UpdateUser(user)
    }

    return nil
}
```

### 2.7 Affected Files

**Modified**:
- `src/payment/services/userSubscriptionService.go` → Deprecate
- `src/payment/models/userSubscription.go` → Add deprecation notice
- `src/courses/middleware/paymentMiddleware.go` → Check org subscription
- All feature limit checks throughout codebase

**New**:
- `src/payment/services/organizationSubscriptionService.go`
- `src/payment/models/organizationSubscription.go`
- `src/payment/repositories/organizationSubscriptionRepository.go`

### 2.8 Testing Requirements

- Subscription creation for organization
- Payment webhook processing (org context)
- Feature limit enforcement (org context)
- Usage metrics aggregation per org
- Billing management by org owners
- Member feature inheritance from org

---

## Phase 3: Simplify Roles (Final Cleanup)

**Goal**: Remove business roles from system, keep only `member` and `administrator`.

### 3.1 Role Removal

#### Delete These Roles
```go
// REMOVE from roles.go
guest           // Use unauthenticated state instead
member_pro      // Use org subscription instead
group_manager   // Use org/group membership instead
trainer         // Use org membership instead
organization    // Use org ownership instead
```

#### Keep Only
```go
member          // Default authenticated user
administrator   // System admin
```

### 3.2 Updated Role Model

**Before** (`src/auth/models/roles.go`):
```go
var RoleHierarchy = map[RoleName][]RoleName{
    Guest:        {},
    Member:       {Guest},
    MemberPro:    {Member, Guest},
    GroupManager: {Member, Guest},
    Trainer:      {GroupManager, MemberPro, Member, Guest},
    Organization: {Trainer, GroupManager, MemberPro, Member, Guest},
    Admin:        {Organization, Trainer, GroupManager, MemberPro, Member, Guest},
}

func GetRoleFeatures(role RoleName) RoleFeatures {
    // 150 lines of role-specific features
}
```

**After** (simplified):
```go
const (
    Member        RoleName = "member"
    Administrator RoleName = "administrator"
)

// No hierarchy needed - only 2 roles
// Features come from organization subscriptions, not roles
```

### 3.3 Permission System Changes

#### Before (Role-based)
```go
// Middleware checks role hierarchy
if HasPermission(user.Role, requiredRole) {
    // Allow
}
```

#### After (Context-based)
```go
// Middleware checks org/group membership
if user.Role == Administrator {
    // System admin - allow everything
} else {
    // Check organization membership
    if IsOrganizationMember(userID, orgID, OrgRoleManager) {
        // Allow
    }
    // Check group membership
    if IsGroupMember(userID, groupID, GroupRoleAdmin) {
        // Allow
    }
}
```

### 3.4 Casdoor Integration Changes

**Before**:
```go
var CasdoorToOCFRoleMap = map[string]RoleName{
    "student":         Member,
    "premium_student": MemberPro,
    "teacher":         GroupManager,
    "admin":           Admin,
    "administrator":   Admin,
}
```

**After**:
```go
var CasdoorToOCFRoleMap = map[string]RoleName{
    "user":          Member,
    "member":        Member,
    "student":       Member,  // All map to member
    "teacher":       Member,  // Role determined by org membership
    "admin":         Administrator,
    "administrator": Administrator,
}
```

### 3.5 Feature Access Pattern

**New Helper Functions**:
```go
// Get user's effective features based on their organizations
func GetUserEffectiveFeatures(userID string) Features {
    orgs := GetUserOrganizations(userID)

    // Aggregate features from all orgs
    maxFeatures := Features{}
    for _, org := range orgs {
        if org.SubscriptionPlan != nil {
            maxFeatures = MaxFeatures(maxFeatures, org.SubscriptionPlan.Features)
        }
    }

    return maxFeatures
}

// Check if user can perform action in specific context
func CanUserPerformAction(userID, resourceType, resourceID, action string) bool {
    // System admin - always allow
    if IsSystemAdmin(userID) {
        return true
    }

    // Check context-specific permissions
    switch resourceType {
    case "organization":
        return IsOrganizationMember(userID, resourceID, OrgRoleManager)
    case "group":
        // Check direct group membership
        if IsGroupMember(userID, resourceID, GroupRoleAdmin) {
            return true
        }
        // Check org membership (cascading)
        group := GetGroup(resourceID)
        if group.OrganizationID != nil {
            return IsOrganizationMember(userID, group.OrganizationID, OrgRoleManager)
        }
    case "terminal":
        return CanAccessTerminal(userID, resourceID)
    }

    return false
}
```

### 3.6 Removed Files

```
src/auth/models/roles.go (keep minimal version)
  - Remove RoleHierarchy
  - Remove GetRoleFeatures()
  - Remove HasPermission()
  - Remove IsRolePayingUser()
  - Remove GetUpgradeRecommendations()
  - Keep only role constants (member, administrator)
```

### 3.7 Updated Files

**Permission Checks** (throughout codebase):
```go
// OLD
if user.Role == models.Trainer || user.Role == models.Admin {
    // Allow
}

// NEW
if user.Role == models.Administrator {
    // System admin
} else if CanUserPerformAction(userID, "organization", orgID, "manage") {
    // Organization manager
}
```

**Middleware** (`src/auth/middlewares/authManagement.go`):
```go
// OLD
requiredRole := models.GroupManager
if !HasPermission(user.Role, requiredRole) {
    return Unauthorized()
}

// NEW
if user.Role != models.Administrator {
    // Check context-specific permission via Casbin
    if !enforcer.Enforce(userID, resource, action) {
        return Unauthorized()
    }
}
```

### 3.8 Migration Script

```go
// Map old roles to org/group memberships
func MigrateRolesToMemberships() error {
    users := GetAllUsers()

    for _, user := range users {
        switch user.Role {
        case models.Trainer, models.Organization:
            // These users should have manager roles in their orgs
            orgs := GetUserOwnedOrganizations(user.ID)
            for _, org := range orgs {
                UpdateOrganizationMemberRole(org.ID, user.ID, OrgRoleManager)
            }

        case models.GroupManager:
            // These users should have admin roles in their groups
            groups := GetUserOwnedGroups(user.ID)
            for _, group := range groups {
                UpdateGroupMemberRole(group.ID, user.ID, GroupRoleAdmin)
            }
        }

        // All users become "member" system role
        if user.Role != models.Admin {
            user.Role = models.Member
            UpdateUser(user)
        }
    }

    return nil
}
```

### 3.9 Testing Requirements

- All existing features work with new permission system
- System admin can access everything
- Context-based permissions enforced correctly
- Feature limits based on org subscriptions
- No references to old roles remain
- Casbin policies work without role hierarchy

---

## Summary Comparison

### Before Migration
```
User → System Role → Features & Limits
                  → Direct Group Membership → Group Access

7 system roles: guest, member, member_pro, group_manager, trainer, organization, admin
Feature limits: Hardcoded per role
Subscriptions: Per user
Access control: Role hierarchy + direct membership
```

### After Migration
```
User (member) → Organization Membership (owner/manager/member) → Org Features & Limits
                                                                → All Org Groups Access
              → Direct Group Membership (owner/admin/assistant/member) → Group Access

2 system roles: member, administrator
Feature limits: Per organization subscription plan
Subscriptions: Per organization
Access control: Organization + Group context-based membership
```

---

## Implementation Order

### Phase 1 (This PR) - Foundation
1. Create Organization models and relationships
2. Create OrganizationService with full CRUD
3. Add organization entity registration
4. Implement personal organization auto-creation
5. Link groups to organizations (optional field)
6. Add cascading permission checks (org → groups)
7. Create API endpoints for organization management
8. Full test coverage

**Deliverable**: Organizations exist, groups can link to orgs, backward compatible

### Phase 2 (Next PR) - Subscriptions
1. Move SubscriptionPlanID to Organization
2. Create OrganizationSubscriptionService
3. Update Stripe webhooks for org context
4. Change feature limit checks to org-based
5. Update usage metrics to org-level
6. Migrate existing user subscriptions
7. Full test coverage

**Deliverable**: Subscriptions work at org level, feature limits enforced per org

### Phase 3 (Final PR) - Simplification
1. Remove business roles from roles.go
2. Update all permission checks to context-based
3. Simplify Casdoor mapping
4. Remove role hierarchy logic
5. Migrate existing users to new system
6. Clean up deprecated code
7. Full regression testing

**Deliverable**: Clean, simple role system (member + admin), all features work

---

## Rollback Strategy

Each phase is independently deployable:

**Phase 1**: Can revert without data loss (OrganizationID is nullable)
**Phase 2**: Can revert by re-enabling user subscriptions (both systems coexist temporarily)
**Phase 3**: Requires role migration reversal (more complex, but Phase 1-2 must be stable first)

---

## Files Affected Summary

### Phase 1 (New)
- `src/organizations/models/organization.go`
- `src/organizations/models/organizationMember.go`
- `src/organizations/dto/organizationDto.go`
- `src/organizations/services/organizationService.go`
- `src/organizations/repositories/organizationRepository.go`
- `src/organizations/entityRegistration/organizationRegistration.go`
- `src/organizations/hooks/organizationHooks.go`
- `src/organizations/hooks/initHooks.go`

### Phase 1 (Modified)
- `src/groups/models/group.go` - Add OrganizationID
- `src/groups/services/groupService.go` - Add org permission checks
- `src/initialization/permissions.go` - Add org policies
- `main.go` - Register organization entity

### Phase 2 (New)
- `src/payment/services/organizationSubscriptionService.go`
- `src/payment/models/organizationSubscription.go`
- `src/payment/repositories/organizationSubscriptionRepository.go`

### Phase 2 (Modified)
- `src/payment/services/userSubscriptionService.go` - Deprecate
- `src/payment/webhooks/stripeWebhooks.go` - Org context
- All feature limit checks throughout codebase

### Phase 3 (Modified)
- `src/auth/models/roles.go` - Simplify to 2 roles
- `src/auth/middlewares/authManagement.go` - Context-based
- All permission checks throughout codebase

**Total**: ~20 new files, ~30 modified files across 3 phases
