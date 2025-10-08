# Subscription Upgrade/Downgrade Implementation

## Summary

The `/subscriptions/upgrade` endpoint has been enhanced to fully integrate with Stripe, enabling users to change their subscription plans with proper proration handling.

## What Was Changed

### Backend Changes

1. **DTO Update** (`src/payment/dto/subscriptionDto.go`):
   - Added `proration_behavior` field to `UpgradePlanInput`
   - Optional field with three valid values: `"always_invoice"`, `"create_prorations"`, `"none"`
   - Defaults to `"always_invoice"` if not provided

2. **Stripe Service** (`src/payment/services/stripeService.go`):
   - Added `UpdateSubscription(subscriptionID, newPriceID, prorationBehavior)` method
   - Handles Stripe subscription updates with proration support
   - Validates proration behavior options

3. **Subscription Service** (`src/payment/services/subscriptionService.go`):
   - Updated `UpgradeUserPlan` signature to accept `prorationBehavior` parameter
   - Enhanced validation to ensure new plan has Stripe price configured

4. **Controller** (`src/payment/routes/subscriptionController.go`):
   - Modified `UpgradeUserPlan` to call Stripe API before updating database
   - Added proper error handling for Stripe integration
   - Updated Swagger documentation

5. **Swagger Documentation**:
   - Regenerated API docs with updated endpoint details
   - Documented proration behavior options

## API Usage

### Endpoint

**POST** `/api/v1/subscriptions/upgrade`

### Request Body

```json
{
  "new_plan_id": "uuid-of-new-plan",
  "proration_behavior": "always_invoice"  // optional
}
```

### Proration Behavior Options

- **`always_invoice`** (default): Immediately creates an invoice for the proration amount (upgrade = charge, downgrade = credit)
- **`create_prorations`**: Records the proration but doesn't invoice immediately (useful for billing cycle alignment)
- **`none`**: No proration applied, new price starts immediately without adjustments

### Example Request

```bash
curl -X POST https://api.example.com/api/v1/subscriptions/upgrade \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "new_plan_id": "123e4567-e89b-12d3-a456-426614174000",
    "proration_behavior": "always_invoice"
  }'
```

### Response (200 OK)

```json
{
  "id": "sub-uuid",
  "user_id": "user-id",
  "subscription_plans": "new-plan-uuid",
  "stripe_subscription_id": "sub_xxxxx",
  "stripe_customer_id": "cus_xxxxx",
  "status": "active",
  "current_period_start": "2025-10-07T00:00:00Z",
  "current_period_end": "2025-11-07T00:00:00Z",
  "cancel_at_period_end": false,
  "created_at": "2025-09-07T00:00:00Z",
  "updated_at": "2025-10-07T20:00:00Z"
}
```

### Error Responses

- **400**: Invalid plan ID format or new plan not configured in Stripe
- **404**: No active subscription or plan not found
- **500**: Stripe update failed or database error

## Flow

1. User sends upgrade request with new plan ID
2. Backend validates user has active subscription
3. Backend validates new plan exists and has Stripe price configured
4. **Stripe subscription is updated first** with proration
5. Database is updated with new plan and usage limits
6. User receives updated subscription details

## Important Notes

- ⚠️ **Breaking Change**: The previous `/subscriptions/upgrade` route only updated the database. It now updates Stripe subscriptions.
- ✅ **Proration**: Upgrades/downgrades properly charge or credit users based on remaining time in billing period
- ✅ **Atomic**: Database updates are transactional, ensuring consistency
- ✅ **Usage Limits**: Automatically updates terminal, course, and lab session limits
- 🔄 **Webhooks**: Stripe webhooks will sync any additional changes back to the database

## Testing Recommendations

1. Test upgrade from lower to higher tier (should create immediate charge)
2. Test downgrade from higher to lower tier (should create credit)
3. Test with different proration behaviors
4. Verify usage limits update correctly
5. Test error cases (invalid plan ID, no Stripe price, etc.)

## Files Modified

- `src/payment/dto/subscriptionDto.go` - Added proration_behavior field
- `src/payment/services/stripeService.go` - Added UpdateSubscription method
- `src/payment/services/subscriptionService.go` - Updated UpgradeUserPlan signature
- `src/payment/routes/subscriptionController.go` - Enhanced upgrade flow with Stripe integration
- `tests/payment/integration/terminalPricing_integration_test.go` - Updated test calls
- `docs/*` - Regenerated Swagger documentation

## Build Status

✅ All packages compile successfully
✅ No compilation errors
✅ Tests updated to use new signature

## Webhook Enhancement (Important!)

The webhook handler (`customer.subscription.updated`) has been enhanced to automatically detect and sync plan changes:

### How It Works

1. When Stripe sends a `customer.subscription.updated` webhook
2. The handler extracts the new Stripe price ID from the subscription items
3. It looks up which plan in your database matches that Stripe price ID
4. If the plan changed, it:
   - Updates the `subscription_plan_id` in the database
   - Automatically updates all usage metric limits for the new plan
   - Logs the plan change for debugging

### What This Means

✅ **Plan changes made via Stripe dashboard** are now automatically synced
✅ **Plan changes via API** trigger webhooks that update usage limits
✅ **Proration invoices** from Stripe are properly handled
✅ **No manual intervention required** for plan synchronization

### Files Modified for Webhook Enhancement

- `src/payment/repositories/paymentRepository.go` - Added `GetSubscriptionPlanByStripePriceID` method
- `src/payment/services/stripeService.go` - Enhanced `handleSubscriptionUpdated` to detect and sync plan changes

## Critical Bug Fix - JSON Field Name

**Issue Found**: The `UserSubscriptionOutput` DTO had an incorrect JSON tag:
- **Before**: `json:"subscription_plans"` (plural, wrong)
- **After**: `json:"subscription_plan_id"` (singular, correct)

This was causing the frontend to not properly display the subscription plan ID because the JSON field name was incorrect.

### Impact

⚠️ **Breaking Change for Frontend**: If your frontend was reading `subscription_plans`, it must now read `subscription_plan_id`.

The correct API response now looks like:
```json
{
  "id": "uuid",
  "user_id": "user-id",
  "subscription_plan_id": "plan-uuid",  // ✅ CORRECTED - was "subscription_plans"
  "stripe_subscription_id": "sub_xxx",
  ...
}
```

### Frontend Update Required

**Old code (if you were using this):**
```typescript
const planId = subscription.subscription_plans;  // ❌ This will now be undefined
```

**New code:**
```typescript
const planId = subscription.subscription_plan_id;  // ✅ Correct field name
```
