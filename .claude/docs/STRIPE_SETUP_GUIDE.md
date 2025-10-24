# Stripe Configuration Guide for Bulk License Management

This guide walks you through configuring Stripe for the bulk license management system with tiered pricing.

---

## Prerequisites

- Stripe account (Test mode for development, Live mode for production)
- Stripe API keys (Secret key and Publishable key)
- Webhook endpoint URL (e.g., `https://yourdomain.com/webhooks/stripe`)

---

## 1. Create Stripe Products and Prices

### Individual Plans (No Tiered Pricing)

**Example: Member Pro Plan**

1. Go to **Products** → **Add product**
2. Fill in:
   - **Name**: Member Pro
   - **Description**: Access to one terminal - Individual user
   - **Pricing model**: Standard pricing
   - **Price**: €12.00 EUR
   - **Billing period**: Monthly
3. Click **Save product**
4. Copy the **Price ID** (e.g., `price_1234abcd...`)
5. Update your database:

```sql
UPDATE subscription_plans
SET stripe_price_id = 'price_1234abcd...'
WHERE name = 'Member Pro';
```

### Bulk Plans with Tiered Pricing

**Example: Trainer Plan**

1. Go to **Products** → **Add product**
2. Fill in:
   - **Name**: Trainer Plan
   - **Description**: For trainers - Bulk purchase with tiered pricing
   - **Pricing model**: **Volume pricing** (graduated tiers)
3. Configure pricing tiers:

   **Tier 1: 1-5 licenses**
   - First: 1
   - Last: 5
   - Per unit: €12.00 EUR

   **Tier 2: 6-15 licenses**
   - First: 6
   - Last: 15
   - Per unit: €10.00 EUR

   **Tier 3: 16-30 licenses**
   - First: 16
   - Last: 30
   - Per unit: €8.00 EUR

   **Tier 4: 31+ licenses**
   - First: 31
   - Last: (leave blank for unlimited)
   - Per unit: €6.00 EUR

4. Set billing period: **Monthly**
5. Click **Save product**
6. Copy the **Price ID** (e.g., `price_5678efgh...`)
7. Update your database:

```sql
UPDATE subscription_plans
SET stripe_price_id = 'price_5678efgh...'
WHERE name = 'Trainer Plan';
```

---

## 2. Configure Environment Variables

Add these to your `.env` file:

```bash
# Stripe API Keys
STRIPE_SECRET_KEY=sk_test_51...  # Test mode
# STRIPE_SECRET_KEY=sk_live_51...  # Live mode

STRIPE_PUBLISHABLE_KEY=pk_test_51...  # Test mode
# STRIPE_PUBLISHABLE_KEY=pk_live_51...  # Live mode

# Webhook Secret (see section 3)
STRIPE_WEBHOOK_SECRET=whsec_...
```

---

## 3. Configure Webhooks

### Create Webhook Endpoint

1. Go to **Developers** → **Webhooks**
2. Click **Add endpoint**
3. Enter endpoint URL: `https://yourdomain.com/webhooks/stripe`
4. Select events to listen to:
   - `customer.subscription.created`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`
   - `customer.subscription.paused`
   - `customer.subscription.resumed`
   - `customer.subscription.trial_will_end`
   - `invoice.created`
   - `invoice.finalized`
   - `invoice.payment_succeeded`
   - `invoice.payment_failed`
   - `payment_method.attached`
   - `payment_method.detached`
   - `customer.updated`
   - `checkout.session.completed`
5. Click **Add endpoint**
6. Copy the **Signing secret** (e.g., `whsec_...`)
7. Add to `.env`:

```bash
STRIPE_WEBHOOK_SECRET=whsec_...
```

### Test Webhooks (Development)

For local testing, use Stripe CLI:

```bash
# Install Stripe CLI
# https://stripe.com/docs/stripe-cli

# Login
stripe login

# Forward webhooks to local server
stripe listen --forward-to localhost:8080/webhooks/stripe
```

This will output a webhook signing secret for local testing.

---

## 4. Database Synchronization

### Option A: Sync from Stripe Dashboard

If you created plans in Stripe first:

```bash
# Call the sync endpoint (requires admin authentication)
curl -X POST http://localhost:8080/api/v1/subscription-plans/sync-stripe \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

This will:
- Fetch all products and prices from Stripe
- Create corresponding `subscription_plans` records
- Set `stripe_price_id` automatically

### Option B: Manual Database Setup

If you have plans in the database first:

1. Create the Stripe product/price as shown in section 1
2. Copy the Stripe Price ID
3. Update your database:

```sql
UPDATE subscription_plans
SET stripe_price_id = 'price_...'
WHERE id = '<your-plan-uuid>';
```

---

## 5. Verify Configuration

### Test Plan Retrieval

```bash
curl http://localhost:8080/api/v1/subscription-plans
```

Expected output should include `stripe_price_id`:

```json
{
  "data": [
    {
      "id": "...",
      "name": "Trainer Plan",
      "stripe_price_id": "price_5678efgh...",
      "use_tiered_pricing": true,
      "pricing_tiers": [
        {"min_quantity": 1, "max_quantity": 5, "unit_amount": 1200},
        {"min_quantity": 6, "max_quantity": 15, "unit_amount": 1000},
        {"min_quantity": 16, "max_quantity": 30, "unit_amount": 800},
        {"min_quantity": 31, "max_quantity": 0, "unit_amount": 600}
      ]
    }
  ]
}
```

### Test Pricing Preview

```bash
curl "http://localhost:8080/api/v1/subscription-plans/pricing-preview?subscription_plan_id=<plan-id>&quantity=25"
```

Expected output:

```json
{
  "subscription_plan_id": "...",
  "subscription_plan_name": "Trainer Plan",
  "quantity": 25,
  "total_monthly_cost": 26500,
  "average_per_unit": 10.6,
  "savings": 3500,
  "tier_breakdown": [
    {
      "range": "1-5",
      "quantity": 5,
      "unit_price": 1200,
      "subtotal": 6000
    },
    {
      "range": "6-15",
      "quantity": 10,
      "unit_price": 1000,
      "subtotal": 10000
    },
    {
      "range": "16-25",
      "quantity": 10,
      "unit_price": 800,
      "subtotal": 8000
    }
  ]
}
```

### Test Bulk Purchase (Requires Authentication)

```bash
# Login first
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"1.supervisor@test.com","password":"test"}' \
  | python3 -c "import sys, json; print(json.load(sys.stdin)['access_token'])")

# Make bulk purchase
curl -X POST http://localhost:8080/api/v1/user-subscriptions/purchase-bulk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "subscription_plan_id": "<trainer-plan-id>",
    "quantity": 10,
    "payment_method_id": "pm_card_visa"
  }'
```

---

## 6. Stripe Dashboard Verification

### Check Subscription Created

1. Go to **Customers** in Stripe Dashboard
2. Find the customer by email or ID
3. Check **Subscriptions** tab
4. Verify:
   - Product: Trainer Plan
   - Quantity: 10
   - Status: Active
   - Metadata includes: `bulk_purchase=true`

### Check Invoices

1. Go to **Payments** → **Invoices**
2. Find the most recent invoice
3. Verify:
   - Amount matches tiered pricing calculation
   - Status: Paid
   - Description includes quantity

### Manually Update Quantity (Test Webhook)

1. Go to customer's subscription in Stripe Dashboard
2. Click **Update subscription**
3. Change quantity (e.g., from 10 to 15)
4. Click **Update**
5. Check your application logs for webhook processing:

```
📦 Detected bulk subscription update: sub_...
🔄 Batch <batch-id> quantity changed from 10 to 15
✅ Added 5 licenses to batch <batch-id>
```

6. Verify in database:

```sql
SELECT total_quantity, assigned_quantity
FROM subscription_batches
WHERE stripe_subscription_id = 'sub_...';

-- Should show total_quantity = 15

SELECT COUNT(*)
FROM user_subscriptions
WHERE subscription_batch_id = '<batch-id>';

-- Should show 15 licenses
```

---

## 7. Production Checklist

Before going live:

- [ ] Switch to **Live mode** in Stripe Dashboard
- [ ] Create products/prices in Live mode (test mode data doesn't transfer)
- [ ] Update `.env` with live Stripe keys: `sk_live_...` and `pk_live_...`
- [ ] Configure webhook endpoint with production URL
- [ ] Update `STRIPE_WEBHOOK_SECRET` with live webhook secret
- [ ] Test webhook delivery in production (use Stripe CLI or dashboard webhook logs)
- [ ] Verify SSL certificate on webhook endpoint (Stripe requires HTTPS)
- [ ] Update subscription plans in production database with live `stripe_price_id`
- [ ] Test full purchase flow in production
- [ ] Monitor first real purchases closely

---

## 8. Common Issues and Troubleshooting

### Issue: "Plan does not have a Stripe price ID configured"

**Solution**: Update database with Stripe price ID:

```sql
UPDATE subscription_plans
SET stripe_price_id = 'price_...'
WHERE name = 'Your Plan Name';
```

### Issue: Webhook signature validation failed

**Causes**:
- Wrong `STRIPE_WEBHOOK_SECRET` in `.env`
- Mixing test/live mode secrets
- Using Stripe CLI secret instead of dashboard secret (or vice versa)

**Solution**:
1. Go to Stripe Dashboard → Developers → Webhooks
2. Copy the correct signing secret for your endpoint
3. Update `.env`
4. Restart server

### Issue: Tiered pricing not calculating correctly

**Check**:
1. Verify Stripe product has "Volume pricing" (not "Package pricing")
2. Ensure tiers don't overlap (e.g., 1-5, 6-15, 16-30, 31+)
3. Confirm database `pricing_tiers` JSON matches Stripe configuration:

```sql
SELECT name, use_tiered_pricing, pricing_tiers
FROM subscription_plans
WHERE name = 'Trainer Plan';
```

### Issue: Webhook events not being received

**Debug steps**:
1. Check webhook endpoint is accessible:
   ```bash
   curl -X POST https://yourdomain.com/webhooks/stripe \
     -H "Content-Type: application/json" \
     -d '{"test": true}'
   ```
2. Check Stripe Dashboard → Developers → Webhooks → Your endpoint
3. Look at **Recent deliveries** for failed attempts
4. Verify firewall/security groups allow Stripe IPs
5. Check application logs for webhook processing

### Issue: Duplicate webhook events

**This is normal!** Stripe may retry webhooks. The application handles this with:
- Event ID tracking (prevents duplicate processing)
- Event age validation (rejects old events)
- Idempotent database operations

No action needed unless you see errors in logs.

---

## 9. Monitoring and Logs

### Application Logs

Look for these log messages:

**Successful bulk purchase:**
```
✅ Created bulk Stripe subscription sub_... for customer cus_... (quantity: 10)
```

**Webhook processing:**
```
📥 Received webhook event: customer.subscription.updated (ID: evt_...)
📦 Detected bulk subscription update: sub_...
🔄 Batch <uuid> quantity changed from 10 to 15
✅ Added 5 licenses to batch <uuid>
```

**License assignment:**
```
License <uuid> assigned to user user-123 from batch <batch-uuid>
```

### Stripe Dashboard

Monitor:
- **Payments** → **Subscriptions**: Active bulk subscriptions
- **Payments** → **Invoices**: Billing history
- **Developers** → **Webhooks**: Delivery success rate
- **Developers** → **Logs**: API calls and errors

---

## 10. Testing Scenarios

### Scenario 1: Purchase 10 Licenses

1. User purchases 10 licenses
2. Expected:
   - Stripe subscription created with quantity=10
   - 1 `subscription_batches` record created
   - 10 `user_subscriptions` records created (all unassigned)
   - `assigned_quantity` = 0

### Scenario 2: Assign 5 Licenses

1. Purchaser assigns 5 licenses to users
2. Expected:
   - 5 `user_subscriptions` updated with `user_id` and `status='active'`
   - `assigned_quantity` = 5
   - 5 unassigned licenses remain

### Scenario 3: Scale Up to 15 Licenses

1. Update subscription quantity in Stripe Dashboard to 15
2. Expected webhook processing:
   - `customer.subscription.updated` webhook received
   - 5 new `user_subscriptions` created (unassigned)
   - `total_quantity` = 15
   - `assigned_quantity` = 5 (unchanged)
   - 10 unassigned licenses now available

### Scenario 4: Scale Down to 12 Licenses

1. Update subscription quantity in Stripe Dashboard to 12
2. Expected webhook processing:
   - `customer.subscription.updated` webhook received
   - 3 unassigned `user_subscriptions` deleted
   - `total_quantity` = 12
   - `assigned_quantity` = 5 (unchanged)
   - 7 unassigned licenses remain

### Scenario 5: Cancel Subscription

1. Cancel subscription in Stripe Dashboard
2. Expected webhook processing:
   - `customer.subscription.deleted` webhook received
   - Batch `status` = 'cancelled'
   - All licenses (assigned + unassigned) `status` = 'cancelled'
   - Users lose access to terminals

---

## 11. Support and Resources

### Stripe Documentation

- [Volume Pricing](https://stripe.com/docs/billing/subscriptions/tiers)
- [Webhooks](https://stripe.com/docs/webhooks)
- [Subscription Quantities](https://stripe.com/docs/billing/subscriptions/quantities)
- [Testing](https://stripe.com/docs/testing)

### OCF Core Documentation

- Implementation Summary: `IMPLEMENTATION_SUMMARY.md`
- Frontend Guide: `BULK_LICENSE_FRONTEND_GUIDE.md`
- API Documentation: `http://localhost:8080/swagger/`

### Test Cards

```
Visa (success):           4242 4242 4242 4242
Visa (requires action):   4000 0025 0000 3155
Visa (declined):          4000 0000 0000 0002
Mastercard (success):     5555 5555 5555 4444
```

Any CVC, any future expiration date, any ZIP code.

---

**Questions?** Check the troubleshooting section or review application logs for detailed error messages.
