# Terminal Hiding Feature - Frontend Implementation Guide

## Overview

The OCF Core backend provides a comprehensive terminal hiding system that allows users to hide inactive terminal sessions from their interface. This guide documents all available API endpoints and implementation patterns for the frontend team.

## Key Concepts

### Hiding vs. Deleting
- **Hiding**: Soft removal from the user's view - terminal remains in database and can be unhidden
- **Deleting**: Permanent removal (different operation, not covered here)

### Who Can Hide Terminals?
1. **Terminal Owner**: Can hide their own terminals (uses `is_hidden_by_owner` flag)
2. **Share Recipients**: Can hide terminals shared with them (uses `is_hidden_by_recipient` flag in TerminalShare)

### Hiding Rules
- **Only inactive terminals can be hidden** (status != "active")
- Active terminals must be stopped/expired before hiding
- Both owner and recipients have independent hiding states

---

## API Endpoints

### 1. Get User Terminal Sessions (with hiding support)

**Endpoint**: `GET /api/v1/terminals/user-sessions`

**Query Parameters**:
- `include_hidden` (boolean, optional): Include hidden terminals in results
  - `false` or omitted: Returns only visible (non-hidden) terminals (default)
  - `true`: Returns all terminals including hidden ones
- `user_id` (string, optional, admin-only): Get sessions for a specific user

**Authentication**: Required (Bearer token)

**Response**: Array of `TerminalOutput` objects

```typescript
interface TerminalOutput {
  id: string;                      // UUID
  session_id: string;              // Terminal Trainer session ID
  user_id: string;                 // Owner's user ID
  name: string;                    // User-friendly name
  status: string;                  // "active", "stopped", "expired", etc.
  expires_at: string;              // ISO 8601 timestamp
  instance_type: string;           // Instance type prefix
  machine_size: string;            // "XS", "S", "M", "L", "XL"
  is_hidden_by_owner: boolean;    // Hidden status (true = hidden)
  hidden_by_owner_at: string | null; // When it was hidden (ISO 8601)
  created_at: string;              // ISO 8601 timestamp
}
```

**Example Requests**:

```javascript
// Get only visible terminals (default behavior)
const response = await fetch('/api/v1/terminals/user-sessions', {
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  }
});
const visibleTerminals = await response.json();

// Get all terminals including hidden ones
const response = await fetch('/api/v1/terminals/user-sessions?include_hidden=true', {
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  }
});
const allTerminals = await response.json();
```

**Frontend Implementation Pattern**:

```javascript
// Component state
const [showHidden, setShowHidden] = useState(false);
const [terminals, setTerminals] = useState([]);

// Fetch terminals based on toggle
const fetchTerminals = async () => {
  const url = showHidden
    ? '/api/v1/terminals/user-sessions?include_hidden=true'
    : '/api/v1/terminals/user-sessions';

  const response = await fetch(url, {
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (response.ok) {
    const data = await response.json();
    setTerminals(data);
  }
};

// UI Toggle
<label>
  <input
    type="checkbox"
    checked={showHidden}
    onChange={(e) => setShowHidden(e.target.checked)}
  />
  Show hidden terminals
</label>
```

---

### 2. Hide a Terminal

**Endpoint**: `POST /api/v1/terminals/{id}/hide`

**Path Parameters**:
- `id` (string, required): Terminal UUID

**Authentication**: Required (Bearer token)

**Business Logic**:
1. **Ownership Check**:
   - If user is the owner → Sets `is_hidden_by_owner = true` on Terminal
   - If user is a share recipient → Sets `is_hidden_by_recipient = true` on TerminalShare
2. **Status Check**: Terminal must NOT be active (status != "active")
3. **Access Check**: User must be owner OR have at least "read" access via share

**Success Response** (200 OK):
```json
{
  "message": "Terminal hidden successfully"
}
```

**Error Responses**:

| Status | Error Message | Reason |
|--------|--------------|--------|
| 400 | "cannot hide active terminals" | Terminal is currently active |
| 403 | "access denied" | User doesn't own or have access to terminal |
| 404 | "terminal not found" | Invalid terminal ID |
| 500 | (various) | Internal server error |

**Example Request**:

```javascript
const hideTerminal = async (terminalId) => {
  const response = await fetch(`/api/v1/terminals/${terminalId}/hide`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (!response.ok) {
    const error = await response.json();

    if (response.status === 400) {
      alert('Cannot hide active terminals. Please stop the terminal first.');
    } else if (response.status === 403) {
      alert('You do not have permission to hide this terminal.');
    } else if (response.status === 404) {
      alert('Terminal not found.');
    } else {
      alert(`Error: ${error.error_message}`);
    }
    return false;
  }

  return true;
};
```

**UI Pattern - Hide Button**:

```javascript
const HideButton = ({ terminal, onHide }) => {
  const [isHiding, setIsHiding] = useState(false);

  const handleHide = async () => {
    // Check if terminal is active
    if (terminal.status === 'active') {
      alert('Cannot hide active terminals. Please stop the terminal first.');
      return;
    }

    setIsHiding(true);
    const success = await hideTerminal(terminal.id);
    setIsHiding(false);

    if (success) {
      onHide(); // Refresh terminal list
    }
  };

  return (
    <button
      onClick={handleHide}
      disabled={isHiding || terminal.status === 'active'}
      title={terminal.status === 'active' ? 'Stop the terminal before hiding' : 'Hide terminal'}
    >
      {isHiding ? 'Hiding...' : 'Hide'}
    </button>
  );
};
```

---

### 3. Unhide a Terminal

**Endpoint**: `DELETE /api/v1/terminals/{id}/hide`

**Path Parameters**:
- `id` (string, required): Terminal UUID

**Authentication**: Required (Bearer token)

**Business Logic**:
1. **Ownership Check**:
   - If user is the owner → Sets `is_hidden_by_owner = false` on Terminal
   - If user is a share recipient → Sets `is_hidden_by_recipient = false` on TerminalShare
2. **Access Check**: User must be owner OR have at least "read" access via share

**Success Response** (200 OK):
```json
{
  "message": "Terminal unhidden successfully"
}
```

**Error Responses**:

| Status | Error Message | Reason |
|--------|--------------|--------|
| 403 | "access denied" | User doesn't own or have access to terminal |
| 404 | "terminal not found" | Invalid terminal ID |
| 500 | (various) | Internal server error |

**Example Request**:

```javascript
const unhideTerminal = async (terminalId) => {
  const response = await fetch(`/api/v1/terminals/${terminalId}/hide`, {
    method: 'DELETE',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (!response.ok) {
    const error = await response.json();
    alert(`Error: ${error.error_message}`);
    return false;
  }

  return true;
};
```

**UI Pattern - Unhide Button**:

```javascript
const UnhideButton = ({ terminal, onUnhide }) => {
  const [isUnhiding, setIsUnhiding] = useState(false);

  const handleUnhide = async () => {
    setIsUnhiding(true);
    const success = await unhideTerminal(terminal.id);
    setIsUnhiding(false);

    if (success) {
      onUnhide(); // Refresh terminal list
    }
  };

  return (
    <button
      onClick={handleUnhide}
      disabled={isUnhiding}
    >
      {isUnhiding ? 'Unhiding...' : 'Unhide'}
    </button>
  );
};
```

---

### 4. Fix Terminal Hide Permissions (Admin/User Utility)

**Endpoint**: `POST /api/v1/terminals/fix-hide-permissions`

**Query Parameters**:
- `user_id` (string, optional, admin-only): Fix permissions for a specific user

**Authentication**: Required (Bearer token)

**Purpose**: Automatically adds Casbin permissions for hide/unhide operations to user's owned terminals and shared terminals. Useful for:
- New users who don't have permissions set up yet
- Migration after permission system changes
- Troubleshooting permission issues

**Success Response** (200 OK):
```typescript
interface FixPermissionsResponse {
  user_id: string;
  success: boolean;
  message: string;
  processed_terminals: number;  // Number of owned terminals processed
  processed_shares: number;     // Number of shared terminals processed
  errors: string[];             // Any errors encountered (optional)
}
```

**Example Response**:
```json
{
  "user_id": "user-123",
  "success": true,
  "message": "Permissions fixed successfully",
  "processed_terminals": 5,
  "processed_shares": 3,
  "errors": []
}
```

**Example Request**:

```javascript
// Fix permissions for current user
const fixPermissions = async () => {
  const response = await fetch('/api/v1/terminals/fix-hide-permissions', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (response.ok) {
    const result = await response.json();
    console.log(`Fixed permissions for ${result.processed_terminals} terminals and ${result.processed_shares} shares`);
  }
};

// Admin: Fix permissions for specific user
const fixUserPermissions = async (userId) => {
  const response = await fetch(`/api/v1/terminals/fix-hide-permissions?user_id=${userId}`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (response.ok) {
    const result = await response.json();
    console.log(`Fixed permissions for user ${result.user_id}`);
  }
};
```

**When to Use**:
- On first login (if user has permission issues)
- In admin panel to troubleshoot user issues
- After sharing terminals (permissions are auto-added, but this can fix if something went wrong)

---

## Complete UI Implementation Example

### Terminal List Component with Hiding Support

```typescript
import React, { useState, useEffect } from 'react';

interface Terminal {
  id: string;
  session_id: string;
  name: string;
  status: string;
  is_hidden_by_owner: boolean;
  hidden_by_owner_at: string | null;
  created_at: string;
  expires_at: string;
  machine_size: string;
}

const TerminalListComponent = () => {
  const [terminals, setTerminals] = useState<Terminal[]>([]);
  const [showHidden, setShowHidden] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Fetch terminals based on showHidden state
  const fetchTerminals = async () => {
    setLoading(true);
    setError(null);

    try {
      const url = showHidden
        ? '/api/v1/terminals/user-sessions?include_hidden=true'
        : '/api/v1/terminals/user-sessions';

      const response = await fetch(url, {
        headers: {
          'Authorization': `Bearer ${getToken()}`,
          'Content-Type': 'application/json'
        }
      });

      if (!response.ok) {
        throw new Error('Failed to fetch terminals');
      }

      const data = await response.json();
      setTerminals(data);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  // Fetch on mount and when showHidden changes
  useEffect(() => {
    fetchTerminals();
  }, [showHidden]);

  // Hide terminal
  const handleHide = async (terminal: Terminal) => {
    // Prevent hiding active terminals
    if (terminal.status === 'active') {
      alert('Cannot hide active terminals. Please stop the terminal first.');
      return;
    }

    try {
      const response = await fetch(`/api/v1/terminals/${terminal.id}/hide`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${getToken()}`,
          'Content-Type': 'application/json'
        }
      });

      if (!response.ok) {
        const error = await response.json();

        if (response.status === 400) {
          alert('Cannot hide active terminals.');
        } else if (response.status === 403) {
          alert('You do not have permission to hide this terminal.');
        } else {
          alert(`Error: ${error.error_message}`);
        }
        return;
      }

      // Refresh the list
      fetchTerminals();
    } catch (err) {
      alert(`Error hiding terminal: ${err.message}`);
    }
  };

  // Unhide terminal
  const handleUnhide = async (terminal: Terminal) => {
    try {
      const response = await fetch(`/api/v1/terminals/${terminal.id}/hide`, {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${getToken()}`,
          'Content-Type': 'application/json'
        }
      });

      if (!response.ok) {
        const error = await response.json();
        alert(`Error: ${error.error_message}`);
        return;
      }

      // Refresh the list
      fetchTerminals();
    } catch (err) {
      alert(`Error unhiding terminal: ${err.message}`);
    }
  };

  // Render loading state
  if (loading) {
    return <div>Loading terminals...</div>;
  }

  // Render error state
  if (error) {
    return <div>Error: {error}</div>;
  }

  return (
    <div className="terminal-list">
      <div className="controls">
        <label>
          <input
            type="checkbox"
            checked={showHidden}
            onChange={(e) => setShowHidden(e.target.checked)}
          />
          Show hidden terminals
        </label>
      </div>

      <div className="terminals">
        {terminals.length === 0 ? (
          <p>No terminals found</p>
        ) : (
          terminals.map(terminal => (
            <div
              key={terminal.id}
              className={`terminal-card ${terminal.is_hidden_by_owner ? 'hidden' : ''}`}
            >
              <h3>
                {terminal.name}
                {terminal.is_hidden_by_owner && <span className="badge">Hidden</span>}
              </h3>

              <div className="terminal-info">
                <p>Status: <span className={`status-${terminal.status}`}>{terminal.status}</span></p>
                <p>Size: {terminal.machine_size}</p>
                <p>Created: {new Date(terminal.created_at).toLocaleString()}</p>
                {terminal.hidden_by_owner_at && (
                  <p>Hidden: {new Date(terminal.hidden_by_owner_at).toLocaleString()}</p>
                )}
              </div>

              <div className="terminal-actions">
                {terminal.is_hidden_by_owner ? (
                  <button
                    onClick={() => handleUnhide(terminal)}
                    className="btn-unhide"
                  >
                    Unhide
                  </button>
                ) : (
                  <button
                    onClick={() => handleHide(terminal)}
                    disabled={terminal.status === 'active'}
                    className="btn-hide"
                    title={terminal.status === 'active' ? 'Stop terminal before hiding' : 'Hide terminal'}
                  >
                    Hide
                  </button>
                )}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};

export default TerminalListComponent;
```

---

## UI/UX Best Practices

### 1. Visual Indicators for Hidden Terminals

When `include_hidden=true`, use visual cues to distinguish hidden terminals:

```css
.terminal-card.hidden {
  opacity: 0.6;
  background-color: #f5f5f5;
  border: 1px dashed #ccc;
}

.terminal-card.hidden .badge {
  background-color: #999;
  color: white;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.8em;
}
```

### 2. Disable Hide Button for Active Terminals

```javascript
<button
  onClick={() => handleHide(terminal)}
  disabled={terminal.status === 'active'}
  title={terminal.status === 'active' ? 'Stop terminal before hiding' : 'Hide terminal'}
  className={terminal.status === 'active' ? 'btn-disabled' : 'btn-hide'}
>
  Hide
</button>
```

### 3. Confirmation Before Hiding

```javascript
const handleHideWithConfirmation = async (terminal) => {
  const confirmed = window.confirm(
    `Are you sure you want to hide "${terminal.name}"? ` +
    `You can unhide it later by toggling "Show hidden terminals".`
  );

  if (confirmed) {
    await handleHide(terminal);
  }
};
```

### 4. Bulk Hide/Unhide

```javascript
const handleBulkHide = async (terminalIds) => {
  const promises = terminalIds.map(id =>
    fetch(`/api/v1/terminals/${id}/hide`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      }
    })
  );

  await Promise.all(promises);
  fetchTerminals(); // Refresh list
};
```

### 5. Filter Buttons

Instead of just a checkbox, provide filter options:

```javascript
const [filter, setFilter] = useState('visible'); // 'visible', 'hidden', 'all'

// In URL construction
const getFilterParam = () => {
  if (filter === 'all') return '?include_hidden=true';
  if (filter === 'hidden') return '?include_hidden=true'; // filter client-side
  return '';
};

// UI
<div className="filter-buttons">
  <button
    className={filter === 'visible' ? 'active' : ''}
    onClick={() => setFilter('visible')}
  >
    Visible Only
  </button>
  <button
    className={filter === 'all' ? 'active' : ''}
    onClick={() => setFilter('all')}
  >
    All Terminals
  </button>
  <button
    className={filter === 'hidden' ? 'active' : ''}
    onClick={() => setFilter('hidden')}
  >
    Hidden Only
  </button>
</div>

// Filter client-side for 'hidden' filter
const displayTerminals = filter === 'hidden'
  ? terminals.filter(t => t.is_hidden_by_owner)
  : terminals;
```

---

## Testing Checklist

### Functional Tests

- [ ] Can hide inactive terminal (status != "active")
- [ ] Cannot hide active terminal (status == "active")
- [ ] Can unhide previously hidden terminal
- [ ] Hidden terminals disappear from default list
- [ ] Hidden terminals appear when `include_hidden=true`
- [ ] Hide/Unhide persists across page reloads
- [ ] Owner can hide their own terminals
- [ ] Share recipient can hide shared terminals
- [ ] User cannot hide terminals they don't own/have access to
- [ ] `is_hidden_by_owner` flag updates correctly in response
- [ ] `hidden_by_owner_at` timestamp is set when hiding
- [ ] `hidden_by_owner_at` is null when unhiding

### Edge Cases

- [ ] Hiding already-hidden terminal (should succeed, idempotent)
- [ ] Unhiding already-visible terminal (should succeed, idempotent)
- [ ] Hiding terminal immediately after stopping it
- [ ] Network errors during hide/unhide operations
- [ ] Multiple rapid hide/unhide clicks (debouncing)
- [ ] Permission errors (403) handled gracefully
- [ ] Invalid terminal ID (404) handled gracefully

### UI/UX Tests

- [ ] Loading states during API calls
- [ ] Success feedback after hide/unhide
- [ ] Error messages display correctly
- [ ] Toggle/filter updates list immediately
- [ ] Visual distinction between hidden and visible terminals
- [ ] Button states (enabled/disabled) update correctly
- [ ] Keyboard navigation works
- [ ] Screen reader accessibility

---

## Error Handling Reference

### HTTP Status Codes

| Code | Meaning | Action |
|------|---------|--------|
| 200 | Success | Update UI, show success message |
| 400 | Bad Request | Show error message (e.g., "Cannot hide active terminals") |
| 403 | Forbidden | Show permission error, check user access |
| 404 | Not Found | Show "Terminal not found", refresh list |
| 500 | Server Error | Show generic error, log for debugging |

### Error Response Format

All error responses follow this format:

```json
{
  "error_code": 400,
  "error_message": "cannot hide active terminals"
}
```

---

## Performance Considerations

### 1. Polling vs. Real-time Updates

If terminals update frequently, consider WebSocket updates instead of polling:

```javascript
// Polling (simple, but less efficient)
useEffect(() => {
  const interval = setInterval(() => {
    fetchTerminals();
  }, 30000); // Every 30 seconds

  return () => clearInterval(interval);
}, [showHidden]);

// WebSocket (more efficient, if available)
useEffect(() => {
  const ws = new WebSocket('ws://api/terminals/updates');

  ws.onmessage = (event) => {
    const update = JSON.parse(event.data);
    // Update specific terminal in state
    setTerminals(prev =>
      prev.map(t => t.id === update.id ? { ...t, ...update } : t)
    );
  };

  return () => ws.close();
}, []);
```

### 2. Caching

Cache terminal list to reduce API calls:

```javascript
import { useQuery, useMutation, useQueryClient } from 'react-query';

const useTerminals = (includeHidden: boolean) => {
  return useQuery(
    ['terminals', includeHidden],
    () => fetchTerminals(includeHidden),
    {
      staleTime: 30000, // 30 seconds
      cacheTime: 300000, // 5 minutes
    }
  );
};

const useHideTerminal = () => {
  const queryClient = useQueryClient();

  return useMutation(
    (terminalId: string) => hideTerminal(terminalId),
    {
      onSuccess: () => {
        // Invalidate cache to refetch
        queryClient.invalidateQueries('terminals');
      },
    }
  );
};
```

### 3. Optimistic Updates

Update UI immediately before API response:

```javascript
const handleHide = async (terminal) => {
  // Optimistically update UI
  setTerminals(prev =>
    prev.map(t =>
      t.id === terminal.id
        ? { ...t, is_hidden_by_owner: true, hidden_by_owner_at: new Date().toISOString() }
        : t
    )
  );

  try {
    await hideTerminal(terminal.id);
    // Success - optimistic update was correct
  } catch (error) {
    // Rollback optimistic update
    setTerminals(prev =>
      prev.map(t =>
        t.id === terminal.id
          ? { ...t, is_hidden_by_owner: false, hidden_by_owner_at: null }
          : t
      )
    );
    alert('Failed to hide terminal');
  }
};
```

---

## Security Notes

1. **Authentication Required**: All endpoints require valid Bearer token
2. **Authorization Checks**:
   - Users can only hide terminals they own or have access to
   - Casbin enforces permissions at the API level
3. **Input Validation**: Terminal IDs must be valid UUIDs
4. **HTTPS Only**: Always use HTTPS in production for token security

---

## Additional Resources

- **Swagger Documentation**: `http://localhost:8080/swagger/` (development)
- **Backend Code Reference**:
  - Controller: `src/terminalTrainer/routes/terminalController.go`
  - Service: `src/terminalTrainer/services/terminalTrainerService.go`
  - Repository: `src/terminalTrainer/repositories/terminalRepository.go`
  - Models: `src/terminalTrainer/models/terminal.go`, `terminalShare.go`
  - DTOs: `src/terminalTrainer/dto/terminalDto.go`

---

## Support

For questions or issues:
1. Check the Swagger documentation first
2. Review this guide for implementation patterns
3. Test endpoints using Postman/Insomnia with sample requests
4. Contact backend team with specific error messages and request details
