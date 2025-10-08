# Feature-Based Usage Metrics

## Overview

Usage metrics are now **feature-aware**: only metrics for enabled features will be created and tracked. Features must be enabled at **TWO levels**:

1. **Global Level** (Environment Variables) - Controls application-wide feature availability
2. **Plan Level** (Subscription Plan Features) - Controls per-plan feature access

**Both must be enabled** for a metric to be created.

## How It Works

### 1. Global Feature Flags (Environment Variables)

Set these in your `.env` file to control application-wide feature availability:

```bash
# .env
FEATURE_COURSES_ENABLED=true    # Enable/disable course generation globally
FEATURE_LABS_ENABLED=true       # Enable/disable lab sessions globally
FEATURE_TERMINALS_ENABLED=true  # Enable/disable terminal sessions globally
```

**Accepted values:** `true`, `false`, `1`, `0`, `yes`, `no` (case insensitive)
**Default:** All features enabled if not specified

**Example: Disable courses globally**
```bash
FEATURE_COURSES_ENABLED=false
```
→ **No user** will have course metrics, regardless of their plan

### 2. Subscription Plan Features

Each `SubscriptionPlan` has a `Features` array that defines which features are enabled:

```json
{
  "name": "Pro Plan",
  "features": ["terminals", "courses"],
  "max_concurrent_terminals": 5,
  "max_courses": 10,
  "max_lab_sessions": 0
}
```

### 3. Supported Features

| Feature Key | Metric Type | Description | Global Flag |
|------------|-------------|-------------|-------------|
| `terminals` | `concurrent_terminals` | Terminal sessions | `FEATURE_TERMINALS_ENABLED` |
| `courses` | `courses_created` | Course creation | `FEATURE_COURSES_ENABLED` |
| `labs` | `lab_sessions` | Lab sessions | `FEATURE_LABS_ENABLED` |

### 4. Metric Creation Logic (Two-Level Check)

When a subscription is created or plan is changed:

```go
// Only creates metrics for features in plan.Features array
InitializeUsageMetrics(userID, subscriptionID, planID)
```

**Example 1: Terminals-Only Plan (Global: All Enabled)**
```bash
# .env
FEATURE_TERMINALS_ENABLED=true
FEATURE_COURSES_ENABLED=true
FEATURE_LABS_ENABLED=true
```
```json
{
  "features": ["terminals"],
  "max_concurrent_terminals": 3
}
```
✅ Creates: `concurrent_terminals` metric (global ✓ + plan ✓)
⊗ Skips: `courses_created` (global ✓ but plan ✗)
⊗ Skips: `lab_sessions` (global ✓ but plan ✗)

**Example 2: Courses Globally Disabled**
```bash
# .env
FEATURE_COURSES_ENABLED=false  # ← Courses turned OFF globally
FEATURE_TERMINALS_ENABLED=true
FEATURE_LABS_ENABLED=true
```
```json
{
  "features": ["terminals", "courses", "labs"],
  "max_concurrent_terminals": 10,
  "max_courses": 50,
  "max_lab_sessions": 20
}
```
✅ Creates: `concurrent_terminals` (global ✓ + plan ✓)
⊗ **Skips: `courses_created` (global ✗)** ← No course metrics for ANY user!
✅ Creates: `lab_sessions` (global ✓ + plan ✓)

**Example 3: Plan Excludes Labs**
```bash
# .env - All features enabled globally
FEATURE_COURSES_ENABLED=true
FEATURE_LABS_ENABLED=true
FEATURE_TERMINALS_ENABLED=true
```
```json
{
  "features": ["courses", "terminals"],
  "max_courses": 100,
  "max_concurrent_terminals": 5
}
```
✅ Creates: `courses_created` (global ✓ + plan ✓)
✅ Creates: `concurrent_terminals` (global ✓ + plan ✓)
⊗ Skips: `lab_sessions` (global ✓ but plan ✗)

## Usage

### Creating a New Plan

When creating a subscription plan, specify which features to enable:

```bash
curl -X POST /api/v1/subscription-plans \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Terminal-Only Plan",
    "price_amount": 999,
    "billing_interval": "month",
    "features": ["terminals"],
    "max_concurrent_terminals": 5,
    "max_courses": 0,
    "max_lab_sessions": 0
  }'
```

### Updating a Plan

When updating features in an existing plan, **existing users' metrics are NOT automatically updated**.

To update existing users:
1. Change the plan's `features` array
2. Call the sync endpoint for each affected user:

```bash
POST /api/v1/subscriptions/sync-usage-limits
```

This will:
- Delete old metrics
- Create new metrics based on the updated plan features

## Migration Guide

### For Existing Plans

If you have existing plans without the `features` field populated:

**Option 1: Update via API**
```bash
PATCH /api/v1/subscription-plans/{id}
{
  "features": ["terminals", "courses", "labs"]
}
```

**Option 2: Update via SQL**
```sql
-- Enable all features for all plans
UPDATE subscription_plans
SET features = '["terminals", "courses", "labs"]';

-- Enable only terminals for specific plan
UPDATE subscription_plans
SET features = '["terminals"]'
WHERE name = 'Terminal-Only Plan';
```

### For Existing Users

After updating plan features, sync user metrics:

```bash
# Sync all users with a specific plan
POST /api/v1/subscriptions/sync-usage-limits
{
  "plan_id": "plan-uuid-here"
}
```

## Debugging

### Check Which Metrics Are Created

When `ENVIRONMENT=development`, you'll see debug logs:

```
[DEBUG] 📊 Adding terminal metrics (limit: 5)
[DEBUG] ⊗ Skipping course metrics (feature disabled)
[DEBUG] ⊗ Skipping lab metrics (feature disabled)
[DEBUG] ✅ Initialized 1 usage metrics for user abc123 (subscription: xyz789)
```

### Verify Plan Features

```bash
GET /api/v1/subscription-plans/{id}
```

Response includes:
```json
{
  "features": ["terminals", "courses"],
  ...
}
```

## Best Practices

1. **Always Specify Features**: When creating a plan, explicitly set the `features` array
2. **Match Limits to Features**: If a feature is disabled, set its limit to 0
3. **Sync After Changes**: After modifying plan features, sync existing users' metrics
4. **Use Descriptive Names**: Name plans to reflect their features (e.g., "Terminals Pro", "Full Platform Access")

## Example Plans

### Free Tier (Terminals Only)
```json
{
  "name": "Free",
  "features": ["terminals"],
  "max_concurrent_terminals": 1,
  "max_courses": 0,
  "max_lab_sessions": 0,
  "price_amount": 0
}
```

### Pro (Terminals + Courses)
```json
{
  "name": "Pro",
  "features": ["terminals", "courses"],
  "max_concurrent_terminals": 5,
  "max_courses": 10,
  "max_lab_sessions": 0,
  "price_amount": 1999
}
```

### Enterprise (All Features)
```json
{
  "name": "Enterprise",
  "features": ["terminals", "courses", "labs"],
  "max_concurrent_terminals": -1,
  "max_courses": -1,
  "max_lab_sessions": -1,
  "price_amount": 9999
}
```

## Implementation Details

### Code Location

`src/payment/services/subscriptionService.go:481-567` - `InitializeUsageMetrics()`

### When Metrics Are Created

1. **New Subscription**: When `handleSubscriptionCreated()` webhook processes a new subscription
2. **Plan Change**: When `UpdateUsageMetricLimits()` is called (deletes old, creates new)
3. **Manual Sync**: When admin calls `/sync-usage-limits` endpoint

### Database Schema

No schema changes required - uses existing `Features` field in `subscription_plans` table:

```sql
features jsonb -- Array of feature strings
```

## Quick Reference

### To Disable Courses Globally

**Step 1:** Update `.env`
```bash
FEATURE_COURSES_ENABLED=false
```

**Step 2:** Restart the application
```bash
docker compose restart ocf-core
# or
go run main.go
```

**Step 3:** Sync existing users (removes course metrics)
```bash
POST /api/v1/subscriptions/sync-usage-limits
```

**Result:**
- ✅ Course metrics disappear from subscription dashboard
- ✅ Applies to ALL users regardless of plan
- ✅ Can be re-enabled anytime by setting to `true`

### To Enable Only Terminals (Disable Courses & Labs)

```bash
# .env
FEATURE_TERMINALS_ENABLED=true
FEATURE_COURSES_ENABLED=false
FEATURE_LABS_ENABLED=false
```

Then sync all users.
