---
description: Performance analysis and optimization recommendations
tags: [performance, optimization, benchmarks, profiling]
---

# Performance Audit Agent

Analyze and optimize application performance.

## Performance Checks

### 1. Database Performance

#### A. N+1 Query Detection
**The Silent Killer**

**Scan for:**
```go
// ❌ BAD: N+1 queries
users := getUserAll()
for _, user := range users {
    org := getOrganization(user.OrgID) // Query per user!
}
```

**Should be:**
```go
// ✅ GOOD: Single query with join
users := db.Preload("Organization").Find(&users)
```

**Method:**
- Use Task agent to scan all repository methods
- Look for loops with queries inside
- Check for missing Preload/Joins

#### B. Missing Indexes
- [ ] Foreign keys have indexes
- [ ] Filtered columns indexed
- [ ] Unique constraints use indexes

**Scan for:**
```go
// ❌ MISSING INDEX:
UserID string `gorm:"type:uuid"` // No index tag!
Email  string `gorm:"type:varchar(255)"` // Frequently filtered, needs index
```

**Should be:**
```go
UserID string `gorm:"type:uuid;index"`
Email  string `gorm:"type:varchar(255);uniqueIndex"`
```

#### C. Unnecessary Data Loading
- [ ] SELECT * avoided when not needed
- [ ] Pagination on large tables
- [ ] Lazy loading for expensive relationships

**Scan for:**
```go
// ❌ BAD: Loading all fields
db.Find(&users) // Loads everything including large fields

// ❌ BAD: No pagination
db.Find(&courses) // Could be thousands of records
```

### 2. Memory Usage

#### A. Memory Leaks
**Scan for:**
```go
// ❌ POTENTIAL LEAKS:
// Goroutines without context cancellation
go func() { /* never returns */ }()

// Large slices that grow unbounded
var allData []LargeStruct
for _, item := range infiniteStream {
    allData = append(allData, item) // Grows forever
}

// Not closing resources
resp, _ := http.Get(url) // Missing defer resp.Body.Close()
```

#### B. Large Allocations
**Scan for:**
```go
// ❌ INEFFICIENT:
result := ""
for _, item := range items {
    result += item // Creates new string each time
}
```

**Should be:**
```go
var builder strings.Builder
for _, item := range items {
    builder.WriteString(item)
}
result := builder.String()
```

### 3. API Response Times

#### A. Slow Endpoints
**Analyze:**
- Check for synchronous external API calls
- Look for expensive computations in handlers
- Verify caching strategies

**Scan for:**
```go
// ❌ SLOW:
func GetCourse(ctx *gin.Context) {
    // Expensive operation in handler
    course := generateCourseMaterials() // Takes 30 seconds
    ctx.JSON(200, course)
}
```

**Should be:**
```go
// ✅ FAST: Async with job queue
func GetCourse(ctx *gin.Context) {
    job := queueCourseGeneration(courseID)
    ctx.JSON(202, gin.H{"job_id": job.ID, "status": "processing"})
}
```

#### B. Payload Size
- [ ] Large responses paginated
- [ ] Unnecessary fields excluded
- [ ] Compression enabled

### 4. Concurrency Issues

#### A. Race Conditions
**Scan for:**
```go
// ❌ RACE CONDITION:
var counter int
for i := 0; i < 10; i++ {
    go func() {
        counter++ // Unsafe concurrent access
    }()
}
```

**Method:**
```bash
# Run race detector
go test -race ./...
```

#### B. Deadlocks
**Scan for:**
```go
// ❌ POTENTIAL DEADLOCK:
mutex1.Lock()
mutex2.Lock() // If another goroutine locks in opposite order
```

### 5. Caching Opportunities

#### A. Missing Caches
**Identify cacheable data:**
- Subscription plans (rarely change)
- User permissions (change on role update)
- Organization settings
- Course metadata

**Scan for:**
```go
// ❌ UNCACHED: Fetched every request
func GetSubscriptionPlans() []Plan {
    var plans []Plan
    db.Find(&plans) // Query every time
    return plans
}
```

**Should implement:**
```go
var planCache *cache.Cache

func GetSubscriptionPlans() []Plan {
    if cached := planCache.Get("plans"); cached != nil {
        return cached.([]Plan)
    }
    var plans []Plan
    db.Find(&plans)
    planCache.Set("plans", plans, 5*time.Minute)
    return plans
}
```

### 6. Code Efficiency

#### A. Inefficient Algorithms
**Scan for:**
```go
// ❌ O(n²): Nested loops
for _, user := range users {
    for _, org := range orgs {
        if user.OrgID == org.ID { ... }
    }
}
```

**Should be:**
```go
// ✅ O(n): Hash map lookup
orgMap := make(map[string]Organization)
for _, org := range orgs {
    orgMap[org.ID] = org
}
for _, user := range users {
    org := orgMap[user.OrgID]
}
```

#### B. Unnecessary Processing
- [ ] Data transformation only when needed
- [ ] Early returns to avoid processing
- [ ] Bulk operations instead of loops

### 7. External Services

#### A. Timeout Configuration
**Scan for:**
```go
// ❌ NO TIMEOUT:
resp, err := http.Get(url) // Hangs forever
```

**Should be:**
```go
client := &http.Client{Timeout: 10 * time.Second}
resp, err := client.Get(url)
```

#### B. Connection Pooling
- [ ] Database connection pool configured
- [ ] HTTP client connection reuse
- [ ] Stripe client pooled

## Execution Modes

### Mode 1: Full Performance Audit
```
/performance-audit
```

Output:
```markdown
⚡ Performance Audit Report

## Critical Issues
1. ❌ N+1 Query in GetCourses (src/courses/repository.go:45)
   - Impact: HIGH (executes 100+ queries per request)
   - Current: 2.5s response time
   - Expected: <100ms
   - Fix: Add Preload("Chapters")

## High Impact Optimizations
2. ⚠️  Missing index on courses.user_id
   - Impact: HIGH (slow filtering on 10k+ records)
   - Fix: Add gorm:"index" tag

## Medium Impact
3. ⚠️  Uncached subscription plans
   - Impact: MEDIUM (unnecessary DB query per request)
   - Fix: Implement caching layer

## Low Impact
4. ℹ️  String concatenation in loop
   - Impact: LOW (small strings, few iterations)
   - Fix: Use strings.Builder

## Benchmarks
- Average API response: 450ms
- P95 response time: 1.2s
- Database queries: 15 per request
- Memory usage: 45MB

## Optimization Potential
- Database: 70% faster with indexes and query optimization
- API: 60% faster with caching
- Memory: 30% reduction with string builder patterns

## Priority Fixes (by impact)
1. Fix N+1 queries → 2x faster
2. Add missing indexes → 1.5x faster
3. Implement caching → 1.4x faster
```

### Mode 2: Benchmark Specific Area
```
/performance-audit
→ "Benchmark the payment webhook handler"
```

### Mode 3: Compare Before/After
```
/performance-audit
→ "Benchmark after optimization"
```

## Benchmarking

**Run benchmarks:**
```bash
make benchmark
```

**Profile CPU:**
```bash
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof
```

**Profile Memory:**
```bash
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof
```

## Automated Optimizations

For each issue, provide:
1. Current performance metrics
2. Expected performance after fix
3. Code changes needed
4. Benchmark to verify improvement

## Continuous Monitoring

**Best Practice:**
- Run benchmarks on each PR
- Set performance budgets (e.g., no endpoint > 500ms)
- Profile before releases
- Monitor production metrics

This agent optimizes your application performance!
