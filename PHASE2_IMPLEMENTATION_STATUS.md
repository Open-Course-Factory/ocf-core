# Phase 2 Implementation Status

## Overview
Phase 2 moves subscription management from individual users/groups to organizations. Organizations subscribe to plans, and all members inherit the features.

## ✅ Completed Tasks

### 1. Database Schema Changes
- ✅ Added `OrganizationSubscription` model in `src/payment/models/subscription.go`
  - Links organizations to subscription plans
  - Tracks Stripe subscription and customer IDs
  - Supports quantity (number of seats/licenses)
  - Matches UserSubscription structure for consistency

- ✅ Marked `UserSubscription` as **DEPRECATED** (kept for backward compatibility)

- ✅ Added `OrganizationSubscription` to database migrations (`src/initialization/database.go`)

- ✅ Deprecated `SubscriptionPlanID` in `ClassGroup` model
  - Added comment: "DEPRECATED Phase 2: Use Organization.SubscriptionPlanID instead"
  - Field kept for backward compatibility but should not be used for new groups

### 2. Organization Model (from Phase 1)
- ✅ Organization already has `SubscriptionPlanID` field (added in Phase 1)
- ✅ Organization already has relationships to Groups and Members

## 🚧 Remaining Tasks

### 3. ✅ Create OrganizationSubscriptionService (COMPLETED)
**File**: `src/payment/services/organizationSubscriptionService.go` ✅

**Implemented Methods**:
- ✅ GetOrganizationSubscription(orgID)
- ✅ GetOrganizationSubscriptionByID(id)
- ✅ CreateOrganizationSubscription(orgID, planID, ownerUserID)
- ✅ UpdateOrganizationSubscription(orgID, planID)
- ✅ CancelOrganizationSubscription(orgID, cancelAtPeriodEnd)
- ✅ GetOrganizationFeatures(orgID)
- ✅ CanOrganizationAccessFeature(orgID, feature)
- ✅ GetOrganizationUsageLimits(orgID)
- ✅ GetUserEffectiveFeatures(userID) - Aggregates across all user's organizations
- ✅ CanUserAccessFeature(userID, feature)
- ✅ GetUserOrganizationWithFeature(userID, feature)

**Repository**: `src/payment/repositories/organizationSubscriptionRepository.go` ✅
- ✅ CreateOrganizationSubscription
- ✅ GetOrganizationSubscription
- ✅ GetOrganizationSubscriptionByOrgID
- ✅ GetOrganizationSubscriptionByStripeID
- ✅ GetActiveOrganizationSubscription
- ✅ GetUserOrganizationSubscriptions
- ✅ UpdateOrganizationSubscription
- ✅ DeleteOrganizationSubscription

### 4. ✅ Create Feature Access Helpers (COMPLETED)
**File**: `src/payment/utils/featureAccess.go` ✅

**Implemented Functions**:
- ✅ GetUserEffectiveFeatures(db, userID) - Returns highest-tier plan
- ✅ CanUserAccessFeature(db, userID, feature) - Checks feature across all orgs
- ✅ GetUserOrganizationWithFeature(db, userID, feature) - Finds org providing feature
- ✅ GetUserEffectiveLimits(db, userID) - Returns max limits across orgs

**Logic Implemented**:
1. ✅ Get all user's organizations (via OrganizationMember)
2. ✅ Get subscription for each organization
3. ✅ Aggregate features (take maximum limits, union of feature flags)
4. ✅ Return combined feature set

### 5. ✅ Update Payment Middleware (COMPLETED)
**Files Updated**:
- ✅ `src/payment/middleware/featureMiddleware.go` - Now checks org subscriptions first
- ✅ `src/payment/middleware/usageLimitMiddleware.go` - Aggregates limits across orgs

**Changes Implemented**:
```go
// Phase 2: Check organization subscriptions first
hasFeature, err := orgSubService.CanUserAccessFeature(userID, feature)
if err == nil && hasFeature {
    // User has access via organization subscription
    return allowed
}

// Backward compatibility: Fall back to user subscription (deprecated)
subscription, err := paymentRepo.GetActiveUserSubscription(userID)
```

**Updated Middleware Functions**:
- ✅ `RequireFeature(feature string)` - Checks org subscriptions first, falls back to user subs
- ✅ `CheckCustomLimit(metricType, increment)` - Uses aggregated limits from orgs
- ✅ `CheckCourseCreationLimit()` - Uses org-level limits
- ✅ `CheckTerminalCreationLimit()` - Uses org-level limits
- ✅ `CheckConcurrentTerminalsLimit()` - Uses org-level limits

**Backward Compatibility**:
- ✅ All middleware functions first try organization subscriptions
- ✅ If no org subscriptions found, fall back to deprecated UserSubscription
- ✅ Existing user subscriptions continue to work

### 6. Update Stripe Webhooks
**File**: `src/payment/webhooks/stripeWebhooks.go`

**Webhook Handlers to Update**:

**`customer.subscription.created`**:
```go
// Check metadata for organization_id
if orgID := event.Data.Object.Metadata["organization_id"]; orgID != "" {
    // Create OrganizationSubscription record
    orgSubscriptionService.CreateFromStripeEvent(event)
} else {
    // Legacy: Create UserSubscription (for backward compat)
    userSubscriptionService.CreateFromStripeEvent(event)
}
```

**`customer.subscription.updated`**:
```go
// Update OrganizationSubscription status/period
if orgSub := findOrgSubscriptionByStripeID(event.Data.Object.ID); orgSub != nil {
    orgSubscriptionService.UpdateFromStripeEvent(event)
}
```

**`customer.subscription.deleted`**:
```go
// Cancel OrganizationSubscription
if orgSub := findOrgSubscriptionByStripeID(event.Data.Object.ID); orgSub != nil {
    orgSubscriptionService.CancelFromStripeEvent(event)
}
```

**`invoice.payment_succeeded`** & **`invoice.payment_failed`**:
- Update OrganizationSubscription.LastInvoiceID
- Send notification to organization owner

### 7. Update Usage Metrics
**File**: `src/payment/models/subscription.go`

**Add New Model**:
```go
// OrganizationUsageMetrics tracks organization-level usage
type OrganizationUsageMetrics struct {
    BaseModel
    OrganizationID     uuid.UUID `gorm:"type:uuid;not null;index"`
    SubscriptionID     uuid.UUID `gorm:"type:uuid;not null"`
    MetricType         string    // courses_created, active_users, total_terminals, storage_used
    CurrentValue       int64
    LimitValue         int64     // -1 = unlimited
    PeriodStart        time.Time
    PeriodEnd          time.Time
    LastUpdated        time.Time
}
```

**Update Metric Tracking**:
- When user creates course → increment org metric
- When user creates terminal → increment org metric
- Periodic job to calculate active_users per org

### 8. Migration Script for Existing Data
**File**: `src/initialization/migrateUserSubscriptionsToOrgs.go` (new file)

**Migration Logic**:
```go
func MigrateUserSubscriptionsToOrganizations(db *gorm.DB) error {
    // 1. Get all active UserSubscriptions
    var userSubs []models.UserSubscription
    db.Where("status = ?", "active").Find(&userSubs)

    // 2. For each subscription:
    for _, userSub := range userSubs {
        // Get or create personal organization for user
        org := getOrCreatePersonalOrganization(userSub.UserID)

        // Create OrganizationSubscription from UserSubscription
        orgSub := &models.OrganizationSubscription{
            OrganizationID:       org.ID,
            SubscriptionPlanID:   userSub.SubscriptionPlanID,
            StripeSubscriptionID: userSub.StripeSubscriptionID,
            StripeCustomerID:     userSub.StripeCustomerID,
            Status:               userSub.Status,
            CurrentPeriodStart:   userSub.CurrentPeriodStart,
            CurrentPeriodEnd:     userSub.CurrentPeriodEnd,
            // ... copy other fields
        }
        db.Create(orgSub)

        // Update Organization.SubscriptionPlanID
        db.Model(org).Update("subscription_plan_id", userSub.SubscriptionPlanID)
    }

    return nil
}
```

**Run Migration**:
Add to `main.go` initialization (one-time only):
```go
if os.Getenv("RUN_SUBSCRIPTION_MIGRATION") == "true" {
    initialization.MigrateUserSubscriptionsToOrganizations(sqldb.DB)
}
```

### 9. Update Organization Service
**File**: `src/organizations/services/organizationService.go`

**Add Methods**:
```go
// GetOrganizationSubscription returns the active subscription for an organization
func (os *organizationService) GetOrganizationSubscription(orgID uuid.UUID) (*paymentModels.OrganizationSubscription, error)

// GetOrganizationFeatures returns the subscription plan features for an organization
func (os *organizationService) GetOrganizationFeatures(orgID uuid.UUID) (*paymentModels.SubscriptionPlan, error)
```

### 10. Update Group Service
**File**: `src/groups/services/groupService.go`

**Change CreateGroup Logic**:
```go
// OLD: Use group.SubscriptionPlanID
if group.SubscriptionPlanID != nil {
    features := getFeatures(group.SubscriptionPlanID)
}

// NEW: Use organization.SubscriptionPlanID
if group.OrganizationID != nil {
    org := getOrganization(group.OrganizationID)
    if org.SubscriptionPlanID != nil {
        features := getFeatures(org.SubscriptionPlanID)
    }
}
```

## 📋 Testing Checklist

### Test Scenarios
- [ ] Create organization subscription via Stripe
- [ ] Organization members can access features from org subscription
- [ ] User in multiple orgs gets highest-tier features
- [ ] Organization owner can manage subscription
- [ ] Stripe webhooks update organization subscription correctly
- [ ] Usage limits enforced at organization level
- [ ] Migration script moves user subscriptions to personal orgs
- [ ] Legacy user subscriptions still work (backward compat)
- [ ] Group creation respects org subscription limits
- [ ] Terminal creation respects org subscription limits

### API Endpoints to Test
```bash
# Create organization subscription
POST /api/v1/organizations/{orgID}/subscribe
{
  "plan_id": "uuid",
  "payment_method_id": "pm_xxx"
}

# Get organization subscription
GET /api/v1/organizations/{orgID}/subscription

# Get user's effective features
GET /api/v1/users/me/features

# Cancel organization subscription
DELETE /api/v1/organizations/{orgID}/subscription
```

## 🔄 Next Steps

1. **Implement OrganizationSubscriptionService** (highest priority)
2. **Create Feature Access Helpers** (critical for middleware)
3. **Update Payment Middleware** (required for feature enforcement)
4. **Update Stripe Webhooks** (required for Stripe integration)
5. **Add Migration Script** (for existing data)
6. **Update Usage Metrics** (for org-level tracking)
7. **Test End-to-End** (full user flow)
8. **Update API Documentation** (Swagger)

## 📝 Notes

- **Backward Compatibility**: UserSubscription model kept for existing subscriptions
- **Gradual Migration**: Old subscriptions work until migrated to organizations
- **Feature Aggregation**: Users inherit features from ALL organizations they belong to
- **Billing Simplification**: One subscription per organization (not per user)
- **Usage Tracking**: Metrics tracked at organization level, not user level

## ⚠️ Breaking Changes

**None** - Phase 2 is fully backward compatible:
- Existing UserSubscriptions continue to work
- Groups without OrganizationID still function
- New subscriptions use OrganizationSubscription
- Migration can be done gradually

## 🎯 Success Criteria

Phase 2 is complete when:
1. ✅ Organizations can subscribe to plans via Stripe
2. ✅ Members inherit features from organization subscriptions
3. ✅ Feature limits enforced at organization level
4. ✅ Stripe webhooks handle organization subscriptions
5. ✅ Existing user subscriptions migrated to personal organizations
6. ✅ All tests pass
7. ✅ API documentation updated
