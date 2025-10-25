# Frontend Subscription System Integration Guide

**Last Updated**: 2025-10-25
**API Version**: v1
**Backend Status**: ✅ All 3 Phases Fully Implemented

---

## Table of Contents

1. [Overview](#overview)
2. [Role System (Phase 3 Simplification)](#role-system-phase-3-simplification)
3. [Authentication](#authentication)
4. [Phase 1: Individual User Subscriptions](#phase-1-individual-user-subscriptions)
5. [Phase 2: Organization Subscriptions](#phase-2-organization-subscriptions)
6. [Phase 3: Bulk License Management](#phase-3-bulk-license-management)
7. [Feature Detection & Limits](#feature-detection--limits)
8. [Stripe Integration](#stripe-integration)
9. [Error Handling](#error-handling)
10. [Complete Use Cases](#complete-use-cases)
11. [API Reference](#api-reference)

---

## Overview

The OCF Core platform supports **three distinct subscription models**:

### 1. Individual Subscriptions (Phase 1)
Users subscribe directly to a plan for personal use.
- **Use Case**: Individual learners, personal projects
- **Payment**: User pays for their own subscription
- **Features**: Assigned to individual user
- **Status**: ✅ Fully Implemented & Tested

### 2. Organization Subscriptions (Phase 2)
Organizations subscribe, all members inherit features.
- **Use Case**: Training companies, teams, schools
- **Payment**: Organization owner/manager handles billing
- **Features**: Shared across all organization members (aggregated if member of multiple orgs)
- **Status**: ✅ Fully Implemented (Backend Complete)

### 3. Bulk Licenses (Phase 3)
Purchase multiple licenses and assign them to specific users.
- **Use Case**: Corporate training, classroom management, license reselling
- **Payment**: Bulk purchaser pays upfront for all licenses
- **Features**: Assigned per license to specific users
- **Status**: ✅ Fully Implemented & Tested

---

## Role System (Phase 3 Simplification)

### Overview of Role Changes

**Phase 3 has simplified the role system from 7 roles to 2 system roles.** Business roles (trainer, manager, etc.) are now determined by organization and group membership, not system roles.

### System Roles (Only 2)

```typescript
type SystemRole = 'member' | 'administrator';
```

- **`member`**: Default role for all authenticated users
- **`administrator`**: System administrators (platform management)

### Business Roles (Context-Based)

Business capabilities are determined by **organization** and **group** membership:

#### Organization Roles
```typescript
type OrganizationRole = 'owner' | 'manager' | 'member';
```

- **`owner`**: Full organization control, manages billing
- **`manager`**: Full access to all org groups and resources
- **`member`**: Basic organization access

#### Group Roles
```typescript
type GroupRole = 'owner' | 'admin' | 'assistant' | 'member';
```

- **`owner`**: Full group control
- **`admin`**: Manages group settings and members
- **`assistant`**: Helper role (e.g., teaching assistant)
- **`member`**: Regular group member (e.g., student)

### Frontend Implementation

#### Check if User is System Administrator

```typescript
// Get current user
const getCurrentUser = async (token: string) => {
  const response = await fetch('http://localhost:8080/api/v1/users/me', {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  return await response.json();
};

// Check if user is system admin
const isSystemAdmin = (user: User): boolean => {
  // Only 2 possible system roles now: 'member' or 'administrator'
  return user.roles?.some(role =>
    role.name === 'administrator' || role.name === 'admin'
  ) ?? false;
};
```

#### Check Organization Membership

```typescript
interface OrganizationMembership {
  organization_id: string;
  user_id: string;
  role: 'owner' | 'manager' | 'member';
  joined_at: string;
  is_active: boolean;
}

// Check if user can manage an organization
const canManageOrganization = (
  user: User,
  orgId: string
): boolean => {
  const membership = user.organization_memberships?.find(
    m => m.organization_id === orgId
  );
  return membership?.role === 'owner' || membership?.role === 'manager';
};

// Check if user owns an organization
const isOrganizationOwner = (
  user: User,
  orgId: string
): boolean => {
  const membership = user.organization_memberships?.find(
    m => m.organization_id === orgId
  );
  return membership?.role === 'owner';
};
```

#### Check Group Membership

```typescript
interface GroupMembership {
  group_id: string;
  user_id: string;
  role: 'owner' | 'admin' | 'assistant' | 'member';
  joined_at: string;
}

// Check if user can manage a group
const canManageGroup = (
  user: User,
  groupId: string
): boolean => {
  const membership = user.group_memberships?.find(
    m => m.group_id === groupId
  );
  return membership?.role === 'owner' || membership?.role === 'admin';
};
```

### Migrating from Old Role System

If your frontend currently checks for old roles (`member_pro`, `trainer`, `group_manager`, `organization`), here's how to migrate:

#### Old Code (Deprecated)
```typescript
// ❌ DON'T USE - Old role system
if (user.role === 'trainer' || user.role === 'organization') {
  // Show advanced features
}
```

#### New Code (Phase 3)
```typescript
// ✅ USE - Check organization membership instead
const hasAdvancedAccess = user.organization_memberships?.some(
  m => m.role === 'manager' || m.role === 'owner'
) ?? false;

if (hasAdvancedAccess) {
  // Show advanced features
}

// OR check effective features from subscriptions
const plan = await getUserEffectivePlan(token);
if (plan.can_create_advanced_labs) {
  // Show advanced features
}
```

### Feature Access Pattern

Features are now determined by **organization subscriptions**, not system roles:

```typescript
// Get user's effective features (aggregated across all organizations)
const getUserEffectivePlan = async (token: string) => {
  // Method 1: Get current user with subscriptions
  const user = await getCurrentUser(token);

  // The user object will have feature flags from their organizations
  // Backend aggregates features from all orgs user belongs to

  // Method 2: Check specific feature via API
  const canExport = await checkUserFeature(token, 'can_export_courses');

  return {
    can_create_advanced_labs: user.can_create_advanced_labs,
    can_export_courses: canExport,
    max_courses: user.max_courses,
    max_terminals: user.max_terminals
  };
};
```

### Important Notes

1. **All authenticated users are `member`** (system role)
2. **Business capabilities come from org/group membership**
3. **Features come from organization subscriptions**
4. **Personal organizations** are auto-created for backward compatibility
5. **Feature aggregation**: Users get MAX features across all their organizations

### Example: Checking Permissions

```typescript
// Complete permission check example
const checkUserPermissions = async (token: string) => {
  const user = await getCurrentUser(token);

  return {
    // System-level
    isSystemAdmin: isSystemAdmin(user),

    // Organization-level
    canCreateOrganizations: true, // All members can create orgs
    ownedOrganizations: user.organization_memberships?.filter(
      m => m.role === 'owner'
    ) ?? [],
    managedOrganizations: user.organization_memberships?.filter(
      m => m.role === 'owner' || m.role === 'manager'
    ) ?? [],

    // Group-level
    ownedGroups: user.group_memberships?.filter(
      m => m.role === 'owner'
    ) ?? [],
    managedGroups: user.group_memberships?.filter(
      m => m.role === 'owner' || m.role === 'admin'
    ) ?? [],

    // Feature-level (from subscriptions)
    features: {
      maxCourses: user.max_courses ?? 0,
      maxTerminals: user.max_terminals ?? 0,
      canExportCourses: user.can_export_courses ?? false,
      canUseAPI: user.can_use_api ?? false,
      // ... other features
    }
  };
};
```

---

## Authentication

All API requests require JWT authentication.

### Login Flow

```typescript
// 1. Login to get access token
const loginResponse = await fetch('http://localhost:8080/api/v1/auth/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    email: 'user@example.com',
    password: 'password123'
  })
});

const { access_token, token_type, expires_in } = await loginResponse.json();

// 2. Use token in subsequent requests
const headers = {
  'Authorization': `Bearer ${access_token}`,
  'Content-Type': 'application/json'
};
```

### Test Credentials (Development Only)
- **Email**: `1.supervisor@test.com`
- **Password**: `test`

---

## Phase 1: Individual User Subscriptions

### Overview

Individual users can subscribe to plans for personal use. Each subscription is tied directly to the user account.

### 1.1 List Available Plans

```typescript
// GET /api/v1/subscription-plans
const getSubscriptionPlans = async (token: string) => {
  const response = await fetch('http://localhost:8080/api/v1/subscription-plans', {
    headers: { 'Authorization': `Bearer ${token}` }
  });

  const data = await response.json();
  return data.data; // Array of SubscriptionPlanOutput
};
```

**Response Structure**:
```typescript
interface SubscriptionPlanOutput {
  id: string;
  name: string;
  description: string;
  priority: number; // Higher = better tier
  stripe_product_id: string;
  stripe_price_id: string;
  price_amount: number; // In cents (e.g., 900 = €9.00)
  currency: string;
  billing_interval: 'month' | 'year';
  trial_days: number;
  features: string[]; // Human-readable features
  max_concurrent_users: number;
  max_courses: number; // -1 = unlimited
  max_lab_sessions: number;
  is_active: boolean;
  required_role: string;

  // Terminal-specific limits
  max_session_duration_minutes: number;
  max_concurrent_terminals: number;
  allowed_machine_sizes: string[]; // ["XS", "S", "M", "L"]
  network_access_enabled: boolean;
  data_persistence_enabled: boolean;
  data_persistence_gb: number;
  allowed_templates: string[];

  // Tiered pricing (for bulk purchases)
  use_tiered_pricing: boolean;
  pricing_tiers?: PricingTier[];
}

interface PricingTier {
  min_quantity: number;
  max_quantity: number; // 0 = unlimited
  unit_amount: number; // Price per license in this tier (in cents)
  description?: string;
}
```

### 1.2 Get User's Current Subscription

```typescript
// GET /api/v1/user-subscriptions/current
const getCurrentSubscription = async (token: string) => {
  const response = await fetch('http://localhost:8080/api/v1/user-subscriptions/current', {
    headers: { 'Authorization': `Bearer ${token}` }
  });

  return await response.json(); // UserSubscriptionOutput
};
```

**Response Structure**:
```typescript
interface UserSubscriptionOutput {
  id: string;
  user_id: string;
  subscription_plan_id: string;
  subscription_plan: SubscriptionPlanOutput;
  stripe_subscription_id: string;
  stripe_customer_id: string;
  status: 'active' | 'trialing' | 'past_due' | 'canceled' | 'unpaid';
  subscription_type: 'personal' | 'assigned';
  is_primary: boolean; // True if this is the active subscription
  current_period_start: string; // ISO 8601
  current_period_end: string;
  trial_end?: string;
  cancel_at_period_end: boolean;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;

  // Bulk license info (if applicable)
  subscription_batch_id?: string;
  batch_owner_id?: string;
  batch_owner_name?: string;
  batch_owner_email?: string;
  assigned_at?: string;
}
```

### 1.3 Create Subscription (Stripe Checkout)

```typescript
// POST /api/v1/user-subscriptions/create-checkout-session
const createCheckoutSession = async (
  token: string,
  planId: string,
  successUrl: string,
  cancelUrl: string
) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/user-subscriptions/create-checkout-session',
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        subscription_plan_id: planId,
        success_url: successUrl,
        cancel_url: cancelUrl,
        allow_replace: true // Allow replacing free subscription with paid
      })
    }
  );

  const { url, session_id } = await response.json();

  // Redirect user to Stripe Checkout
  window.location.href = url;
};
```

### 1.4 Cancel Subscription

```typescript
// DELETE /api/v1/user-subscriptions/current
const cancelSubscription = async (token: string, cancelAtPeriodEnd: boolean = true) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/user-subscriptions/current',
    {
      method: 'DELETE',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        cancel_at_period_end: cancelAtPeriodEnd
      })
    }
  );

  return await response.json();
};
```

### 1.5 Upgrade/Downgrade Plan

```typescript
// POST /api/v1/user-subscriptions/current/upgrade
const upgradePlan = async (token: string, newPlanId: string) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/user-subscriptions/current/upgrade',
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        new_plan_id: newPlanId,
        proration_behavior: 'always_invoice' // 'always_invoice', 'create_prorations', or 'none'
      })
    }
  );

  return await response.json();
};
```

---

## Phase 2: Organization Subscriptions

### Overview

Organizations can subscribe to plans, and all members inherit the organization's features. Users can belong to multiple organizations and will inherit the **maximum** features across all organizations.

### 2.1 Create Organization

```typescript
// POST /api/v1/organizations
const createOrganization = async (
  token: string,
  name: string,
  displayName: string,
  description: string
) => {
  const response = await fetch('http://localhost:8080/api/v1/organizations', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      name,
      display_name: displayName,
      description,
      max_groups: 10,
      max_members: 50
    })
  });

  return await response.json(); // OrganizationOutput
};
```

**Response Structure**:
```typescript
interface OrganizationOutput {
  id: string;
  name: string;
  display_name: string;
  description: string;
  owner_user_id: string;
  subscription_plan_id?: string;
  is_personal: boolean;
  max_groups: number;
  max_members: number;
  is_active: boolean;
  metadata?: object;
  created_at: string;
  updated_at: string;

  // Counts (if preloaded)
  group_count?: number;
  member_count?: number;

  // Related data (if preloaded with ?includes=members,groups)
  members?: OrganizationMemberOutput[];
  groups?: GroupSummary[];
}

interface OrganizationMemberOutput {
  id: string;
  organization_id: string;
  user_id: string;
  role: 'owner' | 'manager' | 'member';
  invited_by: string;
  joined_at: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}
```

### 2.2 Subscribe Organization to Plan

```typescript
// POST /api/v1/organizations/:orgId/subscribe
const subscribeOrganization = async (
  token: string,
  orgId: string,
  planId: string,
  paymentMethodId?: string
) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/subscribe`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        subscription_plan_id: planId,
        payment_method_id: paymentMethodId,
        quantity: 1 // For organization subscriptions, always 1
      })
    }
  );

  return await response.json(); // OrganizationSubscriptionOutput
};
```

**Response Structure**:
```typescript
interface OrganizationSubscriptionOutput {
  id: string;
  organization_id: string;
  subscription_plan_id: string;
  subscription_plan: SubscriptionPlanOutput;
  stripe_subscription_id: string;
  stripe_customer_id: string;
  status: 'active' | 'trialing' | 'past_due' | 'canceled' | 'pending_payment';
  current_period_start: string;
  current_period_end: string;
  cancel_at_period_end: boolean;
  quantity: number; // Always 1 for org subscriptions
  created_at: string;
  updated_at: string;
}
```

### 2.3 Get Organization Subscription

```typescript
// GET /api/v1/organizations/:orgId/subscription
const getOrganizationSubscription = async (token: string, orgId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/subscription`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // OrganizationSubscriptionOutput
};
```

### 2.4 Get Organization Features

```typescript
// GET /api/v1/organizations/:orgId/features
const getOrganizationFeatures = async (token: string, orgId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/features`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json();
};
```

**Response Structure**:
```typescript
interface OrganizationFeaturesOutput {
  organization_id: string;
  organization_name: string;
  subscription_plan?: SubscriptionPlanOutput;
  has_active_subscription: boolean;
  features: string[];
  usage_limits: UsageLimits;
}

interface UsageLimits {
  max_concurrent_terminals: number;
  max_session_duration_minutes: number;
  max_courses: number;
  allowed_machine_sizes: string[];
  network_access_enabled: boolean;
  data_persistence_enabled: boolean;
  data_persistence_gb: number;
}
```

### 2.5 Get User's Effective Features (Aggregated)

**IMPORTANT**: Users can belong to multiple organizations. Their effective features are the **maximum** across all organizations.

```typescript
// GET /api/v1/users/me/features
const getUserEffectiveFeatures = async (token: string) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/users/me/features',
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json();
};
```

**Response Structure**:
```typescript
interface UserEffectiveFeaturesOutput {
  user_id: string;
  effective_features: SubscriptionPlanOutput; // Aggregated maximum features
  source_organizations: OrganizationFeatureSource[];
  has_personal_subscription: boolean;
  personal_subscription?: UserSubscriptionOutput;
}

interface OrganizationFeatureSource {
  organization_id: string;
  organization_name: string;
  role: 'owner' | 'manager' | 'member';
  contributing_features: string[];
}
```

### 2.6 Manage Organization Members

```typescript
// GET /api/v1/organizations/:orgId?includes=members
const getOrganizationMembers = async (token: string, orgId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}?includes=members`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  const org = await response.json();
  return org.members; // OrganizationMemberOutput[]
};

// To add/remove members, use organization management endpoints
// POST /api/v1/organizations/:orgId/members
// DELETE /api/v1/organizations/:orgId/members/:userId
```

### 2.7 Cancel Organization Subscription

```typescript
// DELETE /api/v1/organizations/:orgId/subscription
const cancelOrganizationSubscription = async (
  token: string,
  orgId: string,
  cancelAtPeriodEnd: boolean = true
) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/subscription`,
    {
      method: 'DELETE',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        cancel_at_period_end: cancelAtPeriodEnd
      })
    }
  );

  return await response.json();
};
```

---

## Phase 3: Bulk License Management

### Overview

Bulk licenses allow purchasing multiple subscriptions at once and assigning them to specific users. Perfect for corporate training, classroom management, or reselling licenses.

### 3.1 Purchase Bulk Licenses

**Option A: Direct Purchase with Payment Method**

```typescript
// POST /api/v1/user-subscriptions/purchase-bulk
const purchaseBulkLicenses = async (
  token: string,
  planId: string,
  quantity: number,
  paymentMethodId?: string,
  groupId?: string
) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/user-subscriptions/purchase-bulk',
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        subscription_plan_id: planId,
        quantity,
        payment_method_id: paymentMethodId, // Optional
        group_id: groupId // Optional: link to a group
      })
    }
  );

  return await response.json(); // SubscriptionBatchOutput
};
```

**Option B: Stripe Checkout Session (Recommended)**

```typescript
// POST /api/v1/subscription-batches/create-checkout-session
const createBulkCheckoutSession = async (
  token: string,
  planId: string,
  quantity: number,
  successUrl: string,
  cancelUrl: string,
  groupId?: string
) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/subscription-batches/create-checkout-session',
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        subscription_plan_id: planId,
        quantity,
        success_url: successUrl,
        cancel_url: cancelUrl,
        group_id: groupId
      })
    }
  );

  const { url, session_id } = await response.json();
  window.location.href = url;
};
```

**Response Structure**:
```typescript
interface SubscriptionBatchOutput {
  id: string;
  purchaser_user_id: string;
  subscription_plan_id: string;
  subscription_plan: SubscriptionPlanOutput;
  group_id?: string;
  stripe_subscription_id: string;
  stripe_subscription_item_id: string;
  total_quantity: number;
  assigned_quantity: number;
  available_quantity: number; // Calculated: total - assigned
  status: 'pending_payment' | 'active' | 'cancelled';
  current_period_start: string;
  current_period_end: string;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;
}
```

### 3.2 List User's Batches

```typescript
// GET /api/v1/subscription-batches
const getMyBatches = async (token: string) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/subscription-batches',
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // SubscriptionBatchOutput[]
};
```

### 3.3 Get Batch Details

```typescript
// GET /api/v1/subscription-batches/:batchId
const getBatchDetails = async (token: string, batchId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // SubscriptionBatchOutput
};
```

### 3.4 Get Licenses in Batch

```typescript
// GET /api/v1/subscription-batches/:batchId/licenses
const getBatchLicenses = async (token: string, batchId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/licenses`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // UserSubscriptionOutput[]
};
```

### 3.5 Assign License to User

```typescript
// POST /api/v1/subscription-batches/:batchId/assign
const assignLicense = async (token: string, batchId: string, userId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/assign`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        user_id: userId
      })
    }
  );

  return await response.json(); // UserSubscriptionOutput
};
```

### 3.6 Revoke License Assignment

**CRITICAL**: Revoking a license will **terminate all active terminals** for that user!

```typescript
// DELETE /api/v1/subscription-batches/:batchId/licenses/:licenseId/revoke
const revokeLicense = async (token: string, batchId: string, licenseId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/licenses/${licenseId}/revoke`,
    {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json();
};
```

### 3.7 Update Batch Quantity

**IMPORTANT**: You cannot reduce quantity below the number of assigned licenses!

```typescript
// PATCH /api/v1/subscription-batches/:batchId/quantity
const updateBatchQuantity = async (
  token: string,
  batchId: string,
  newQuantity: number
) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/quantity`,
    {
      method: 'PATCH',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        new_quantity: newQuantity
      })
    }
  );

  return await response.json();
};
```

### 3.8 Permanently Delete Batch

**CRITICAL**: This will:
1. Cancel Stripe subscription
2. Terminate all terminals for users with assigned licenses
3. Delete all licenses
4. Delete the batch record

```typescript
// DELETE /api/v1/subscription-batches/:batchId/permanent
const permanentlyDeleteBatch = async (token: string, batchId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/permanent`,
    {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json();
};
```

---

## Feature Detection & Limits

### Checking User Features

```typescript
// Use the effective features endpoint for accurate feature detection
const canUserCreateTerminal = async (token: string) => {
  const features = await getUserEffectiveFeatures(token);

  // Check if user has terminal access
  return features.effective_features.max_concurrent_terminals > 0;
};

const getMaxConcurrentTerminals = async (token: string) => {
  const features = await getUserEffectiveFeatures(token);
  return features.effective_features.max_concurrent_terminals;
};
```

### Usage Metrics

```typescript
// GET /api/v1/usage-metrics
const getUserUsageMetrics = async (token: string) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/usage-metrics',
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // UsageMetricsOutput[]
};
```

**Response Structure**:
```typescript
interface UsageMetricsOutput {
  id: string;
  user_id: string;
  metric_type: 'concurrent_terminals' | 'courses' | 'lab_sessions';
  current_value: number;
  limit_value: number;
  period_start: string;
  period_end: string;
  last_updated: string;
  usage_percent: number; // Calculated
}
```

---

## Stripe Integration

### Webhook Events

The backend handles these Stripe webhook events automatically:

#### Individual Subscriptions
- `customer.subscription.created` → Create user subscription
- `customer.subscription.updated` → Update subscription status
- `customer.subscription.deleted` → Cancel subscription, terminate terminals
- `invoice.payment_succeeded` → Activate subscription
- `invoice.payment_failed` → Suspend subscription

#### Organization Subscriptions
- `customer.subscription.created` (with `organization_id` metadata) → Create org subscription
- `customer.subscription.updated` → Update org subscription
- `customer.subscription.deleted` → Cancel org subscription

#### Bulk Licenses
- `customer.subscription.created` (with `bulk_purchase: "true"` metadata) → Create batch & licenses
- `customer.subscription.updated` → Handle quantity changes
- `customer.subscription.deleted` → Cancel batch, revoke all licenses, terminate all terminals
- `invoice.payment_succeeded` → Activate licenses (change from `pending_payment` to `unassigned`)

### Metadata Structure

**Individual Subscription**:
```json
{
  "user_id": "uuid",
  "subscription_plan_id": "uuid"
}
```

**Organization Subscription**:
```json
{
  "organization_id": "uuid",
  "subscription_plan_id": "uuid",
  "user_id": "uuid" (purchaser)
}
```

**Bulk License**:
```json
{
  "bulk_purchase": "true",
  "user_id": "uuid" (purchaser),
  "subscription_plan_id": "uuid",
  "quantity": "5",
  "group_id": "uuid" (optional)
}
```

---

## Error Handling

### Common Error Responses

```typescript
interface APIError {
  error_code: number;
  error_message: string;
  details?: {
    field?: string;
    operation?: string;
    original?: string;
  };
}
```

### Error Codes

| Code | Meaning | Common Causes |
|------|---------|---------------|
| 400  | Bad Request | Invalid input data, validation errors |
| 401  | Unauthorized | Missing or invalid token |
| 403  | Forbidden | Insufficient permissions, not organization member |
| 404  | Not Found | Resource doesn't exist |
| 409  | Conflict | Already subscribed, duplicate resource |
| 500  | Internal Server Error | Backend failure, database issues |

### Example Error Handling

```typescript
const handleSubscriptionError = (error: APIError) => {
  switch (error.error_code) {
    case 400:
      if (error.error_message.includes('validation')) {
        return 'Please check your input fields';
      }
      break;

    case 403:
      if (error.error_message.includes('not a member')) {
        return 'You must be a member of this organization';
      }
      break;

    case 409:
      if (error.error_message.includes('already subscribed')) {
        return 'You already have an active subscription';
      }
      break;

    default:
      return 'An unexpected error occurred. Please try again.';
  }
};
```

---

## Complete Use Cases

### Use Case 1: Individual User Subscribes

```typescript
const individualSubscribeFlow = async (token: string) => {
  // 1. Get available plans
  const plans = await getSubscriptionPlans(token);

  // 2. Display plans to user, let them choose
  const selectedPlan = plans.find(p => p.name === 'Pro');

  // 3. Create Stripe checkout session
  await createCheckoutSession(
    token,
    selectedPlan.id,
    'https://app.example.com/subscription/success',
    'https://app.example.com/subscription/cancel'
  );

  // User is redirected to Stripe, completes payment
  // Webhook activates subscription in background

  // 4. After redirect back, check subscription status
  const subscription = await getCurrentSubscription(token);
  console.log('Subscription status:', subscription.status);
};
```

### Use Case 2: Organization Subscribes, Members Inherit Features

```typescript
const organizationSubscribeFlow = async (token: string) => {
  // 1. Create organization
  const org = await createOrganization(
    token,
    'acme-corp',
    'ACME Corporation',
    'Training organization for ACME employees'
  );

  // 2. Wait for organization setup (owner membership created async)
  await new Promise(resolve => setTimeout(resolve, 2000));

  // 3. Subscribe organization to plan
  const orgSub = await subscribeOrganization(
    token,
    org.id,
    'organization-plan-id'
  );

  // 4. Add team members to organization
  // (Use organization member management endpoints)

  // 5. Members automatically inherit org features
  const memberFeatures = await getUserEffectiveFeatures(memberToken);
  console.log('Member can use:', memberFeatures.effective_features);
};
```

### Use Case 3: Bulk License Purchase & Assignment

```typescript
const bulkLicenseFlow = async (token: string) => {
  // 1. Create Stripe checkout for bulk purchase
  await createBulkCheckoutSession(
    token,
    'plan-id',
    10, // Buy 10 licenses
    'https://app.example.com/bulk/success',
    'https://app.example.com/bulk/cancel'
  );

  // Webhook creates batch and licenses after payment

  // 2. Get purchased batches
  const batches = await getMyBatches(token);
  const batch = batches[0]; // Most recent

  // 3. View available licenses
  const licenses = await getBatchLicenses(token, batch.id);
  const unassigned = licenses.filter(l => l.status === 'unassigned');

  console.log(`${unassigned.length} licenses available to assign`);

  // 4. Assign license to a user
  const assignedLicense = await assignLicense(
    token,
    batch.id,
    'target-user-id'
  );

  console.log('License assigned:', assignedLicense.id);

  // 5. Target user can now use the subscription
  const userSub = await getCurrentSubscription(targetUserToken);
  console.log('User subscription type:', userSub.subscription_type); // 'assigned'
};
```

### Use Case 4: Display User's Effective Features (Multi-Org)

```typescript
const displayUserFeatures = async (token: string) => {
  const userFeatures = await getUserEffectiveFeatures(token);

  // Show aggregated features
  console.log('Your effective features:');
  console.log('- Max terminals:', userFeatures.effective_features.max_concurrent_terminals);
  console.log('- Session duration:', userFeatures.effective_features.max_session_duration_minutes, 'minutes');
  console.log('- Allowed machines:', userFeatures.effective_features.allowed_machine_sizes);

  // Show source organizations
  console.log('\nYour organizations:');
  for (const org of userFeatures.source_organizations) {
    console.log(`- ${org.organization_name} (${org.role})`);
  }

  // Show personal subscription if exists
  if (userFeatures.has_personal_subscription) {
    console.log('\nPersonal subscription:', userFeatures.personal_subscription.subscription_plan.name);
  }
};
```

---

## API Reference

### Base URL
```
http://localhost:8080/api/v1
```

### Individual Subscriptions

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/subscription-plans` | List all plans |
| GET | `/user-subscriptions/current` | Get current subscription |
| POST | `/user-subscriptions/create-checkout-session` | Create Stripe checkout |
| POST | `/user-subscriptions/current/upgrade` | Upgrade/downgrade plan |
| DELETE | `/user-subscriptions/current` | Cancel subscription |
| GET | `/usage-metrics` | Get usage metrics |

### Organization Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/organizations` | Create organization |
| GET | `/organizations/:id` | Get organization details |
| GET | `/organizations/:id?includes=members,groups` | Get with relationships |
| PATCH | `/organizations/:id` | Update organization |
| DELETE | `/organizations/:id` | Delete organization |

### Organization Subscriptions

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/organizations/:id/subscribe` | Subscribe organization |
| GET | `/organizations/:id/subscription` | Get org subscription |
| DELETE | `/organizations/:id/subscription` | Cancel org subscription |
| GET | `/organizations/:id/features` | Get org features |
| GET | `/organizations/:id/usage-limits` | Get org usage limits |
| GET | `/users/me/features` | Get effective features (aggregated) |

### Bulk Licenses

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/subscription-batches/create-checkout-session` | Create bulk checkout |
| POST | `/user-subscriptions/purchase-bulk` | Direct bulk purchase |
| GET | `/subscription-batches` | List my batches |
| GET | `/subscription-batches/:id` | Get batch details |
| GET | `/subscription-batches/:id/licenses` | List licenses in batch |
| POST | `/subscription-batches/:id/assign` | Assign license to user |
| DELETE | `/subscription-batches/:id/licenses/:licenseId/revoke` | Revoke license |
| PATCH | `/subscription-batches/:id/quantity` | Update batch quantity |
| DELETE | `/subscription-batches/:id/permanent` | Permanently delete batch |

---

## Important Notes

### 1. Personal Organizations
Every user automatically gets a personal organization on first login/registration. This is used for backward compatibility with individual subscriptions.

### 2. Feature Aggregation
Users in multiple organizations inherit the **maximum** features across all organizations. For example:
- Org A: 5 concurrent terminals
- Org B: 10 concurrent terminals
- User's effective limit: **10 concurrent terminals**

### 3. License Status Flow
```
pending_payment → unassigned → (assign to user) → active → (revoke) → unassigned
                                                           → cancelled
```

### 4. Terminal Termination
When licenses are revoked or subscriptions cancelled, **all active terminals for affected users are automatically terminated** to enforce feature limits immediately.

### 5. Stripe Incomplete Status
You cannot modify subscription quantity when it's in `incomplete` status (payment pending). Wait for `invoice.payment_succeeded` webhook to activate the subscription first.

### 6. Authorization
- **Organization subscriptions**: Only owners and managers can manage billing
- **Bulk licenses**: Only the purchaser can assign/revoke licenses
- **Organization members**: All members can view features but not manage billing

---

## Testing & Development

### Test Script Locations
- **Individual subscriptions**: `/tmp/test_user_subscription.sh`
- **Organization subscriptions**: `/tmp/test_org_subscription.sh`
- **Bulk licenses**: `/tmp/test_bulk_licenses.sh`

### Swagger Documentation
Full API documentation with request/response schemas:
```
http://localhost:8080/swagger/index.html
```

### Test Credentials
```
Email: 1.supervisor@test.com
Password: test
```

---

## Questions & Support

For implementation questions or issues:
1. Check Swagger documentation at `/swagger/`
2. Review test scripts in `/tmp/`
3. Check server logs at `/tmp/server.log` or `/tmp/server_bulk_test.log`

---

**Document Version**: 1.0
**Last Updated**: 2025-10-25
**Backend Implementation Status**: ✅ All 3 Phases Complete
