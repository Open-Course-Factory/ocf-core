# Terminal Pricing Plan - Implementation Guide

## Overview

This document describes the terminal-focused pricing strategy for OCF Core.

**Philosophy:**
- **Free = Trial only** (1 hour sessions, no network/storage, 3 tests total)
- **Paid = Real use** (8 hour sessions, network, storage, restarts)
- **Simple billing** (no add-ons, all-inclusive pricing)

## Plan Structure

### 1. Free Plan - "Trial"
**Target:** Testing the platform before committing

**Price:** €0/month

**Terminal Limits:**
- `max_terminal_sessions_per_month`: -1 (unlimited restarts - no limit!)
- `max_session_duration_minutes`: 60 (1 hour - test only)
- `allowed_machine_sizes`: ["small"]
- `max_concurrent_terminals`: 1 (only 1 at a time)
- `network_access_enabled`: false (no external access)
- `data_persistence_enabled`: false (ephemeral only)
- `data_persistence_gb`: 0 (no storage)
- `allowed_templates`: ["ubuntu-basic", "alpine-basic"]

**Restrictions:**
- Only 1 terminal at a time (must stop before starting another)
- 1 hour max per session (auto-terminates after 60min)
- No network, no storage - completely ephemeral

**Stripe Configuration:**
```json
{
  "product_name": "OCF Developer",
  "price": 0,
  "currency": "eur",
  "billing_interval": "month",
  "required_role": "member"
}
```

---

### 2. Solo Plan - "Individual Learner"
**Target:** Students, hobbyists, individual learning

**Price:** €9/month (affordable entry point)

**Terminal Limits:**
- `max_terminal_sessions_per_month`: -1 (unlimited restarts - no limit!)
- `max_session_duration_minutes`: 480 (8 hours - full work day)
- `allowed_machine_sizes`: ["small"]
- `max_concurrent_terminals`: 1 (only 1 at a time)
- `network_access_enabled`: true (included - can install packages!)
- `data_persistence_enabled`: true
- `data_persistence_gb`: 2 (enough for personal configs + small projects)
- `allowed_templates`: ["ubuntu-basic", "ubuntu-dev", "alpine-basic", "debian-basic", "python", "nodejs", "docker"]

**Key benefits:**
- Full-featured terminal (network + storage)
- 8 hour sessions (no rushing)
- Unlimited restarts throughout the day
- Affordable for individuals (€9/mo)
- Perfect for learning, personal projects

**Stripe Configuration:**
```json
{
  "product_name": "OCF Solo",
  "price": 900,
  "currency": "eur",
  "billing_interval": "month",
  "required_role": "member"
}
```

---

### 3. Trainer Plan - "Professional Trainer"
**Target:** Individual trainers, small training sessions

**Price:** €19/month (all-inclusive, no add-ons)

**Terminal Limits:**
- `max_terminal_sessions_per_month`: -1 (unlimited restarts - no limit!)
- `max_session_duration_minutes`: 480 (8 hours - full training day)
- `allowed_machine_sizes`: ["small", "medium"]
- `max_concurrent_terminals`: 3 (up to 3 running at once)
- `network_access_enabled`: true (included - essential for training)
- `data_persistence_enabled`: true
- `data_persistence_gb`: 5 (plenty for config files + small projects)
- `allowed_templates`: ["ubuntu-basic", "ubuntu-dev", "alpine-basic", "debian-basic", "python", "nodejs", "docker"]

**Key benefits:**
- Unlimited restarts throughout the day
- Up to 3 concurrent terminals
- Network + storage included, no surprise costs

**Stripe Configuration:**
```json
{
  "product_name": "OCF Trainer",
  "price": 1900,
  "currency": "eur",
  "billing_interval": "month",
  "required_role": "trainer",
  "metadata": {
    "addon_network_price_id": "price_xxx",
    "addon_storage_price_id": "price_yyy"
  }
}
```

---

### 4. Organization Plan - "Team Training"
**Target:** Organizations, training companies, schools

**Price:** €49/month (all-inclusive, simple billing)

**Terminal Limits:**
- `max_terminal_sessions_per_month`: -1 (unlimited restarts)
- `max_session_duration_minutes`: 480 (8 hours - same as other plans for consistency)
- `allowed_machine_sizes`: ["small", "medium", "large"]
- `max_concurrent_terminals`: 10 (enough for small training groups)
- `network_access_enabled`: true
- `data_persistence_enabled`: true
- `data_persistence_gb`: 20 (generous for config files)
- `allowed_templates`: ["all"] + custom Docker images

**Key benefits:**
- Unlimited restarts throughout the day
- 10 concurrent terminals for group training
- All machine sizes for any training scenario
- Custom Docker images for specialized training

**Stripe Configuration:**
```json
{
  "product_name": "OCF Organization",
  "price": 4900,
  "currency": "eur",
  "billing_interval": "month",
  "required_role": "organization",
  "metadata": {
    "addon_terminal_price_id": "price_xxx",
    "addon_storage_price_id": "price_yyy"
  }
}
```

---

## Machine Size Definitions

### Small
- CPU: 1 core
- RAM: 512MB
- Disk: 5GB ephemeral
- Use cases: Basic CLI, simple scripts
- Cost multiplier: 1x

### Medium
- CPU: 2 cores
- RAM: 2GB
- Disk: 10GB ephemeral
- Use cases: Development, compilation, Docker
- Cost multiplier: 2x

### Large
- CPU: 4 cores
- RAM: 4GB
- Disk: 20GB ephemeral
- Use cases: Heavy workloads, multiple containers
- Cost multiplier: 4x

---

## Implementation Database Schema Changes

### Enhanced SubscriptionPlan Model

Add these fields to `SubscriptionPlan`:

```go
// Terminal-specific limits
MaxTerminalSessionsPerMonth int      `gorm:"default:-1" json:"max_terminal_sessions_per_month"` // -1 = unlimited
MaxSessionDurationMinutes   int      `gorm:"default:60" json:"max_session_duration_minutes"`
MaxConcurrentTerminals      int      `gorm:"default:1" json:"max_concurrent_terminals"`
AllowedMachineSizes         []string `gorm:"serializer:json" json:"allowed_machine_sizes"` // ["small", "medium", "large"]
NetworkAccessEnabled        bool     `gorm:"default:false" json:"network_access_enabled"`
DataPersistenceEnabled      bool     `gorm:"default:false" json:"data_persistence_enabled"`
DataPersistenceGB           int      `gorm:"default:0" json:"data_persistence_gb"`
AllowedTemplates            []string `gorm:"serializer:json" json:"allowed_templates"`

// Add-on pricing (store Stripe Price IDs for metered billing)
AddonNetworkPriceID         *string  `gorm:"type:varchar(100)" json:"addon_network_price_id,omitempty"`
AddonStoragePriceID         *string  `gorm:"type:varchar(100)" json:"addon_storage_price_id,omitempty"`
AddonTerminalPriceID        *string  `gorm:"type:varchar(100)" json:"addon_terminal_price_id,omitempty"`
```

### New UsageMetrics Types

Add these metric types:
- `terminal_sessions_created` - Count of sessions started this month
- `terminal_session_minutes` - Total minutes consumed
- `terminal_storage_gb` - Current storage usage
- `concurrent_terminals` - Current active terminals
- `network_data_gb` - Network data transferred (future)

---

## Middleware Updates

### Terminal Session Creation Check

```go
// In usageLimitMiddleware.go
func (ulm *usageLimitMiddleware) CheckTerminalCreationLimit() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        userId := ctx.GetString("userId")

        // Check terminal sessions per month
        limitCheck, err := ulm.subscriptionService.CheckUsageLimit(
            userId,
            "terminal_sessions_created",
            1,
        )

        if !limitCheck.Allowed {
            ctx.JSON(http.StatusForbidden, gin.H{
                "error": limitCheck.Message,
                "upgrade_url": "/subscription/upgrade",
            })
            ctx.Abort()
            return
        }

        // Check concurrent terminals
        activeConcurrent, _ := ulm.subscriptionService.GetCurrentUsage(
            userId,
            "concurrent_terminals",
        )

        subscription, _ := ulm.subscriptionService.GetActiveUserSubscription(userId)
        plan, _ := ulm.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)

        if activeConcurrent >= plan.MaxConcurrentTerminals {
            ctx.JSON(http.StatusForbidden, gin.H{
                "error": fmt.Sprintf(
                    "Maximum concurrent terminals (%d) reached. Upgrade for more.",
                    plan.MaxConcurrentTerminals,
                ),
                "upgrade_url": "/subscription/upgrade",
            })
            ctx.Abort()
            return
        }

        ctx.Next()
    }
}
```

### Session Duration Enforcement

```go
// In terminal creation service
func (ts *TerminalService) CreateSession(userId, machineSize, template string) (*Terminal, error) {
    subscription, _ := ts.subscriptionService.GetActiveUserSubscription(userId)
    plan, _ := ts.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)

    // Check machine size allowed
    if !contains(plan.AllowedMachineSizes, machineSize) {
        return nil, fmt.Errorf("machine size %s not allowed in your plan", machineSize)
    }

    // Check template allowed
    if !contains(plan.AllowedTemplates, "all") && !contains(plan.AllowedTemplates, template) {
        return nil, fmt.Errorf("template %s not allowed in your plan", template)
    }

    // Create terminal with max duration
    terminal := &Terminal{
        UserID: userId,
        MachineSize: machineSize,
        Template: template,
        MaxDurationMinutes: plan.MaxSessionDurationMinutes,
        NetworkEnabled: plan.NetworkAccessEnabled,
        StorageGB: plan.DataPersistenceGB,
        ExpiresAt: time.Now().Add(time.Duration(plan.MaxSessionDurationMinutes) * time.Minute),
    }

    // Increment concurrent terminals metric
    ts.subscriptionService.IncrementUsage(userId, "concurrent_terminals", 1)

    return terminal, nil
}
```

---

## Stripe Configuration Script

### Creating Plans in Stripe

```bash
#!/bin/bash
# create_terminal_plans.sh

# Free Plan
stripe products create \
  --name "OCF Developer" \
  --description "Free plan for individual developers"

stripe prices create \
  --product prod_XXX \
  --unit-amount 0 \
  --currency eur \
  --recurring[interval]=month

# Starter Plan
stripe products create \
  --name "OCF Trainer" \
  --description "Perfect for individual trainers"

stripe prices create \
  --product prod_YYY \
  --unit-amount 1900 \
  --currency eur \
  --recurring[interval]=month

# Network Add-on (Starter)
stripe prices create \
  --product prod_YYY \
  --unit-amount 500 \
  --currency eur \
  --recurring[interval]=month \
  --metadata[addon]="network"

# Storage Add-on (metered)
stripe prices create \
  --product prod_YYY \
  --unit-amount 200 \
  --currency eur \
  --recurring[interval]=month \
  --recurring[usage_type]=metered

# Pro Plan
stripe products create \
  --name "OCF Organization" \
  --description "For organizations and training companies"

stripe prices create \
  --product prod_ZZZ \
  --unit-amount 4900 \
  --currency eur \
  --recurring[interval]=month
```

---

## User-Facing Plan Comparison

### For your website/pricing page:

| Feature | Trial (Free) | Trainer (€19/mo) | Organization (€49/mo) |
|---------|--------------|------------------|------------------------|
| **Price** | Free | €19/mo | €49/mo |
| **Best For** | Testing only | Individual trainers | Training companies |
| **Restarts** | Unlimited | Unlimited | Unlimited |
| **Session Duration** | 1 hour max | 8 hours max | 8 hours max |
| **Concurrent Terminals** | 1 only | 3 | 10 |
| **Machine Sizes** | Small only | Small, Medium | All (S/M/L) |
| **Templates** | 2 basic | 7 standard | All + custom Docker |
| **Network Access** | ❌ No | ✅ Included | ✅ Included |
| **Storage (configs)** | ❌ No | 5GB | 20GB |
| **Support** | None | Email | Priority |

**Pricing Philosophy:** No limits on restarts - only limits on concurrent terminals and session duration. Restart as many times as you need!

---

### Recommended 4-Tier Structure

Based on feedback, consider adding a **Solo** tier between Free and Trainer:

| Feature | Trial (Free) | **Solo (€9/mo)** | Trainer (€19/mo) | Organization (€49/mo) |
|---------|--------------|------------------|------------------|------------------------|
| **Price** | Free | **€9/mo** | €19/mo | €49/mo |
| **Best For** | Testing only | **Individual learning** | Individual trainers | Training companies |
| **Restarts** | Unlimited | **Unlimited** | Unlimited | Unlimited |
| **Session Duration** | 1 hour max | **8 hours max** | 8 hours max | 8 hours max |
| **Concurrent Terminals** | 1 only | **1 only** | 3 | 10 |
| **Machine Sizes** | Small only | **Small only** | Small, Medium | All (S/M/L) |
| **Templates** | 2 basic | **All standard** | All standard | All + custom Docker |
| **Network Access** | ❌ No | **✅ Included** | ✅ Included | ✅ Included |
| **Storage (configs)** | ❌ No | **2GB** | 5GB | 20GB |
| **Support** | None | **Email** | Email | Priority |

**Solo Plan Benefits:**
- Perfect for students, hobbyists, personal learning
- Full 8-hour sessions with network and storage
- Affordable price point (€9/mo)
- Only restriction: 1 terminal at a time (fine for individual use)

---

## Migration Steps

### 1. Database Migration
```bash
# Create migration for new SubscriptionPlan fields
go run scripts/create_terminal_plan_fields_migration.go
```

### 2. Seed Initial Plans
```bash
# Create the 3 plans in database + Stripe
go run scripts/seed_terminal_plans.go
```

### 3. Update Middleware
- Add terminal-specific limit checks
- Update usage tracking for new metrics

### 4. Update Frontend
- Display plan comparison
- Show current usage dashboard
- Add upgrade prompts when limits reached

---

## Next Steps

1. **Add new fields to SubscriptionPlan model**
2. **Create database migration**
3. **Write seed script to create plans in Stripe + DB**
4. **Update middleware to check terminal limits**
5. **Add usage tracking for terminal sessions**
6. **Create pricing page UI**
7. **Test subscription flow end-to-end**

---

## Questions to Consider

1. **Annual billing discount?** (e.g., 2 months free if paying yearly)
2. **Educational discounts?** (50% off for verified students/teachers)
3. **Grace period?** (e.g., 7 days after subscription expires before terminals are deleted)
4. **Free tier abuse prevention?** (require credit card verification, email verification, rate limiting)
5. **Overage pricing?** (allow going over limits with pay-per-use, or hard stop?)
