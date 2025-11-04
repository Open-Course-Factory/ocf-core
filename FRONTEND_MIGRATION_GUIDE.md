# Frontend Migration Guide - Security Updates

**Date:** 2025-11-04
**Priority:** HIGH - Breaking Changes
**Affected:** WebSocket connections, CORS configuration

---

## 🚨 Breaking Changes

### 1. WebSocket Authentication (RESOLVED ✅)

**Status:** ✅ **FIXED** - WebSocket connections now work with query parameter authentication

#### What Changed
JWT tokens are no longer accepted in URL query parameters for regular HTTP requests. However, **WebSocket connections have a special exception** that allows query parameter authentication because browsers don't support custom headers in WebSocket connections.

#### Why This Approach is Secure
- **WebSocket Detection:** Query parameter auth only works when the `Upgrade: websocket` header is present
- **No Logging:** WebSocket upgrade requests bypass standard HTTP logging
- **Immediate Consumption:** Token is consumed immediately during upgrade
- **Not in History:** Browser history only shows initial page, not WebSocket connections

#### Recommended Approach (WORKS ✅)
```javascript
// ✅ SECURE - Token in query parameter (WebSocket only)
const token = localStorage.getItem('authToken');
const terminalId = '9ab23678-1336-49ca-9d17-7c227645940f';

const ws = new WebSocket(
  `ws://localhost:8080/api/v1/terminals/${terminalId}/console?token=${token}&width=80&height=24`
);

ws.onopen = () => {
  console.log('✅ Terminal WebSocket connected securely');
};

ws.onerror = (error) => {
  console.error('❌ WebSocket connection failed:', error);
  // Show user-friendly error message
};

ws.onclose = (event) => {
  console.log('Terminal connection closed:', event.code, event.reason);
};
```

#### Alternative Approach (If your WebSocket library supports headers)
```javascript
// Note: Most browser WebSocket implementations don't support this
const token = localStorage.getItem('authToken');

const ws = new WebSocket('ws://localhost:8080/api/v1/terminals/ID/console', {
  headers: {
    'Authorization': `Bearer ${token}`
  }
});
```

**Important:** The standard browser WebSocket API doesn't actually support custom headers. The query parameter approach above is the correct solution.

#### Testing the Fix

**1. Test Authentication Failure:**
```javascript
// Should fail with 401
const ws = new WebSocket('ws://localhost:8080/api/v1/ssh');

ws.onerror = (error) => {
  console.log('✅ Correctly rejected - no auth token');
};
```

**2. Test Successful Connection:**
```javascript
const token = '<your-valid-token>';
const ws = new WebSocket('ws://localhost:8080/api/v1/ssh', {
  headers: { 'Authorization': `Bearer ${token}` }
});

ws.onopen = () => {
  console.log('✅ WebSocket authenticated successfully!');
};
```

#### Files to Update
Search your frontend codebase for:
- `new WebSocket(` with query parameters
- `?Authorization=` in WebSocket URLs
- Any WebSocket connection to `/api/v1/ssh`

**Common locations:**
- SSH terminal components
- Real-time notification handlers
- Live collaboration features
- Any WebSocket service files

---

### 2. CORS Configuration (Non-Breaking for Localhost)

**Status:** ⚠️ **PRODUCTION BREAKING** - Will block requests from non-whitelisted domains

#### What Changed
The API now only accepts requests from whitelisted domains instead of allowing all origins (`*`).

#### Development Environment (Localhost)
**No changes needed!** The following origins are automatically allowed in development:
- `http://localhost:3000`
- `http://localhost:5173`
- `http://localhost:8080`

#### Production Environment
**Action Required:** Ensure your production frontend URL is whitelisted.

**Backend Environment Variables (Already Configured):**
```bash
FRONTEND_URL=https://app.yourdomain.com
ADMIN_FRONTEND_URL=https://admin.yourdomain.com
ENVIRONMENT=production
```

#### Testing CORS

**1. Verify Localhost Works:**
```bash
# Should succeed (200 OK)
curl -H "Origin: http://localhost:3000" \
     -H "Access-Control-Request-Method: POST" \
     -X OPTIONS http://localhost:8080/api/v1/users
```

**2. Verify Unauthorized Domain is Blocked:**
```bash
# Should be rejected (no CORS headers)
curl -H "Origin: https://evil.com" \
     -H "Access-Control-Request-Method: POST" \
     -X OPTIONS http://localhost:8080/api/v1/users
```

**Frontend Test:**
```javascript
// This should work from localhost:3000
fetch('http://localhost:8080/api/v1/version')
  .then(r => r.json())
  .then(data => console.log('✅ CORS working:', data))
  .catch(err => console.error('❌ CORS blocked:', err));
```

#### Potential Issues

**Symptom:** API calls fail with CORS errors in production
**Cause:** Frontend domain not whitelisted
**Solution:** Contact backend team to add your domain to `FRONTEND_URL` env var

**Browser Console Error:**
```
Access to fetch at 'https://api.example.com/...' from origin 'https://yourapp.com'
has been blocked by CORS policy: No 'Access-Control-Allow-Origin' header present.
```

**Fix:** Add your domain to backend environment variables.

---

## 🎨 User Experience Improvements

### Better Error Messages

Update your error handling to show user-friendly messages:

```javascript
// WebSocket connection with user feedback
const connectToSSH = (token) => {
  const ws = new WebSocket('ws://localhost:8080/api/v1/ssh', {
    headers: { 'Authorization': `Bearer ${token}` }
  });

  ws.onerror = (error) => {
    // Show user-friendly message
    showNotification({
      type: 'error',
      title: 'Connection Failed',
      message: 'Unable to connect to terminal. Please check your authentication and try again.',
      duration: 5000
    });

    console.error('WebSocket error:', error);
  };

  ws.onclose = (event) => {
    if (event.code === 1008) {
      // Policy violation (likely auth failure)
      showNotification({
        type: 'error',
        title: 'Authentication Required',
        message: 'Your session has expired. Please log in again.',
        duration: 5000
      });
    }
  };

  return ws;
};
```

### Retry Logic

Add automatic retry with exponential backoff:

```javascript
class WebSocketManager {
  constructor(url, token, maxRetries = 3) {
    this.url = url;
    this.token = token;
    this.maxRetries = maxRetries;
    this.retryCount = 0;
    this.retryDelay = 1000; // Start with 1 second
  }

  connect() {
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(this.url, {
        headers: { 'Authorization': `Bearer ${this.token}` }
      });

      ws.onopen = () => {
        this.retryCount = 0;
        this.retryDelay = 1000;
        resolve(ws);
      };

      ws.onerror = (error) => {
        if (this.retryCount < this.maxRetries) {
          this.retryCount++;
          console.log(`Retrying connection (${this.retryCount}/${this.maxRetries})...`);

          setTimeout(() => {
            this.connect().then(resolve).catch(reject);
          }, this.retryDelay);

          this.retryDelay *= 2; // Exponential backoff
        } else {
          reject(new Error('WebSocket connection failed after multiple attempts'));
        }
      };
    });
  }
}

// Usage
const wsManager = new WebSocketManager(
  'ws://localhost:8080/api/v1/ssh',
  authToken
);

wsManager.connect()
  .then(ws => console.log('✅ Connected!'))
  .catch(err => console.error('❌ Failed:', err));
```

---

## ✅ Testing Checklist

### Developer Testing

- [ ] **WebSocket Authentication**
  - [ ] Old query parameter method fails with 401
  - [ ] New header method works successfully
  - [ ] Error messages are user-friendly
  - [ ] Reconnection logic works after network interruption

- [ ] **CORS**
  - [ ] API calls work from localhost:3000
  - [ ] API calls work from localhost:5173
  - [ ] Version endpoint accessible without auth
  - [ ] Authenticated endpoints require valid token

- [ ] **User Experience**
  - [ ] Clear error messages when connection fails
  - [ ] Loading states during connection
  - [ ] Graceful reconnection after network issues
  - [ ] No console errors during normal operation

### QA Testing (After Frontend Deployment)

- [ ] SSH terminal opens successfully
- [ ] Terminal remains connected during long sessions
- [ ] Reconnects automatically after brief disconnection
- [ ] Shows appropriate error if authentication expires
- [ ] Works across different browsers (Chrome, Firefox, Safari)
- [ ] Works on mobile devices

---

## 📦 Example Implementation

Complete example with error handling and retry logic:

```javascript
// services/websocket.service.js
export class SecureWebSocketService {
  constructor() {
    this.ws = null;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 5;
    this.reconnectDelay = 1000;
  }

  connect(endpoint, token) {
    return new Promise((resolve, reject) => {
      try {
        // ✅ SECURE: Token in Authorization header
        this.ws = new WebSocket(endpoint, {
          headers: {
            'Authorization': `Bearer ${token}`
          }
        });

        this.ws.onopen = () => {
          console.log('✅ WebSocket connected');
          this.reconnectAttempts = 0;
          this.reconnectDelay = 1000;
          resolve(this.ws);
        };

        this.ws.onerror = (error) => {
          console.error('❌ WebSocket error:', error);
          this.handleError(error, endpoint, token, resolve, reject);
        };

        this.ws.onclose = (event) => {
          console.log(`WebSocket closed: ${event.code} - ${event.reason}`);

          if (event.code === 1008) {
            // Authentication failure - don't retry
            reject(new Error('Authentication failed. Please log in again.'));
          } else if (this.reconnectAttempts < this.maxReconnectAttempts) {
            // Network issue - attempt reconnect
            this.reconnect(endpoint, token);
          }
        };

      } catch (error) {
        reject(error);
      }
    });
  }

  handleError(error, endpoint, token, resolve, reject) {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      console.log(`Retry ${this.reconnectAttempts}/${this.maxReconnectAttempts}`);

      setTimeout(() => {
        this.connect(endpoint, token).then(resolve).catch(reject);
      }, this.reconnectDelay);

      this.reconnectDelay *= 2; // Exponential backoff
    } else {
      reject(new Error('WebSocket connection failed after multiple attempts'));
    }
  }

  disconnect() {
    if (this.ws) {
      this.ws.close(1000, 'Client disconnecting');
      this.ws = null;
    }
  }

  send(data) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    } else {
      console.error('WebSocket not connected');
      throw new Error('WebSocket connection not available');
    }
  }
}

// Usage in React component
import { SecureWebSocketService } from './services/websocket.service';

function SSHTerminal() {
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState(null);
  const wsService = useRef(new SecureWebSocketService());

  useEffect(() => {
    const token = localStorage.getItem('authToken');
    const endpoint = 'ws://localhost:8080/api/v1/ssh';

    wsService.current.connect(endpoint, token)
      .then(ws => {
        setConnected(true);

        ws.onmessage = (event) => {
          // Handle terminal output
          console.log('Terminal output:', event.data);
        };
      })
      .catch(err => {
        setError(err.message);
        setConnected(false);
      });

    return () => {
      wsService.current.disconnect();
    };
  }, []);

  return (
    <div>
      {error && <div className="error">{error}</div>}
      {connected ? (
        <div className="terminal">Terminal connected ✅</div>
      ) : (
        <div className="loading">Connecting to terminal...</div>
      )}
    </div>
  );
}
```

---

## 🆘 Support

If you encounter issues:

1. **Check browser console** for detailed error messages
2. **Verify token** is valid and not expired
3. **Check network tab** to see actual WebSocket request/response
4. **Test with curl** to isolate frontend vs backend issues
5. **Contact backend team** if CORS errors in production

---

## 📚 Additional Resources

- [MDN: WebSocket API](https://developer.mozilla.org/en-US/docs/Web/API/WebSocket)
- [CORS Explained](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS)
- [WebSocket Security Best Practices](https://owasp.org/www-community/vulnerabilities/Insecure_WebSocket)

---

**Document Created:** 2025-11-04
**Last Updated:** 2025-11-04
**Backend Version:** Requires OCF Core with security fixes applied
**Migration Deadline:** Before next production deployment
