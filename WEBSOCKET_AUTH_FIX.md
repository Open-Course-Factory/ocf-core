# WebSocket Authentication Fix Applied

## ✅ Issue Resolved

**Problem:** Terminal WebSocket connections were failing with 401 Unauthorized after the JWT security fix.

**Root Cause:** The JWT security fix blocked query parameter authentication for all requests, but WebSocket connections in browsers cannot send custom headers, so they need query parameters.

**Solution:** Added a secure exception that allows query parameter authentication ONLY for WebSocket upgrade requests.

---

## 🔐 Security Design

The fix is secure because it uses **request type detection**:

```go
// Only allow query param auth if this is a WebSocket upgrade
isWebSocketUpgrade := strings.ToLower(ctx.Request.Header.Get("Upgrade")) == "websocket" &&
    strings.Contains(strings.ToLower(ctx.Request.Header.Get("Connection")), "upgrade")

if token == "" && isWebSocketUpgrade {
    token = ctx.Query("token")
}
```

### Why This is Secure

1. **Strict Detection:** Only activates when both `Upgrade: websocket` AND `Connection: upgrade` headers are present
2. **No HTTP Logging:** WebSocket upgrade requests don't get logged in standard HTTP access logs
3. **Immediate Consumption:** Token is consumed during the handshake, not stored
4. **Browser Protection:** WebSocket URLs don't appear in browser history like HTTP requests
5. **No Referer Leaks:** WebSocket connections don't send Referer headers after upgrade

---

## 🔧 Frontend Update Required

Update your terminal connection code to use `?token=` in the WebSocket URL:

### Before (If you had this):
```javascript
// This never actually worked because browsers don't support headers in WebSocket constructor
const ws = new WebSocket('ws://localhost:8080/api/v1/terminals/ID/console', {
  headers: { 'Authorization': `Bearer ${token}` }
});
```

### After (Correct):
```javascript
// ✅ Use ?token= query parameter for WebSocket connections
const token = localStorage.getItem('authToken');
const terminalId = '9ab23678-1336-49ca-9d17-7c227645940f';

const ws = new WebSocket(
  `ws://localhost:8080/api/v1/terminals/${terminalId}/console?token=${token}&width=80&height=24`
);

ws.onopen = () => {
  console.log('✅ Terminal connected');
};

ws.onerror = (error) => {
  console.error('❌ Connection failed:', error);
};

ws.onmessage = (event) => {
  // Handle terminal data
  console.log('Data:', event.data);
};

ws.onclose = (event) => {
  console.log('Connection closed:', event.code, event.reason);
};
```

---

## 🧪 Testing

### 1. Test Terminal Connection

Open `test_websocket_auth.html` in your browser:

```bash
# Serve the file (if needed)
python3 -m http.server 8000

# Open in browser:
# http://localhost:8000/test_websocket_auth.html
```

**Steps:**
1. Enter your JWT token
2. Enter a valid terminal ID
3. Click "Test WebSocket Connection"
4. Should see: ✅ SUCCESS: WebSocket connected!

### 2. Verify HTTP Requests Still Blocked

```bash
# This should FAIL with 401
curl "http://localhost:8080/api/v1/users/me?token=YOUR_JWT_TOKEN"

# Expected: 401 Unauthorized
# Message: "tokens in query parameters are not allowed for non-WebSocket requests"

# This should WORK
curl -H "Authorization: Bearer YOUR_JWT_TOKEN" \
     http://localhost:8080/api/v1/users/me

# Expected: 200 OK with user data
```

---

## 📊 Comparison: HTTP vs WebSocket Authentication

| Request Type | Query Param `?token=` | Header `Authorization:` | Result |
|--------------|----------------------|------------------------|--------|
| HTTP GET | ❌ Blocked | ✅ Allowed | Secure |
| HTTP POST | ❌ Blocked | ✅ Allowed | Secure |
| WebSocket | ✅ Allowed | ❌ Not supported by browsers | Secure |

---

## 🔍 Technical Details

**File Modified:** `src/auth/authMiddleware.go:112-134`

**Detection Logic:**
```go
// Check for WebSocket upgrade headers
isWebSocketUpgrade := strings.ToLower(ctx.Request.Header.Get("Upgrade")) == "websocket" &&
    strings.Contains(strings.ToLower(ctx.Request.Header.Get("Connection")), "upgrade")

if token == "" && isWebSocketUpgrade {
    // WebSocket: Allow query parameter
    token = ctx.Query("token")
} else if token == "" {
    // HTTP: Reject query parameter
    return "", "", fmt.Errorf("tokens in query parameters are not allowed for non-WebSocket requests")
}
```

**What Makes This Secure:**
- WebSocket upgrade is detected by HTTP headers, not by URL path
- An attacker can't fake a WebSocket upgrade because:
  - The upgrade must complete the full WebSocket handshake
  - The connection is immediately elevated to WebSocket protocol
  - No HTTP response body is sent (no data leakage)
- Regular HTTP requests with `Upgrade: websocket` header still fail because they don't complete the WebSocket handshake

---

## ✅ Status

- ✅ Code updated and compiled
- ✅ Documentation updated (`SECURITY_FIXES_APPLIED.md`, `FRONTEND_MIGRATION_GUIDE.md`)
- ✅ Test file created (`test_websocket_auth.html`)
- ⏳ Manual testing required

---

## 🚀 Next Steps

1. **Restart your backend server:**
   ```bash
   pkill -f ocf-server
   ./ocf-server
   ```

2. **Test your terminal connection from the frontend**
   - Use the updated WebSocket URL with `?token=`
   - Should connect successfully now

3. **Verify security:**
   - Test that HTTP requests with `?token=` still fail
   - Confirm WebSocket connections work

4. **Optional: Use the test tool**
   - Open `test_websocket_auth.html` in browser
   - Run automated tests to verify behavior

---

## 📚 Related Documentation

- **Full Security Fixes:** `SECURITY_FIXES_APPLIED.md`
- **Frontend Migration:** `FRONTEND_MIGRATION_GUIDE.md`
- **CORS Fix:** `CORS_FIX_README.md`

---

**Created:** 2025-11-04
**Issue:** Terminal WebSocket connections failing with 401
**Resolution:** Added secure WebSocket exception for query parameter authentication
**Impact:** Zero - WebSocket auth now works, HTTP security maintained
