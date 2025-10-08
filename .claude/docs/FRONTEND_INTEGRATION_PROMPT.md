# Frontend Integration Prompt - Feature Flags System

## Context

The backend now has a **modular feature flag system** that allows enabling/disabling features (courses, labs, terminals) globally. This affects what users see in their subscription dashboard and what features are available.

## API Endpoints

### Get All Feature Flags
```http
GET /api/v1/features
```

**Response:**
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "key": "course_conception",
    "name": "Course Generation",
    "description": "Enable/disable course generation and management features including Marp and Slidev engines",
    "enabled": true,
    "category": "modules",
    "module": "courses",
    "created_at": "2025-01-10T12:00:00Z",
    "updated_at": "2025-01-10T12:00:00Z"
  },
  {
    "id": "660e8400-e29b-41d4-a716-446655440000",
    "key": "labs",
    "name": "Lab Sessions",
    "description": "Enable/disable lab environment and session management",
    "enabled": true,
    "category": "modules",
    "module": "labs",
    "created_at": "2025-01-10T12:00:00Z",
    "updated_at": "2025-01-10T12:00:00Z"
  },
  {
    "id": "770e8400-e29b-41d4-a716-446655440000",
    "key": "terminals",
    "name": "Terminal Trainer",
    "description": "Enable/disable interactive terminal training sessions",
    "enabled": true,
    "category": "modules",
    "module": "terminals",
    "created_at": "2025-01-10T12:00:00Z",
    "updated_at": "2025-01-10T12:00:00Z"
  }
]
```

### Get Single Feature
```http
GET /api/v1/features/{id}
```

### Update Feature (Toggle Enable/Disable)
```http
PATCH /api/v1/features/{id}
Content-Type: application/json

{
  "enabled": false
}
```

**Response:** Same as GET single feature

### Sync User Metrics (After Toggling)
```http
POST /api/v1/subscriptions/sync-usage-limits
```

This removes/creates usage metrics based on new feature states.

## User-Facing Frontend Changes

### 1. Subscription Dashboard - Hide Disabled Feature Limits

**Current behavior:** Shows all limits (courses, labs, terminals) regardless of global feature state

**Required change:** Only show limits for **enabled** features

**Implementation:**

```javascript
// Example: Vue/React component
async function loadSubscriptionLimits() {
  // 1. Fetch user's subscription
  const subscription = await fetch('/api/v1/subscriptions/current')
    .then(r => r.json())

  // 2. Fetch enabled features
  const features = await fetch('/api/v1/features')
    .then(r => r.json())

  // 3. Create a map of enabled features
  const enabledFeatures = features
    .filter(f => f.enabled)
    .reduce((acc, f) => {
      acc[f.key] = true
      return acc
    }, {})

  // 4. Filter subscription limits based on enabled features
  const visibleLimits = []

  if (enabledFeatures['course_conception']) {
    visibleLimits.push({
      name: 'Courses',
      current: subscription.courses_used,
      limit: subscription.max_courses,
      icon: '📚'
    })
  }

  if (enabledFeatures['labs']) {
    visibleLimits.push({
      name: 'Lab Sessions',
      current: subscription.labs_used,
      limit: subscription.max_lab_sessions,
      icon: '🧪'
    })
  }

  if (enabledFeatures['terminals']) {
    visibleLimits.push({
      name: 'Concurrent Terminals',
      current: subscription.terminals_active,
      limit: subscription.max_concurrent_terminals,
      icon: '💻'
    })
  }

  return visibleLimits
}
```

**UI Example:**

Before (all features shown):
```
Your Subscription - Pro Plan
─────────────────────────────
📚 Courses: 3 / 10
🧪 Lab Sessions: 5 / 20
💻 Terminals: 2 / 5
```

After (only enabled features shown, e.g., courses disabled):
```
Your Subscription - Pro Plan
─────────────────────────────
🧪 Lab Sessions: 5 / 20
💻 Terminals: 2 / 5
```

### 2. Navigation Menu - Hide Disabled Features

**Required change:** Hide menu items for disabled features

```javascript
async function buildNavigationMenu() {
  const features = await fetch('/api/v1/features').then(r => r.json())

  const enabledFeatures = features
    .filter(f => f.enabled)
    .reduce((acc, f) => ({ ...acc, [f.key]: true }), {})

  const menuItems = []

  if (enabledFeatures['course_conception']) {
    menuItems.push({
      label: 'Courses',
      icon: '📚',
      path: '/courses',
      children: [
        { label: 'My Courses', path: '/courses/mine' },
        { label: 'Create Course', path: '/courses/create' }
      ]
    })
  }

  if (enabledFeatures['labs']) {
    menuItems.push({
      label: 'Labs',
      icon: '🧪',
      path: '/labs'
    })
  }

  if (enabledFeatures['terminals']) {
    menuItems.push({
      label: 'Terminals',
      icon: '💻',
      path: '/terminals'
    })
  }

  return menuItems
}
```

### 3. Feature Availability Check (for Deep Links)

**Problem:** Users might have bookmarked disabled features

**Solution:** Check feature availability before rendering page

```javascript
// Example: Vue Router Guard
router.beforeEach(async (to, from, next) => {
  // Check if route requires a feature
  const requiredFeature = to.meta.requiredFeature // e.g., 'course_conception'

  if (requiredFeature) {
    const features = await fetch('/api/v1/features').then(r => r.json())
    const feature = features.find(f => f.key === requiredFeature)

    if (!feature || !feature.enabled) {
      // Feature disabled, redirect to home with message
      next({
        path: '/',
        query: {
          message: `Feature "${feature?.name || requiredFeature}" is currently unavailable`
        }
      })
      return
    }
  }

  next()
})
```

### 4. Caching Strategy

**Recommended:** Cache features in localStorage/sessionStorage with TTL

```javascript
class FeatureCache {
  static CACHE_KEY = 'feature_flags'
  static CACHE_TTL = 5 * 60 * 1000 // 5 minutes

  static async getFeatures() {
    const cached = localStorage.getItem(this.CACHE_KEY)

    if (cached) {
      const { features, timestamp } = JSON.parse(cached)
      const age = Date.now() - timestamp

      if (age < this.CACHE_TTL) {
        return features
      }
    }

    // Cache miss or expired, fetch fresh
    const features = await fetch('/api/v1/features').then(r => r.json())

    localStorage.setItem(this.CACHE_KEY, JSON.stringify({
      features,
      timestamp: Date.now()
    }))

    return features
  }

  static clearCache() {
    localStorage.removeItem(this.CACHE_KEY)
  }

  static isFeatureEnabled(key) {
    const cached = localStorage.getItem(this.CACHE_KEY)
    if (!cached) return null

    const { features } = JSON.parse(cached)
    const feature = features.find(f => f.key === key)
    return feature?.enabled ?? null
  }
}
```

### 5. Real-Time Updates (Optional)

**For admin dashboard:** Notify users when features are toggled

```javascript
// Using WebSocket or SSE
const eventSource = new EventSource('/api/v1/events')

eventSource.addEventListener('feature_updated', (event) => {
  const { key, enabled } = JSON.parse(event.data)

  // Clear cache and reload features
  FeatureCache.clearCache()

  // Show notification
  showNotification(
    `Feature "${key}" has been ${enabled ? 'enabled' : 'disabled'}.
     Please refresh the page to see changes.`
  )
})
```

## Admin Section Changes

### 1. Feature Management Page

**Create new page:** `/admin/features`

**Layout:**

```
┌─────────────────────────────────────────────────────────────┐
│  Feature Management                            [Sync Metrics]│
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  Courses Module                                               │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 📚 Course Generation                          [ON/OFF] │  │
│  │ Enable/disable course generation features              │  │
│  │ Module: courses | Category: modules                    │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                               │
│  Labs Module                                                  │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 🧪 Lab Sessions                               [ON/OFF] │  │
│  │ Enable/disable lab environment features                │  │
│  │ Module: labs | Category: modules                       │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                               │
│  Terminals Module                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 💻 Terminal Trainer                           [ON/OFF] │  │
│  │ Enable/disable terminal training sessions              │  │
│  │ Module: terminals | Category: modules                  │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

**Implementation (React example):**

```jsx
import { useState, useEffect } from 'react'

function FeatureManagementPage() {
  const [features, setFeatures] = useState([])
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)

  useEffect(() => {
    loadFeatures()
  }, [])

  async function loadFeatures() {
    setLoading(true)
    try {
      const data = await fetch('/api/v1/features').then(r => r.json())
      setFeatures(data)
    } catch (error) {
      console.error('Failed to load features:', error)
    } finally {
      setLoading(false)
    }
  }

  async function toggleFeature(featureId, currentState) {
    // Confirm before disabling
    if (currentState) {
      const confirmed = confirm(
        'Disabling this feature will hide it from all users. ' +
        'Their metrics will be removed. Continue?'
      )
      if (!confirmed) return
    }

    try {
      await fetch(`/api/v1/features/${featureId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !currentState })
      })

      // Update local state
      setFeatures(prev => prev.map(f =>
        f.id === featureId ? { ...f, enabled: !currentState } : f
      ))

      // Show success notification
      showNotification('Feature updated successfully')
    } catch (error) {
      showNotification('Failed to update feature', 'error')
    }
  }

  async function syncMetrics() {
    setSyncing(true)
    try {
      await fetch('/api/v1/subscriptions/sync-usage-limits', {
        method: 'POST'
      })
      showNotification('Metrics synced successfully')
    } catch (error) {
      showNotification('Failed to sync metrics', 'error')
    } finally {
      setSyncing(false)
    }
  }

  // Group features by module
  const featuresByModule = features.reduce((acc, feature) => {
    const module = feature.module || 'other'
    if (!acc[module]) acc[module] = []
    acc[module].push(feature)
    return acc
  }, {})

  return (
    <div className="feature-management">
      <header>
        <h1>Feature Management</h1>
        <button onClick={syncMetrics} disabled={syncing}>
          {syncing ? 'Syncing...' : 'Sync User Metrics'}
        </button>
      </header>

      {loading ? (
        <div>Loading features...</div>
      ) : (
        Object.entries(featuresByModule).map(([module, moduleFeatures]) => (
          <section key={module} className="module-section">
            <h2>{module.charAt(0).toUpperCase() + module.slice(1)} Module</h2>
            {moduleFeatures.map(feature => (
              <div key={feature.id} className="feature-card">
                <div className="feature-info">
                  <h3>{feature.name}</h3>
                  <p>{feature.description}</p>
                  <div className="feature-meta">
                    <span>Module: {feature.module}</span>
                    <span>Category: {feature.category}</span>
                    <span>Key: {feature.key}</span>
                  </div>
                </div>
                <div className="feature-toggle">
                  <label className="switch">
                    <input
                      type="checkbox"
                      checked={feature.enabled}
                      onChange={() => toggleFeature(feature.id, feature.enabled)}
                    />
                    <span className="slider"></span>
                  </label>
                  <span className={feature.enabled ? 'status-on' : 'status-off'}>
                    {feature.enabled ? 'ON' : 'OFF'}
                  </span>
                </div>
              </div>
            ))}
          </section>
        ))
      )}
    </div>
  )
}
```

### 2. Feature Impact Dashboard

**Show what happens when toggling:**

```jsx
function FeatureImpactCard({ feature, affectedUsers }) {
  return (
    <div className="impact-card">
      <h4>Impact Analysis: {feature.name}</h4>
      <div className="impact-stats">
        <div className="stat">
          <span className="label">Affected Users:</span>
          <span className="value">{affectedUsers.total}</span>
        </div>
        <div className="stat">
          <span className="label">Active Usage:</span>
          <span className="value">{affectedUsers.activelyUsing}</span>
        </div>
        <div className="stat">
          <span className="label">Subscription Plans:</span>
          <span className="value">{affectedUsers.plans.length}</span>
        </div>
      </div>
      <div className="warning">
        ⚠️ Disabling this feature will:
        <ul>
          <li>Hide {feature.name} from navigation menu</li>
          <li>Remove {feature.name} limits from subscription dashboard</li>
          <li>Prevent new {feature.name} usage metrics creation</li>
          <li>Require metrics sync to take effect for existing users</li>
        </ul>
      </div>
    </div>
  )
}
```

### 3. Audit Log

**Track feature toggle history:**

```jsx
function FeatureAuditLog({ featureKey }) {
  const [logs, setLogs] = useState([])

  useEffect(() => {
    // Fetch audit logs (you may need to create this endpoint)
    fetch(`/api/v1/features/${featureKey}/audit-log`)
      .then(r => r.json())
      .then(setLogs)
  }, [featureKey])

  return (
    <div className="audit-log">
      <h3>Change History</h3>
      <table>
        <thead>
          <tr>
            <th>Date</th>
            <th>Action</th>
            <th>Changed By</th>
            <th>Old Value</th>
            <th>New Value</th>
          </tr>
        </thead>
        <tbody>
          {logs.map(log => (
            <tr key={log.id}>
              <td>{new Date(log.timestamp).toLocaleString()}</td>
              <td>{log.action}</td>
              <td>{log.user_email}</td>
              <td>{log.old_value ? 'ON' : 'OFF'}</td>
              <td>{log.new_value ? 'ON' : 'OFF'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
```

## Testing Checklist

### User Frontend
- [ ] Subscription dashboard only shows enabled features
- [ ] Navigation menu hides disabled features
- [ ] Bookmarked links to disabled features redirect with message
- [ ] Feature cache works (no excessive API calls)
- [ ] UI updates immediately after admin toggles feature (with cache clear)

### Admin Frontend
- [ ] Can view all features grouped by module
- [ ] Can toggle features ON/OFF
- [ ] Confirmation dialog before disabling
- [ ] "Sync Metrics" button works
- [ ] Success/error notifications appear
- [ ] UI shows loading states properly

## CSS Styling Suggestions

```css
/* Feature toggle switch */
.switch {
  position: relative;
  display: inline-block;
  width: 60px;
  height: 34px;
}

.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: #ccc;
  transition: 0.4s;
  border-radius: 34px;
}

.slider:before {
  position: absolute;
  content: "";
  height: 26px;
  width: 26px;
  left: 4px;
  bottom: 4px;
  background-color: white;
  transition: 0.4s;
  border-radius: 50%;
}

input:checked + .slider {
  background-color: #2196F3;
}

input:checked + .slider:before {
  transform: translateX(26px);
}

/* Feature card */
.feature-card {
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.feature-card:hover {
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}

.status-on {
  color: #4caf50;
  font-weight: bold;
}

.status-off {
  color: #f44336;
  font-weight: bold;
}
```

## Summary

**User Frontend:**
- Fetch features on load → Cache → Filter UI based on enabled features
- Hide navigation items + subscription limits for disabled features

**Admin Frontend:**
- Show all features grouped by module
- Toggle switch to enable/disable
- Sync metrics button after changes
- Show impact/warnings before disabling

**Key API calls:**
1. `GET /api/v1/features` - Get all features
2. `PATCH /api/v1/features/{id}` - Toggle feature
3. `POST /api/v1/subscriptions/sync-usage-limits` - Sync metrics after toggle
