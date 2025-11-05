# Personal-to-Team Organization System

## Overview

The OCF Core platform implements a progressive disclosure system for organizations, allowing users to start with a simple "personal" organization and upgrade to a "team" organization when they need collaboration features.

## Organization Types

### Personal Organization
- **Type**: `personal`
- **Auto-created**: Automatically created when a new user registers
- **Default Name**: `personal_{userId}`
- **Member Limit**: 1 (owner only)
- **Group Limit**: Unlimited (-1)
- **Purpose**: Simple workspace for individual users

### Team Organization
- **Type**: `team`
- **Created**: Manually created by users OR converted from personal organizations
- **Member Limit**: 100 (default, configurable)
- **Group Limit**: 30 (default, configurable)
- **Purpose**: Collaborative workspace with multiple members

## Database Schema

### New Fields

The `organizations` table includes the following fields:

```sql
organization_type VARCHAR(20) DEFAULT 'team' -- 'personal' or 'team'
is_personal BOOLEAN DEFAULT false           -- Deprecated, kept for backward compatibility
```

**Note**: `OrganizationType` and `IsPersonal` are kept in sync via GORM's `BeforeSave` hook to ensure backward compatibility.

## API Endpoints

### Convert Personal to Team Organization

**Endpoint**: `POST /api/v1/organizations/{id}/convert-to-team`

**Authorization**: Owner only

**Request Body** (optional):
```json
{
  "name": "My Awesome Team"  // Optional: new name for the organization
}
```

**Response** (200 OK):
```json
{
  "id": "uuid",
  "name": "My Awesome Team",
  "display_name": "My Awesome Team",
  "description": "Your personal workspace",
  "owner_user_id": "user-id",
  "organization_type": "team",
  "is_personal": false,
  "max_groups": 30,
  "max_members": 100,
  "is_active": true,
  "member_count": 1,
  "created_at": "2025-10-31T17:00:00Z",
  "updated_at": "2025-10-31T18:00:00Z"
}
```

**Error Responses**:
- `400 Bad Request`: Organization is already a team organization
- `403 Forbidden`: Only organization owner can convert
- `404 Not Found`: Organization not found

### Get Organization

All organization GET endpoints now include:
- `organization_type`: Either "personal" or "team"
- `member_count`: Number of active members (always populated)

**Example**:
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/organizations/{id}
```

## User Registration Flow

When a new user registers, the system automatically:

1. Creates the user account in Casdoor
2. Assigns default roles (student, member)
3. **Creates a personal organization**:
   - Name: `personal_{userId}`
   - Display Name: "Personal Organization"
   - Type: `personal`
   - Member limit: 1
   - Group limit: Unlimited
4. Adds user as organization owner
5. Grants organization permissions

## Migration

### Running the Migration

To backfill `organization_type` for existing organizations and create personal organizations for users without any:

```bash
cd /workspaces/ocf-core
go run scripts/migrate_organization_types.go
```

The migration script:
1. **Backfills organization_type**: Sets type based on `IsPersonal` flag
2. **Creates personal organizations**: For any users without organization membership

### Migration Output

```
================================================
Organization Type Migration
================================================

Step 1: Backfilling organization_type for existing organizations...
Found 42 organizations to check
✅ Updated organization Company A → type: team
✅ Updated organization personal_user123 → type: personal

Step 2: Creating personal organizations for users without any organization...
Found 150 users to check
✅ Created personal organization for user john.doe (Name: personal_john123)

================================================
Migration Summary
================================================
Organizations updated:          40
Organizations already set:      2
Personal orgs created:          5
Errors:                         0
================================================
✅ Migration completed successfully!
```

## Model Changes

### Organization Model

**New Constants**:
```go
type OrganizationType string

const (
    OrgTypePersonal OrganizationType = "personal"
    OrgTypeTeam     OrganizationType = "team"
)
```

**New Fields**:
```go
type Organization struct {
    // ... existing fields ...
    IsPersonal       bool             // Deprecated: use OrganizationType instead
    OrganizationType OrganizationType // 'personal' or 'team'
    // ... existing fields ...
}
```

**New Methods**:
```go
// IsPersonalOrg checks if this is a personal organization
func (o *Organization) IsPersonalOrg() bool

// IsTeamOrg checks if this is a team organization
func (o *Organization) IsTeamOrg() bool

// SetOrganizationType sets type and keeps IsPersonal in sync
func (o *Organization) SetOrganizationType(orgType OrganizationType)

// BeforeSave GORM hook keeps fields synchronized
func (o *Organization) BeforeSave(tx *gorm.DB) error
```

## Service Methods

### New Service Methods

```go
// ConvertToTeam converts a personal organization to team
func (os *organizationService) ConvertToTeam(
    orgID uuid.UUID,
    requestingUserID string,
    newName string,
) (*models.Organization, error)
```

**Validation**:
- User must be organization owner
- Organization must currently be personal type
- New name (if provided) must be unique for owner

**Changes on Conversion**:
- `organization_type`: `personal` → `team`
- `max_members`: 1 → 100
- `max_groups`: -1 → 30
- `name`: Optional rename

### Updated Service Methods

All organization creation methods now explicitly set `OrganizationType`:

```go
// CreateOrganization - creates team organization
org.OrganizationType = models.OrgTypeTeam

// CreatePersonalOrganization - creates personal organization
org.OrganizationType = models.OrgTypePersonal
```

## Frontend Integration

### Example: Display UI Based on Organization Type

```typescript
interface Organization {
  id: string;
  name: string;
  organization_type: 'personal' | 'team';
  member_count: number;
  // ... other fields
}

function OrganizationSettings({ org }: { org: Organization }) {
  if (org.organization_type === 'personal') {
    return (
      <div>
        <h2>Personal Workspace</h2>
        <p>Upgrade to a team organization to collaborate with others.</p>
        <button onClick={() => convertToTeam(org.id)}>
          Upgrade to Team
        </button>
      </div>
    );
  }

  return (
    <div>
      <h2>Team Organization</h2>
      <p>Members: {org.member_count}</p>
      <button onClick={() => inviteMembers()}>
        Invite Members
      </button>
    </div>
  );
}
```

### Example: Convert Personal to Team

```typescript
async function convertToTeam(orgId: string, newName?: string) {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/convert-to-team`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ name: newName }),
    }
  );

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error_message);
  }

  return await response.json();
}
```

## Testing

### Manual Testing with cURL

1. **Get your personal organization**:
```bash
TOKEN="your-jwt-token"
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/organizations | jq '.[] | select(.organization_type == "personal")'
```

2. **Convert to team**:
```bash
ORG_ID="personal-org-id"
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "My Team"}' \
  http://localhost:8080/api/v1/organizations/$ORG_ID/convert-to-team
```

3. **Verify conversion**:
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/organizations/$ORG_ID | jq '.organization_type'
```

### Test Scenarios

The system supports the following test scenarios:

1. **New User**: Personal organization auto-created on registration
2. **Personal Workspace**: User with only personal organization
3. **Converted Team**: Personal org converted to team, still 1 member
4. **Active Team**: Team organization with multiple members
5. **Multi-Organization**: User with both personal and team memberships
6. **External Group Member**: User with personal org + group membership in another org

## Backward Compatibility

The implementation maintains backward compatibility:

1. **IsPersonal field**: Still present and synchronized with `OrganizationType`
2. **GORM Hook**: Automatically keeps both fields in sync on save
3. **API responses**: Include both `is_personal` and `organization_type`
4. **Existing code**: All references to `IsPersonal` updated to use `IsPersonalOrg()` method

## Security & Permissions

### Convert-to-Team Endpoint

- **Authentication**: Required (JWT token)
- **Authorization**: Organization owner only
- **Validation**:
  - Organization must exist
  - User must be the owner
  - Organization must be personal type (not already team)
  - New name (if provided) must be unique

### Permissions Setup

The convert-to-team endpoint is accessible to all authenticated users (member role and above). Owner validation is performed in the service layer.

```go
// From organizationRoutes.go
roles := []string{"administrator", "supervisor", "trainer", "member", "user", "admin"}
for _, role := range roles {
    casdoor.Enforcer.AddPolicy(role, "/api/v1/organizations/*/convert-to-team", "POST")
}
```

## Troubleshooting

### Issue: Organization type shows as empty string

**Solution**: Run the migration script to backfill `organization_type` for existing organizations.

### Issue: User has no personal organization

**Solution**: Run the migration script to create personal organizations for users without memberships.

### Issue: Cannot convert organization - "already a team"

**Cause**: Organization is already type `team`

**Solution**: Check organization type first. Only personal organizations can be converted.

### Issue: 403 Forbidden when converting

**Cause**: User is not the organization owner

**Solution**: Only the organization owner can convert. Check ownership with:
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/organizations/$ORG_ID | jq '.owner_user_id'
```

## Related Documentation

- [Organization & Groups System](.claude/docs/ORGANIZATION_GROUPS_SYSTEM.md)
- [Bulk Import System](BULK_IMPORT_FRONTEND_SPEC.md)
- [AI Assistant Guide](.claude/docs/AI_ASSISTANT_GUIDE.md)

## Implementation Summary

**Files Modified**:
- `src/organizations/models/organization.go` - Added OrganizationType field and methods
- `src/organizations/dto/organizationDto.go` - Added organization_type to output
- `src/organizations/services/organizationService.go` - Added ConvertToTeam method
- `src/organizations/controller/organizationController.go` - Added ConvertToTeam handler
- `src/organizations/routes/organizationRoutes.go` - Registered new route
- `src/organizations/entityRegistration/organizationRegistration.go` - Populate organization_type
- `src/organizations/hooks/organizationHooks.go` - Updated to use IsPersonalOrg()

**Files Created**:
- `scripts/migrate_organization_types.go` - Migration script

**API Changes**:
- **New endpoint**: `POST /api/v1/organizations/{id}/convert-to-team`
- **Response fields added**: `organization_type`, `member_count` (always populated)
- **Backward compatible**: `is_personal` field still present

**Database Changes**:
- **New column**: `organization_type VARCHAR(20) DEFAULT 'team'`
- **Index added**: On `organization_type` for query performance
- **Backward compatible**: `is_personal` field unchanged
