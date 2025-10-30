# Stripe Webhook Configuration Guide

## Required Webhook Events

Based on the current code implementation, you **MUST** configure these webhook events in Stripe Dashboard:

### ✅ Currently Implemented Events

| Event | Priority | Purpose | What It Does |
|-------|----------|---------|--------------|
| `customer.subscription.created` | 🔴 **Critical** | New subscriptions | Creates subscription record in database when user subscribes |
| `customer.subscription.updated` | 🔴 **Critical** | Plan changes | **Updates plan ID and usage limits when subscription changes** |
| `customer.subscription.deleted` | 🔴 **Critical** | Cancellations | Marks subscription as cancelled in database |
| `invoice.payment_succeeded` | 🔴 **Critical** | Payment tracking | Creates/updates invoice records for successful payments |
| `invoice.payment_failed` | 🟡 Important | Payment failures | Marks subscription as `past_due` when payment fails |
| `checkout.session.completed` | 🟡 Important | Checkout metadata | Ensures metadata is propagated to subscription |

---

## How to Configure in Stripe Dashboard

### Step 1: Access Webhooks
1. Go to [Stripe Dashboard](https://dashboard.stripe.com)
2. Navigate to **Developers** → **Webhooks**
3. Find your webhook endpoint (should point to your app URL + `/webhooks/stripe`)

### Step 2: Add Events
1. Click on your webhook
2. Click **"+ Add events"** or **"Select events to listen to"**
3. Search for and enable each event from the list above
4. Click **"Add events"** to save

### Step 3: Verify Configuration
Your webhook should listen to these **6 events minimum**:
- ✅ `customer.subscription.created`
- ✅ `customer.subscription.updated` ← **THIS IS THE ONE FIXING YOUR ISSUE**
- ✅ `customer.subscription.deleted`
- ✅ `invoice.payment_succeeded`
- ✅ `invoice.payment_failed`
- ✅ `checkout.session.completed`

---

## Missing Events (Recommendations)

These events are **not currently handled** in the code but **should be added** for a production-ready system:

### 🟠 Recommended to Add

| Event | Why You Need It | Current Impact |
|-------|-----------------|----------------|
| `customer.subscription.paused_entitlement` | Subscription paused (Stripe pauses service) | ❌ Not handled - subscription remains active in your DB |
| `customer.subscription.resumed` | Subscription resumed after pause | ❌ Not handled - won't sync resumed state |
| `customer.subscription.trial_will_end` | Trial ending in 3 days | ❌ Not handled - can't notify users |
| `invoice.created` | Invoice generated (before payment) | ❌ Not handled - only tracking paid invoices |
| `invoice.finalized` | Invoice ready for payment | ❌ Not handled - can't show pending invoices |
| `payment_method.attached` | Payment method added | ❌ Not handled - payment methods not synced |
| `payment_method.detached` | Payment method removed | ❌ Not handled - stale payment methods in DB |
| `customer.updated` | Customer info changed | ❌ Not handled - customer data might be stale |

### 🔵 Nice to Have (Future Improvements)

| Event | Purpose |
|-------|---------|
| `invoice.upcoming` | Preview next invoice (for prorated charges) |
| `invoice.voided` | Invoice cancelled |
| `payment_intent.succeeded` | Alternative payment tracking |
| `payment_intent.payment_failed` | Failed payment details |
| `customer.subscription.pending_update_applied` | Scheduled plan changes applied |
| `customer.subscription.pending_update_expired` | Scheduled change expired |

---

## What's Missing in the Code

### 1. ❌ Payment Method Webhooks (Important)

**Missing Events:**
- `payment_method.attached`
- `payment_method.detached`
- `payment_method.updated`

**Impact:** Your `payment_methods` table won't automatically sync when users add/remove cards in Stripe.

**Should Add:**
```go
case "payment_method.attached":
    return ss.handlePaymentMethodAttached(event)
case "payment_method.detached":
    return ss.handlePaymentMethodDetached(event)
```

### 2. ❌ Subscription Pause/Resume (Moderate)

**Missing Events:**
- `customer.subscription.paused`
- `customer.subscription.resumed`

**Impact:** If you use Stripe's pause collection feature, your database won't reflect the paused state.

**Should Add:**
```go
case "customer.subscription.paused":
    return ss.handleSubscriptionPaused(event)
case "customer.subscription.resumed":
    return ss.handleSubscriptionResumed(event)
```

### 3. ❌ Trial Expiration Warning (Low Priority)

**Missing Event:**
- `customer.subscription.trial_will_end`

**Impact:** Can't send "your trial ends in 3 days" notifications.

### 4. ❌ Invoice Created/Finalized (Moderate)

**Missing Events:**
- `invoice.created`
- `invoice.finalized`

**Impact:** Only tracking **paid** invoices. Users can't see pending/upcoming invoices.

**Current Behavior:**
- ✅ Invoice paid → appears in database
- ❌ Invoice pending → invisible to user until payment

### 5. ⚠️ Customer Data Updates (Low)

**Missing Event:**
- `customer.updated`

**Impact:** If users update email/name in Stripe Dashboard, it won't sync to your system.

---

## Testing Your Webhooks

### Test in Stripe Dashboard

1. Go to **Developers** → **Webhooks** → Your webhook
2. Click **"Send test webhook"**
3. Select each event type and send
4. Check your application logs for the webhook processing messages

### Expected Log Output

When you test `customer.subscription.updated`:
```bash
📥 Received webhook event: customer.subscription.updated (ID: evt_test_xxx)
🔄 Processing subscription updated
🔄 Plan change detected in webhook: old-plan-uuid -> new-plan-uuid (Stripe price: price_xxx)
✅ Updated usage limits for user user-id to plan new-plan-uuid
✅ Successfully processed webhook event evt_test_xxx
```

### Manual Test with Stripe CLI

```bash
# Install Stripe CLI
brew install stripe/stripe-cli/stripe

# Login
stripe login

# Forward webhooks to local server
stripe listen --forward-to localhost:8080/webhooks/stripe

# Trigger test event
stripe trigger customer.subscription.updated
```

---

## Troubleshooting

### Webhook Not Arriving

**Symptoms:**
- Log shows `invoice.payment_succeeded` but not `customer.subscription.updated`
- Plan changes don't reflect in database

**Solution:**
1. Check Stripe Dashboard → Webhooks → Your webhook → Events to listen to
2. Verify `customer.subscription.updated` is checked
3. Save and retry the upgrade

### Webhook Failing

**Symptoms:**
- Stripe shows webhook failed (5xx error)
- Logs show errors in webhook processing

**Common Causes:**
- Subscription not found in database → Check that checkout created it
- Invalid plan ID → Ensure Stripe price ID matches a plan in your database
- Database connection issue → Check database is accessible

**Check Stripe Dashboard:**
- Go to **Developers** → **Webhooks** → Your webhook
- Click on a failed event
- View the response and error details

### Duplicate Events

**Symptoms:**
- Same webhook processed multiple times
- Duplicate invoices created

**Current Protection:**
- ✅ Anti-replay protection implemented (10-minute window)
- ✅ Duplicate event tracking in memory
- ✅ Invoice checking before creation

---

## Webhook Endpoint Security

### Current Security Measures

✅ **Implemented:**
- Signature verification (validates request is from Stripe)
- User-Agent checking (must contain "Stripe")
- Content-Type validation
- Payload size limit (1MB max)
- Event age check (rejects events older than 10 minutes)
- Anti-replay protection (tracks processed event IDs)

### Webhook Secret

Your webhook secret is stored in environment variable:
```bash
STRIPE_WEBHOOK_SECRET=whsec_xxxxxxxxxxxxx
```

**IMPORTANT:** Never commit this to git or share publicly.

---

## Summary

### ✅ Full Production Configuration (All Implemented!)

All **14 webhook events** are now implemented and ready to use:

**Core Subscription Events:**
1. ✅ `customer.subscription.created`
2. ✅ `customer.subscription.updated`
3. ✅ `customer.subscription.deleted`
4. ✅ `customer.subscription.paused`
5. ✅ `customer.subscription.resumed`
6. ✅ `customer.subscription.trial_will_end`

**Invoice Events:**
7. ✅ `invoice.created`
8. ✅ `invoice.finalized`
9. ✅ `invoice.payment_succeeded`
10. ✅ `invoice.payment_failed`

**Payment Method Events:**
11. ✅ `payment_method.attached`
12. ✅ `payment_method.detached`

**Customer & Checkout Events:**
13. ✅ `customer.updated`
14. ✅ `checkout.session.completed`

### Handlers Implemented

All webhook handlers are now available in `stripeService.go`:
- ✅ `handleSubscriptionPaused()` - Marks subscription as paused
- ✅ `handleSubscriptionResumed()` - Reactivates paused subscription
- ✅ `handleTrialWillEnd()` - Logs trial ending (ready for notifications)
- ✅ `handleInvoiceCreated()` - Creates draft/pending invoices
- ✅ `handleInvoiceFinalized()` - Updates invoice to "open" status
- ✅ `handlePaymentMethodAttached()` - Syncs new payment methods
- ✅ `handlePaymentMethodDetached()` - Marks payment methods as inactive
- ✅ `handleCustomerUpdated()` - Syncs customer data changes

**Status:** Production-ready! All webhook events you enabled in Stripe are now fully supported.

---

## Quick Fix for Your Issue

1. Go to Stripe Dashboard → Webhooks
2. Find your webhook endpoint
3. Click "Add events"
4. Search for `customer.subscription.updated`
5. Enable it
6. Save
7. Try upgrading your plan again
8. Check logs - you should now see the subscription update webhook

That's it! Your plan changes will now sync correctly. 🎉
