# Terminal Permissions Fix

## Issue
User with "member" role was denied access to terminal-related endpoints:
```
[DEBUG] Role 'member' enforcement result: false
[DEBUG] ❌ AUTHORIZATION FAILED for user 027ee7be-9843-486e-93f3-60ce9f8dd10b
trying to access POST /api/v1/user-terminal-keys/regenerate
```

## Root Cause
Custom terminal routes (`/regenerate`, `/my-key`, `/console`, etc.) were not covered by the entity registration permissions system. These routes require explicit Casbin policy entries.

## Solution
Added comprehensive terminal permissions for the "member" role in `src/initialization/permissions.go`.

### Permissions Added

#### User Terminal Key Routes
- ✅ `POST /api/v1/user-terminal-keys/regenerate` - Regenerate user's terminal key
- ✅ `GET /api/v1/user-terminal-keys/my-key` - Get user's terminal key info

#### Terminal Management Routes
- ✅ `POST /api/v1/terminals/start-session` - Start new terminal session
- ✅ `GET /api/v1/terminals/user-sessions` - Get user's terminal sessions
- ✅ `GET /api/v1/terminals/shared-with-me` - Get terminals shared with user
- ✅ `POST /api/v1/terminals/sync-all` - Sync all sessions
- ✅ `GET /api/v1/terminals/instance-types` - Get available instance types
- ✅ `GET /api/v1/terminals/metrics` - Get server metrics

#### Terminal Instance Operations (with :id)
- ✅ `GET /api/v1/terminals/:id/console` - Access terminal console
- ✅ `POST /api/v1/terminals/:id/stop` - Stop terminal session
- ✅ `POST /api/v1/terminals/:id/share` - Share terminal with another user
- ✅ `DELETE /api/v1/terminals/:id/share/:user_id` - Revoke terminal access
- ✅ `GET /api/v1/terminals/:id/shares` - Get terminal shares
- ✅ `GET /api/v1/terminals/:id/info` - Get terminal info
- ✅ `POST /api/v1/terminals/:id/hide` - Hide terminal from list
- ✅ `DELETE /api/v1/terminals/:id/hide` - Unhide terminal
- ✅ `POST /api/v1/terminals/:id/sync` - Sync terminal status
- ✅ `GET /api/v1/terminals/:id/status` - Get terminal status

## Files Modified
- ✅ `src/initialization/permissions.go` - Added terminal permissions for member role

## How to Apply

### 1. Restart the Server
The server needs to be restarted to initialize the new permissions:

```bash
# Stop current server
pkill -f "go run main.go"

# Start server (will automatically call SetupPaymentRolePermissions)
go run main.go
```

### 2. Verify Permissions Were Added
Check the server logs for confirmation messages:

```bash
grep "terminal.*permission" /path/to/server.log
```

Expected output:
```
Setting up user terminal key custom route permissions...
✅ Added member permission for /api/v1/user-terminal-keys/regenerate
✅ Added member permission for /api/v1/user-terminal-keys/my-key
Setting up terminal custom route permissions...
✅ Added member permission for POST /api/v1/terminals/start-session
✅ Added member permission for GET /api/v1/terminals/user-sessions
... (and all other routes)
✅ Terminal permissions setup completed
```

### 3. Test the Fix

#### Test Terminal Key Regeneration
```bash
TOKEN="<your-jwt-token>"

curl -X POST http://localhost:8080/api/v1/user-terminal-keys/regenerate \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json"
```

Expected response:
```json
{
  "message": "Key regenerated successfully"
}
```

#### Test Get My Key
```bash
curl -X GET http://localhost:8080/api/v1/user-terminal-keys/my-key \
  -H "Authorization: Bearer $TOKEN"
```

Expected response:
```json
{
  "id": "...",
  "user_id": "...",
  "key_name": "...",
  "is_active": true,
  "max_sessions": 5,
  "created_at": "..."
}
```

#### Test Terminal Session Start
```bash
curl -X POST http://localhost:8080/api/v1/terminals/start-session \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "instance_type": "small",
    "name": "Test Terminal"
  }'
```

## Verification Checklist

After restarting the server, verify:

- [ ] Server starts without errors
- [ ] Log shows "Setting up user terminal key custom route permissions..."
- [ ] Log shows "✅ Terminal permissions setup completed"
- [ ] User with "member" role can access `/api/v1/user-terminal-keys/regenerate`
- [ ] User with "member" role can access `/api/v1/user-terminal-keys/my-key`
- [ ] User with "member" role can start terminal sessions
- [ ] User with "member" role can access terminal console
- [ ] No more "Role 'member' enforcement result: false" for terminal routes

## Database Verification

You can verify the permissions were added to the database:

```sql
SELECT * FROM casbin_rule
WHERE v0 = 'member'
  AND v1 LIKE '/api/v1/%terminal%'
ORDER BY v1, v2;
```

Expected results should include all the routes listed above.

## Architecture Notes

### Why Custom Routes Need Explicit Permissions

1. **Entity Registration System**: Handles standard CRUD operations (GET, POST, PATCH, DELETE on entity endpoints)
2. **Custom Routes**: Routes like `/regenerate`, `/my-key`, `/console` are not CRUD operations and need explicit Casbin policies

### Permission Flow

```
main.go
  └─> initialization.SetupPaymentRolePermissions(enforcer)
       └─> Adds policies for custom terminal routes
            └─> enforcer.AddPolicy("member", "/api/v1/user-terminal-keys/regenerate", "POST")
                 └─> Saved to casbin_rule table
```

### Layer 2 Security

Note that some terminal routes have additional Layer 2 security checks via middleware:
- `RequireTerminalAccess("read")` - User must have read access to specific terminal
- `RequireTerminalAccess("admin")` - User must be terminal owner or admin

These checks happen AFTER the Casbin permission check (Layer 1).

## Troubleshooting

### Issue: Permissions still not working after restart

**Solution**: Manually add the permissions via SQL:

```sql
-- Add regenerate permission
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES ('p', 'member', '/api/v1/user-terminal-keys/regenerate', 'POST')
ON CONFLICT DO NOTHING;

-- Add my-key permission
INSERT INTO casbin_rule (ptype, v0, v1, v2)
VALUES ('p', 'member', '/api/v1/user-terminal-keys/my-key', 'GET')
ON CONFLICT DO NOTHING;

-- Reload enforcer
-- (restart server or call enforcer.LoadPolicy())
```

### Issue: Some terminal operations still fail

Check if the route uses Layer 2 security (terminal ownership check). If so:
1. Verify user owns the terminal or has it shared with them
2. Check terminal_shares table for proper relationships
3. Review terminal access logs

### Issue: Old permissions cached

```bash
# Clear Redis cache if using caching
redis-cli FLUSHDB

# Or restart with clean enforcer
pkill -f "go run main.go"
go run main.go
```

## Related Documentation

- [Entity Registration System](src/entityManagement/README.md)
- [Casbin Authorization](https://casbin.org/docs/overview)
- [Terminal Middleware](src/terminalTrainer/middleware/terminalAccessMiddleware.go)
- [Terminal Routes](src/terminalTrainer/routes/terminalRoutes.go)

## Future Improvements

Consider:
1. Automating custom route permission registration
2. Adding permission tests to CI/CD
3. Creating permission audit scripts
4. Documenting all custom routes that need explicit permissions

---

**Status**: ✅ Fixed
**Date**: 2025-11-03
**Issue**: Terminal permissions for member role
**Solution**: Added explicit Casbin policies for all terminal custom routes
