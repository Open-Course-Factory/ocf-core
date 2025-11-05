# Frontend Investigation: Terminal Menu Not Displaying

## Issue
After fixing backend permissions, terminal menus are still not visible in the frontend for users with "member" or "student" roles.

## Backend Changes Completed ✅

### 1. Permission System Fixed
- **Entity Registration**: Terminal permissions are correctly registered for "member" role
- **Casbin Policies**: Confirmed in database:
  ```
  member    | /api/v1/terminals    | (GET|POST)
  member    | /api/v1/terminals/*  | (GET|POST)
  student   | /api/v1/terminals    | (GET|POST)
  student   | /api/v1/terminals/*  | (GET|POST)
  ```
- **Deprecated Role Removed**: All `member_pro` references removed from backend

### 2. Middleware Issue Fixed
- Removed global `EnsureSubscriptionRole()` middleware that was running before authentication
- This middleware was preventing role assignment because `userId` was empty

### 3. Database Cleanup
- Deleted all `member_pro` policies from Casbin
- Updated subscription plans to use `member` instead of `member_pro`

## Frontend Investigation Checklist

### 1. Check Permission/Role Checks in Menu Components

**Look for:**
- Hardcoded role checks for `member_pro` (should use `member` instead)
- Permission checks that might be looking for deprecated roles
- Feature flags that might be gating terminal access

**Files to check:**
```typescript
// Common patterns to search for:
- "member_pro"
- "role === 'student'"  // Should also check role === 'member'
- hasPermission("terminal")
- canAccessTerminals
- features.includes("terminals")
```

### 2. Verify User Permissions Endpoint Response

**Test the permissions endpoint:**
```bash
# Get a user token and check what permissions they receive
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/auth/permissions
```

**Expected response should include:**
```json
{
  "user_id": "...",
  "roles": ["student", "member"],  // or just ["member"]
  "permissions": [
    {
      "resource": "/api/v1/terminals",
      "methods": ["GET", "POST"]
    },
    {
      "resource": "/api/v1/terminals/*",
      "methods": ["GET", "POST"]
    }
  ],
  "organization_memberships": [...],
  "can_create_organization": true,
  "can_create_group": true
}
```

### 3. Check for Subscription/Feature Requirements

**The frontend might be checking:**
- `hasAnySubscription` - This should be `false` for free tier users, but terminals should still be accessible
- `aggregated_features` - Check if terminal access is gated behind a feature flag
- Organization membership - Check if frontend requires user to be in an organization

**Verify:**
```typescript
// Frontend should allow terminal access if ANY of these are true:
// 1. User has "member" role (even without subscription)
// 2. User has "student" role (legacy support)
// 3. User is in ANY organization (regardless of subscription)
```

### 4. Common Frontend Permission Patterns to Update

**Pattern 1: Role-based menu display**
```typescript
// ❌ OLD (broken):
if (user.roles.includes('member_pro') || user.roles.includes('premium_student')) {
  showTerminalMenu = true;
}

// ✅ NEW (correct):
if (user.roles.includes('member') || user.roles.includes('student')) {
  showTerminalMenu = true;
}
```

**Pattern 2: Permission-based menu display**
```typescript
// ✅ BEST APPROACH:
const canAccessTerminals = user.permissions.some(p =>
  p.resource.includes('/terminals') && p.methods.includes('GET')
);
```

**Pattern 3: Feature-based (if using feature flags)**
```typescript
// ❌ DON'T require subscription features for basic terminal access:
if (user.aggregated_features.includes('terminals')) {
  showTerminalMenu = true;
}

// ✅ DO check permissions directly:
if (hasPermission('/api/v1/terminals', 'GET')) {
  showTerminalMenu = true;
}
```

### 5. Check Navigation/Routing Configuration

**Files to investigate:**
- Route definitions (e.g., `routes.tsx`, `navigation.tsx`)
- Menu configuration files
- Sidebar/navigation components
- Protected route wrappers

**Look for:**
```typescript
// Guards that might be preventing access:
{
  path: '/terminals',
  component: TerminalPage,
  guard: requireRole('member_pro')  // ❌ Should be 'member'
}

// Or:
{
  path: '/terminals',
  component: TerminalPage,
  guard: requirePermission('/api/v1/terminals', 'GET')  // ✅ Better approach
}
```

### 6. Verify Token/Session State

**Check that:**
1. User token is valid and not expired
2. Frontend is correctly parsing the JWT token
3. User roles are being extracted correctly from token
4. Session state is being updated after login

**Debug steps:**
```typescript
// Add console logging to verify:
console.log('User roles:', currentUser.roles);
console.log('User permissions:', currentUser.permissions);
console.log('Has terminal access:', canAccessTerminals());
```

### 7. Check for Caching Issues

**Frontend might be caching:**
- User permissions in local storage
- Role information in session storage
- Menu visibility state

**Try:**
1. Clear browser cache/local storage
2. Hard refresh (Ctrl+Shift+R)
3. Check if old `member_pro` role is cached anywhere

### 8. API Integration Points

**Verify these API calls work correctly:**
```bash
# 1. Login and get token
POST /api/v1/auth/login

# 2. Get user permissions (should include terminals)
GET /api/v1/auth/permissions

# 3. Verify terminal access works
GET /api/v1/terminals
POST /api/v1/terminals/start-session
```

## Testing Scenarios

### Test Case 1: New User (Member Role Only)
- User: Newly registered user with only "member" role
- Expected: Should see terminal menu and be able to start sessions

### Test Case 2: Existing User (Student Role)
- User: Legacy user with "student" role
- Expected: Should see terminal menu and be able to start sessions

### Test Case 3: User with Organization Membership
- User: Member of an organization (regardless of subscription)
- Expected: Should see terminal menu and be able to start sessions

## Quick Debug Commands

### Backend Verification
```bash
# 1. Check user roles in database
psql -c "SELECT v0 as user_id, STRING_AGG(v1, ', ') as roles
FROM casbin_rule
WHERE ptype = 'g' AND v0 = 'USER_ID_HERE'
GROUP BY v0;"

# 2. Check terminal permissions for member role
psql -c "SELECT * FROM casbin_rule
WHERE v0 = 'member' AND v1 LIKE '%terminal%';"

# 3. Test API endpoint directly
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/terminals
```

### Frontend Debug Checklist
```javascript
// In browser console:
1. localStorage.clear()  // Clear cached permissions
2. Check: currentUser.roles
3. Check: currentUser.permissions
4. Check: menuItems.filter(m => m.id === 'terminals')
5. Network tab: Verify /api/v1/auth/permissions response
```

## Expected Resolution

After frontend updates, users should see terminal menu if:
1. They have "member" OR "student" role (from JWT token)
2. OR they have explicit permission for `/api/v1/terminals` in permissions array
3. Regardless of subscription status (terminals are available on free tier)

## Contact

If you need backend verification or have questions:
- Backend permissions are confirmed working ✅
- API endpoints are accessible with correct roles ✅
- Issue is frontend permission checking logic or caching

---

**Priority:** HIGH - Blocking user access to terminals
**Backend Status:** ✅ Fixed
**Frontend Status:** ⏳ Investigation needed
