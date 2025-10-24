# Bulk License Management - Frontend Integration Guide

## 📋 Table of Contents
1. [Overview](#overview)
2. [Key Concepts](#key-concepts)
3. [API Endpoints](#api-endpoints)
4. [Data Structures](#data-structures)
5. [User Workflows](#user-workflows)
6. [UI/UX Requirements](#uiux-requirements)
7. [Code Examples](#code-examples)
8. [Error Handling](#error-handling)
9. [Testing Guide](#testing-guide)

---

## 🎯 Overview

The bulk license management system allows users (trainers, group administrators, training centers) to:
- Purchase multiple licenses at once with **volume discounts**
- Assign licenses to individual users (e.g., students in a class)
- Manage license assignments (assign, revoke, reassign)
- Scale up/down the number of licenses mid-subscription
- Track license usage and availability

**Key Feature**: Tiered pricing automatically applies volume discounts based on quantity.

---

## 🔑 Key Concepts

### 1. **Subscription Plans with Tiered Pricing**

Subscription plans can now have **volume pricing tiers**:

```json
{
  "id": "uuid",
  "name": "Trainer Plan",
  "use_tiered_pricing": true,
  "pricing_tiers": [
    {
      "min_quantity": 1,
      "max_quantity": 5,
      "unit_amount": 1200,  // €12 per license
      "description": "1-5 licenses"
    },
    {
      "min_quantity": 6,
      "max_quantity": 15,
      "unit_amount": 1000,  // €10 per license
      "description": "6-15 licenses"
    },
    {
      "min_quantity": 16,
      "max_quantity": 30,
      "unit_amount": 800,   // €8 per license
      "description": "16-30 licenses"
    },
    {
      "min_quantity": 31,
      "max_quantity": 0,    // 0 = unlimited
      "unit_amount": 600,   // €6 per license
      "description": "31+ licenses"
    }
  ]
}
```

**Features Required for Bulk Purchase**:
- User's plan must include `"bulk_purchase"` or `"group_management"` in `features` array

### 2. **Subscription Batch**

A **batch** represents a bulk license purchase:
- One Stripe subscription with `quantity > 1`
- Contains multiple individual licenses (UserSubscription records)
- Tracks: Total licenses, Assigned licenses, Available licenses

### 3. **License States**

Individual licenses can be in different states:
- `unassigned` - Not yet assigned to anyone
- `active` - Assigned to a user and active
- `cancelled` - Subscription cancelled
- `past_due` - Payment issues

---

## 📡 API Endpoints

### Authentication Required
All endpoints require a valid JWT token in the `Authorization` header:
```
Authorization: Bearer <token>
```

---

### 0. **Stripe Plan Synchronization** (Admin Only)

#### Import Plans from Stripe
Import subscription plans from Stripe into the database. This is useful when plans are created/modified in the Stripe Dashboard.

**Endpoint**: `POST /api/v1/subscription-plans/import-stripe`

**Headers**:
- `Authorization: Bearer <admin-token>` ✅ Required (Admin only)

**Response** (200 OK):
```json
{
  "processed_plans": 4,
  "created_plans": 1,
  "updated_plans": 3,
  "skipped_plans": 0,
  "failed_plans": [],
  "created_details": [
    "Created plan: XS (Stripe price: price_1SJdyX2VDBCbFKoanstbeLH9, pricing: tiered (4 tiers))"
  ],
  "updated_details": [
    "Updated plan: Solo (Stripe price: price_1SFMxN2VDBCbFKoaRgQEsZ9I, pricing: 900 eur/month)",
    "Updated plan: Trainer (Stripe price: price_1SFMxO2VDBCbFKoaIE0gFxPi, pricing: 1900 eur/month)"
  ],
  "skipped_details": []
}
```

**Features**:
- ✅ Automatically detects **tiered pricing** (volume/graduated pricing in Stripe)
- ✅ Creates new plans that exist in Stripe but not in database
- ✅ Updates existing plans with current Stripe data
- ✅ Properly converts Stripe tier structure to database format
- ✅ Handles both flat-rate and volume-based pricing

**Important Notes**:
- This endpoint imports plans FROM Stripe TO your database (reverse sync)
- Tiered pricing is detected using Stripe's `tiers` field
- The API automatically expands tier data from Stripe using `priceParams.AddExpand("data.tiers")`
- For existing plans, this updates prices, tiers, and metadata

**Use Cases**:
- After creating a new plan in Stripe Dashboard
- After modifying pricing tiers in Stripe
- Initial setup to sync existing Stripe products

---

### 1. **Get Pricing Preview** (Public)

Get a detailed pricing breakdown BEFORE purchase.

**Endpoint**: `GET /api/v1/subscription-plans/pricing-preview`

**Query Parameters**:
- `subscription_plan_id` (string, required) - UUID of the plan
- `quantity` (int, required) - Number of licenses

**Response**:
```json
{
  "plan_name": "Trainer Plan",
  "total_quantity": 30,
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
      "range": "16-30",
      "quantity": 15,
      "unit_price": 800,
      "subtotal": 12000
    }
  ],
  "total_monthly_cost": 28000,      // €280 total
  "average_per_license": 9.33,      // €9.33 average
  "savings_vs_individual": 8000,    // €80 saved vs individual pricing
  "currency": "eur"
}
```

**Use Cases**:
- Display pricing calculator on plan selection page
- Show savings in real-time as user adjusts quantity
- Preview before checkout

---

### 2. **Purchase Bulk Licenses**

Create a bulk license purchase.

**Endpoint**: `POST /api/v1/user-subscriptions/purchase-bulk`

**Headers**:
- `Authorization: Bearer <token>` ✅ Required
- `Content-Type: application/json`

**Request Body**:
```json
{
  "subscription_plan_id": "uuid",
  "quantity": 30,
  "group_id": "uuid",           // Optional: link to a group
  "payment_method_id": "pm_xxx", // Optional: Stripe payment method
  "coupon_code": "SAVE20"        // Optional: discount coupon
}
```

**Response** (201 Created):
```json
{
  "id": "batch-uuid",
  "purchaser_user_id": "user-123",
  "subscription_plan_id": "plan-uuid",
  "subscription_plan": { /* plan details */ },
  "group_id": "group-uuid",
  "stripe_subscription_id": "sub_xxx",
  "stripe_subscription_item_id": "si_xxx",
  "total_quantity": 30,
  "assigned_quantity": 0,
  "available_quantity": 30,
  "status": "active",
  "current_period_start": "2025-01-01T00:00:00Z",
  "current_period_end": "2025-02-01T00:00:00Z",
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-01T00:00:00Z"
}
```

**Errors**:
- `403 Forbidden` - User's plan doesn't include `bulk_purchase` feature
- `400 Bad Request` - Invalid plan ID or quantity
- `500 Internal Server Error` - Server/Stripe error

---

### 3. **List My Batches**

Get all bulk purchases made by the current user.

**Endpoint**: `GET /api/v1/subscription-batches`

**Response** (200 OK):
```json
[
  {
    "id": "batch-uuid-1",
    "subscription_plan": {
      "name": "Trainer Plan",
      "price_amount": 1200
    },
    "total_quantity": 30,
    "assigned_quantity": 25,
    "available_quantity": 5,
    "status": "active",
    "current_period_end": "2025-02-01T00:00:00Z"
  },
  {
    "id": "batch-uuid-2",
    "subscription_plan": {
      "name": "Basic Plan",
      "price_amount": 800
    },
    "total_quantity": 10,
    "assigned_quantity": 10,
    "available_quantity": 0,
    "status": "active",
    "current_period_end": "2025-03-01T00:00:00Z"
  }
]
```

**Use Cases**:
- Dashboard showing all purchased batches
- Overview of license pools

---

### 4. **Get Batch Licenses**

Get all licenses (assigned & unassigned) in a batch.

**Endpoint**: `GET /api/v1/subscription-batches/:batch_id/licenses`

**Response** (200 OK):
```json
[
  {
    "id": "license-uuid-1",
    "user_id": "student-123",
    "subscription_batch_id": "batch-uuid",
    "status": "active",
    "created_at": "2025-01-01T00:00:00Z"
  },
  {
    "id": "license-uuid-2",
    "user_id": "",               // Unassigned
    "subscription_batch_id": "batch-uuid",
    "status": "unassigned",
    "created_at": "2025-01-01T00:00:00Z"
  }
]
```

**Use Cases**:
- View all licenses in a batch
- See who has assignments
- Identify available licenses

---

### 5. **Assign License to User**

Assign an unassigned license to a specific user.

**Endpoint**: `POST /api/v1/subscription-batches/:batch_id/assign`

**Request Body**:
```json
{
  "user_id": "student-456"
}
```

**Response** (200 OK):
```json
{
  "id": "license-uuid",
  "user_id": "student-456",
  "subscription_batch_id": "batch-uuid",
  "subscription_plan": { /* plan details */ },
  "status": "active",
  "current_period_start": "2025-01-01T00:00:00Z",
  "current_period_end": "2025-02-01T00:00:00Z"
}
```

**Errors**:
- `403 Forbidden` - Not the purchaser
- `400 Bad Request` - No available licenses or invalid user ID
- `404 Not Found` - Batch not found

**Use Cases**:
- Teacher assigns license to student
- Admin assigns license to new employee
- Bulk assignment UI

---

### 6. **Revoke License Assignment**

Remove a license assignment and return it to the pool.

**Endpoint**: `DELETE /api/v1/subscription-batches/:batch_id/licenses/:license_id/revoke`

**Response** (200 OK):
```json
{
  "message": "License revoked successfully"
}
```

**Errors**:
- `403 Forbidden` - Not the purchaser
- `404 Not Found` - License not found

**Use Cases**:
- Student leaves class
- Employee leaves organization
- Reassignment needed

---

### 7. **Update Batch Quantity**

Scale up or down the number of licenses (updates Stripe subscription).

**Endpoint**: `PATCH /api/v1/subscription-batches/:batch_id/quantity`

**Request Body**:
```json
{
  "new_quantity": 40  // Scale from 30 to 40
}
```

**Response** (200 OK):
```json
{
  "message": "Batch quantity updated to 40"
}
```

**Errors**:
- `400 Bad Request` - Cannot reduce below assigned quantity
- `403 Forbidden` - Not the purchaser

**Use Cases**:
- Class grows, need more licenses
- Reduce licenses to save costs
- Mid-subscription adjustments

---

## 📦 Data Structures

### SubscriptionPlanOutput
```typescript
interface SubscriptionPlanOutput {
  id: string;
  name: string;
  description: string;
  price_amount: number;           // Base price in cents (first tier if tiered)
  currency: string;               // "eur", "usd"
  billing_interval: string;       // "month", "year"
  features: string[];             // ["bulk_purchase", "group_management"]
  use_tiered_pricing: boolean;    // TRUE if plan has volume pricing
  pricing_tiers: PricingTier[];   // Empty array if not tiered
  is_active: boolean;

  // Additional fields
  stripe_product_id: string;
  stripe_price_id: string;
  trial_days: number;
  max_concurrent_users: number;
  max_courses: number;
  max_lab_sessions: number;
  required_role: string;
  created_at: string;             // ISO 8601
  updated_at: string;
}

interface PricingTier {
  min_quantity: number;           // Start of tier (e.g., 1, 6, 16)
  max_quantity: number;           // End of tier (0 = unlimited)
  unit_amount: number;            // Price per license in cents
  description?: string;           // Optional description
}
```

**Example Response** (XS Plan with Volume Pricing):
```json
{
  "id": "0199f869-83ef-734c-8387-4441da13f598",
  "name": "XS",
  "description": "Small volume plan with graduated pricing",
  "stripe_product_id": "prod_TGAJ4N07Uf1GKL",
  "stripe_price_id": "price_1SJdyX2VDBCbFKoanstbeLH9",
  "price_amount": 400,
  "currency": "eur",
  "billing_interval": "month",
  "use_tiered_pricing": true,
  "pricing_tiers": [
    {
      "min_quantity": 1,
      "max_quantity": 1,
      "unit_amount": 400
    },
    {
      "min_quantity": 2,
      "max_quantity": 5,
      "unit_amount": 350
    },
    {
      "min_quantity": 6,
      "max_quantity": 10,
      "unit_amount": 300
    },
    {
      "min_quantity": 11,
      "max_quantity": 0,
      "unit_amount": 250
    }
  ]
}
```

**Important**: The `pricing_tiers` array is **always present** in the response. Check `use_tiered_pricing` to determine if the plan uses volume pricing.

### SubscriptionBatchOutput
```typescript
interface SubscriptionBatchOutput {
  id: string;
  purchaser_user_id: string;
  subscription_plan_id: string;
  subscription_plan: SubscriptionPlanOutput;
  group_id?: string;
  stripe_subscription_id: string;
  total_quantity: number;
  assigned_quantity: number;
  available_quantity: number;     // Calculated field
  status: "active" | "cancelled" | "expired";
  current_period_start: string;   // ISO 8601
  current_period_end: string;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;
}
```

### PricingBreakdown
```typescript
interface PricingBreakdown {
  plan_name: string;
  total_quantity: number;
  tier_breakdown: TierCost[];
  total_monthly_cost: number;     // In cents
  average_per_license: number;    // In currency (e.g., 9.33)
  savings_vs_individual: number;  // In cents
  currency: string;
}

interface TierCost {
  range: string;                  // "1-10", "11-25", "26+"
  quantity: number;
  unit_price: number;             // In cents
  subtotal: number;               // In cents
}
```

---

## 👤 User Workflows

### Workflow 1: Teacher Purchases 30 Licenses for Class

1. **View Plans**
   - `GET /api/v1/subscription-plans`
   - Filter plans with `use_tiered_pricing: true`

2. **Calculate Pricing**
   - User adjusts quantity slider (1-50)
   - Call `GET /api/v1/subscription-plans/pricing-preview?subscription_plan_id=xxx&quantity=30`
   - Display breakdown in real-time

3. **Purchase**
   - `POST /api/v1/user-subscriptions/purchase-bulk`
   - Redirect to Stripe checkout (or handle payment)
   - On success, show batch details

4. **View Batch**
   - `GET /api/v1/subscription-batches`
   - Display batch with 30 total, 0 assigned, 30 available

5. **Assign to Students**
   - For each student:
     - `POST /api/v1/subscription-batches/:id/assign` with `{"user_id": "student-xxx"}`
   - Or bulk assign via CSV upload

6. **Monitor Usage**
   - `GET /api/v1/subscription-batches/:id/licenses`
   - Show table: Student Name | Email | Status | Assigned Date

### Workflow 2: Adding More Licenses Mid-Subscription

1. **View Current Batch**
   - `GET /api/v1/subscription-batches`
   - Identify batch needing more licenses

2. **Calculate New Cost**
   - `GET /api/v1/subscription-plans/pricing-preview?subscription_plan_id=xxx&quantity=40`
   - Show proration calculation (done by Stripe)

3. **Update Quantity**
   - `PATCH /api/v1/subscription-batches/:id/quantity` with `{"new_quantity": 40}`
   - Stripe prorates the difference automatically

4. **Assign New Licenses**
   - `POST /api/v1/subscription-batches/:id/assign` for new students

### Workflow 3: Revoking a License

1. **Find License**
   - `GET /api/v1/subscription-batches/:id/licenses`
   - Identify license assigned to departing student

2. **Revoke**
   - `DELETE /api/v1/subscription-batches/:id/licenses/:license_id/revoke`

3. **Reassign** (Optional)
   - License returns to pool
   - `POST /api/v1/subscription-batches/:id/assign` with new user

---

## 🎨 UI/UX Requirements

### 1. **Pricing Calculator Component**

**Location**: Plan selection page

**Features**:
- Quantity slider/input (1-100+)
- Real-time pricing preview
- Visual breakdown by tier
- Savings badge

**Example UI**:
```
┌────────────────────────────────────────┐
│ Trainer Plan                           │
│                                        │
│ How many licenses do you need?         │
│ [■■■■■■■■■■░░░░░░░░░░] 30 licenses    │
│                                        │
│ Pricing Breakdown:                     │
│ ┌────────────────────────────────────┐ │
│ │ 1-5 licenses:   5 × €12 = €60    │ │
│ │ 6-15 licenses: 10 × €10 = €100   │ │
│ │ 16-30 licenses: 15 × €8 = €120   │ │
│ └────────────────────────────────────┘ │
│                                        │
│ Total: €280/month                      │
│ Average: €9.33/license                 │
│ 💰 Save €80 vs individual pricing!    │
│                                        │
│ [Purchase 30 Licenses]                 │
└────────────────────────────────────────┘
```

### 2. **License Management Dashboard**

**Location**: User dashboard (for purchasers)

**Features**:
- List of all batches
- Quick stats: Total, Assigned, Available
- Actions: View details, Add licenses, Manage assignments

**Example UI**:
```
┌─────────────────────────────────────────────────────────────┐
│ My License Batches                              [+ Purchase]│
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Trainer Plan - Class 2024A                             │ │
│ │ 30 Total │ 25 Assigned │ 5 Available                   │ │
│ │ Renews: Feb 1, 2025                                     │ │
│ │ [View Details] [Add Licenses] [Manage]                 │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Basic Plan - Training Center                           │ │
│ │ 50 Total │ 50 Assigned │ 0 Available ⚠️                 │ │
│ │ Renews: Mar 15, 2025                                    │ │
│ │ [View Details] [Add Licenses] [Manage]                 │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### 3. **License Assignment Interface**

**Location**: Batch details page

**Features**:
- Table of all licenses (assigned & available)
- Bulk actions: Import CSV, Assign all
- Individual actions: Assign, Revoke, View user
- Filters: Show assigned, Show available

**Example UI**:
```
┌─────────────────────────────────────────────────────────────┐
│ Trainer Plan - Class 2024A                                  │
│ 30 Total │ 25 Assigned │ 5 Available                        │
│                                                              │
│ [Assign License] [Import CSV] [Export]  Filters: [All ▼]   │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐│
│ │ User          │ Email              │ Status    │ Actions ││
│ ├──────────────────────────────────────────────────────────┤│
│ │ John Doe      │ john@school.com    │ Active    │ Revoke  ││
│ │ Jane Smith    │ jane@school.com    │ Active    │ Revoke  ││
│ │ ...           │ ...                │ ...       │ ...     ││
│ │ (Unassigned)  │ -                  │ Available │ Assign  ││
│ │ (Unassigned)  │ -                  │ Available │ Assign  ││
│ └──────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

### 4. **Assign License Modal**

**Trigger**: Click "Assign" button

**Features**:
- User search/autocomplete
- Email input (if user doesn't exist, send invite)
- Confirmation message

**Example UI**:
```
┌────────────────────────────────────────┐
│ Assign License                         │
│                                        │
│ Search for user:                       │
│ [jane@school.com        ] 🔍          │
│                                        │
│ Results:                               │
│ ┌────────────────────────────────────┐ │
│ │ ● Jane Smith (jane@school.com)    │ │
│ │   Already registered              │ │
│ │                                    │ │
│ │   [Assign to this user]           │ │
│ └────────────────────────────────────┘ │
│                                        │
│ Or send email invite:                  │
│ [Send Invitation Email]                │
│                                        │
│ [Cancel]                               │
└────────────────────────────────────────┘
```

---

## 💡 Implementation Notes

### Displaying Tiered Pricing

When displaying plans to users, check the `use_tiered_pricing` field:

**Simple Pricing** (`use_tiered_pricing: false`):
- Display `price_amount` as the per-license cost
- Calculate total: `price_amount × quantity`

**Tiered Pricing** (`use_tiered_pricing: true`):
- Show the `pricing_tiers` array with ranges and prices
- Use the pricing preview API for accurate calculations
- Display savings compared to individual pricing

**Example Display Logic**:
```
if (plan.use_tiered_pricing) {
  // Show tier breakdown: "1-5 licenses: €4.00 each"
  // Call GET /pricing-preview for exact total
} else {
  // Show simple price: "€12.00 per license"
  // Calculate: price_amount × quantity
}
```

### Handling the `max_quantity` Field

In `pricing_tiers`, the `max_quantity` field uses a special convention:
- **0 = unlimited** (last tier, e.g., "31+ licenses")
- **Non-zero** = hard limit for that tier (e.g., "1-5 licenses")

**Display Examples**:
- Tier with `max_quantity: 5` → "1-5 licenses"
- Tier with `max_quantity: 0` → "31+ licenses" or "31-∞ licenses"

---

## ⚠️ Error Handling

### Common Errors

| Status | Error | Meaning | User Action |
|--------|-------|---------|-------------|
| 403 | Feature not available | User's plan doesn't include `bulk_purchase` | Upgrade plan |
| 400 | No available licenses | All licenses in batch are assigned | Add more licenses |
| 400 | Cannot reduce quantity | Trying to reduce below assigned count | Revoke licenses first |
| 404 | Batch not found | Invalid batch ID | Check batch list |
| 401 | Unauthorized | Invalid or expired token | Re-login |

### Error Response Format

```json
{
  "error_code": 403,
  "error_message": "Your current plan does not include bulk_purchase. Please upgrade your subscription."
}
```

### Recommended UI Messages

```typescript
const ERROR_MESSAGES = {
  403: "Your plan doesn't support bulk purchases. Please upgrade to continue.",
  400: "All licenses are assigned. Add more licenses or revoke existing ones.",
  404: "License batch not found. It may have been cancelled.",
  500: "Something went wrong. Please try again or contact support.",
};
```

---

## 🧪 Testing Guide

### Test Scenarios

#### 0. **Stripe Import** (Admin Only)
```bash
# Import/sync plans from Stripe
curl -X POST http://localhost:8080/api/v1/subscription-plans/import-stripe \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json"

# Expected response:
# {
#   "processed_plans": 4,
#   "created_plans": 1,
#   "updated_plans": 3,
#   "created_details": [
#     "Created plan: XS (Stripe price: price_1SJdyX..., pricing: tiered (4 tiers))"
#   ],
#   "updated_details": [...]
# }

# Verify tiered pricing was imported correctly
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/subscription-plans | \
  jq '.data[] | select(.name == "XS") | {name, use_tiered_pricing, tier_count: (.pricing_tiers | length)}'

# Expected output:
# {
#   "name": "XS",
#   "use_tiered_pricing": true,
#   "tier_count": 4
# }
```

#### 1. **Pricing Preview**
```bash
# Test various quantities
curl "http://localhost:8080/api/v1/subscription-plans/pricing-preview?subscription_plan_id=<PLAN_ID>&quantity=5"
curl "http://localhost:8080/api/v1/subscription-plans/pricing-preview?subscription_plan_id=<PLAN_ID>&quantity=30"
curl "http://localhost:8080/api/v1/subscription-plans/pricing-preview?subscription_plan_id=<PLAN_ID>&quantity=100"

# Verify tier calculations match expectations
```

#### 2. **Bulk Purchase**
```bash
curl -X POST http://localhost:8080/api/v1/user-subscriptions/purchase-bulk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "subscription_plan_id": "<PLAN_ID>",
    "quantity": 10
  }'

# Verify:
# - Batch created with correct quantity
# - 10 UserSubscription records created
# - All licenses are "unassigned"
```

#### 3. **License Assignment**
```bash
# List batches
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/subscription-batches

# Get licenses in batch
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/subscription-batches/<BATCH_ID>/licenses

# Assign a license
curl -X POST http://localhost:8080/api/v1/subscription-batches/<BATCH_ID>/assign \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "student-123"}'

# Verify:
# - License status changed to "active"
# - user_id set correctly
# - assigned_quantity incremented
```

#### 4. **License Revocation**
```bash
curl -X DELETE http://localhost:8080/api/v1/subscription-batches/<BATCH_ID>/licenses/<LICENSE_ID>/revoke \
  -H "Authorization: Bearer $TOKEN"

# Verify:
# - License status back to "unassigned"
# - user_id cleared
# - assigned_quantity decremented
```

#### 5. **Quantity Update**
```bash
# Scale up
curl -X PATCH http://localhost:8080/api/v1/subscription-batches/<BATCH_ID>/quantity \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"new_quantity": 20}'

# Verify:
# - 10 new licenses created
# - total_quantity updated to 20
```

### Edge Cases to Test

1. **Permissions**:
   - User without `bulk_purchase` feature tries to purchase → 403
   - User tries to assign license from another user's batch → 403

2. **Constraints**:
   - Try to reduce quantity below assigned count → 400
   - Try to assign when no licenses available → 400

3. **Pricing Tiers**:
   - Purchase exactly at tier boundary (5, 15, 30)
   - Purchase quantity spanning multiple tiers (e.g., 25)
   - Purchase in unlimited tier (40+)

---

## 📚 Additional Notes

### Recent Updates (January 2025)

#### ✅ Fixed: Tiered Pricing Display Issue
**Problem**: Plans with tiered pricing were being saved correctly to the database but API responses showed `use_tiered_pricing: false` with empty `pricing_tiers` array.

**Root Cause**: The entity registration converter (`subscriptionPlanRegistration.go`) was missing the tiered pricing fields when converting from model to DTO.

**Solution**: Updated `subscriptionPlanPtrModelToOutput()` to include:
- Convert `plan.PricingTiers` (model) → `dto.PricingTier` array
- Include `UseTieredPricing` field in output
- Include `PricingTiers` array in output

**Files Modified**:
- `/src/payment/entityRegistration/subscriptionPlanRegistration.go:63-110`

**Verification**:
```bash
# All plans now correctly display tiered pricing
GET /api/v1/subscription-plans
# XS plan now shows:
# "use_tiered_pricing": true
# "pricing_tiers": [4 tiers with proper min/max/amount]
```

#### ✅ Added: Stripe Import Functionality
**Feature**: New endpoint to import plans from Stripe Dashboard into database (reverse sync).

**Key Implementation Details**:
- Uses `priceParams.AddExpand("data.tiers")` to fetch tier data from Stripe API
- Automatically detects volume/graduated pricing schemes
- Converts Stripe's `UpTo` boundaries to `min_quantity`/`max_quantity` ranges
- Handles unlimited tiers (where `UpTo = 0`)

**Endpoint**: `POST /api/v1/subscription-plans/import-stripe`

**Files Created/Modified**:
- `/src/payment/services/stripeService.go:1987-2200` - Import logic
- `/src/payment/routes/userSubscriptionController.go` - Controller
- `/src/payment/routes/subscriptionPlanRoutes.go` - Route registration

**Use Cases**:
1. Initial setup: Import existing Stripe products
2. New plans: Create plan in Stripe Dashboard, then import
3. Updates: Modify pricing in Stripe Dashboard, then sync

### Feature Flags

To enable bulk purchase for a plan, ensure it has the correct feature:

```sql
UPDATE subscription_plans
SET features = features || '["bulk_purchase"]'::jsonb
WHERE name = 'Trainer Plan';
```

### Stripe Integration (TODO)

The current implementation uses placeholder Stripe IDs. To complete Stripe integration:

1. **Create Tiered Prices in Stripe Dashboard**:
   - Create Product: "Trainer Plan"
   - Add Price with `billing_scheme: tiered`
   - Define tiers matching your pricing model

2. **Update `bulkLicenseService.go`** (line 46):
   Replace placeholder with actual Stripe call:
   ```go
   stripeSub, err := s.stripeService.CreateSubscriptionWithQuantity(
       purchaserUserID,
       plan,
       input.Quantity,
   )
   ```

3. **Handle Stripe Webhooks**:
   - `invoice.payment_succeeded` - Mark subscription active
   - `invoice.payment_failed` - Mark subscription past_due
   - `customer.subscription.updated` - Update quantity/status

### Database Schema

```sql
-- New tables created automatically via AutoMigrate
SELECT * FROM subscription_batches;
SELECT * FROM user_subscriptions WHERE subscription_batch_id IS NOT NULL;
```

### Performance Considerations

- **Pagination**: List endpoints support pagination (add `?page=1&limit=20`)
- **Caching**: Cache pricing previews for 5 minutes
- **Bulk Operations**: Batch assign/revoke operations for better performance

---

## 🎉 Summary

You now have a complete bulk license management system with:

✅ **Tiered pricing** with volume discounts
✅ **Bulk purchase** API
✅ **License assignment/revocation**
✅ **Dynamic quantity updates**
✅ **Feature-based access control**
✅ **Real-time pricing preview**

### Quick Start Checklist for Frontend

- [ ] Implement pricing calculator component
- [ ] Create bulk purchase flow
- [ ] Build license management dashboard
- [ ] Add license assignment interface
- [ ] Handle all error states
- [ ] Test with real Stripe integration
- [ ] Add loading states and optimistic updates

---
  🚀 Deployment Checklist

  - Backend code implemented
  - Database migrations ready
  - API endpoints tested
  - Feature gates in place
  - Documentation complete
  - Stripe integration (API keys needed)
  - Frontend UI implementation
  - End-to-end testing

  ---
  🎯 Key Features Summary

  | Feature             | Status     | Notes                                   |
  |---------------------|------------|-----------------------------------------|
  | Tiered Pricing      | ✅ Complete | 4 tiers defined in sample plans         |
  | Pricing Preview API | ✅ Complete | Public endpoint, no auth                |
  | Bulk Purchase       | ✅ Complete | Feature-gated, creates batch + licenses |
  | License Assignment  | ✅ Complete | Assign/revoke/reassign                  |
  | Quantity Scaling    | ✅ Complete | Add/remove licenses dynamically         |
  | Feature Middleware  | ✅ Complete | Checks bulk_purchase in plan features   |
  | Sample Data         | ✅ Complete | 2 plans created on first startup        |
  | Stripe Placeholders | ✅ Complete | Ready for API key integration           |
  | Frontend Docs       | ✅ Complete | Full guide with code examples           |

  ---
  💡 Example: Teacher Workflow

  1. Teacher visits Plan Selection page
  2. Selects "Trainer Plan" with tiered pricing
  3. Adjusts slider to 30 students
  4. Sees breakdown:
    - 5 licenses × €12 = €60
    - 10 licenses × €10 = €100
    - 15 licenses × €8 = €120
    - Total: €280/month (saves €80!)
  5. Clicks "Purchase 30 Licenses"
  6. Goes to License Dashboard
  7. Clicks "Assign License" for each student
  8. Mid-year: 5 more students join
  9. Clicks "Add 5 Licenses" → Stripe prorates automatically


