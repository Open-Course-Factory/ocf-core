# OCF Core Security & Production Readiness Roadmap

**Document Version:** 1.0
**Created:** 2025-11-02
**Status:** Active Development Roadmap
**Target Completion:** 4-6 weeks
**Estimated Effort:** 160-240 developer hours

---

## 📊 Executive Summary

This roadmap addresses critical security vulnerabilities and missing features identified in the comprehensive security audit. The current system scores **33% overall (47/125 items)** with critical gaps in:

- **CORS Security** - Allows any origin (CSRF vulnerability)
- **Rate Limiting** - Code exists but not enforced (DDoS vulnerable)
- **Feature Gates** - Logic inverted (blocks paid features)
- **Audit Logging** - Completely missing (compliance failure)
- **Quota Enforcement** - Partially implemented
- **Payment Security** - Missing 3D Secure (EU legal requirement)

**Priority:** Complete P0 (Critical) and P1 (High) items before production launch.

---

## 🎯 Roadmap Overview

### Phase 1: Critical Security Fixes (Week 1) - P0
**Blockers for ANY production deployment**
- [ ] Fix CORS configuration
- [ ] Implement rate limiting enforcement
- [ ] Fix feature gate logic bug
- [ ] Remove JWT from query parameters
- [ ] Migrate webhook replay protection to database
- [ ] Add 3D Secure to Stripe checkout
- [ ] Add basic audit logging infrastructure

**Deliverable:** System safe for controlled production rollout

### Phase 2: Quota & Resource Management (Week 2) - P1
**Revenue protection and abuse prevention**
- [ ] Implement member limit enforcement
- [ ] Implement terminal limit enforcement
- [ ] Add session duration auto-termination
- [ ] Verify Stripe quantity sync for bulk licenses
- [ ] Add proration to subscription changes

**Deliverable:** Resource quotas enforced, billing accurate

### Phase 3: Audit Logging & Compliance (Week 3) - P1
**Compliance and observability**
- [ ] Implement comprehensive audit logging
- [ ] Add security event tracking
- [ ] Add billing event tracking
- [ ] Add organization/group event tracking
- [ ] Implement log retention and cleanup

**Deliverable:** Full audit trail for compliance

### Phase 4: Testing & Polish (Week 4) - P1/P2
**Production hardening**
- [ ] Security testing suite
- [ ] Rate limit testing
- [ ] Payment flow testing (3D Secure)
- [ ] Input validation audit
- [ ] Transaction safety audit
- [ ] Performance testing

**Deliverable:** Production-ready system with 90%+ security score

---

## 🔴 PHASE 1: CRITICAL SECURITY FIXES (Week 1)

### Task 1.1: Fix CORS Configuration (2 hours)

**Priority:** P0 - CRITICAL
**Risk:** CSRF attacks, unauthorized API access
**File:** `main.go:104-111`

#### Current Code (INSECURE)
```go
r.Use(cors.New(cors.Options{
    AllowedOrigins:     []string{"*"},  // ⚠️ ALLOWS ANY ORIGIN
    AllowCredentials:   true,
    Debug:              true,
    AllowedMethods:     []string{"GET", "POST", "PUT", "PATCH", "OPTIONS", "DELETE"},
    AllowedHeaders:     []string{"*"},
    OptionsPassthrough: true,
}))
```

#### Fixed Code (SECURE)
```go
// Get allowed origins from environment
allowedOrigins := []string{
    os.Getenv("FRONTEND_URL"),           // e.g., https://app.yourdomain.com
    os.Getenv("ADMIN_FRONTEND_URL"),     // e.g., https://admin.yourdomain.com
}

// For local development, add localhost
if os.Getenv("ENVIRONMENT") == "development" {
    allowedOrigins = append(allowedOrigins,
        "http://localhost:3000",
        "http://localhost:5173",
    )
}

r.Use(cors.New(cors.Options{
    AllowedOrigins:   allowedOrigins,
    AllowCredentials: true,
    AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
    AllowedHeaders:   []string{
        "Authorization",
        "Content-Type",
        "Accept",
        "X-Requested-With",
    },
    ExposedHeaders: []string{
        "X-RateLimit-Limit",
        "X-RateLimit-Remaining",
        "X-RateLimit-Reset",
    },
    MaxAge: 300, // 5 minutes
}))
```

#### Environment Variables to Add
```bash
# .env
FRONTEND_URL=https://app.yourdomain.com
ADMIN_FRONTEND_URL=https://admin.yourdomain.com
ENVIRONMENT=production  # or development, staging
```

#### Testing
```bash
# Test that unauthorized origins are blocked
curl -H "Origin: https://evil.com" \
     -H "Access-Control-Request-Method: POST" \
     -X OPTIONS http://localhost:8080/api/v1/users

# Should NOT include Access-Control-Allow-Origin: https://evil.com

# Test that authorized origins work
curl -H "Origin: http://localhost:3000" \
     -H "Access-Control-Request-Method: POST" \
     -X OPTIONS http://localhost:8080/api/v1/users

# Should include Access-Control-Allow-Origin: http://localhost:3000
```

---

### Task 1.2: Implement Rate Limiting Enforcement (16 hours)

**Priority:** P0 - CRITICAL
**Risk:** Brute force attacks, DDoS, API abuse
**Files:**
- `src/payment/middleware/rateLimitMiddleware.go` (rewrite)
- `go.mod` (add dependencies)

#### Step 1: Add Dependencies
```bash
go get github.com/redis/go-redis/v9
go get github.com/ulule/limiter/v3
go get github.com/ulule/limiter/v3/drivers/store/redis
```

#### Step 2: Create Redis Client (`src/db/redis.go`)
```go
package db

import (
    "context"
    "os"
    "github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

func InitRedisConnection() error {
    RedisClient = redis.NewClient(&redis.Options{
        Addr:     os.Getenv("REDIS_URL"), // e.g., "localhost:6379"
        Password: os.Getenv("REDIS_PASSWORD"),
        DB:       0,
    })

    ctx := context.Background()
    _, err := RedisClient.Ping(ctx).Result()
    if err != nil {
        return fmt.Errorf("failed to connect to Redis: %w", err)
    }

    log.Println("✅ Redis connection established")
    return nil
}
```

#### Step 3: Rewrite Rate Limit Middleware
```go
// src/payment/middleware/rateLimitMiddleware.go
package middleware

import (
    "fmt"
    "net/http"
    "time"

    "soli/formations/src/auth/errors"
    sqldb "soli/formations/src/db"
    "soli/formations/src/payment/services"

    "github.com/gin-gonic/gin"
    "github.com/ulule/limiter/v3"
    "github.com/ulule/limiter/v3/drivers/middleware/gin"
    "github.com/ulule/limiter/v3/drivers/store/redis"
    "gorm.io/gorm"
)

type RateLimitMiddleware interface {
    ApplyRateLimit() gin.HandlerFunc
    AuthEndpointLimit() gin.HandlerFunc
    PaymentEndpointLimit() gin.HandlerFunc
    ResourceCreationLimit() gin.HandlerFunc
}

type rateLimitMiddleware struct {
    subscriptionService services.UserSubscriptionService
    store               limiter.Store
}

func NewRateLimitMiddleware(db *gorm.DB) RateLimitMiddleware {
    // Create Redis store
    store, err := redis.NewStore(sqldb.RedisClient)
    if err != nil {
        panic(fmt.Sprintf("Failed to create Redis store: %v", err))
    }

    return &rateLimitMiddleware{
        subscriptionService: services.NewSubscriptionService(db),
        store:               store,
    }
}

// ApplyRateLimit - General API rate limiting based on subscription
func (rlm *rateLimitMiddleware) ApplyRateLimit() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        userId := ctx.GetString("userId")

        // Default limit for unauthenticated users
        rate := limiter.Rate{
            Period: 15 * time.Minute,
            Limit:  60,
        }

        // Get user's subscription plan for dynamic limits
        if userId != "" {
            subscription, err := rlm.subscriptionService.GetActiveUserSubscription(userId)
            if err == nil {
                sPlan, errSPlan := rlm.subscriptionService.GetSubscriptionPlan(subscription.SubscriptionPlanID)
                if errSPlan == nil {
                    rate = rlm.getRateForPlan(sPlan.RequiredRole)
                }
            }
        }

        // Create limiter instance
        instance := limiter.New(rlm.store, rate)

        // Get identifier (userId or IP)
        identifier := userId
        if identifier == "" {
            identifier = ctx.ClientIP()
        }

        // Check rate limit
        context := limiter.Context{Limit: rate.Limit, Remaining: 0}
        context, err := instance.Get(ctx, identifier)
        if err != nil {
            ctx.JSON(http.StatusInternalServerError, &errors.APIError{
                ErrorCode:    http.StatusInternalServerError,
                ErrorMessage: "Rate limit check failed",
            })
            ctx.Abort()
            return
        }

        // Set rate limit headers
        ctx.Header("X-RateLimit-Limit", fmt.Sprintf("%d", context.Limit))
        ctx.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", context.Remaining))
        ctx.Header("X-RateLimit-Reset", fmt.Sprintf("%d", context.Reset))

        // Check if limit exceeded
        if context.Reached {
            ctx.Header("Retry-After", fmt.Sprintf("%d", int(time.Until(time.Unix(context.Reset, 0)).Seconds())))
            ctx.JSON(http.StatusTooManyRequests, &errors.APIError{
                ErrorCode:    http.StatusTooManyRequests,
                ErrorMessage: "Rate limit exceeded. Please try again later.",
            })
            ctx.Abort()
            return
        }

        ctx.Next()
    }
}

// AuthEndpointLimit - Strict limits for authentication endpoints
func (rlm *rateLimitMiddleware) AuthEndpointLimit() gin.HandlerFunc {
    rate := limiter.Rate{
        Period: 15 * time.Minute,
        Limit:  5, // Max 5 login attempts per 15 minutes
    }

    instance := limiter.New(rlm.store, rate)
    middleware := ginlimiter.NewMiddleware(instance, ginlimiter.WithKeyGetter(func(c *gin.Context) string {
        // Use IP address for auth endpoints (prevent account enumeration)
        return c.ClientIP()
    }))

    return middleware
}

// PaymentEndpointLimit - Strict limits for payment operations
func (rlm *rateLimitMiddleware) PaymentEndpointLimit() gin.HandlerFunc {
    rate := limiter.Rate{
        Period: 1 * time.Hour,
        Limit:  10, // Max 10 checkout sessions per hour
    }

    instance := limiter.New(rlm.store, rate)
    middleware := ginlimiter.NewMiddleware(instance, ginlimiter.WithKeyGetter(func(c *gin.Context) string {
        userId := c.GetString("userId")
        if userId == "" {
            return c.ClientIP()
        }
        return "payment:" + userId
    }))

    return middleware
}

// ResourceCreationLimit - Limits for resource creation
func (rlm *rateLimitMiddleware) ResourceCreationLimit() gin.HandlerFunc {
    rate := limiter.Rate{
        Period: 1 * time.Hour,
        Limit:  50, // Max 50 resource creations per hour
    }

    instance := limiter.New(rlm.store, rate)
    middleware := ginlimiter.NewMiddleware(instance, ginlimiter.WithKeyGetter(func(c *gin.Context) string {
        userId := c.GetString("userId")
        if userId == "" {
            return c.ClientIP()
        }
        return "resource:" + userId
    }))

    return middleware
}

// Helper function to determine rate based on plan
func (rlm *rateLimitMiddleware) getRateForPlan(requiredRole string) limiter.Rate {
    switch {
    case contains(requiredRole, "enterprise"):
        return limiter.Rate{Period: 15 * time.Minute, Limit: 1000}
    case contains(requiredRole, "organization"):
        return limiter.Rate{Period: 15 * time.Minute, Limit: 500}
    case contains(requiredRole, "premium"), contains(requiredRole, "pro"):
        return limiter.Rate{Period: 15 * time.Minute, Limit: 200}
    default:
        return limiter.Rate{Period: 15 * time.Minute, Limit: 60}
    }
}

func contains(s, substr string) bool {
    return len(s) >= len(substr) && strings.Contains(s, substr)
}
```

#### Step 4: Apply Middleware in main.go
```go
// main.go

// Initialize Redis
sqldb.InitRedisConnection()

// Create rate limit middleware
rateLimitMiddleware := paymentMiddleware.NewRateLimitMiddleware(sqldb.DB)

// Apply global rate limiting
r.Use(rateLimitMiddleware.ApplyRateLimit())

// Apply specific rate limits to auth endpoints
authGroup := apiGroup.Group("/auth")
authGroup.Use(rateLimitMiddleware.AuthEndpointLimit())

// Apply to payment endpoints
paymentGroup := apiGroup.Group("/user-subscriptions")
paymentGroup.Use(rateLimitMiddleware.PaymentEndpointLimit())

// Apply to resource creation endpoints
terminalGroup := apiGroup.Group("/terminals")
terminalGroup.Use(rateLimitMiddleware.ResourceCreationLimit())
```

#### Step 5: Docker Compose for Local Redis
```yaml
# docker-compose.yml (add Redis service)
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes

volumes:
  redis_data:
```

#### Environment Variables
```bash
# .env
REDIS_URL=localhost:6379
REDIS_PASSWORD=  # Empty for local dev
```

#### Testing
```bash
# Start Redis
docker-compose up -d redis

# Test rate limiting - should block after 5 attempts
for i in {1..10}; do
  curl -X POST http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"test","password":"wrong"}' \
    -v 2>&1 | grep "HTTP/1.1"
done

# Should see 429 Too Many Requests after 5 attempts
```

---

### Task 1.3: Fix Feature Gate Logic Bug (1 hour)

**Priority:** P0 - CRITICAL
**Risk:** Paid features blocked, free users get premium access
**File:** `src/payment/middleware/featureGateMiddleware.go:60-68`

#### Current Code (BUGGY)
```go
features := sPlan.Features
if containsFeature(features, featureName) {  // ⚠️ INVERTED LOGIC!
    ctx.JSON(http.StatusForbidden, &errors.APIError{
        ErrorCode:    http.StatusForbidden,
        ErrorMessage: fmt.Sprintf("Feature '%s' is not included in your current plan", featureName),
    })
    ctx.Abort()
    return
}
ctx.Next()
```

#### Fixed Code
```go
features := sPlan.Features
if !containsFeature(features, featureName) {  // ✅ CORRECT LOGIC
    ctx.JSON(http.StatusForbidden, &errors.APIError{
        ErrorCode:    http.StatusForbidden,
        ErrorMessage: fmt.Sprintf("Feature '%s' is not included in your current plan. Upgrade to access this feature.", featureName),
    })
    ctx.Abort()
    return
}

// Feature is included - allow access
ctx.Next()
```

#### Also Fix RequireAnyFeature (Line 114)
```go
if !hasRequiredFeature {  // ✅ This is already correct
    ctx.JSON(http.StatusForbidden, &errors.APIError{
        ErrorCode:    http.StatusForbidden,
        ErrorMessage: "This feature is not included in your current plan. Please upgrade.",
    })
    ctx.Abort()
    return
}
```

#### Testing
```go
// Add to tests/payment/featureGate_test.go
func TestFeatureGateMiddleware_RequireFeature_AllowsWhenFeatureIncluded(t *testing.T) {
    // Setup user with plan that HAS the feature
    // Call endpoint
    // Assert 200 OK (not 403)
}

func TestFeatureGateMiddleware_RequireFeature_BlocksWhenFeatureMissing(t *testing.T) {
    // Setup user with plan that LACKS the feature
    // Call endpoint
    // Assert 403 Forbidden
}
```

---

### Task 1.4: Remove JWT from Query Parameters (2 hours)

**Priority:** P0 - CRITICAL
**Risk:** Token exposure in logs, browser history, referrer headers
**File:** `src/auth/authMiddleware.go:113-115`

#### Current Code (INSECURE)
```go
token := ctx.Request.Header.Get("Authorization")

// WebSocket Hack
if token == "" {
    token = ctx.Query("Authorization")  // ⚠️ SECURITY RISK
}
```

#### Step 1: Remove Query Parameter Support
```go
token := ctx.Request.Header.Get("Authorization")

// JWT tokens must ONLY come from Authorization header
// For WebSockets, use Sec-WebSocket-Protocol header
if token == "" {
    ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
        "msg": "Missing Authorization header",
    })
    return
}
```

#### Step 2: Fix WebSocket Authentication
```go
// src/webSsh/routes/sshClientRoutes/sshClientController.go
// Find the WebSocket upgrade handler

// BEFORE WebSocket upgrade, validate JWT from header
func (sshc *sshClientController) HandleWebSocket(ctx *gin.Context) {
    // Get token from Authorization header BEFORE upgrade
    token := ctx.Request.Header.Get("Authorization")
    if token == "" {
        ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authorization"})
        return
    }

    // Validate token
    claims, err := casdoorsdk.ParseJwtToken(strings.TrimPrefix(token, "Bearer "))
    if err != nil {
        ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
        return
    }

    // Store user ID in context for use after upgrade
    ctx.Set("userId", claims.Id)

    // Proceed with WebSocket upgrade
    upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
}
```

#### Alternative: Protocol-Based Auth
```javascript
// Frontend WebSocket connection
const ws = new WebSocket(
    'wss://api.yourdomain.com/ws/ssh',
    ['Bearer', jwtToken]  // Use Sec-WebSocket-Protocol
);
```

```go
// Backend: Extract from Sec-WebSocket-Protocol header
func (sshc *sshClientController) HandleWebSocket(ctx *gin.Context) {
    protocols := ctx.Request.Header.Get("Sec-WebSocket-Protocol")
    parts := strings.Split(protocols, ", ")

    var token string
    for i, part := range parts {
        if part == "Bearer" && i+1 < len(parts) {
            token = parts[i+1]
            break
        }
    }

    if token == "" {
        ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Missing token in protocol"})
        return
    }

    // Validate and upgrade...
}
```

#### Testing
```bash
# Test that query parameter auth is rejected
curl "http://localhost:8080/api/v1/users/me?Authorization=eyJhbGc..." \
  -v 2>&1 | grep "401"

# Should return 401 Unauthorized

# Test that header auth works
curl http://localhost:8080/api/v1/users/me \
  -H "Authorization: Bearer eyJhbGc..." \
  -v 2>&1 | grep "200"

# Should return 200 OK
```

---

### Task 1.5: Migrate Webhook Event Tracking to Database (4 hours)

**Priority:** P0 - CRITICAL
**Risk:** Duplicate webhook processing after server restart
**Files:**
- `src/payment/models/webhookEvent.go` (new)
- `src/payment/routes/webHookController.go:23-24, 178-211`

#### Step 1: Create Database Model
```go
// src/payment/models/webhookEvent.go
package models

import (
    "time"
    "github.com/google/uuid"
)

type WebhookEvent struct {
    ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
    EventID      string    `gorm:"type:varchar(255);uniqueIndex;not null"` // Stripe event ID
    EventType    string    `gorm:"type:varchar(100);not null"`
    ProcessedAt  time.Time `gorm:"not null"`
    ExpiresAt    time.Time `gorm:"index;not null"` // For cleanup
    Payload      string    `gorm:"type:text"` // Optional: store full payload for debugging
    CreatedAt    time.Time
}

func (WebhookEvent) TableName() string {
    return "webhook_events"
}
```

#### Step 2: Add Migration
```go
// src/initialization/migrations.go
// Add to AutoMigrateAll function
func AutoMigrateAll(db *gorm.DB) {
    // ... existing migrations ...
    db.AutoMigrate(&paymentModels.WebhookEvent{})
}
```

#### Step 3: Update Webhook Controller
```go
// src/payment/routes/webHookController.go
package paymentController

import (
    "time"
    "soli/formations/src/payment/models"
    // ... other imports
)

type webhookController struct {
    stripeService services.StripeService
    db            *gorm.DB  // Add database
}

func NewWebhookController(db *gorm.DB) WebhookController {
    return &webhookController{
        stripeService: services.NewStripeService(db),
        db:            db,
    }
    // Remove go controller.cleanupProcessedEvents() - now handled by cron
}

// Replace in-memory map methods with database operations
func (wc *webhookController) isEventProcessed(eventID string) bool {
    var count int64
    wc.db.Model(&models.WebhookEvent{}).
        Where("event_id = ? AND expires_at > ?", eventID, time.Now()).
        Count(&count)

    return count > 0
}

func (wc *webhookController) markEventProcessed(eventID, eventType string) error {
    webhookEvent := &models.WebhookEvent{
        EventID:     eventID,
        EventType:   eventType,
        ProcessedAt: time.Now(),
        ExpiresAt:   time.Now().Add(24 * time.Hour), // Keep for 24 hours
    }

    return wc.db.Create(webhookEvent).Error
}

// Update HandleStripeWebhook to pass event type
func (wc *webhookController) HandleStripeWebhook(ctx *gin.Context) {
    // ... validation code ...

    // Mark as processed with event type
    if err := wc.markEventProcessed(event.ID, string(event.Type)); err != nil {
        utils.Debug("⚠️ Failed to mark event as processed: %v", err)
        // Continue anyway - better to process twice than not at all
    }

    // ... rest of handler ...
}
```

#### Step 4: Create Cleanup Cron Job
```go
// src/cron/webhookCleanup.go
package cron

import (
    "log"
    "time"
    "soli/formations/src/payment/models"
    "gorm.io/gorm"
)

func StartWebhookCleanupJob(db *gorm.DB) {
    ticker := time.NewTicker(1 * time.Hour)

    go func() {
        for range ticker.C {
            result := db.Where("expires_at < ?", time.Now()).
                Delete(&models.WebhookEvent{})

            if result.Error != nil {
                log.Printf("❌ Webhook cleanup failed: %v", result.Error)
            } else if result.RowsAffected > 0 {
                log.Printf("🧹 Cleaned up %d expired webhook events", result.RowsAffected)
            }
        }
    }()

    log.Println("✅ Webhook cleanup job started")
}
```

#### Step 5: Initialize in main.go
```go
// main.go
import "soli/formations/src/cron"

func main() {
    // ... existing initialization ...

    // Start background jobs
    cron.StartWebhookCleanupJob(sqldb.DB)

    // ... rest of main ...
}
```

#### Testing
```bash
# Simulate duplicate webhook
EVENT_ID="evt_test_123"
for i in {1..3}; do
  curl -X POST http://localhost:8080/webhooks/stripe \
    -H "Stripe-Signature: t=..." \
    -d "{\"id\":\"$EVENT_ID\",\"type\":\"invoice.paid\"}"
done

# Check database
psql -c "SELECT event_id, processed_at FROM webhook_events WHERE event_id='evt_test_123';"

# Should only have 1 row, and 2nd/3rd requests returned "Event already processed"
```

---

### Task 1.6: Add 3D Secure to Stripe Checkout (2 hours)

**Priority:** P0 - CRITICAL (EU Legal Requirement)
**Risk:** PSD2/SCA non-compliance, transactions declined in EU
**File:** `src/payment/services/stripeService.go`

#### Find CreateCheckoutSession Method
```go
// Search for: func (ss *stripeService) CreateCheckoutSession
```

#### Add 3D Secure Configuration
```go
func (ss *stripeService) CreateCheckoutSession(
    userID string,
    input dto.CreateCheckoutSessionInput,
    replaceSubscriptionID *uuid.UUID,
) (*dto.CheckoutSessionOutput, error) {
    // ... existing code to get plan, customer, etc. ...

    params := &stripe.CheckoutSessionParams{
        Customer:   stripe.String(customerID),
        SuccessURL: stripe.String(input.SuccessURL),
        CancelURL:  stripe.String(input.CancelURL),
        Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
        LineItems: []*stripe.CheckoutSessionLineItemParams{
            {
                Price:    stripe.String(plan.StripePriceID),
                Quantity: stripe.Int64(1),
            },
        },

        // ✅ ADD THIS: Enable 3D Secure / SCA
        PaymentMethodOptions: &stripe.CheckoutSessionPaymentMethodOptionsParams{
            Card: &stripe.CheckoutSessionPaymentMethodOptionsCardParams{
                RequestThreeDSecure: stripe.String("automatic"), // Required for EU
            },
        },

        // Enable automatic tax calculation (recommended)
        AutomaticTax: &stripe.CheckoutSessionAutomaticTaxParams{
            Enabled: stripe.Bool(true),
        },

        // ... rest of params ...
    }

    // ... rest of method ...
}
```

#### Also Update Bulk Checkout Session
```go
func (ss *stripeService) CreateBulkCheckoutSession(
    userID string,
    input dto.CreateBulkCheckoutSessionInput,
) (*dto.CheckoutSessionOutput, error) {
    // ... existing code ...

    params := &stripe.CheckoutSessionParams{
        // ... existing params ...

        // ✅ ADD THIS
        PaymentMethodOptions: &stripe.CheckoutSessionPaymentMethodOptionsParams{
            Card: &stripe.CheckoutSessionPaymentMethodOptionsCardParams{
                RequestThreeDSecure: stripe.String("automatic"),
            },
        },

        // ... rest of params ...
    }
}
```

#### Testing with Stripe Test Cards
```bash
# Test successful 3D Secure
# Card: 4000002500003155
# Result: Should trigger 3D Secure authentication

# Test failed 3D Secure
# Card: 4000008400001629
# Result: Should fail with authentication_required

# Test without 3D Secure requirement
# Card: 4242424242424242
# Result: Should succeed without 3D Secure (no SCA required)
```

#### Verification in Stripe Dashboard
1. Go to Stripe Dashboard → Payments
2. Find test payment
3. Check "Payment Details" → should show "3D Secure: authenticated"

---

### Task 1.7: Basic Audit Logging Infrastructure (8 hours)

**Priority:** P0 - CRITICAL (Foundation for compliance)
**Files:**
- `src/audit/models/auditLog.go` (new)
- `src/audit/auditLogger.go` (new)
- `src/audit/middleware/auditMiddleware.go` (new)

#### Step 1: Create Audit Log Model
```go
// src/audit/models/auditLog.go
package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/datatypes"
)

type AuditLog struct {
    ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
    UserID        string         `gorm:"type:varchar(255);index;not null"` // Casdoor user ID
    Action        string         `gorm:"type:varchar(100);not null;index"` // e.g., "user.login", "subscription.created"
    ResourceType  string         `gorm:"type:varchar(100);index"` // e.g., "user", "subscription", "organization"
    ResourceID    string         `gorm:"type:varchar(255);index"` // UUID or ID of affected resource
    Details       datatypes.JSON `gorm:"type:jsonb"` // Additional context
    IPAddress     string         `gorm:"type:varchar(45)"` // IPv4 or IPv6
    UserAgent     string         `gorm:"type:text"`
    RequestMethod string         `gorm:"type:varchar(10)"` // GET, POST, etc.
    RequestPath   string         `gorm:"type:varchar(500)"`
    StatusCode    int            // HTTP status code
    Timestamp     time.Time      `gorm:"index;not null"`
    CreatedAt     time.Time
}

func (AuditLog) TableName() string {
    return "audit_logs"
}

// Action constants
const (
    // Authentication
    ActionLoginSuccess    = "auth.login.success"
    ActionLoginFailed     = "auth.login.failed"
    ActionLogout          = "auth.logout"
    ActionPasswordReset   = "auth.password_reset"
    ActionTokenRevoked    = "auth.token_revoked"

    // Authorization
    ActionAuthDenied      = "auth.access_denied"

    // Subscriptions
    ActionSubscriptionCreated  = "subscription.created"
    ActionSubscriptionCanceled = "subscription.canceled"
    ActionSubscriptionUpgraded = "subscription.upgraded"
    ActionPaymentSucceeded     = "payment.succeeded"
    ActionPaymentFailed        = "payment.failed"

    // Licenses
    ActionLicenseAssigned = "license.assigned"
    ActionLicenseRevoked  = "license.revoked"

    // Organizations
    ActionOrgCreated        = "organization.created"
    ActionOrgDeleted        = "organization.deleted"
    ActionOrgMemberAdded    = "organization.member_added"
    ActionOrgMemberRemoved  = "organization.member_removed"
    ActionOrgMemberRoleChanged = "organization.member_role_changed"
    ActionOrgConvertedToTeam = "organization.converted_to_team"

    // Groups
    ActionGroupCreated      = "group.created"
    ActionGroupDeleted      = "group.deleted"
    ActionGroupMemberAdded  = "group.member_added"
    ActionGroupMemberRemoved = "group.member_removed"
)
```

#### Step 2: Create Audit Logger Service
```go
// src/audit/auditLogger.go
package audit

import (
    "context"
    "encoding/json"
    "time"

    "soli/formations/src/audit/models"
    "soli/formations/src/utils"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type AuditLogger interface {
    Log(ctx *gin.Context, action string, resourceType string, resourceID string, details map[string]interface{})
    LogWithUserID(userID string, action string, resourceType string, resourceID string, details map[string]interface{}, ipAddress string)
    GetUserAuditLogs(userID string, limit int) ([]models.AuditLog, error)
    GetOrganizationAuditLogs(orgID string, limit int) ([]models.AuditLog, error)
}

type auditLogger struct {
    db *gorm.DB
}

func NewAuditLogger(db *gorm.DB) AuditLogger {
    return &auditLogger{db: db}
}

// Log creates an audit log entry from Gin context
func (al *auditLogger) Log(ctx *gin.Context, action string, resourceType string, resourceID string, details map[string]interface{}) {
    userID := ctx.GetString("userId")
    if userID == "" {
        userID = "anonymous"
    }

    detailsJSON, _ := json.Marshal(details)

    auditLog := &models.AuditLog{
        UserID:        userID,
        Action:        action,
        ResourceType:  resourceType,
        ResourceID:    resourceID,
        Details:       detailsJSON,
        IPAddress:     ctx.ClientIP(),
        UserAgent:     ctx.Request.UserAgent(),
        RequestMethod: ctx.Request.Method,
        RequestPath:   ctx.Request.URL.Path,
        StatusCode:    ctx.Writer.Status(),
        Timestamp:     time.Now(),
    }

    // Insert asynchronously to avoid blocking the request
    go func() {
        if err := al.db.Create(auditLog).Error; err != nil {
            utils.Debug("❌ Failed to create audit log: %v", err)
        }
    }()
}

// LogWithUserID creates an audit log entry with explicit user ID (for background jobs)
func (al *auditLogger) LogWithUserID(userID string, action string, resourceType string, resourceID string, details map[string]interface{}, ipAddress string) {
    detailsJSON, _ := json.Marshal(details)

    auditLog := &models.AuditLog{
        UserID:       userID,
        Action:       action,
        ResourceType: resourceType,
        ResourceID:   resourceID,
        Details:      detailsJSON,
        IPAddress:    ipAddress,
        Timestamp:    time.Now(),
    }

    // Insert asynchronously
    go func() {
        if err := al.db.Create(auditLog).Error; err != nil {
            utils.Debug("❌ Failed to create audit log: %v", err)
        }
    }()
}

// GetUserAuditLogs retrieves audit logs for a specific user
func (al *auditLogger) GetUserAuditLogs(userID string, limit int) ([]models.AuditLog, error) {
    var logs []models.AuditLog
    err := al.db.Where("user_id = ?", userID).
        Order("timestamp DESC").
        Limit(limit).
        Find(&logs).Error
    return logs, err
}

// GetOrganizationAuditLogs retrieves audit logs for organization-related actions
func (al *auditLogger) GetOrganizationAuditLogs(orgID string, limit int) ([]models.AuditLog, error) {
    var logs []models.AuditLog
    err := al.db.Where("resource_type = ? AND resource_id = ?", "organization", orgID).
        Order("timestamp DESC").
        Limit(limit).
        Find(&logs).Error
    return logs, err
}
```

#### Step 3: Create Audit Middleware (Auto-log Failed Auth)
```go
// src/audit/middleware/auditMiddleware.go
package middleware

import (
    "soli/formations/src/audit"
    "soli/formations/src/audit/models"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

type AuditMiddleware interface {
    LogFailedAuth() gin.HandlerFunc
}

type auditMiddleware struct {
    logger audit.AuditLogger
}

func NewAuditMiddleware(db *gorm.DB) AuditMiddleware {
    return &auditMiddleware{
        logger: audit.NewAuditLogger(db),
    }
}

// LogFailedAuth logs failed authorization attempts
func (am *auditMiddleware) LogFailedAuth() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        ctx.Next() // Process request first

        // If request was forbidden or unauthorized, log it
        if ctx.Writer.Status() == 403 || ctx.Writer.Status() == 401 {
            action := models.ActionAuthDenied
            if ctx.Writer.Status() == 401 {
                action = models.ActionLoginFailed
            }

            am.logger.Log(ctx, action, "endpoint", ctx.Request.URL.Path, map[string]interface{}{
                "method": ctx.Request.Method,
                "status": ctx.Writer.Status(),
            })
        }
    }
}
```

#### Step 4: Add Migration and Initialize
```go
// src/initialization/migrations.go
func AutoMigrateAll(db *gorm.DB) {
    // ... existing migrations ...
    db.AutoMigrate(&auditModels.AuditLog{})
}

// main.go
import (
    auditMiddleware "soli/formations/src/audit/middleware"
)

func main() {
    // ... existing setup ...

    // Create audit middleware
    auditMW := auditMiddleware.NewAuditMiddleware(sqldb.DB)

    // Apply to all API routes
    apiGroup.Use(auditMW.LogFailedAuth())

    // ... rest of main ...
}
```

#### Step 5: Usage Examples (Add to controllers later)
```go
// Example: Log successful login
// src/auth/authController.go
auditLogger := audit.NewAuditLogger(db)
auditLogger.Log(ctx, models.ActionLoginSuccess, "user", userId, map[string]interface{}{
    "email": email,
})

// Example: Log subscription creation
// src/payment/services/stripeService.go
auditLogger.LogWithUserID(userID, models.ActionSubscriptionCreated, "subscription", subscriptionID, map[string]interface{}{
    "plan": planName,
    "amount": amount,
}, ipAddress)
```

---

## Summary of Phase 1 Deliverables

After completing Phase 1, you will have:

✅ **CORS** - Restricted to whitelisted domains
✅ **Rate Limiting** - Redis-based, enforced on all critical endpoints
✅ **Feature Gates** - Correct logic (blocks when feature missing)
✅ **JWT Security** - No tokens in query parameters
✅ **Webhook Protection** - Database-backed duplicate prevention
✅ **3D Secure** - EU PSD2/SCA compliant payments
✅ **Audit Logging** - Infrastructure ready for comprehensive logging

**Estimated Total Time: 35 hours (1 week, 1 developer)**

---

# 🟡 PHASE 2-4 ROADMAP (Abbreviated)

Due to output token limits, here's a condensed version of the remaining phases:

## Phase 2: Quota & Resource Management (Week 2) - 40 hours

### Task 2.1: Member Limit Enforcement (8h)
- Add transaction-safe member count checks
- Files: `src/organizations/services/organizationService.go`
- Verify `subscription_plan.max_members` enforced

### Task 2.2: Terminal Limit Enforcement (8h)
- Verify usage limit middleware applied to terminal creation
- Add concurrent terminal count checks
- Files: `src/terminalTrainer/routes/terminalRoutes.go`

### Task 2.3: Session Duration Auto-Termination (12h)
- Create background job to stop expired terminal sessions
- Add 5-minute warning before auto-stop
- Files: `src/cron/sessionTimeout.go`

### Task 2.4: Stripe Quantity Sync Verification (8h)
- Verify bulk license assign/revoke updates Stripe quantity
- Add tests for quantity synchronization
- Files: `src/payment/services/bulkLicenseService.go`

### Task 2.5: Add Proration (4h)
- Add `proration_behavior: 'create_prorations'` to upgrades
- Schedule downgrades for end of period
- Files: `src/payment/services/stripeService.go` (UpdateSubscription)

---

## Phase 3: Comprehensive Audit Logging (Week 3) - 32 hours

### Task 3.1: Security Event Logging (8h)
- Log all login attempts (success/failure)
- Log password resets
- Log failed authorization
- Update: `src/auth/authController.go`, `src/auth/authMiddleware.go`

### Task 3.2: Billing Event Logging (8h)
- Log subscription lifecycle events
- Log payment success/failure
- Log license operations
- Update: `src/payment/services/stripeService.go`, webhook handlers

### Task 3.3: Organization/Group Event Logging (8h)
- Log member additions/removals
- Log role changes
- Log organization conversions
- Update: `src/organizations/controller/*.go`, `src/groups/controller/*.go`

### Task 3.4: Audit Log API Endpoints (8h)
- `GET /api/v1/organizations/:id/audit-logs`
- `GET /api/v1/users/me/audit-logs`
- Export to CSV/JSON
- Add pagination and filtering
- Files: `src/audit/routes/auditRoutes.go`

---

## Phase 4: Testing & Production Hardening (Week 4) - 48 hours

### Task 4.1: Security Testing Suite (12h)
- Authorization bypass tests
- SQL injection tests
- XSS/CSRF tests
- Rate limit tests
- Files: `tests/security/*.go`

### Task 4.2: Payment Flow Testing (8h)
- Test 3D Secure flow (all test cards)
- Test proration calculations
- Test webhook idempotency
- Test bulk license flows
- Files: `tests/payment/*.go`

### Task 4.3: Input Validation Audit (8h)
- Review all DTOs for validation tags
- Add missing validations
- Test boundary conditions
- Files: All `src/*/dto/*.go`

### Task 4.4: Transaction Safety Audit (8h)
- Review critical mutations for transactions
- Add transactions where missing
- Test rollback scenarios
- Files: All service files with mutations

### Task 4.5: Performance & Load Testing (12h)
- Benchmark critical endpoints
- Load test with 1000 concurrent users
- Identify slow queries
- Optimize bottlenecks
- Tools: `go test -bench`, `vegeta`, `pprof`

---

## 🎯 Definition of Done

### Phase 1 (Week 1) - Complete When:
- [ ] CORS allows only whitelisted domains
- [ ] Rate limiting returns 429 after limit exceeded
- [ ] Feature gates block free users from premium features
- [ ] JWT query parameter auth rejected (401)
- [ ] Duplicate webhooks return "already processed"
- [ ] 3D Secure triggers on test card 4000002500003155
- [ ] Audit logs table contains failed auth attempts

### Phase 2 (Week 2) - Complete When:
- [ ] Adding member beyond limit returns 403 with upgrade message
- [ ] Creating terminal beyond limit blocked
- [ ] Terminal auto-stops after max_session_duration
- [ ] Bulk license assign updates Stripe quantity correctly
- [ ] Subscription upgrade charges prorated amount

### Phase 3 (Week 3) - Complete When:
- [ ] All login attempts logged to audit_logs
- [ ] All payment events logged
- [ ] All organization changes logged
- [ ] GET /organizations/:id/audit-logs returns events
- [ ] Audit logs exported to CSV successfully

### Phase 4 (Week 4) - Complete When:
- [ ] All security tests pass
- [ ] Payment tests pass with 3D Secure cards
- [ ] Input validation tests pass
- [ ] Load test handles 1000 concurrent users
- [ ] No queries slower than 100ms
- [ ] Overall security score > 90%

---

## 📚 Testing Commands Reference

```bash
# Run all tests
go test ./tests/...

# Run only security tests
go test ./tests/security/...

# Run with race detection
go test -race ./tests/...

# Run with coverage
go test -cover ./tests/...
go tool cover -html=coverage.out

# Benchmark tests
go test -bench=. ./tests/...

# Load testing with vegeta
echo "GET http://localhost:8080/api/v1/version" | \
  vegeta attack -duration=30s -rate=100 | \
  vegeta report
```

---

## 🔧 Environment Setup for Production

```bash
# Required Environment Variables
ENVIRONMENT=production
FRONTEND_URL=https://app.yourdomain.com
ADMIN_FRONTEND_URL=https://admin.yourdomain.com

# Redis (Rate Limiting)
REDIS_URL=production-redis.yourdomain.com:6379
REDIS_PASSWORD=<secure-password>

# Stripe (Production Keys)
STRIPE_SECRET_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...

# Database
DATABASE_URL=postgresql://user:pass@host:5432/dbname?sslmode=require

# CORS
ALLOWED_ORIGINS=https://app.yourdomain.com,https://admin.yourdomain.com
```

---

## 📊 Progress Tracking

Use this checklist to track your team's progress:

### Week 1: Critical Security Fixes
- [ ] Day 1: CORS + Rate Limiting Setup
- [ ] Day 2: Rate Limiting Implementation
- [ ] Day 3: Feature Gate Fix + JWT Security
- [ ] Day 4: Webhook Migration + 3D Secure
- [ ] Day 5: Audit Logging Infrastructure + Testing

### Week 2: Quotas & Resources
- [ ] Day 1-2: Member & Terminal Limits
- [ ] Day 3-4: Session Timeouts
- [ ] Day 5: Stripe Sync + Proration

### Week 3: Comprehensive Logging
- [ ] Day 1-2: Security Event Logging
- [ ] Day 3: Billing Event Logging
- [ ] Day 4: Org/Group Logging
- [ ] Day 5: Audit API Endpoints

### Week 4: Testing & Hardening
- [ ] Day 1-2: Security Testing
- [ ] Day 3: Payment Testing
- [ ] Day 4: Validation & Transaction Audits
- [ ] Day 5: Performance Testing + Fixes

---

## 🆘 Support & Questions

If you encounter issues during implementation:

1. **Check the file paths** - All paths are relative to `/workspaces/ocf-core`
2. **Review the original audit report** - Detailed analysis in `/workspaces/ocf-core/AUDIT_REPORT.md`
3. **Test incrementally** - Don't implement all changes at once
4. **Use version control** - Create a branch for each phase
5. **Run tests frequently** - Catch issues early

---

**Document Maintainer:** DevOps Team
**Last Updated:** 2025-11-02
**Next Review:** After Phase 1 Completion
