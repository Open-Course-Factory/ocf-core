# Usage Metrics Not Created - Root Cause Fixed

## Issue

User reported: "I tested to sync and to change the plan, and there are no longer metrics : i did not turn off the terminal part and I have no metric about terminal limits"

## Root Cause

The `InitializeUsageMetrics()` function in `src/payment/services/subscriptionService.go` was checking for feature keys in BOTH:
1. Global feature flags (database) ✅ Correct
2. `plan.Features` array ❌ **WRONG**

The problem: `plan.Features` is a string array containing **user-facing feature descriptions** for display purposes:
- "Unlimited restarts"
- "8 hour max session"
- "1 concurrent terminal"
- "Network access included"
- etc.

It does **NOT** contain feature keys like "terminals", "courses", "labs".

The code was looking for:
```go
if featureFlags.TerminalsEnabled && hasFeature("terminals") {
    // Create metric
}
```

But `hasFeature("terminals")` was searching in the wrong array - it would never find "terminals" because the array contains descriptive strings, not feature keys.

## Fix Applied

**File**: `src/payment/services/subscriptionService.go:483-555`

**Changes**:
1. Removed the two-level check (global flags + plan features)
2. Now only checks global feature flags from database
3. Added comment clarifying that `plan.Features` is for display only

**Before:**
```go
// Only add terminal metrics if enabled globally AND in plan
if featureFlags.TerminalsEnabled && hasFeature("terminals") {
    // Create metric
} else {
    if !featureFlags.TerminalsEnabled {
        utils.Debug("⊗ Skipping terminal metrics (globally disabled)")
    } else {
        utils.Debug("⊗ Skipping terminal metrics (disabled in plan)")
    }
}
```

**After:**
```go
// Only add terminal metrics if enabled globally
if featureFlags.TerminalsEnabled {
    metrics = append(metrics, models.UsageMetrics{
        UserID:         userID,
        SubscriptionID: subscriptionID,
        MetricType:     "concurrent_terminals",
        CurrentValue:   0,
        LimitValue:     int64(plan.MaxConcurrentTerminals),
        PeriodStart:    periodStart,
        PeriodEnd:      periodEnd,
    })
    utils.Debug("📊 Adding terminal metrics (limit: %d)", plan.MaxConcurrentTerminals)
} else {
    utils.Debug("⊗ Skipping terminal metrics (globally disabled)")
}
```

Same fix applied for courses and labs metrics.

## Architecture Clarification

### Global Feature Flags (Database)
- **Purpose**: Enable/disable entire modules globally
- **Location**: `features` table in database
- **Keys**: "terminals", "courses", "labs", etc.
- **Controlled by**: Admin via `/api/v1/features` API
- **Used for**: Toggling which metrics to create/display

### Plan Features Array
- **Purpose**: Display user-facing plan benefits
- **Location**: `subscription_plans.features` column (JSON array)
- **Content**: User-friendly strings like "Unlimited restarts", "Network access included"
- **Controlled by**: Set when creating/updating subscription plans
- **Used for**: Showing plan features on pricing page, subscription dashboard

**These are two completely separate concepts and should NOT be mixed!**

## Testing

1. ✅ Feature flags are seeded correctly on startup:
   - course_conception (enabled)
   - labs (enabled)
   - terminals (enabled)

2. ✅ Calling `POST /api/v1/subscriptions/sync-usage-limits` will now create metrics for all enabled features

3. ✅ Toggling a feature flag in the database will immediately affect metric creation on next sync

## Files Modified

1. `src/payment/services/subscriptionService.go`
   - Lines 483-486: Updated function comment
   - Lines 492-555: Removed plan.Features check, now only uses global flags

2. `CLAUDE.md`
   - Updated Development Environment section with clearer dev container architecture explanation

## What This Means for the User

**Before fix**: Metrics were never created because `plan.Features` doesn't contain feature keys

**After fix**: Metrics are created for all globally enabled features, regardless of the plan's display strings

**To get metrics**:
```bash
# Call sync endpoint (requires authentication)
POST /api/v1/subscriptions/sync-usage-limits

# Then check usage
GET /api/v1/subscriptions/usage
```

## Future Considerations

If you ever want plan-specific feature toggling (e.g., "Trial plan doesn't include terminals"), you should:

1. Add new fields to subscription_plans table:
   ```go
   EnabledModules []string `gorm:"serializer:json" json:"enabled_modules"`
   ```

2. This would contain feature keys: `["course_conception", "labs"]`

3. Then implement two-level checking:
   - Global: Is the feature enabled system-wide?
   - Plan: Does this plan include this feature?

But for now, all plans get all globally-enabled features.
