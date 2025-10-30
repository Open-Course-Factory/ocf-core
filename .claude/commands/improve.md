---
description: Continuous code improvement suggestions and automated refactoring
tags: [improve, refactor, quality, evolution]
---

# Code Improvement Agent

Proactively suggest and apply code improvements based on best practices and patterns.

## Improvement Strategies

### 1. Identify Improvement Opportunities

**Scan codebase for:**

#### A. Code Duplication
**Find repeated code blocks:**
```bash
# Find similar functions
# Find copy-pasted logic
# Find repeated validation patterns
```

**Suggest:**
- Extract to shared utility
- Create reusable function
- Use composition

#### B. Overly Complex Functions
**Identify:**
- Functions > 50 lines
- Cyclomatic complexity > 10
- Deep nesting (> 3 levels)

**Suggest:**
- Extract smaller functions
- Use early returns
- Simplify logic

#### C. Missing Abstractions
**Look for:**
- Repeated interface implementations
- Similar service patterns
- Common data transformations

**Suggest:**
- Create base interface
- Use generic functions
- Extract common patterns

### 2. Code Smell Detection

#### A. Long Parameter Lists
```go
// ❌ SMELL: Too many parameters
func CreateUser(name, email, password, role, org, group, status, avatar string) error
```

**Improvement:**
```go
// ✅ BETTER: Use DTO
type CreateUserInput struct {
    Name     string
    Email    string
    Password string
    Role     string
    // ...
}
func CreateUser(input CreateUserInput) error
```

#### B. God Objects
**Detect:**
- Structs with > 20 methods
- Services handling multiple concerns
- Repository doing business logic

**Suggest:**
- Split into focused services
- Single Responsibility Principle
- Extract related functionality

#### C. Feature Envy
```go
// ❌ SMELL: Using other object's data too much
func (u *User) GetOrganizationName() string {
    return u.Organization.Name // Feature envy
}
```

**Improvement:**
```go
// ✅ BETTER: Move to Organization
func (o *Organization) GetName() string {
    return o.Name
}
```

### 3. Modern Go Patterns

#### A. Use Generics (Go 1.18+)
**Before:**
```go
func MapStrings(items []string, fn func(string) string) []string {
    result := make([]string, len(items))
    for i, item := range items {
        result[i] = fn(item)
    }
    return result
}

func MapInts(items []int, fn func(int) int) []int {
    // Duplicate code
}
```

**After:**
```go
func Map[T any, R any](items []T, fn func(T) R) []R {
    result := make([]R, len(items))
    for i, item := range items {
        result[i] = fn(item)
    }
    return result
}
```

#### B. Use Context Properly
**Improve:**
```go
// ❌ BEFORE: No context
func ProcessItems(items []Item) error

// ✅ AFTER: With cancellation support
func ProcessItems(ctx context.Context, items []Item) error
```

#### C. Error Wrapping
**Improve:**
```go
// ❌ BEFORE: Lose context
return err

// ✅ AFTER: Wrap with context
return fmt.Errorf("failed to process user %s: %w", userID, err)
```

### 4. Performance Improvements

#### A. Preallocate Slices
```go
// ❌ BEFORE
var results []Result
for _, item := range items {
    results = append(results, process(item))
}

// ✅ AFTER
results := make([]Result, 0, len(items))
for _, item := range items {
    results = append(results, process(item))
}
```

#### B. Use sync.Pool for Frequent Allocations
```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func ProcessData(data []byte) string {
    buf := bufferPool.Get().(*bytes.Buffer)
    defer bufferPool.Put(buf)
    buf.Reset()
    // Use buffer
    return buf.String()
}
```

#### C. Concurrent Processing
```go
// ❌ BEFORE: Sequential
for _, user := range users {
    sendEmail(user)
}

// ✅ AFTER: Concurrent with rate limiting
var wg sync.WaitGroup
sem := make(chan struct{}, 10) // Max 10 concurrent
for _, user := range users {
    wg.Add(1)
    go func(u User) {
        defer wg.Done()
        sem <- struct{}{}        // Acquire
        defer func() { <-sem }() // Release
        sendEmail(u)
    }(user)
}
wg.Wait()
```

### 5. Testing Improvements

#### A. Table-Driven Tests
```go
// ❌ BEFORE: Repetitive tests
func TestValidateEmail_Valid(t *testing.T) {
    err := ValidateEmail("test@example.com")
    assert.NoError(t, err)
}

func TestValidateEmail_Invalid(t *testing.T) {
    err := ValidateEmail("invalid")
    assert.Error(t, err)
}

// ✅ AFTER: Table-driven
func TestValidateEmail(t *testing.T) {
    tests := []struct {
        name    string
        email   string
        wantErr bool
    }{
        {"valid email", "test@example.com", false},
        {"invalid format", "invalid", true},
        {"missing @", "testexample.com", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateEmail(tt.email)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

#### B. Test Helpers
```go
// Extract common test setup
func setupTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
    require.NoError(t, err)
    // Common migrations
    return db
}
```

### 6. Documentation Improvements

#### A. Add Package Comments
```go
// Package services provides business logic implementations
// for the OCF Core application.
//
// Services follow clean architecture principles and handle:
// - Business rule validation
// - Permission checks
// - Orchestration of repository operations
package services
```

#### B. Improve Function Comments
```go
// ❌ BEFORE: Vague
// CreateUser creates a user

// ✅ AFTER: Descriptive
// CreateUser creates a new user with the provided details.
// It validates the input, checks for duplicates, hashes the password,
// creates the user's personal organization, and grants appropriate permissions.
//
// Returns ErrEntityAlreadyExists if email is taken.
// Returns ErrValidationFailed if input is invalid.
```

### 7. Structural Improvements

#### A. Consistent Error Handling
**Create error types:**
```go
// errors.go
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s: %s", e.Field, e.Message)
}
```

#### B. Middleware Composition
```go
// Compose middleware for reusability
auth := AuthManagement()
rateLimit := RateLimitMiddleware(100)
cors := CORSMiddleware()

router.Use(cors, rateLimit, auth)
```

## Execution Modes

### Mode 1: Scan for Improvements
```
/improve
→ "Scan codebase for improvement opportunities"
```

**Output:**
```markdown
🔧 Code Improvement Opportunities

## High Impact (Quick Wins)
1. **Code Duplication in Services** (15 instances)
   - Location: src/*/services/*.go
   - Impact: 300 lines of duplicate code
   - Effort: 2 hours
   - Fix: Extract to utils package

2. **Missing Error Wrapping** (45 instances)
   - Impact: Lost error context
   - Effort: 1 hour
   - Fix: Add fmt.Errorf wrapping

## Medium Impact
3. **Non-Table-Driven Tests** (23 tests)
   - Impact: Test maintenance difficulty
   - Effort: 3 hours
   - Fix: Convert to table-driven

4. **Preallocate Slices** (12 instances)
   - Impact: Memory allocations
   - Effort: 30 minutes
   - Fix: Add capacity hints

## Low Impact (Nice to Have)
5. **Missing Package Comments** (8 packages)
   - Impact: Documentation completeness
   - Effort: 1 hour
   - Fix: Add package docs

Total Potential Impact:
- Code reduction: 15%
- Performance gain: 5-10%
- Maintainability: +20%
```

### Mode 2: Apply Specific Improvement
```
/improve
→ "Extract duplicate validation code to utils"
```

### Mode 3: Continuous Improvement
```
/improve
→ "Apply top 3 high-impact improvements"
```

## Improvement Workflow

1. **Scan** → Identify opportunities
2. **Prioritize** → High impact, low effort first
3. **Plan** → Show exact changes
4. **Apply** → Make improvements
5. **Test** → Verify no regressions
6. **Measure** → Confirm improvements

## Progress Tracking

```
📊 Code Quality Evolution

Week 1: 75% quality score
Week 2: 78% (+3%)
Week 3: 82% (+4%)

Improvements Applied:
✅ Extracted 15 duplicate code blocks
✅ Added error wrapping (45 instances)
✅ Converted 12 tests to table-driven
✅ Added missing indexes (8 tables)

Next Targets:
- [ ] Reduce cyclomatic complexity in 5 functions
- [ ] Add package documentation
- [ ] Implement caching layer
```

## Safety

- Always show changes before applying
- Run full test suite after changes
- Revert if any issues
- One improvement category at a time

This agent helps your code evolve continuously!
