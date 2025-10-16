# Terminal Sharing and Access Rights System

## Overview

The OCF Core terminal sharing system allows terminal owners to grant controlled access to other users with granular permission levels. The system supports three access levels (read, write, admin), time-based expiration, activation/deactivation, and independent hiding functionality for both owners and recipients.

## Architecture Components

### 1. Data Model

**Location**: `src/terminalTrainer/models/terminalShare.go`

```go
type TerminalShare struct {
    BaseModel
    TerminalID           uuid.UUID  // UUID of the shared terminal
    SharedWithUserID     string     // User receiving access (Casdoor user ID)
    SharedByUserID       string     // Terminal owner (creator of share)
    AccessLevel          string     // read | write | admin
    ExpiresAt            *time.Time // Optional expiration timestamp
    IsActive             bool       // Manual activation toggle
    IsHiddenByRecipient  bool       // Recipient can hide from their list
    HiddenAt             *time.Time // Timestamp when hidden
    Terminal             Terminal   `gorm:"foreignKey:TerminalID"`
}
```

**Key Fields:**
- **TerminalID**: Links to the terminal being shared
- **SharedWithUserID**: Casdoor user ID of the recipient
- **SharedByUserID**: Casdoor user ID of the terminal owner
- **AccessLevel**: Permission level (see Access Levels section)
- **ExpiresAt**: Optional auto-expiration (NULL = never expires)
- **IsActive**: Manual toggle to enable/disable share without deletion
- **IsHiddenByRecipient**: Allows recipient to hide shared terminal from their interface
- **HiddenAt**: Tracks when terminal was hidden

### 2. Access Levels

The system implements a hierarchical access control model with three levels:

#### Level 1: READ (`"read"`)
**Permissions:**
- View terminal console output
- Read terminal metadata (name, status, configuration)
- Access terminal info endpoint

**Casbin Permissions Added:**
- `GET /api/v1/terminals/:id`
- `POST /api/v1/terminals/:id/hide` (hide functionality)
- `DELETE /api/v1/terminals/:id/hide` (unhide functionality)

**Use Case**: Observers, students viewing instructor demonstrations

#### Level 2: WRITE (`"write"`)
**Permissions:**
- All READ permissions
- Modify terminal settings (name, configuration)
- Send commands to terminal (via WebSocket)
- Edit terminal metadata

**Casbin Permissions Added:**
- All READ permissions
- `PATCH /api/v1/terminals/:id`

**Use Case**: Collaborators, teaching assistants who need to interact with terminals

#### Level 3: ADMIN (`"admin"`)
**Permissions:**
- All WRITE permissions
- Stop/delete terminal sessions
- Share terminal with other users
- Revoke access from other users
- View all shares for the terminal

**Casbin Permissions Added:**
- All WRITE permissions
- `DELETE /api/v1/terminals/:id`
- Full control equivalent to terminal owner (except ownership transfer)

**Use Case**: Co-administrators, trusted collaborators who need full management capabilities

### Access Level Hierarchy Implementation

**Location**: `src/terminalTrainer/models/terminalShare.go:44-67`

```go
func (ts *TerminalShare) HasAccess(requiredLevel string) bool {
    if !ts.IsActive || ts.IsExpired() {
        return false
    }

    accessLevels := map[string]int{
        "read":  1,
        "write": 2,
        "admin": 3,
    }

    currentLevel := accessLevels[ts.AccessLevel]
    requiredLevelInt := accessLevels[requiredLevel]

    return currentLevel >= requiredLevelInt
}
```

**Logic**: Higher access levels inherit all permissions from lower levels (admin includes write and read, write includes read).

## 3. API Endpoints

### Sharing Management Routes

**Location**: `src/terminalTrainer/routes/terminalRoutes.go:27-36`

#### Share Terminal
```
POST /api/v1/terminals/:id/share
Authorization: Bearer {JWT_TOKEN}
Content-Type: application/json

Request Body:
{
  "shared_with_user_id": "casdoor-user-id",
  "access_level": "read|write|admin",
  "expires_at": "2025-12-31T23:59:59Z" // Optional
}

Response (200 OK):
{
  "message": "Terminal shared successfully"
}

Errors:
- 404: Terminal not found
- 403: User is not terminal owner
- 400: Invalid access level or user ID
```

#### Revoke Terminal Access
```
DELETE /api/v1/terminals/:id/share/:user_id
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
{
  "message": "Access revoked successfully"
}

Errors:
- 404: Terminal or share not found
- 403: User is not terminal owner
```

#### Get Terminal Shares
```
GET /api/v1/terminals/:id/shares
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
[
  {
    "id": "uuid",
    "terminal_id": "uuid",
    "shared_with_user_id": "user-id",
    "shared_by_user_id": "owner-id",
    "access_level": "write",
    "expires_at": "2025-12-31T23:59:59Z",
    "is_active": true,
    "is_hidden_by_recipient": false,
    "hidden_at": null,
    "created_at": "2025-01-15T10:30:00Z"
  }
]

Errors:
- 403: User is not terminal owner
- 404: Terminal not found
```

#### Get Shared Terminals (Recipient View)
```
GET /api/v1/terminals/shared-with-me?include_hidden=false
Authorization: Bearer {JWT_TOKEN}

Query Parameters:
- include_hidden (optional): boolean, default false

Response (200 OK):
[
  {
    "terminal": {
      "id": "uuid",
      "name": "Production Server",
      "session_id": "term-123",
      "status": "running",
      ...
    },
    "shared_by": "owner-user-id",
    "shared_by_display_name": "John Doe",
    "access_level": "write",
    "expires_at": "2025-12-31T23:59:59Z",
    "shared_at": "2025-01-15T10:30:00Z",
    "shares": [] // Empty for recipients, populated for owners
  }
]
```

#### Get Shared Terminal Info
```
GET /api/v1/terminals/:id/info
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
{
  "terminal": { /* terminal details */ },
  "shared_by": "owner-user-id",
  "shared_by_display_name": "John Doe",
  "access_level": "write", // Current user's access level
  "expires_at": "2025-12-31T23:59:59Z",
  "shared_at": "2025-01-15T10:30:00Z",
  "shares": [ /* all shares - owner only */ ]
}

Errors:
- 403: User has no access to terminal
- 404: Terminal not found
```

### Hiding Management Routes

**Location**: `src/terminalTrainer/routes/terminalRoutes.go:38-43`

#### Hide Terminal
```
POST /api/v1/terminals/:id/hide
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
{
  "message": "Terminal hidden successfully"
}

Errors:
- 403: User is not owner or recipient
- 400: Terminal is still active (must be stopped first)
- 404: Terminal not found
```

**Constraints**: Can only hide inactive (stopped/expired) terminals.

#### Unhide Terminal
```
DELETE /api/v1/terminals/:id/hide
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
{
  "message": "Terminal unhidden successfully"
}

Errors:
- 403: User is not owner or recipient
- 404: Terminal not found or not hidden
```

## 4. Service Layer Logic

**Location**: `src/terminalTrainer/services/terminalTrainerService.go`

### ShareTerminal Method (Lines 860-956)

**Workflow:**
1. **Validate Terminal Ownership**: Check if `sharedByUserID` is the terminal owner
2. **Prevent Self-Sharing**: Ensure `sharedByUserID != sharedWithUserID`
3. **Validate Access Level**: Must be `"read"`, `"write"`, or `"admin"`
4. **Check Existing Share**: Query database for existing share
5. **Update or Create**:
   - If share exists: Update access level, expiration, reactivate if needed
   - If new: Create new `TerminalShare` record
6. **Add Casbin Permissions**:
   - Call `addTerminalHidePermissions()` to allow hiding
   - Call `addTerminalSharePermissions()` with specific access level
7. **Persist**: Save to database

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:860-956`

### RevokeTerminalAccess Method (Lines 971-1017)

**Workflow:**
1. **Validate Ownership**: Check if `requestingUserID` owns the terminal
2. **Find Share**: Query for active share with `sharedWithUserID`
3. **Remove Permissions**: Call `removeTerminalSharePermissions()`
4. **Delete Share**: Hard delete from database (not soft delete)

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:971-1017`

### HasTerminalAccess Method (Lines 1019-1037)

**Workflow:**
1. **Owner Check**: If `userID` matches terminal owner, return `true` immediately
2. **Query Share**: Find active share for user
3. **Validate Expiration**: Check if share is expired
4. **Check Access Level**: Use `HasAccess()` method to verify hierarchy
5. **Return Result**: `true` if access granted, `false` otherwise

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1019-1037`

### GetSharedTerminals Method (Lines 1039-1074)

**Workflow:**
1. **Query Shares**: Get all active shares for `userID` from repository
2. **Filter by Hidden Status**: Apply `include_hidden` filter if specified
3. **Load Terminals**: Preload terminal data and owner information
4. **Fetch Owner Display Names**: Query Casdoor API for owner details
5. **Build Response**: Create `SharedTerminalInfo` DTOs with sharing context

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1039-1074`

### HideTerminal Method (Lines 1133-1162)

**Workflow:**
1. **Validate Access**: Check if user is owner OR recipient with valid share
2. **Check Terminal Status**: Ensure terminal is inactive (stopped/expired)
3. **Update Hidden Status**:
   - If owner: Set `is_hidden` on `Terminal` model
   - If recipient: Set `is_hidden_by_recipient` on `TerminalShare` model
4. **Set Timestamp**: Record `hidden_at` timestamp

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1133-1162`

## 5. Permission System Integration (Casbin)

### Permission Structure

**Format**: `userID, route, methods`

**Example**:
```
("user-123", "/api/v1/terminals/abc-uuid", "GET")
("user-123", "/api/v1/terminals/abc-uuid", "PATCH")
("user-123", "/api/v1/terminals/abc-uuid/hide", "POST|DELETE")
```

### Permission Assignment Methods

**Location**: `src/terminalTrainer/services/terminalTrainerService.go:1190-1298`

#### addTerminalHidePermissions (Lines 1190-1211)

**Purpose**: Grant hide/unhide capabilities to shared users

**Permissions Added**:
- `POST /api/v1/terminals/:id/hide`
- `DELETE /api/v1/terminals/:id/hide`

**Called**: During terminal sharing (all access levels)

```go
func (tts *terminalTrainerService) addTerminalHidePermissions(
    userID string,
    sessionID string,
) error {
    route := "/api/v1/terminals/" + sessionID + "/hide"
    methods := "POST|DELETE"

    _, err := casdoor.Enforcer.AddPolicy(userID, route, methods)
    return err
}
```

#### addTerminalSharePermissions (Lines 1213-1258)

**Purpose**: Grant access-level-specific permissions to shared users

**Permissions by Access Level**:

**READ Level**:
- `GET /api/v1/terminals/:id`

**WRITE Level**:
- `GET /api/v1/terminals/:id`
- `PATCH /api/v1/terminals/:id`

**ADMIN Level**:
- `GET /api/v1/terminals/:id`
- `PATCH /api/v1/terminals/:id`
- `DELETE /api/v1/terminals/:id`

**Called**: During terminal sharing after validating access level

```go
func (tts *terminalTrainerService) addTerminalSharePermissions(
    userID string,
    sessionID string,
    accessLevel string,
) error {
    route := "/api/v1/terminals/" + sessionID

    var methods string
    switch accessLevel {
    case "read":
        methods = "GET"
    case "write":
        methods = "GET|PATCH"
    case "admin":
        methods = "GET|PATCH|DELETE"
    }

    _, err := casdoor.Enforcer.AddPolicy(userID, route, methods)
    return err
}
```

#### removeTerminalSharePermissions (Lines 1260-1298)

**Purpose**: Revoke all sharing permissions when access is revoked

**Permissions Removed**:
- Terminal access route: `/api/v1/terminals/:id`
- Hide route: `/api/v1/terminals/:id/hide`

**Called**: During access revocation

```go
func (tts *terminalTrainerService) removeTerminalSharePermissions(
    userID string,
    sessionID string,
) error {
    terminalRoute := "/api/v1/terminals/" + sessionID
    hideRoute := "/api/v1/terminals/" + sessionID + "/hide"

    casdoor.Enforcer.RemoveFilteredPolicy(0, userID, terminalRoute)
    casdoor.Enforcer.RemoveFilteredPolicy(0, userID, hideRoute)

    return casdoor.Enforcer.SavePolicy()
}
```

### Automatic Permission Setup

**During Terminal Creation** (`src/terminalTrainer/services/terminalTrainerService.go:1300-1330`):
- Automatically calls `addTerminalHidePermissions()` for the owner
- Grants owner hide/unhide capabilities for their own terminals

**During Terminal Sharing**:
1. Hide permissions added (all access levels)
2. Access-level-specific permissions added (based on `accessLevel` parameter)

**During Access Revocation**:
- All permissions removed via `removeTerminalSharePermissions()`

## 6. Database Queries and Repository

**Location**: `src/terminalTrainer/repositories/terminalRepository.go`

### Key Repository Methods

#### GetSharedTerminalsForUser (Lines 362-379)
```go
func (tr *terminalRepository) GetSharedTerminalsForUser(
    userID string,
) (*[]models.Terminal, error) {
    var terminals []models.Terminal

    err := tr.db.
        Joins("JOIN terminal_shares ON terminals.id = terminal_shares.terminal_id").
        Where("terminal_shares.shared_with_user_id = ?", userID).
        Where("terminal_shares.is_active = ?", true).
        Where("terminal_shares.expires_at IS NULL OR terminal_shares.expires_at > ?", time.Now()).
        Preload("TerminalShares").
        Find(&terminals).Error

    return &terminals, err
}
```

**Features**:
- Joins `terminals` with `terminal_shares` table
- Filters by recipient user ID
- Only returns active, non-expired shares
- Preloads sharing metadata

#### GetSharedTerminalsForUserWithHidden (Lines 398-414)
```go
func (tr *terminalRepository) GetSharedTerminalsForUserWithHidden(
    userID string,
    includeHidden bool,
) (*[]models.Terminal, error) {
    query := tr.db.
        Joins("JOIN terminal_shares ON terminals.id = terminal_shares.terminal_id").
        Where("terminal_shares.shared_with_user_id = ?", userID).
        Where("terminal_shares.is_active = ?", true).
        Where("terminal_shares.expires_at IS NULL OR terminal_shares.expires_at > ?", time.Now())

    if !includeHidden {
        query = query.Where("terminal_shares.is_hidden_by_recipient = ?", false)
    }

    return query.Preload("TerminalShares").Find(&terminals).Error
}
```

**Features**:
- Same as `GetSharedTerminalsForUser` but with hidden filter
- Conditional `WHERE` clause based on `includeHidden` parameter

#### HideTerminalForUser (Lines 416-429)
```go
func (tr *terminalRepository) HideTerminalForUser(
    terminalID uuid.UUID,
    userID string,
) error {
    return tr.db.
        Model(&models.TerminalShare{}).
        Where("terminal_id = ?", terminalID).
        Where("shared_with_user_id = ?", userID).
        Updates(map[string]interface{}{
            "is_hidden_by_recipient": true,
            "hidden_at": time.Now(),
        }).Error
}
```

#### UnhideTerminalForUser (Lines 431-447)
```go
func (tr *terminalRepository) UnhideTerminalForUser(
    terminalID uuid.UUID,
    userID string,
) error {
    return tr.db.
        Model(&models.TerminalShare{}).
        Where("terminal_id = ?", terminalID).
        Where("shared_with_user_id = ?", userID).
        Updates(map[string]interface{}{
            "is_hidden_by_recipient": false,
            "hidden_at": nil,
        }).Error
}
```

#### GetTerminalShareByUserAndTerminal (Lines 381-396)
```go
func (tr *terminalRepository) GetTerminalShareByUserAndTerminal(
    userID string,
    terminalID uuid.UUID,
) (*models.TerminalShare, error) {
    var share models.TerminalShare

    err := tr.db.
        Where("shared_with_user_id = ?", userID).
        Where("terminal_id = ?", terminalID).
        Where("is_active = ?", true).
        Where("expires_at IS NULL OR expires_at > ?", time.Now()).
        First(&share).Error

    return &share, err
}
```

**Features**:
- Validates active, non-expired shares
- Used for access level checking

## 7. Entity Registration and Generic CRUD

**Location**: `src/terminalTrainer/entityRegistration/terminalShareRegistration.go`

### Generic Entity Permissions (Lines 277-286)

```go
func (tsr TerminalShareRegistration) GetEntityRoles() EntityRoles {
    roleMap := make(map[string]string)

    // Members can view and create shares
    roleMap[string(authModels.Member)] = "(GET|POST)"

    // Group managers can view, create, and delete shares
    roleMap[string(authModels.GroupManager)] = "(GET|POST|DELETE)"

    // Admins have full control (including PATCH for updates)
    roleMap[string(authModels.Admin)] = "(GET|POST|DELETE|PATCH)"

    return roleMap
}
```

**Note**: These are generic entity permissions for the `/api/v1/terminal-shares` route. **Specific terminal sharing routes** (e.g., `/terminals/:id/share`) use **custom Casbin policies** added in service methods.

### Queryable Fields (Lines 147-153)

**Filterable Fields**:
- `terminal_id` - Filter shares by terminal
- `shared_with_user_id` - Filter by recipient
- `shared_by_user_id` - Filter by owner
- `access_level` - Filter by permission level
- `is_active` - Filter by active status

**Example Query**:
```
GET /api/v1/terminal-shares?terminal_id=abc-uuid&is_active=true
```

### Validation Rules (Lines 134-135)

```go
type CreateTerminalShareInput struct {
    TerminalID       uuid.UUID  `json:"terminal_id" binding:"required"`
    SharedWithUserID string     `json:"shared_with_user_id" binding:"required"`
    AccessLevel      string     `json:"access_level" binding:"required,oneof=read write admin"`
    ExpiresAt        *time.Time `json:"expires_at"`
}
```

**Validation**:
- `terminal_id`: Required, must be valid UUID
- `shared_with_user_id`: Required, non-empty string
- `access_level`: Required, must be exactly "read", "write", or "admin"
- `expires_at`: Optional, must be valid timestamp if provided

## 8. Terminal Hiding System

### Dual Hiding Mechanism

The system supports **two independent hiding mechanisms**:

#### 1. Owner Hiding (Terminal Model)
**Location**: `src/terminalTrainer/models/terminal.go`

```go
type Terminal struct {
    // ... other fields
    IsHidden bool       `gorm:"default:false"`
    HiddenAt *time.Time `gorm:"default:null"`
}
```

**Behavior**:
- Owner can hide their own terminals
- Hides terminal from owner's terminal list
- Does NOT affect recipient visibility
- Persisted on `terminals.is_hidden` and `terminals.hidden_at`

#### 2. Recipient Hiding (TerminalShare Model)
**Location**: `src/terminalTrainer/models/terminalShare.go`

```go
type TerminalShare struct {
    // ... other fields
    IsHiddenByRecipient bool       `gorm:"default:false"`
    HiddenAt            *time.Time `gorm:"default:null"`
}
```

**Behavior**:
- Recipient can hide shared terminals from their "shared with me" list
- Does NOT affect owner's visibility
- Does NOT affect other recipients
- Each recipient has independent hiding status
- Persisted on `terminal_shares.is_hidden_by_recipient` and `terminal_shares.hidden_at`

### Hiding Constraints

**Constraint**: Can only hide **inactive terminals**

**Enforcement**: `src/terminalTrainer/services/terminalTrainerService.go:1147-1150`

```go
if terminal.Status != "stopped" && !terminal.IsExpired() {
    return errors.New("can only hide inactive terminals")
}
```

**Rationale**: Prevents hiding active terminals that may have ongoing operations.

### Hiding API Endpoints

#### Hide Terminal
```
POST /api/v1/terminals/:id/hide
```

**Authorization**: Owner OR recipient with valid share

**Logic**:
- If user is owner: Sets `terminals.is_hidden = true`
- If user is recipient: Sets `terminal_shares.is_hidden_by_recipient = true` for that user's share

#### Unhide Terminal
```
DELETE /api/v1/terminals/:id/hide
```

**Authorization**: Owner OR recipient with valid share

**Logic**:
- If user is owner: Sets `terminals.is_hidden = false`
- If user is recipient: Sets `terminal_shares.is_hidden_by_recipient = false` for that user's share

### Hiding Permissions

**Added During**:
- Terminal creation (for owner)
- Terminal sharing (for recipients)

**Permission Format**:
```
userID, /api/v1/terminals/:id/hide, POST|DELETE
```

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1190-1211`

## 9. Use Case Examples

### Example 1: Instructor Sharing Terminal with Students

**Scenario**: Instructor wants to share a demo terminal with students for read-only observation.

**Steps**:
1. Instructor creates terminal (becomes owner)
2. Instructor shares terminal with student:
   ```bash
   curl -X POST http://localhost:8080/api/v1/terminals/{terminal-id}/share \
     -H "Authorization: Bearer {instructor-token}" \
     -H "Content-Type: application/json" \
     -d '{
       "shared_with_user_id": "student-123",
       "access_level": "read"
     }'
   ```
3. System creates `TerminalShare` record with `access_level = "read"`
4. Casbin permissions added:
   - `GET /api/v1/terminals/{terminal-id}` (read terminal)
   - `POST|DELETE /api/v1/terminals/{terminal-id}/hide` (hide/unhide)
5. Student can now:
   - View terminal console output (WebSocket connection)
   - Read terminal metadata
   - Hide terminal from their list when done
6. Student CANNOT:
   - Send commands to terminal
   - Modify terminal settings
   - Delete terminal
   - Share with others

### Example 2: Time-Limited Collaboration

**Scenario**: Developer grants temporary write access to a colleague for pair programming.

**Steps**:
1. Developer shares terminal with 24-hour expiration:
   ```bash
   curl -X POST http://localhost:8080/api/v1/terminals/{terminal-id}/share \
     -H "Authorization: Bearer {developer-token}" \
     -H "Content-Type: application/json" \
     -d '{
       "shared_with_user_id": "colleague-456",
       "access_level": "write",
       "expires_at": "2025-01-16T10:30:00Z"
     }'
   ```
2. Colleague can edit and send commands for 24 hours
3. After expiration, `HasAccess()` returns `false` automatically
4. Casbin permissions remain but are ineffective (service layer blocks access)
5. No manual revocation needed

### Example 3: Admin Delegation

**Scenario**: Team lead delegates terminal management to trusted team member.

**Steps**:
1. Team lead shares terminal with admin access:
   ```bash
   curl -X POST http://localhost:8080/api/v1/terminals/{terminal-id}/share \
     -H "Authorization: Bearer {lead-token}" \
     -H "Content-Type: application/json" \
     -d '{
       "shared_with_user_id": "team-member-789",
       "access_level": "admin"
     }'
   ```
2. Team member can:
   - Stop/delete terminal
   - Share with additional users
   - Revoke access from others (except owner)
   - Full control equivalent to owner
3. Team member CANNOT:
   - Transfer ownership
   - Delete the share granted by owner to themselves (owner must revoke)

### Example 4: Hiding Shared Terminals

**Scenario**: Student receives 20 shared terminals from various instructors over a semester. After completing courses, student wants to hide old terminals.

**Steps**:
1. Student views shared terminals:
   ```bash
   curl -X GET http://localhost:8080/api/v1/terminals/shared-with-me \
     -H "Authorization: Bearer {student-token}"
   ```
2. Student hides completed terminals:
   ```bash
   curl -X POST http://localhost:8080/api/v1/terminals/{terminal-id}/hide \
     -H "Authorization: Bearer {student-token}"
   ```
3. Hidden terminals no longer appear in default list
4. Student can unhide anytime:
   ```bash
   curl -X DELETE http://localhost:8080/api/v1/terminals/{terminal-id}/hide \
     -H "Authorization: Bearer {student-token}"
   ```
5. Instructor's view is unaffected (still sees terminal as shared)

## 10. Security Considerations

### Access Control Validation

**Multi-Layer Security**:
1. **Authentication Middleware**: Validates JWT token
2. **Casbin Authorization**: Checks route-level permissions
3. **Service Layer Validation**: Verifies ownership and access levels
4. **Repository Layer**: Filters expired/inactive shares

### Permission Isolation

**User-Specific Permissions**:
- Each permission includes `userID` as first parameter
- Permissions are scoped to specific terminal UUIDs
- No wildcard permissions (e.g., cannot grant access to all terminals)

**Example**:
```
Allowed: ("user-123", "/api/v1/terminals/abc-uuid", "GET")
NOT Allowed: ("user-123", "/api/v1/terminals/*", "GET")
```

### Expiration Handling

**Automatic Expiration**:
- `IsExpired()` method checks `expires_at` timestamp
- `HasAccess()` returns `false` for expired shares
- Repository queries filter out expired shares automatically
- No cron job needed (validation on-demand)

**Code Reference**: `src/terminalTrainer/models/terminalShare.go:33-42`

```go
func (ts *TerminalShare) IsExpired() bool {
    if ts.ExpiresAt == nil {
        return false // NULL = never expires
    }
    return time.Now().After(*ts.ExpiresAt)
}
```

### Owner Protection

**Cannot Be Locked Out**:
- Owner ALWAYS has full access (service layer override)
- Owner cannot share terminal with themselves (validation blocks)
- Owner cannot revoke their own access
- Shares cannot modify ownership

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1025-1028`

```go
// Owner always has full access
if terminal.UserID == userID {
    return true, nil
}
```

### Activation Toggle

**Manual Disable Without Deletion**:
- `is_active` field allows temporary deactivation
- Preserves share configuration for reactivation
- Safer than deletion (no data loss)

**Use Case**: Temporarily revoke access during sensitive operations, then restore.

## 11. Database Schema

### TerminalShares Table

```sql
CREATE TABLE terminal_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL,

    terminal_id UUID NOT NULL REFERENCES terminals(id) ON DELETE CASCADE,
    shared_with_user_id VARCHAR(255) NOT NULL,
    shared_by_user_id VARCHAR(255) NOT NULL,
    access_level VARCHAR(50) NOT NULL CHECK (access_level IN ('read', 'write', 'admin')),
    expires_at TIMESTAMP DEFAULT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    is_hidden_by_recipient BOOLEAN DEFAULT FALSE,
    hidden_at TIMESTAMP DEFAULT NULL,

    UNIQUE (terminal_id, shared_with_user_id)
);

CREATE INDEX idx_terminal_shares_terminal_id ON terminal_shares(terminal_id);
CREATE INDEX idx_terminal_shares_shared_with_user_id ON terminal_shares(shared_with_user_id);
CREATE INDEX idx_terminal_shares_shared_by_user_id ON terminal_shares(shared_by_user_id);
CREATE INDEX idx_terminal_shares_expires_at ON terminal_shares(expires_at) WHERE expires_at IS NOT NULL;
```

**Key Constraints**:
- `UNIQUE (terminal_id, shared_with_user_id)`: One share per terminal-user pair
- `CHECK (access_level IN ('read', 'write', 'admin'))`: Valid access levels only
- `ON DELETE CASCADE`: Deleting terminal removes all shares

### Terminals Table (Hiding Fields)

```sql
CREATE TABLE terminals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- ... other fields
    is_hidden BOOLEAN DEFAULT FALSE,
    hidden_at TIMESTAMP DEFAULT NULL
);

CREATE INDEX idx_terminals_is_hidden ON terminals(is_hidden);
```

## 12. Testing Guidelines

### Unit Testing Access Levels

**Test Cases**:
1. READ access cannot modify terminals
2. WRITE access can modify but not delete
3. ADMIN access has full control
4. Owner always has access regardless of shares
5. Expired shares return `false` for `HasAccess()`
6. Inactive shares block access

**Code Reference**: `tests/terminalTrainer/terminalShare_test.go` (if exists)

### Integration Testing Sharing Flow

**Test Scenarios**:
1. Share terminal → verify Casbin permissions added
2. Revoke access → verify permissions removed
3. Update access level → verify permissions updated
4. Expired share → verify access blocked
5. Deactivate share → verify access blocked
6. Hide terminal (owner) → verify visibility change
7. Hide terminal (recipient) → verify independent visibility

### Testing Casbin Permission Enforcement

**Verification Steps**:
1. Share terminal with READ access
2. Attempt PATCH request as recipient → should return 403
3. Share same terminal with WRITE access
4. Attempt PATCH request as recipient → should return 200
5. Revoke access
6. Attempt GET request as recipient → should return 403

## 13. Future Enhancements

### Potential Improvements

1. **Group Sharing**: Share terminal with entire groups instead of individual users
2. **Share Templates**: Predefined access configurations for common use cases
3. **Audit Logging**: Track all sharing actions for compliance
4. **Notification System**: Notify users when terminals are shared with them
5. **Access Request System**: Allow users to request access to terminals
6. **Cascade Sharing**: Allow admin-level users to re-share terminals
7. **Custom Access Levels**: Define organization-specific permission sets
8. **Usage Analytics**: Track shared terminal usage metrics

### Known Limitations

1. **No Transfer Ownership**: Original owner cannot transfer ownership to another user
2. **No Revoke by Recipient**: Recipients cannot decline/remove shared access themselves
3. **No Share Limits**: No maximum number of shares per terminal
4. **No Bulk Operations**: Cannot share one terminal with multiple users in single request
5. **No Share History**: Deleted shares are not tracked historically

## 14. Troubleshooting

### Common Issues

#### Issue: User cannot access shared terminal (403 Forbidden)

**Possible Causes**:
1. Share is expired (`expires_at` passed)
2. Share is inactive (`is_active = false`)
3. Casbin permissions not properly added
4. JWT token doesn't match `shared_with_user_id`

**Debugging Steps**:
1. Check database: `SELECT * FROM terminal_shares WHERE shared_with_user_id = 'user-id'`
2. Verify expiration: `SELECT expires_at, is_active FROM terminal_shares WHERE id = 'share-id'`
3. Check Casbin policies: Query enforcer for user's permissions
4. Verify JWT token claims

#### Issue: Permissions not updated after access level change

**Possible Causes**:
1. Casbin cache not refreshed
2. Old permissions not removed before adding new ones

**Solution**:
1. Restart application to reload Casbin enforcer
2. Manually remove old permissions via `removeTerminalSharePermissions()`
3. Re-add permissions via `addTerminalSharePermissions()`

#### Issue: Cannot hide active terminal

**Cause**: Hiding restricted to inactive terminals only

**Solution**:
1. Stop terminal first
2. Then hide terminal

#### Issue: Hidden terminals still appear in shared list

**Possible Causes**:
1. `include_hidden=true` query parameter used
2. Hiding applied to wrong user (owner vs recipient)

**Solution**:
1. Verify query parameter: `GET /api/v1/terminals/shared-with-me?include_hidden=false`
2. Check `is_hidden_by_recipient` field for recipient, `is_hidden` for owner

## 15. Code References Summary

### Models
- `src/terminalTrainer/models/terminalShare.go` - TerminalShare model and access logic

### Services
- `src/terminalTrainer/services/terminalTrainerService.go:860-1330` - Sharing service methods
  - Lines 860-956: ShareTerminal
  - Lines 971-1017: RevokeTerminalAccess
  - Lines 1019-1037: HasTerminalAccess
  - Lines 1039-1074: GetSharedTerminals
  - Lines 1133-1162: HideTerminal
  - Lines 1164-1188: UnhideTerminal
  - Lines 1190-1298: Casbin permission management

### Repositories
- `src/terminalTrainer/repositories/terminalRepository.go:362-447` - Database queries
  - Lines 362-379: GetSharedTerminalsForUser
  - Lines 381-396: GetTerminalShareByUserAndTerminal
  - Lines 398-414: GetSharedTerminalsForUserWithHidden
  - Lines 416-429: HideTerminalForUser
  - Lines 431-447: UnhideTerminalForUser

### Routes
- `src/terminalTrainer/routes/terminalRoutes.go:27-43` - API endpoints

### Controllers
- `src/terminalTrainer/routes/terminalController.go:874-1201` - HTTP handlers

### Entity Registration
- `src/terminalTrainer/entityRegistration/terminalShareRegistration.go` - Generic CRUD config

## 16. Conclusion

The OCF Core terminal sharing system provides a robust, hierarchical access control mechanism with:

- **Three access levels** (read, write, admin) with inheritance
- **Time-based expiration** for temporary access
- **Manual activation toggle** for flexible control
- **Independent hiding** for both owners and recipients
- **Automatic Casbin integration** for route-level enforcement
- **Owner protection** ensuring owners cannot be locked out
- **Granular permissions** scoped to specific terminals and users

The system is production-ready and extensible for future enhancements like group sharing, audit logging, and custom access levels.
