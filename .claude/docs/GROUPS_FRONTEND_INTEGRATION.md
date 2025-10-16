# Groups Management - Frontend Integration Guide

This document provides frontend developers with all necessary information to integrate the new groups management system.

## Overview

The groups management system allows users to create classes/teams and share terminals with entire groups instead of individual users. The system uses a hybrid approach with OCF database entities and Casbin for permissions.

### Feature Flag

The groups module is controlled by the `class_groups` feature flag:
- **Key**: `class_groups`
- **Module**: `groups`
- **Default**: Enabled (`enabled: true`)

You can check if the feature is enabled:
```http
GET /api/v1/features?filter[key]=class_groups
```

The feature can be toggled via API (admin only):
```http
PATCH /api/v1/features/{feature_id}
Content-Type: application/json

{
  "enabled": false
}
```

**Note**: Feature flags are global database toggles. This is different from subscription-based feature access (which controls whether a user's plan includes group functionality).

## Authentication Requirements

All endpoints require JWT authentication via Bearer token in the `Authorization` header:

```http
Authorization: Bearer {access_token}
```

Get the token via the login endpoint (see `CLAUDE.md` for test credentials).

## API Endpoints

Base URL: `http://localhost:8080/api/v1`

### Class Groups Endpoints

#### 1. List All Groups

```http
GET /api/v1/class-groups
```

**Query Parameters:**
- `page` (int, optional): Page number for pagination (default: 1)
- `pageSize` (int, optional): Items per page (default: 20)
- `includes` (string, optional): Comma-separated list of relations to preload (e.g., `Members`)

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "name": "group-slug",
      "displayName": "My Training Class",
      "description": "Description of the group",
      "ownerUserID": "user-id",
      "subscriptionPlanID": "uuid-or-null",
      "maxMembers": 50,
      "memberCount": 12,
      "expiresAt": "2025-12-31T23:59:59Z",
      "casdoorGroupName": "optional-casdoor-sync-name",
      "isActive": true,
      "isExpired": false,
      "isFull": false,
      "metadata": {},
      "createdAt": "2025-01-15T10:00:00Z",
      "updatedAt": "2025-01-15T10:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "pageSize": 20,
    "total": 5
  }
}
```

#### 2. Get Groups for Current User

```http
GET /api/v1/class-groups?filter[ownerUserID]={userId}
```

Or use the repository method to get all groups where user is a member (includes owned groups):

```http
GET /api/v1/class-groups?includes=Members&filter[Members.UserID]={userId}
```

#### 3. Create a Group

```http
POST /api/v1/class-groups
Content-Type: application/json
```

**Request Body:**
```json
{
  "name": "python-bootcamp-2025",
  "displayName": "Python Bootcamp 2025",
  "description": "6-week intensive Python training program",
  "subscriptionPlanID": "uuid-or-null",
  "maxMembers": 30,
  "expiresAt": "2025-12-31T23:59:59Z",
  "metadata": {
    "startDate": "2025-02-01",
    "instructor": "John Doe",
    "level": "intermediate"
  }
}
```

**Notes:**
- `ownerUserID` is automatically set to the authenticated user (via hook)
- The creator is automatically added as a member with `owner` role
- Casbin permissions are automatically granted to the owner

**Response:**
```json
{
  "id": "uuid",
  "name": "python-bootcamp-2025",
  "displayName": "Python Bootcamp 2025",
  "ownerUserID": "authenticated-user-id",
  "memberCount": 1,
  "isActive": true,
  ...
}
```

#### 4. Update a Group

```http
PATCH /api/v1/class-groups/{groupId}
Content-Type: application/json
```

**Request Body** (all fields optional):
```json
{
  "displayName": "Updated Display Name",
  "description": "Updated description",
  "maxMembers": 40,
  "isActive": false
}
```

**Permissions Required:**
- User must be the group owner or have admin role in the group

#### 5. Delete a Group

```http
DELETE /api/v1/class-groups/{groupId}
```

**Notes:**
- Soft delete (sets `deletedAt` timestamp)
- Automatically removes all group members (cascade)
- Revokes all Casbin permissions for group members
- Only group owner can delete

### Group Members Endpoints

#### 1. List Group Members

```http
GET /api/v1/class-group-members?filter[groupID]={groupId}
```

**Query Parameters:**
- `includes` (string, optional): Preload relations (currently none defined)
- `filter[groupID]` (uuid): Filter by group ID
- `filter[role]` (string): Filter by role (owner, admin, assistant, member)
- `filter[isActive]` (bool): Filter by active status

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "groupID": "uuid",
      "userID": "user-id",
      "role": "admin",
      "invitedBy": "inviter-user-id",
      "joinedAt": "2025-01-15T10:00:00Z",
      "isActive": true,
      "metadata": {
        "invitationEmail": "user@example.com",
        "notes": "Teaching assistant"
      },
      "createdAt": "2025-01-15T10:00:00Z",
      "updatedAt": "2025-01-15T10:00:00Z"
    }
  ]
}
```

#### 2. Add Members to Group

**Note:** Use the service method for proper validation and permission handling.

```http
POST /api/v1/class-group-members
Content-Type: application/json
```

**Request Body:**
```json
{
  "groupID": "uuid",
  "userID": "user-to-add-id",
  "role": "member",
  "metadata": {
    "invitationEmail": "user@example.com"
  }
}
```

**Roles Available:**
- `owner` - Full control, can delete group
- `admin` - Can manage members and settings
- `assistant` - Can moderate but not manage
- `member` - Standard access

**Validations:**
- Group must not be full (`memberCount < maxMembers`)
- Group must not be expired (`expiresAt > now`)
- Group must be active (`isActive = true`)
- User must have admin or owner role to add members
- Cannot add duplicate members

**Auto-granted Permissions:**
- User automatically gets Casbin group permissions
- User can access group-shared terminals

#### 3. Update Member Role

```http
PATCH /api/v1/class-group-members/{memberId}
Content-Type: application/json
```

**Request Body:**
```json
{
  "role": "admin",
  "isActive": true
}
```

**Permissions Required:**
- User must be group owner or admin
- Cannot demote the group owner

#### 4. Remove Member from Group

```http
DELETE /api/v1/class-group-members/{memberId}
```

**Notes:**
- Soft delete by default
- Revokes Casbin group permissions
- User loses access to group-shared terminals
- Owner cannot be removed (must delete group instead)

### Terminal Sharing with Groups

The existing terminal sharing system has been extended to support group sharing.

#### Share Terminal with Group

```http
POST /api/v1/terminal-shares
Content-Type: application/json
```

**Request Body:**
```json
{
  "terminalID": "uuid",
  "sharedWithGroupID": "group-uuid",
  "accessLevel": "read",
  "expiresAt": "2025-12-31T23:59:59Z",
  "metadata": {
    "purpose": "Lab exercise for class"
  }
}
```

**Access Levels:**
- `read` - View only
- `write` - Can execute commands
- `admin` - Full control including sharing

**Notes:**
- Either `sharedWithUserID` OR `sharedWithGroupID` must be provided (not both)
- All group members automatically get access to the terminal
- Share creator must have admin access to the terminal

#### List Terminal Shares for Group

```http
GET /api/v1/terminal-shares?filter[sharedWithGroupID]={groupId}
```

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "terminalID": "uuid",
      "sharedWithUserID": null,
      "sharedWithGroupID": "group-uuid",
      "sharedByUserID": "owner-user-id",
      "shareType": "group",
      "accessLevel": "read",
      "expiresAt": "2025-12-31T23:59:59Z",
      "isActive": true,
      "createdAt": "2025-01-15T10:00:00Z"
    }
  ]
}
```

**Share Types:**
- `shareType: "user"` - Shared with individual user
- `shareType: "group"` - Shared with entire group

## Frontend Implementation Examples

### React/TypeScript Example

```typescript
// types.ts
export interface ClassGroup {
  id: string;
  name: string;
  displayName: string;
  description: string;
  ownerUserID: string;
  subscriptionPlanID: string | null;
  maxMembers: number;
  memberCount: number;
  expiresAt: string | null;
  isActive: boolean;
  isExpired: boolean;
  isFull: boolean;
  metadata: Record<string, any>;
  createdAt: string;
  updatedAt: string;
}

export interface GroupMember {
  id: string;
  groupID: string;
  userID: string;
  role: 'owner' | 'admin' | 'assistant' | 'member';
  invitedBy: string;
  joinedAt: string;
  isActive: boolean;
  metadata: Record<string, any>;
}

export interface CreateGroupInput {
  name: string;
  displayName: string;
  description: string;
  subscriptionPlanID?: string;
  maxMembers?: number;
  expiresAt?: string;
  metadata?: Record<string, any>;
}

// api.ts
import axios from 'axios';

const API_BASE_URL = 'http://localhost:8080/api/v1';

export const groupsApi = {
  // List all groups
  listGroups: async (page = 1, pageSize = 20, includes?: string) => {
    const params = new URLSearchParams({
      page: page.toString(),
      pageSize: pageSize.toString(),
    });
    if (includes) params.append('includes', includes);

    const response = await axios.get<{ data: ClassGroup[] }>(
      `${API_BASE_URL}/class-groups?${params}`,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
        },
      }
    );
    return response.data;
  },

  // Create a group
  createGroup: async (input: CreateGroupInput) => {
    const response = await axios.post<ClassGroup>(
      `${API_BASE_URL}/class-groups`,
      input,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
          'Content-Type': 'application/json',
        },
      }
    );
    return response.data;
  },

  // Get group by ID
  getGroup: async (groupId: string, includes?: string) => {
    const params = includes ? `?includes=${includes}` : '';
    const response = await axios.get<ClassGroup>(
      `${API_BASE_URL}/class-groups/${groupId}${params}`,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
        },
      }
    );
    return response.data;
  },

  // Update group
  updateGroup: async (groupId: string, updates: Partial<CreateGroupInput>) => {
    const response = await axios.patch<ClassGroup>(
      `${API_BASE_URL}/class-groups/${groupId}`,
      updates,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
          'Content-Type': 'application/json',
        },
      }
    );
    return response.data;
  },

  // Delete group
  deleteGroup: async (groupId: string) => {
    await axios.delete(`${API_BASE_URL}/class-groups/${groupId}`, {
      headers: {
        Authorization: `Bearer ${localStorage.getItem('access_token')}`,
      },
    });
  },

  // List group members
  listMembers: async (groupId: string) => {
    const response = await axios.get<{ data: GroupMember[] }>(
      `${API_BASE_URL}/class-group-members?filter[groupID]=${groupId}`,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
        },
      }
    );
    return response.data;
  },

  // Add member to group
  addMember: async (groupId: string, userID: string, role: GroupMember['role']) => {
    const response = await axios.post<GroupMember>(
      `${API_BASE_URL}/class-group-members`,
      { groupID: groupId, userID, role },
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
          'Content-Type': 'application/json',
        },
      }
    );
    return response.data;
  },

  // Remove member
  removeMember: async (memberId: string) => {
    await axios.delete(`${API_BASE_URL}/class-group-members/${memberId}`, {
      headers: {
        Authorization: `Bearer ${localStorage.getItem('access_token')}`,
      },
    });
  },
};

// React Component Example
import React, { useEffect, useState } from 'react';

export const GroupsList: React.FC = () => {
  const [groups, setGroups] = useState<ClassGroup[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadGroups = async () => {
      try {
        const data = await groupsApi.listGroups(1, 20, 'Members');
        setGroups(data.data);
      } catch (error) {
        console.error('Failed to load groups:', error);
      } finally {
        setLoading(false);
      }
    };
    loadGroups();
  }, []);

  const handleCreateGroup = async () => {
    const newGroup = await groupsApi.createGroup({
      name: 'new-group',
      displayName: 'New Group',
      description: 'A new training group',
      maxMembers: 30,
    });
    setGroups([...groups, newGroup]);
  };

  if (loading) return <div>Loading...</div>;

  return (
    <div>
      <button onClick={handleCreateGroup}>Create Group</button>
      <ul>
        {groups.map(group => (
          <li key={group.id}>
            {group.displayName} ({group.memberCount}/{group.maxMembers} members)
            {group.isFull && <span> - FULL</span>}
            {group.isExpired && <span> - EXPIRED</span>}
          </li>
        ))}
      </ul>
    </div>
  );
};
```

### Vue.js Example

```typescript
// composables/useGroups.ts
import { ref, computed } from 'vue';
import axios from 'axios';

export function useGroups() {
  const groups = ref<ClassGroup[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const API_BASE_URL = 'http://localhost:8080/api/v1';

  const authHeaders = computed(() => ({
    Authorization: `Bearer ${localStorage.getItem('access_token')}`,
  }));

  const fetchGroups = async () => {
    loading.value = true;
    error.value = null;
    try {
      const response = await axios.get(
        `${API_BASE_URL}/class-groups?includes=Members`,
        { headers: authHeaders.value }
      );
      groups.value = response.data.data;
    } catch (err) {
      error.value = 'Failed to fetch groups';
      console.error(err);
    } finally {
      loading.value = false;
    }
  };

  const createGroup = async (input: CreateGroupInput) => {
    try {
      const response = await axios.post(
        `${API_BASE_URL}/class-groups`,
        input,
        { headers: { ...authHeaders.value, 'Content-Type': 'application/json' } }
      );
      groups.value.push(response.data);
      return response.data;
    } catch (err) {
      error.value = 'Failed to create group';
      throw err;
    }
  };

  return {
    groups,
    loading,
    error,
    fetchGroups,
    createGroup,
  };
}
```

## UI/UX Recommendations

### Group Creation Form
- **Required Fields**: `displayName`, `name` (auto-generate from displayName if not provided)
- **Optional Fields**: `description`, `maxMembers`, `expiresAt`
- **Validation**:
  - Name must be URL-friendly (lowercase, hyphens only)
  - Max members must be > 0
  - Expiration date must be in the future

### Group Card/List Item
Display:
- Group display name (prominent)
- Member count with progress bar (`12/30 members`)
- Status badges: `FULL`, `EXPIRED`, `INACTIVE`
- Owner name or "You" if current user
- Action buttons based on role (Edit/Delete for owner, Leave for members)

### Member Management
- **Role Badge Colors**:
  - Owner: Gold/Yellow
  - Admin: Blue
  - Assistant: Green
  - Member: Gray
- **Actions by Role**:
  - Owner: Can manage all members, delete group
  - Admin: Can add/remove members (except owner)
  - Assistant/Member: Read-only view

### Terminal Sharing
- Add "Share with Group" option alongside "Share with User"
- Show group icon/badge for group-shared terminals
- Display "Shared with X groups" count in terminal list
- Allow filtering terminals by "Shared with my groups"

## Permissions & Access Control

### Who Can Do What

**Group Creation:**
- Any authenticated user can create a group
- Creator becomes owner automatically

**Group Management:**
- **Owner**: Full control (update, delete, manage members, change all settings)
- **Admin**: Manage members, update group settings (except ownership)
- **Assistant**: View members, cannot modify
- **Member**: View members, cannot modify

**Terminal Sharing:**
- Group owner/admin can share terminals with the group
- Individual terminal owner can share their terminal with any group
- All group members inherit terminal access based on share's access level

**Automatic Permission Handling:**
- Backend automatically grants Casbin permissions when adding members
- Backend automatically revokes permissions when removing members
- No manual permission management needed in frontend

## Error Handling

### Common Error Responses

```json
{
  "error": "Error message",
  "code": "ERROR_CODE",
  "details": {}
}
```

**Common Error Codes:**
- `401 Unauthorized` - Invalid or missing JWT token
- `403 Forbidden` - User lacks permission for this action
- `404 Not Found` - Group/member not found
- `400 Bad Request` - Validation error (e.g., group full, expired, duplicate member)
- `409 Conflict` - Member already exists in group

**Frontend Error Handling Example:**
```typescript
try {
  await groupsApi.addMember(groupId, userId, 'member');
} catch (error) {
  if (axios.isAxiosError(error)) {
    switch (error.response?.status) {
      case 400:
        alert('Cannot add member: Group may be full or expired');
        break;
      case 403:
        alert('You do not have permission to add members');
        break;
      case 409:
        alert('User is already a member of this group');
        break;
      default:
        alert('Failed to add member');
    }
  }
}
```

## Testing

### Test Data Available

Use the test credentials from `CLAUDE.md`:
- Email: `1.supervisor@test.com`
- Password: `test`

### Manual Testing Steps

1. **Login and get token:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"email":"1.supervisor@test.com","password":"test"}'
   ```

2. **Create a test group:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/class-groups \
     -H "Authorization: Bearer {token}" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "test-group",
       "displayName": "Test Group",
       "description": "Testing groups feature",
       "maxMembers": 10
     }'
   ```

3. **List groups:**
   ```bash
   curl -X GET http://localhost:8080/api/v1/class-groups \
     -H "Authorization: Bearer {token}"
   ```

## Swagger Documentation

Full interactive API documentation is available at:
```
http://localhost:8080/swagger/
```

Search for tags:
- `class-groups` - Group management endpoints
- `class-group-members` - Member management endpoints
- `terminal-shares` - Terminal sharing (includes group sharing)

## Migration Notes

### Backward Compatibility

The terminal sharing system is fully backward compatible:
- Existing user-only shares continue to work
- `sharedWithUserID` field changed from `string` to `*string` (nullable pointer)
- Frontend should check `shareType` field to determine share type:
  - `shareType: "user"` → Check `sharedWithUserID`
  - `shareType: "group"` → Check `sharedWithGroupID`

### Database Changes

New tables created:
- `groups` - Main group entity
- `group_members` - Join table for group membership

Modified tables:
- `terminal_shares` - Added `shared_with_group_id` column (nullable UUID)

## Support & Troubleshooting

### Common Issues

**Issue: "Route conflict" or 404 on `/api/v1/groups`**
- Solution: Use `/api/v1/class-groups` instead (legacy routes disabled)

**Issue: "User not found when adding member"**
- Solution: Ensure the user ID is a valid Casdoor user ID
- Check that user exists in the system first

**Issue: "Cannot add member - group full"**
- Solution: Increase `maxMembers` or remove inactive members

**Issue: Terminal not visible to group members**
- Solution: Verify terminal share was created with correct `sharedWithGroupID`
- Check that share is active and not expired

### Debugging

Enable debug logging by setting in `.env`:
```
ENVIRONMENT=development
```

Check logs for:
- Group creation events: `"Creating group for user: ..."`
- Permission grants: `"Granting group permissions to user: ..."`
- Member additions: `"Adding X members to group ..."`

## Future Enhancements

Planned features (not yet implemented):
- Email invitations with accept/reject workflow
- Group analytics and activity tracking
- Nested group hierarchies (subgroups)
- Group-level subscriptions and billing
- Casdoor synchronization for external auth providers
- Bulk member import from CSV
- Group templates for quick setup

## Questions or Issues?

For backend issues or questions:
- Check the main codebase documentation in `CLAUDE.md`
- Review Swagger docs at `/swagger/`
- Examine entity registration files in `src/groups/entityRegistration/`

For permission issues:
- Verify JWT token is valid and not expired
- Check user roles in Casdoor dashboard (port 8000)
- Review Casbin policies in database `casbin_rule` table
