# Usage Endpoint Returning Null - Fixed

## Issue

`GET /api/v1/subscriptions/usage` was returning `null` instead of usage metrics.

## Root Cause

After implementing the modular feature flag system, usage metrics may have been:
1. **Never created** - If user subscribed before feature flags existed
2. **Deleted during migration** - If database was reset or sync was called
3. **Filtered out** - Query filters by `period_end > NOW()`, expired metrics don't show

## Fix Applied

### 1. Return Empty Array Instead of Null

**File**: `src/payment/routes/subscriptionController.go:590-593`

**Before:**
```go
usageMetrics, err := sc.subscriptionService.GetUserUsageMetrics(userId)
// Returns null if no metrics found
ctx.JSON(http.StatusOK, usageMetricsDTO)
```

**After:**
```go
usageMetrics, err := sc.subscriptionService.GetUserUsageMetrics(userId)
// If no metrics found, return empty array
if usageMetrics == nil || len(*usageMetrics) == 0 {
    ctx.JSON(http.StatusOK, []interface{}{})
    return
}
ctx.JSON(http.StatusOK, usageMetricsDTO)
```

**Why**: Frontend expects an array, not null. Empty array is more API-friendly.

## Solution: Sync User Metrics

If a user has no metrics, they need to be initialized. Call the sync endpoint:

```bash
POST /api/v1/subscriptions/sync-usage-limits
Authorization: Bearer <user-token>
```

This will:
1. Get user's active subscription
2. Get subscription plan
3. Check feature flags (database)
4. Delete old metrics
5. Create new metrics for enabled features

## Testing

### Before Sync
```bash
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/subscriptions/usage
```

**Response:**
```json
[]
```

### After Sync
```bash
# 1. Sync metrics
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/subscriptions/sync-usage-limits

# 2. Get usage
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/subscriptions/usage
```

**Response:**
```json
[
  {
    "id": "abc123",
    "user_id": "user-id",
    "metric_type": "concurrent_terminals",
    "current_value": 2,
    "limit_value": 5,
    "period_start": "2025-01-01T00:00:00Z",
    "period_end": "2025-02-01T00:00:00Z"
  },
  {
    "id": "def456",
    "user_id": "user-id",
    "metric_type": "courses_created",
    "current_value": 3,
    "limit_value": 10,
    "period_start": "2025-01-01T00:00:00Z",
    "period_end": "2025-02-01T00:00:00Z"
  }
]
```

## Frontend Handling

### Option 1: Auto-Sync on Empty Response

```javascript
async function getUserUsage() {
  let usage = await fetch('/api/v1/subscriptions/usage')
    .then(r => r.json())

  // If empty, trigger sync and retry
  if (usage.length === 0) {
    await fetch('/api/v1/subscriptions/sync-usage-limits', {
      method: 'POST'
    })

    // Retry
    usage = await fetch('/api/v1/subscriptions/usage')
      .then(r => r.json())
  }

  return usage
}
```

### Option 2: Show Message to User

```javascript
async function getUserUsage() {
  const usage = await fetch('/api/v1/subscriptions/usage')
    .then(r => r.json())

  if (usage.length === 0) {
    return {
      isEmpty: true,
      message: 'Usage metrics not initialized. Please sync your subscription.',
      action: 'sync'
    }
  }

  return { isEmpty: false, data: usage }
}
```

## When Metrics Need Sync

Metrics should be synced when:
1. **User first subscribes** - Automatically done by webhook
2. **Plan changes** - Automatically done by webhook
3. **Feature flags toggled** - Admin must call sync
4. **User has no metrics** - User or admin calls sync
5. **Metrics expired** - Automatic monthly reset (future feature)

## Database Check (for debugging)

```sql
-- Check if user has metrics
SELECT user_id, metric_type, current_value, limit_value,
       period_start, period_end
FROM usage_metrics
WHERE user_id = 'your-user-id'
  AND period_end > NOW();

-- Check if features are enabled
SELECT key, name, enabled, module
FROM features;

-- Check subscription plan features
SELECT sp.name, sp.features
FROM user_subscriptions us
JOIN subscription_plans sp ON us.subscription_plan_id = sp.id
WHERE us.user_id = 'your-user-id'
  AND us.status = 'active';
```

## Prevention

To avoid this in the future:

1. **Automatic initialization** - Metrics are created automatically on:
   - New subscription (webhook: `customer.subscription.created`)
   - Plan upgrade (webhook: `customer.subscription.updated`)

2. **Health check endpoint** - Could add:
   ```
   GET /api/v1/subscriptions/health
   ```
   Returns:
   ```json
   {
     "has_subscription": true,
     "has_metrics": false,
     "needs_sync": true
   }
   ```

3. **Frontend check** - On login, check if metrics exist, auto-sync if missing

## Summary

✅ **Fixed**: Returns `[]` instead of `null` when no metrics
✅ **Solution**: Call `POST /api/v1/subscriptions/sync-usage-limits`
✅ **Prevention**: Metrics auto-created on subscription webhooks
