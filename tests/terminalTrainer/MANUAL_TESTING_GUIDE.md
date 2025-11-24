# Manual Testing Guide: Terminal PATCH Permissions

This guide provides step-by-step instructions for manually testing the user-specific Casbin permissions for terminal PATCH operations.

## What We're Testing

The hook-based permission system that automatically:
1. ✅ Grants PATCH permission to terminal owners when a terminal is created
2. ✅ Grants PATCH permission to users with "write" or "admin" share access
3. ❌ Denies PATCH permission to users with "read" share access
4. ✅ Removes PATCH permission when shares are revoked
5. ✅ Cleans up all permissions when terminals are deleted

## Prerequisites

### 1. Start the Server

```bash
cd /workspaces/ocf-core
go run main.go
```

**Verify hooks are registered** - Look for these log messages on startup:
```
🔗 Initializing terminal hooks...
✅ Terminal owner permission hook registered
✅ Terminal cleanup hook registered
✅ TerminalShare permission hook registered
✅ TerminalShare revoke hook registered
🔗 Terminal hooks initialization complete
```

### 2. Get Authentication Tokens

You need tokens for at least 2 users (preferably 3):
- **Owner User**: Will create and own the terminal
- **Shared User**: Will receive terminal shares
- **Other User** (optional): Will test unauthorized access

#### Option A: Login via API

```bash
# Owner login
OWNER_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{
        "username": "owner-username",
        "password": "owner-password"
    }')

OWNER_TOKEN=$(echo "$OWNER_RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
export OWNER_TOKEN

# Shared user login
SHARED_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{
        "username": "shared-username",
        "password": "shared-password"
    }')

SHARED_USER_TOKEN=$(echo "$SHARED_RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
SHARED_USER_ID=$(echo "$SHARED_RESPONSE" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
export SHARED_USER_TOKEN
export SHARED_USER_ID
```

#### Option B: Use Existing Tokens

If you have tokens from previous sessions:

```bash
export OWNER_TOKEN="your-owner-token-here"
export SHARED_USER_TOKEN="your-shared-user-token-here"
export SHARED_USER_ID="shared-user-id-here"
export OTHER_USER_TOKEN="your-other-user-token-here"  # Optional
```

## Automated Testing

Run the automated test script:

```bash
cd /workspaces/ocf-core
./scripts/test_terminal_patch_permissions.sh
```

This script will execute all test scenarios and report results.

## Manual Step-by-Step Testing

### Test 1: Owner Creates Terminal and Can PATCH

#### 1.1 Create a UserTerminalKey

```bash
KEY_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/user-terminal-keys \
    -H "Authorization: Bearer $OWNER_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "keyName": "Manual Test Key",
        "maxSessions": 5
    }')

KEY_ID=$(echo "$KEY_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "Key ID: $KEY_ID"
```

**Expected**: Key created successfully with an ID

#### 1.2 Create Terminal

```bash
TERMINAL_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/terminals \
    -H "Authorization: Bearer $OWNER_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"sessionID\": \"test-session-$(date +%s)\",
        \"name\": \"Test Terminal\",
        \"expiresAt\": \"$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)\",
        \"terminalTrainerKeyID\": \"$KEY_ID\"
    }")

TERMINAL_ID=$(echo "$TERMINAL_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "Terminal ID: $TERMINAL_ID"
```

**Expected**:
- HTTP 201 Created
- Terminal ID returned
- Server log shows: `✅ Granted PATCH permission to terminal owner <user-id> for terminal <terminal-id>`

#### 1.3 Owner PATCH Terminal

```bash
curl -s -w "\nHTTP Status: %{http_code}\n" -X PATCH "http://localhost:8080/api/v1/terminals/$TERMINAL_ID" \
    -H "Authorization: Bearer $OWNER_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "Renamed by Owner"
    }'
```

**Expected**:
- ✅ HTTP 200 OK
- Terminal name updated successfully

### Test 2: Non-Owner Cannot PATCH

```bash
curl -s -w "\nHTTP Status: %{http_code}\n" -X PATCH "http://localhost:8080/api/v1/terminals/$TERMINAL_ID" \
    -H "Authorization: Bearer $OTHER_USER_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "Attempt by non-owner"
    }'
```

**Expected**:
- ❌ HTTP 403 Forbidden
- Error message about insufficient permissions

### Test 3: Share with "read" Access - Cannot PATCH

#### 3.1 Create Share

```bash
SHARE_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/terminal-shares \
    -H "Authorization: Bearer $OWNER_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"terminalID\": \"$TERMINAL_ID\",
        \"sharedWithUserID\": \"$SHARED_USER_ID\",
        \"accessLevel\": \"read\"
    }")

READ_SHARE_ID=$(echo "$SHARE_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "Read Share ID: $READ_SHARE_ID"
```

**Expected**:
- HTTP 201 Created
- Server log shows: `🔒 Not granting PATCH permission - access level 'read' doesn't allow editing`

#### 3.2 Try to PATCH with Read Access

```bash
sleep 1  # Give hooks time to execute

curl -s -w "\nHTTP Status: %{http_code}\n" -X PATCH "http://localhost:8080/api/v1/terminals/$TERMINAL_ID" \
    -H "Authorization: Bearer $SHARED_USER_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "Attempt with read access"
    }'
```

**Expected**:
- ❌ HTTP 403 Forbidden
- User with "read" access should NOT be able to PATCH

#### 3.3 Delete Read Share

```bash
curl -s -w "\nHTTP Status: %{http_code}\n" -X DELETE "http://localhost:8080/api/v1/terminal-shares/$READ_SHARE_ID" \
    -H "Authorization: Bearer $OWNER_TOKEN"
```

### Test 4: Share with "write" Access - Can PATCH

#### 4.1 Create Write Share

```bash
SHARE_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/terminal-shares \
    -H "Authorization: Bearer $OWNER_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"terminalID\": \"$TERMINAL_ID\",
        \"sharedWithUserID\": \"$SHARED_USER_ID\",
        \"accessLevel\": \"write\"
    }")

WRITE_SHARE_ID=$(echo "$SHARE_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "Write Share ID: $WRITE_SHARE_ID"
```

**Expected**:
- HTTP 201 Created
- Server log shows: `✅ Granted PATCH permission to shared user <user-id> for terminal <terminal-id> (access level: write)`

#### 4.2 PATCH with Write Access

```bash
sleep 1  # Give hooks time to execute

curl -s -w "\nHTTP Status: %{http_code}\n" -X PATCH "http://localhost:8080/api/v1/terminals/$TERMINAL_ID" \
    -H "Authorization: Bearer $SHARED_USER_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "Successfully renamed with write access"
    }'
```

**Expected**:
- ✅ HTTP 200 OK
- Terminal name updated successfully

### Test 5: Revoke Share Removes Permission

#### 5.1 Delete Write Share

```bash
curl -s -w "\nHTTP Status: %{http_code}\n" -X DELETE "http://localhost:8080/api/v1/terminal-shares/$WRITE_SHARE_ID" \
    -H "Authorization: Bearer $OWNER_TOKEN"
```

**Expected**:
- HTTP 200 OK
- Server log shows: `✅ Removed PATCH permission from shared user <user-id> for terminal <terminal-id>`

#### 5.2 Try to PATCH After Revoke

```bash
sleep 1  # Give hooks time to execute

curl -s -w "\nHTTP Status: %{http_code}\n" -X PATCH "http://localhost:8080/api/v1/terminals/$TERMINAL_ID" \
    -H "Authorization: Bearer $SHARED_USER_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "Attempt after revoke"
    }'
```

**Expected**:
- ❌ HTTP 403 Forbidden
- Permission should be removed after share is revoked

### Test 6: Delete Terminal Cleans Up All Permissions

```bash
curl -s -w "\nHTTP Status: %{http_code}\n" -X DELETE "http://localhost:8080/api/v1/terminals/$TERMINAL_ID" \
    -H "Authorization: Bearer $OWNER_TOKEN"
```

**Expected**:
- HTTP 200 OK
- Server log shows: `✅ Removed all permissions for terminal <terminal-id>`
- All user-specific policies for this terminal are cleaned up

## Cleanup

```bash
# Delete test key
curl -s -X DELETE "http://localhost:8080/api/v1/user-terminal-keys/$KEY_ID" \
    -H "Authorization: Bearer $OWNER_TOKEN"
```

## Verifying Hook Execution

Watch the server logs during testing to see hooks executing:

```
✅ Granted PATCH permission to terminal owner user-123 for terminal 019abc...
✅ Granted PATCH permission to shared user user-456 for terminal 019abc... (access level: write)
🔒 Not granting PATCH permission - access level 'read' doesn't allow editing
✅ Removed PATCH permission from shared user user-456 for terminal 019abc...
✅ Removed all permissions for terminal 019abc...
```

## Troubleshooting

### Hook not executing

Check that hooks are initialized in `main.go`:
```go
terminalHooks.InitTerminalHooks(sqldb.DB)
```

### Permission still exists after revoke

- Verify the AfterDelete hook is registered and executing
- Check server logs for hook execution messages
- Verify Casbin policies with: `casdoor.Enforcer.GetPolicy()`

### 403 when it should be 200

- Verify the token is valid and not expired
- Check that the user ID in the token matches the expected user
- Verify hook execution in server logs

### 200 when it should be 403

- Check that role-level permissions don't override user-specific denials
- Verify the user doesn't have an admin role that bypasses restrictions

## Success Criteria

All tests should pass with:
- ✅ Owner can PATCH their terminals
- ✅ Users with "write" or "admin" share access can PATCH
- ❌ Users with "read" share access cannot PATCH
- ❌ Non-shared users cannot PATCH
- ✅ Permissions are removed when shares are revoked
- ✅ All permissions are cleaned up when terminals are deleted
