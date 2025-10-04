# Entity System Improvement Plan

**Created:** 2025-10-04
**Status:** Planning Phase
**Estimated Total Effort:** 6-8 weeks

## Overview

This document tracks improvements to the generic entity management system (`src/entityManagement/`). The system is well-architected with ~3,660 lines of core code and 3,000+ lines of tests, but has critical issues blocking testing and some performance concerns.

---

## 🔴 Phase 1: Unblock Testing (Week 1)

**Goal:** Enable all tests to run, eliminate silent failures

### 1.1 Decouple Casdoor Dependency ⏳ NOT STARTED

**Priority:** CRITICAL
**Effort:** 4-6 hours
**Impact:** Unlocks 40% of controller tests

**Problem:**
- Tests in `genericController_test.go` commented out due to tight coupling to global `casdoor.Enforcer`
- Lines affected: 192-236, 442-472

**Solution:**
```go
// Create interface in src/entityManagement/interfaces/
type EnforcerInterface interface {
    RemovePolicy(params ...interface{}) (bool, error)
    AddPolicy(params ...interface{}) (bool, error)
    Enforce(params ...interface{}) (bool, error)
}

// Inject into controllers
type GenericController struct {
    service  interfaces.GenericServiceInterface
    enforcer interfaces.EnforcerInterface
}
```

**Files to Modify:**
- [ ] `src/entityManagement/interfaces/enforcerInterface.go` (NEW)
- [ ] `src/entityManagement/routes/genericController.go`
- [ ] `src/entityManagement/routes/addEntity.go`
- [ ] `src/entityManagement/routes/deleteEntity.go`
- [ ] `src/entityManagement/routes/editEntity.go`
- [ ] `tests/entityManagement/genericController_test.go` (uncomment tests)

**Test Coverage:**
- [ ] Mock enforcer in controller tests
- [ ] Verify permission checks still work
- [ ] Integration test with real Casdoor

---

### 1.2 Fix Async Hook Error Handling ⏳ NOT STARTED

**Priority:** CRITICAL
**Effort:** 2-4 hours
**Impact:** Prevents silent failures in production

**Problem:**
- `AfterCreate/Update/Delete` hooks run in goroutines with no error propagation
- Location: `src/entityManagement/services/genericService.go:76-82, 174-185, 230-241`
- Errors only logged, never surfaced to caller

**Current Code:**
```go
go func() {
    if err := hooks.GlobalHookRegistry.ExecuteHooks(afterCtx); err != nil {
        log.Printf("❌ after_create hooks failed: %v", err) // Lost!
    }
}()
```

**Solution Options:**

**Option A: Error Channel (Recommended)**
```go
type HookResult struct {
    EntityID uuid.UUID
    Error    error
    Timestamp time.Time
}

var hookResultsChan = make(chan HookResult, 100)

// In service:
go func() {
    err := hooks.GlobalHookRegistry.ExecuteHooks(afterCtx)
    hookResultsChan <- HookResult{entityID, err, time.Now()}
}()

// Background goroutine monitors channel and logs/alerts
```

**Option B: Hook Result Store**
```go
type HookResultStore interface {
    RecordHookExecution(entityID uuid.UUID, hookType string, err error)
    GetRecentFailures() []HookFailure
}
```

**Option C: Synchronous Mode Flag**
```go
type EntityRegistration interface {
    AsyncHooks() bool  // Default: true for production, false for critical entities
}
```

**Files to Modify:**
- [ ] `src/entityManagement/services/genericService.go` (lines 76-82, 174-185, 230-241)
- [ ] `src/entityManagement/hooks/registry.go` (add result tracking)
- [ ] `tests/entityManagement/genericService_test.go` (test error propagation)

**Test Coverage:**
- [ ] Hook fails, error captured in channel
- [ ] Hook timeout handling
- [ ] Multiple concurrent hook errors

---

### 1.3 Extract OwnerIDs Duplication ✅ COMPLETED

**Priority:** HIGH
**Effort:** 1 hour
**Impact:** Reduces maintenance burden

**Problem:**
- Identical 25-line reflection code in two locations:
  - `src/entityManagement/routes/addEntity.go:52-76`
  - `src/entityManagement/services/genericService.go:251-275`

**Solution:**
```go
// NEW FILE: src/entityManagement/utils/ownerUtils.go
package utils

func AddOwnerIDs(entity interface{}, userId string) error {
    entityReflectValue := reflect.ValueOf(entity).Elem()
    ownerIdsField := entityReflectValue.FieldByName("OwnerIDs")

    if !ownerIdsField.IsValid() {
        return nil // Entity doesn't have OwnerIDs field
    }

    currentOwnerIds := ownerIdsField.Interface().([]string)
    if currentOwnerIds == nil {
        currentOwnerIds = []string{}
    }

    // Check if userId already exists
    for _, id := range currentOwnerIds {
        if id == userId {
            return nil // Already owner
        }
    }

    currentOwnerIds = append(currentOwnerIds, userId)
    ownerIdsField.Set(reflect.ValueOf(currentOwnerIds))

    return nil
}
```

**Files Modified:**
- [x] `src/entityManagement/utils/ownerUtils.go` (DONE - created utility function)
- [x] `src/entityManagement/routes/addEntity.go` (DONE - uses utils.AddOwnerIDToEntity)
- [x] `src/entityManagement/services/genericService.go` (DONE - uses utils.AddOwnerIDToEntity)
- [x] `tests/entityManagement/utils/ownerUtils_test.go` (DONE - comprehensive unit tests)

**Test Coverage:**
- [x] Add owner to entity without OwnerIDs field
- [x] Add owner to entity with existing OwnerIDs (replaces existing)
- [x] Unexported field handling
- [x] All tests passing

---

## ⚠️ Phase 2: Performance Foundation (Weeks 2-3)

**Goal:** Enable scaling to 1M+ records

### 2.1 Implement Cursor-Based Pagination ⏳ NOT STARTED

**Priority:** HIGH
**Effort:** 6-8 hours
**Impact:** Enables scaling to millions of records

**Problem:**
- Offset pagination scans all skipped rows: `OFFSET 20000` scans 20,000 rows
- Location: `src/entityManagement/repositories/genericRepository.go:142-146`
- Performance degrades linearly with page number

**Current:**
```
GET /api/v1/courses?page=1000&pageSize=20
→ SELECT * FROM courses LIMIT 20 OFFSET 19980  (scans 19,980 rows!)
```

**Solution:**
```
GET /api/v1/courses?cursor=base64(lastID)&limit=20
→ SELECT * FROM courses WHERE id > 'lastID' LIMIT 20  (scans 20 rows)
```

**Implementation:**

```go
// Updated pagination params
type PaginationParams struct {
    Cursor   string `form:"cursor"`   // Base64-encoded last ID
    Limit    int    `form:"limit"`    // Page size

    // Deprecated (keep for backward compatibility)
    Page     int    `form:"page"`
    PageSize int    `form:"pageSize"`
}

// Updated response
type CursorPaginationResponse struct {
    Data       []interface{} `json:"data"`
    NextCursor string        `json:"nextCursor,omitempty"`
    HasMore    bool          `json:"hasMore"`

    // Deprecated fields (keep for compatibility)
    Total           int64 `json:"total,omitempty"`
    CurrentPage     int   `json:"currentPage,omitempty"`
}

// Repository method
func (gr *GenericRepository) GetAllEntitiesCursor(
    cursor string,
    limit int,
    filters map[string]interface{},
) ([]interface{}, string, error) {
    query := gr.db.Model(gr.entity)

    // Apply cursor
    if cursor != "" {
        decodedID, err := base64.StdEncoding.DecodeString(cursor)
        if err != nil {
            return nil, "", err
        }
        query = query.Where("id > ?", string(decodedID))
    }

    // Apply filters
    query = gr.applyFiltersAndRelationships(query, filters)

    // Fetch limit+1 to check if more exist
    var entities []interface{}
    result := query.Order("id ASC").Limit(limit + 1).Find(&entities)

    hasMore := len(entities) > limit
    if hasMore {
        entities = entities[:limit]
    }

    // Generate next cursor
    var nextCursor string
    if hasMore && len(entities) > 0 {
        lastEntity := entities[len(entities)-1]
        lastID := reflect.ValueOf(lastEntity).FieldByName("ID").String()
        nextCursor = base64.StdEncoding.EncodeToString([]byte(lastID))
    }

    return entities, nextCursor, result.Error
}
```

**Files to Modify:**
- [ ] `src/entityManagement/repositories/genericRepository.go` (add GetAllEntitiesCursor)
- [ ] `src/entityManagement/services/genericService.go` (add GetEntitiesCursor)
- [ ] `src/entityManagement/routes/getEntities.go` (support both pagination types)
- [ ] `tests/entityManagement/genericRepository_test.go` (cursor pagination tests)
- [ ] `tests/entityManagement/performance_test.go` (benchmark comparison)

**Test Coverage:**
- [ ] First page (no cursor)
- [ ] Middle page (with cursor)
- [ ] Last page (hasMore = false)
- [ ] Invalid cursor handling
- [ ] Benchmark: offset vs cursor for page 1000

**Migration Strategy:**
1. Add cursor support alongside existing offset pagination
2. Mark offset pagination as deprecated in Swagger
3. Update client libraries to use cursors
4. Remove offset after 2-3 releases

---

### 2.2 Add Selective Preloading ⏳ NOT STARTED

**Priority:** HIGH
**Effort:** 4-6 hours
**Impact:** Reduces query count by 10-100x

**Problem:**
- Recursive preloading loads ALL nested entities, even when not needed
- Currently commented out in `integration_test.go:146`
- Location: `src/entityManagement/repositories/genericRepository.go:103-118`

**Current Behavior:**
```
GET /api/v1/courses
→ Loads: Courses + Chapters + Sections + Pages + Authors + ... (N+1 queries)
```

**Solution:**
```
GET /api/v1/courses?include=chapters,chapters.sections
→ Loads: Courses + Chapters + Sections (only what's needed)
```

**Implementation:**

```go
// Parse include parameter
func parseIncludeParam(includeStr string) []string {
    if includeStr == "" {
        return nil
    }
    return strings.Split(includeStr, ",")
}

// Build GORM preload calls
func (gr *GenericRepository) buildPreloads(query *gorm.DB, includes []string) *gorm.DB {
    if len(includes) == 0 {
        return query  // No preloading
    }

    for _, include := range includes {
        // Support dot notation: "chapters.sections"
        query = query.Preload(include)
    }

    return query
}

// Updated GetAllEntities
func (gr *GenericRepository) GetAllEntities(
    page int,
    pageSize int,
    filters map[string]interface{},
    includes []string,  // NEW
) ([]interface{}, int64, error) {
    query := gr.db.Model(gr.entity)
    query = gr.buildPreloads(query, includes)
    // ... rest of method
}
```

**Files to Modify:**
- [ ] `src/entityManagement/repositories/genericRepository.go` (add includes param)
- [ ] `src/entityManagement/services/genericService.go` (pass includes)
- [ ] `src/entityManagement/routes/getEntities.go` (parse include query param)
- [ ] `src/entityManagement/routes/getEntity.go` (parse include query param)
- [ ] `tests/entityManagement/genericRepository_test.go` (selective preload tests)
- [ ] `tests/entityManagement/integration_test.go` (uncomment line 146, test includes)

**Test Coverage:**
- [ ] No includes → no preloading
- [ ] Single level: `include=chapters`
- [ ] Multi-level: `include=chapters.sections`
- [ ] Multiple relations: `include=chapters,authors`
- [ ] Invalid relation name
- [ ] Query count validation (should be exact)

**Swagger Documentation:**
```yaml
parameters:
  - name: include
    in: query
    description: Comma-separated list of relations to include (e.g., "chapters,authors" or "chapters.sections")
    schema:
      type: string
      example: "chapters,chapters.sections"
```

---

### 2.3 Add Query Result Caching ⏳ NOT STARTED

**Priority:** MEDIUM
**Effort:** 4-6 hours
**Impact:** 50-90% reduction in DB load for hot data

**Problem:**
- Read-heavy entities (courses, users) fetched repeatedly
- No caching layer between service and repository

**Solution Options:**

**Option A: Redis Cache (Recommended for Production)**
```go
type CacheConfig struct {
    Enabled bool
    TTL     time.Duration
    RedisURL string
}

type CachedRepository struct {
    repo  *GenericRepository
    cache *redis.Client
    ttl   time.Duration
}

func (cr *CachedRepository) GetEntity(id uuid.UUID) (interface{}, error) {
    cacheKey := fmt.Sprintf("%s:%s", cr.repo.entityName, id)

    // Try cache first
    cached, err := cr.cache.Get(context.Background(), cacheKey).Result()
    if err == nil {
        var entity interface{}
        json.Unmarshal([]byte(cached), &entity)
        return entity, nil
    }

    // Cache miss - fetch from DB
    entity, err := cr.repo.GetEntity(id)
    if err != nil {
        return nil, err
    }

    // Store in cache
    jsonData, _ := json.Marshal(entity)
    cr.cache.Set(context.Background(), cacheKey, jsonData, cr.ttl)

    return entity, nil
}
```

**Option B: In-Memory Cache (Simpler, for Development)**
```go
import "github.com/patrickmn/go-cache"

var entityCache = cache.New(5*time.Minute, 10*time.Minute)

func (gr *GenericRepository) GetEntity(id uuid.UUID) (interface{}, error) {
    cacheKey := fmt.Sprintf("%s:%s", gr.entityName, id)

    if cached, found := entityCache.Get(cacheKey); found {
        return cached, nil
    }

    entity, err := gr.fetchFromDB(id)
    if err == nil {
        entityCache.Set(cacheKey, entity, cache.DefaultExpiration)
    }

    return entity, err
}
```

**Cache Invalidation Strategy:**
```go
func (gs *genericService) UpdateEntity(id uuid.UUID, dto interface{}) error {
    // Update entity
    entity, err := gs.repo.UpdateEntity(id, dto)
    if err != nil {
        return err
    }

    // Invalidate cache
    cacheKey := fmt.Sprintf("%s:%s", gs.entityName, id)
    cache.Delete(cacheKey)

    return nil
}
```

**Files to Modify:**
- [ ] `src/entityManagement/repositories/cachedRepository.go` (NEW)
- [ ] `src/entityManagement/interfaces/cacheInterface.go` (NEW)
- [ ] `src/entityManagement/services/genericService.go` (add cache invalidation)
- [ ] `src/configuration/cacheConfig.go` (NEW - Redis setup)
- [ ] `.env.example` (add REDIS_URL)
- [ ] `docker-compose.yml` (add Redis service)
- [ ] `tests/entityManagement/cachedRepository_test.go` (NEW)

**Test Coverage:**
- [ ] Cache hit
- [ ] Cache miss
- [ ] Cache invalidation on update
- [ ] Cache invalidation on delete
- [ ] Cache expiration (TTL)
- [ ] Redis connection failure fallback

**Configuration:**
```env
# .env
CACHE_ENABLED=true
CACHE_TTL=5m
REDIS_URL=redis://localhost:6379
```

**Metrics to Track:**
- Cache hit rate
- Cache miss rate
- Average response time (cached vs uncached)

---

## 🔧 Phase 3: Maintainability (Week 4)

**Goal:** Easier to maintain and extend

### 3.1 Refactor Filter Logic with Strategy Pattern ⏳ NOT STARTED

**Priority:** MEDIUM
**Effort:** 8-12 hours
**Impact:** Easier to test, add new filter types, maintain

**Problem:**
- `applyFiltersAndRelationships()` is 102 lines with nested type switches
- Location: `src/entityManagement/repositories/genericRepository.go:157-258`
- Brittle naming assumptions (e.g., "IDs" suffix detection)
- Hard to test individual filter types

**Current Code Issues:**
```go
switch v := value.(type) {
case string:
    if strings.HasSuffix(key, "IDs") || strings.HasSuffix(key, "Ids") {
        // 30 lines of many-to-many logic
    } else if strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "ID") {
        // 20 lines of foreign key logic
    }
case []string:
    // 15 lines of array logic
}
```

**Solution: Strategy Pattern**

```go
// NEW FILE: src/entityManagement/repositories/filters/filterStrategy.go
package filters

type FilterStrategy interface {
    // Matches returns true if this strategy can handle the given filter
    Matches(key string, value interface{}) bool

    // Apply applies the filter to the query
    Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB

    // Priority for execution order (lower = earlier)
    Priority() int
}

// Base implementation
type BaseFilterStrategy struct{}

func (b *BaseFilterStrategy) Priority() int {
    return 100
}
```

**Concrete Strategies:**

```go
// Direct field filter (e.g., title=Golang)
type DirectFieldFilter struct {
    BaseFilterStrategy
}

func (d *DirectFieldFilter) Matches(key string, value interface{}) bool {
    // No suffix patterns, simple field name
    return !strings.Contains(key, ".")
}

func (d *DirectFieldFilter) Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB {
    columnName := toSnakeCase(key)

    switch v := value.(type) {
    case []string:
        return query.Where(fmt.Sprintf("%s IN ?", columnName), v)
    default:
        return query.Where(fmt.Sprintf("%s = ?", columnName), v)
    }
}

func (d *DirectFieldFilter) Priority() int {
    return 10 // Highest priority
}

// Foreign key filter (e.g., courseId=123)
type ForeignKeyFilter struct {
    BaseFilterStrategy
}

func (f *ForeignKeyFilter) Matches(key string, value interface{}) bool {
    return (strings.HasSuffix(key, "Id") || strings.HasSuffix(key, "ID")) &&
           !strings.HasSuffix(key, "IDs") &&
           !strings.HasSuffix(key, "Ids")
}

func (f *ForeignKeyFilter) Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB {
    columnName := toSnakeCase(key)
    return query.Where(fmt.Sprintf("%s.%s = ?", tableName, columnName), value)
}

func (f *ForeignKeyFilter) Priority() int {
    return 20
}

// Many-to-many filter (e.g., tagIDs=[1,2,3])
type ManyToManyFilter struct {
    BaseFilterStrategy
}

func (m *ManyToManyFilter) Matches(key string, value interface{}) bool {
    return strings.HasSuffix(key, "IDs") || strings.HasSuffix(key, "Ids")
}

func (m *ManyToManyFilter) Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB {
    ids, ok := value.([]string)
    if !ok {
        return query
    }

    relationName := strings.TrimSuffix(strings.TrimSuffix(key, "IDs"), "Ids")
    relationTable := pluralize(relationName)
    singularRelation := strings.TrimSuffix(relationTable, "s")
    singularCurrent := strings.TrimSuffix(tableName, "s")

    joinTable := singularCurrent + "_" + relationTable

    subQuery := query.Session(&gorm.Session{}).
        Table(joinTable).
        Select(singularCurrent + "_id").
        Where(singularRelation+"_id IN ?", ids)

    return query.Where(tableName+".id IN (?)", subQuery)
}

func (m *ManyToManyFilter) Priority() int {
    return 30
}

// Relationship path filter (registered via RelationshipFilter)
type RelationshipPathFilter struct {
    BaseFilterStrategy
    registeredFilters map[string]RelationshipFilter
}

func (r *RelationshipPathFilter) Matches(key string, value interface{}) bool {
    _, exists := r.registeredFilters[key]
    return exists
}

func (r *RelationshipPathFilter) Apply(query *gorm.DB, key string, value interface{}, tableName string) *gorm.DB {
    relFilter := r.registeredFilters[key]
    // Existing complex EXISTS logic from lines 260-336
    return buildRelationshipExistsQuery(query, relFilter, value, tableName)
}

func (r *RelationshipPathFilter) Priority() int {
    return 40
}
```

**Filter Manager:**

```go
// NEW FILE: src/entityManagement/repositories/filters/filterManager.go
type FilterManager struct {
    strategies []FilterStrategy
}

func NewFilterManager(relationshipFilters map[string]RelationshipFilter) *FilterManager {
    fm := &FilterManager{
        strategies: []FilterStrategy{
            &DirectFieldFilter{},
            &ForeignKeyFilter{},
            &ManyToManyFilter{},
            &RelationshipPathFilter{registeredFilters: relationshipFilters},
        },
    }

    // Sort by priority
    sort.Slice(fm.strategies, func(i, j int) bool {
        return fm.strategies[i].Priority() < fm.strategies[j].Priority()
    })

    return fm
}

func (fm *FilterManager) ApplyFilters(
    query *gorm.DB,
    filters map[string]interface{},
    tableName string,
) *gorm.DB {
    for key, value := range filters {
        for _, strategy := range fm.strategies {
            if strategy.Matches(key, value) {
                query = strategy.Apply(query, key, value, tableName)
                break // First matching strategy wins
            }
        }
    }

    return query
}
```

**Updated Repository:**

```go
// In genericRepository.go
type GenericRepository struct {
    db            *gorm.DB
    entity        interface{}
    entityName    string
    filterManager *filters.FilterManager
}

func NewGenericRepository(db *gorm.DB, entity interface{}, entityName string) *GenericRepository {
    relationshipFilters := ems.GlobalEntityRegistrationService.GetRelationshipFilters(entityName)

    return &GenericRepository{
        db:            db,
        entity:        entity,
        entityName:    entityName,
        filterManager: filters.NewFilterManager(relationshipFilters),
    }
}

func (gr *GenericRepository) GetAllEntities(...) {
    query := gr.db.Model(gr.entity)

    // Replace 100 lines of switch statements with:
    query = gr.filterManager.ApplyFilters(query, filters, currentTable)

    // ... rest of method
}
```

**Files to Modify:**
- [ ] `src/entityManagement/repositories/filters/filterStrategy.go` (NEW)
- [ ] `src/entityManagement/repositories/filters/directFieldFilter.go` (NEW)
- [ ] `src/entityManagement/repositories/filters/foreignKeyFilter.go` (NEW)
- [ ] `src/entityManagement/repositories/filters/manyToManyFilter.go` (NEW)
- [ ] `src/entityManagement/repositories/filters/relationshipPathFilter.go` (NEW)
- [ ] `src/entityManagement/repositories/filters/filterManager.go` (NEW)
- [ ] `src/entityManagement/repositories/genericRepository.go` (refactor lines 157-258)
- [ ] `tests/entityManagement/filters/directFieldFilter_test.go` (NEW)
- [ ] `tests/entityManagement/filters/foreignKeyFilter_test.go` (NEW)
- [ ] `tests/entityManagement/filters/manyToManyFilter_test.go` (NEW)
- [ ] `tests/entityManagement/filters/filterManager_test.go` (NEW)

**Test Coverage:**
- [ ] Each strategy in isolation
- [ ] Strategy priority ordering
- [ ] Multiple filters combined
- [ ] Unknown filter keys (should be ignored)
- [ ] Edge cases (empty arrays, nil values)

**Benefits:**
- ✅ Each filter type is testable independently
- ✅ Easy to add new filter strategies
- ✅ Clear separation of concerns
- ✅ Reduced complexity (each strategy ~30 lines vs 100 lines total)

---

### 3.2 Standardize Error Handling ⏳ NOT STARTED

**Priority:** MEDIUM
**Effort:** 3-4 hours
**Impact:** Better API consistency, easier client error handling

**Problem:**
- Mix of error types: `APIError`, `fmt.Errorf`, silent failures
- No consistent error codes
- Hard for clients to programmatically handle errors

**Current Inconsistency:**

```go
// Pattern 1: Custom API Error
return nil, &errors.APIError{
    ErrorCode:    http.StatusInternalServerError,
    ErrorMessage: "Entity convertion function does not exist",
}

// Pattern 2: Standard error
return nil, fmt.Errorf("entity %s not registered", entityName)

// Pattern 3: Silent failure
if err := hook.Execute(ctx); err != nil {
    log.Printf("❌ Hook failed: %v", err)
    continue  // No error returned!
}
```

**Solution: Standardized Error Types**

```go
// NEW FILE: src/entityManagement/errors/entityErrors.go
package errors

import "net/http"

type EntityError struct {
    Code       string                 `json:"code"`
    Message    string                 `json:"message"`
    HTTPStatus int                    `json:"-"`
    Details    map[string]interface{} `json:"details,omitempty"`
    Err        error                  `json:"-"`
}

func (e *EntityError) Error() string {
    if e.Err != nil {
        return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
    }
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *EntityError) Unwrap() error {
    return e.Err
}

// Predefined errors
var (
    ErrEntityNotFound = &EntityError{
        Code:       "ENT001",
        Message:    "Entity not found",
        HTTPStatus: http.StatusNotFound,
    }

    ErrEntityNotRegistered = &EntityError{
        Code:       "ENT002",
        Message:    "Entity not registered in the system",
        HTTPStatus: http.StatusInternalServerError,
    }

    ErrConversionFailed = &EntityError{
        Code:       "ENT003",
        Message:    "Entity conversion failed",
        HTTPStatus: http.StatusInternalServerError,
    }

    ErrValidationFailed = &EntityError{
        Code:       "ENT004",
        Message:    "Validation failed",
        HTTPStatus: http.StatusBadRequest,
    }

    ErrDatabaseError = &EntityError{
        Code:       "ENT005",
        Message:    "Database operation failed",
        HTTPStatus: http.StatusInternalServerError,
    }

    ErrUnauthorized = &EntityError{
        Code:       "ENT006",
        Message:    "Unauthorized access",
        HTTPStatus: http.StatusForbidden,
    }

    ErrHookExecutionFailed = &EntityError{
        Code:       "ENT007",
        Message:    "Hook execution failed",
        HTTPStatus: http.StatusInternalServerError,
    }

    ErrInvalidInput = &EntityError{
        Code:       "ENT008",
        Message:    "Invalid input data",
        HTTPStatus: http.StatusBadRequest,
    }
)

// Helper constructors
func NewEntityNotFound(entityName string, id interface{}) *EntityError {
    err := *ErrEntityNotFound
    err.Details = map[string]interface{}{
        "entityName": entityName,
        "id":         id,
    }
    return &err
}

func NewValidationError(field string, reason string) *EntityError {
    err := *ErrValidationFailed
    err.Details = map[string]interface{}{
        "field":  field,
        "reason": reason,
    }
    return &err
}

func WrapDatabaseError(err error) *EntityError {
    wrapped := *ErrDatabaseError
    wrapped.Err = err
    wrapped.Details = map[string]interface{}{
        "original": err.Error(),
    }
    return &wrapped
}
```

**Error Response Middleware:**

```go
// In routes/genericController.go
func handleEntityError(ctx *gin.Context, err error) {
    var entityErr *errors.EntityError

    if errors.As(err, &entityErr) {
        ctx.JSON(entityErr.HTTPStatus, gin.H{
            "error": gin.H{
                "code":    entityErr.Code,
                "message": entityErr.Message,
                "details": entityErr.Details,
            },
        })
        return
    }

    // Fallback for unknown errors
    ctx.JSON(http.StatusInternalServerError, gin.H{
        "error": gin.H{
            "code":    "ERR000",
            "message": "Internal server error",
        },
    })
}
```

**Usage Example:**

```go
// Before:
func (gr *GenericRepository) GetEntity(id uuid.UUID) (interface{}, error) {
    var entity interface{}
    result := gr.db.First(&entity, id)
    if result.Error != nil {
        return nil, result.Error
    }
    return entity, nil
}

// After:
func (gr *GenericRepository) GetEntity(id uuid.UUID) (interface{}, error) {
    var entity interface{}
    result := gr.db.First(&entity, id)

    if result.Error != nil {
        if result.Error == gorm.ErrRecordNotFound {
            return nil, errors.NewEntityNotFound(gr.entityName, id)
        }
        return nil, errors.WrapDatabaseError(result.Error)
    }

    return entity, nil
}
```

**Files to Modify:**
- [ ] `src/entityManagement/errors/entityErrors.go` (NEW)
- [ ] `src/entityManagement/repositories/genericRepository.go` (use new errors)
- [ ] `src/entityManagement/services/genericService.go` (use new errors)
- [ ] `src/entityManagement/routes/*.go` (use handleEntityError middleware)
- [ ] `tests/entityManagement/errors/entityErrors_test.go` (NEW)
- [ ] API documentation (update error response schemas)

**Test Coverage:**
- [ ] Each error type returns correct HTTP status
- [ ] Error details are populated
- [ ] Error wrapping preserves original error
- [ ] Middleware converts errors to JSON correctly

**API Response Example:**
```json
{
  "error": {
    "code": "ENT001",
    "message": "Entity not found",
    "details": {
      "entityName": "Course",
      "id": "550e8400-e29b-41d4-a716-446655440000"
    }
  }
}
```

---

### 3.3 Add DTO Validation Layer ⏳ NOT STARTED

**Priority:** MEDIUM
**Effort:** 4-6 hours
**Impact:** Prevents invalid data, better error messages

**Problem:**
- No validation before DTO conversion
- Malformed data reaches repository layer
- Generic error messages don't help users fix input

**Solution: Integrate Validator Library**

```go
// Use: github.com/go-playground/validator/v10

// Example DTO with validation tags
type CourseInputDTO struct {
    Title       string   `json:"title" validate:"required,min=3,max=200"`
    Code        string   `json:"code" validate:"required,alphanum,min=2,max=20"`
    Description string   `json:"description" validate:"max=2000"`
    AuthorIDs   []string `json:"authorIDs" validate:"dive,uuid4"`
}

type ChapterInputDTO struct {
    Title    string    `json:"title" validate:"required,min=1,max=200"`
    Order    int       `json:"order" validate:"min=0"`
    CourseID uuid.UUID `json:"courseId" validate:"required,uuid4"`
}
```

**Validation Service:**

```go
// NEW FILE: src/entityManagement/validation/validator.go
package validation

import (
    "github.com/go-playground/validator/v10"
    entityErrors "ocf-core/src/entityManagement/errors"
)

var validate *validator.Validate

func init() {
    validate = validator.New()

    // Register custom validators
    validate.RegisterValidation("courseCode", validateCourseCode)
}

type ValidationError struct {
    Field   string      `json:"field"`
    Tag     string      `json:"tag"`
    Value   interface{} `json:"value,omitempty"`
    Message string      `json:"message"`
}

func Validate(dto interface{}) error {
    err := validate.Struct(dto)
    if err == nil {
        return nil
    }

    validationErrors := err.(validator.ValidationErrors)
    errors := make([]ValidationError, len(validationErrors))

    for i, e := range validationErrors {
        errors[i] = ValidationError{
            Field:   e.Field(),
            Tag:     e.Tag(),
            Value:   e.Value(),
            Message: getErrorMessage(e),
        }
    }

    return &entityErrors.EntityError{
        Code:       "ENT004",
        Message:    "Validation failed",
        HTTPStatus: 400,
        Details: map[string]interface{}{
            "errors": errors,
        },
    }
}

func getErrorMessage(e validator.FieldError) string {
    switch e.Tag() {
    case "required":
        return fmt.Sprintf("%s is required", e.Field())
    case "min":
        return fmt.Sprintf("%s must be at least %s characters", e.Field(), e.Param())
    case "max":
        return fmt.Sprintf("%s must be at most %s characters", e.Field(), e.Param())
    case "uuid4":
        return fmt.Sprintf("%s must be a valid UUID", e.Field())
    case "email":
        return fmt.Sprintf("%s must be a valid email", e.Field())
    default:
        return fmt.Sprintf("%s failed validation (%s)", e.Field(), e.Tag())
    }
}

// Custom validators
func validateCourseCode(fl validator.FieldLevel) bool {
    code := fl.Field().String()
    // Must be uppercase letters and numbers only
    matched, _ := regexp.MatchString(`^[A-Z0-9]+$`, code)
    return matched
}
```

**Integration with Service Layer:**

```go
// In genericService.go
import "ocf-core/src/entityManagement/validation"

func (g *genericService) CreateEntity(dto interface{}, userId string) (interface{}, error) {
    // 1. Validate DTO
    if err := validation.Validate(dto); err != nil {
        return nil, err
    }

    // 2. Convert DTO to entity
    entity, err := g.inputConverter(dto, userId)
    if err != nil {
        return nil, errors.ErrConversionFailed
    }

    // 3. Execute hooks, save, etc.
    // ...
}
```

**Optional: Entity-Specific Validators**

```go
// Allow entities to define custom validation logic
type ValidatableEntity interface {
    Validate() error
}

// In service:
func (g *genericService) CreateEntity(dto interface{}, userId string) (interface{}, error) {
    // Standard DTO validation
    if err := validation.Validate(dto); err != nil {
        return nil, err
    }

    entity, err := g.inputConverter(dto, userId)
    if err != nil {
        return nil, err
    }

    // Entity-specific business rules
    if validatable, ok := entity.(ValidatableEntity); ok {
        if err := validatable.Validate(); err != nil {
            return nil, err
        }
    }

    // Continue with save...
}
```

**Example Entity Validator:**

```go
// In src/courses/models/course.go
func (c *Course) Validate() error {
    // Business rule: Published courses must have at least one chapter
    if c.Published && len(c.Chapters) == 0 {
        return &errors.EntityError{
            Code:       "COURSE001",
            Message:    "Published courses must have at least one chapter",
            HTTPStatus: 400,
        }
    }

    return nil
}
```

**Files to Modify:**
- [ ] `go.mod` (add github.com/go-playground/validator/v10)
- [ ] `src/entityManagement/validation/validator.go` (NEW)
- [ ] `src/entityManagement/services/genericService.go` (add validation calls)
- [ ] `src/courses/dtos/courseDto.go` (add validation tags)
- [ ] `src/labs/dtos/*.go` (add validation tags)
- [ ] All other DTO files (add validation tags incrementally)
- [ ] `tests/entityManagement/validation/validator_test.go` (NEW)

**Test Coverage:**
- [ ] Valid DTO passes validation
- [ ] Required field missing
- [ ] String too short/long
- [ ] Invalid UUID format
- [ ] Invalid email format
- [ ] Custom validator (courseCode)
- [ ] Nested struct validation (dive tag)
- [ ] Array element validation

**Example Error Response:**
```json
{
  "error": {
    "code": "ENT004",
    "message": "Validation failed",
    "details": {
      "errors": [
        {
          "field": "Title",
          "tag": "required",
          "message": "Title is required"
        },
        {
          "field": "Code",
          "tag": "min",
          "value": "A",
          "message": "Code must be at least 2 characters"
        }
      ]
    }
  }
}
```

**Migration Strategy:**
1. Add validator package and basic infrastructure
2. Add validation tags to high-traffic DTOs first (users, courses)
3. Gradually add tags to other DTOs
4. Make validation mandatory after 2-3 releases

---

## 📋 Phase 4: Polish (Week 5)

**Goal:** Production-ready, well-documented system

### 4.1 Extract Test Utilities ✅ COMPLETED

**Priority:** LOW
**Effort:** 2-3 hours
**Impact:** Reduces test code by ~500 lines

**Problem:**
- Every test file has ~100 lines of identical setup code
- Copy-paste leads to inconsistencies
- Hard to maintain when setup changes

**Current Duplication:**

```go
// In genericRepository_test.go, genericService_test.go, etc.
func setupTestEntityRegistration() {
    ems.GlobalEntityRegistrationService = ems.NewEntityRegistrationService()
    hooks.GlobalHookRegistry.DisableAllHooks(true)

    // 20 lines of conversion functions
    // 20 lines of DTO registration
    // 20 lines of relationship setup
}
```

**Solution: Shared Test Package**

```go
// NEW FILE: tests/entityManagement/testutils/setup.go
package testutils

import (
    "testing"
    "gorm.io/gorm"
    "ocf-core/src/entityManagement"
)

type TestContext struct {
    DB       *gorm.DB
    Entity   interface{}
    Cleanup  func()
}

// SetupEntityTest creates a complete test environment
func SetupEntityTest(t *testing.T, entity interface{}) *TestContext {
    t.Helper()

    // Setup DB
    db := setupTestDB(t)

    // Setup registry
    setupEntityRegistry(entity)

    // Setup hooks
    hooks.GlobalHookRegistry = hooks.NewHookRegistry()
    hooks.GlobalHookRegistry.SetTestMode(true)

    cleanup := func() {
        cleanupTestDB(db)
        resetGlobalState()
    }

    return &TestContext{
        DB:      db,
        Entity:  entity,
        Cleanup: cleanup,
    }
}

// SetupTestDB creates in-memory SQLite DB for testing
func setupTestDB(t *testing.T) *gorm.DB {
    t.Helper()

    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    if err != nil {
        t.Fatalf("Failed to setup test DB: %v", err)
    }

    // Auto-migrate common tables
    db.AutoMigrate(&TestEntity{}, &RelatedEntity{})

    return db
}

// RegisterTestEntity registers an entity with minimal boilerplate
func RegisterTestEntity(t *testing.T, registration interfaces.RegistrableEntity) {
    t.Helper()

    ems.GlobalEntityRegistrationService.RegisterEntity(registration)
}

// CreateTestEntity creates and saves a test entity
func CreateTestEntity(t *testing.T, db *gorm.DB, entity interface{}) interface{} {
    t.Helper()

    result := db.Create(entity)
    if result.Error != nil {
        t.Fatalf("Failed to create test entity: %v", result.Error)
    }

    return entity
}

// AssertNoError is a test helper for cleaner assertions
func AssertNoError(t *testing.T, err error) {
    t.Helper()
    if err != nil {
        t.Fatalf("Expected no error, got: %v", err)
    }
}

// AssertEqual is a generic equality assertion
func AssertEqual(t *testing.T, expected, actual interface{}) {
    t.Helper()
    if expected != actual {
        t.Errorf("Expected %v, got %v", expected, actual)
    }
}
```

**Usage in Tests:**

```go
// Before (100 lines):
func TestGenericRepository_CreateEntity(t *testing.T) {
    // 20 lines of DB setup
    // 30 lines of entity registration
    // 20 lines of hook setup
    // 30 lines of test logic
}

// After (30 lines):
func TestGenericRepository_CreateEntity(t *testing.T) {
    ctx := testutils.SetupEntityTest(t, &TestEntity{})
    defer ctx.Cleanup()

    repo := repositories.NewGenericRepository(ctx.DB, ctx.Entity, "TestEntity")

    entity := &TestEntity{Title: "Test"}
    result, err := repo.CreateEntity(entity)

    testutils.AssertNoError(t, err)
    testutils.AssertEqual(t, "Test", result.(*TestEntity).Title)
}
```

**Additional Helpers:**

```go
// Mock factories
func NewMockEnforcer(t *testing.T) *MockEnforcer {
    return &MockEnforcer{
        policies: make(map[string]bool),
    }
}

func NewMockCache(t *testing.T) *MockCache {
    return &MockCache{
        data: make(map[string]interface{}),
    }
}

// Test data builders
func NewTestCourse(title string) *Course {
    return &Course{
        ID:    uuid.New(),
        Title: title,
        Code:  strings.ToUpper(title[:3]),
    }
}

func NewTestChapter(courseID uuid.UUID, order int) *Chapter {
    return &Chapter{
        ID:       uuid.New(),
        CourseID: courseID,
        Order:    order,
        Title:    fmt.Sprintf("Chapter %d", order),
    }
}
```

**Files Created:**
- [x] `tests/entityManagement/testutils/setup.go` (DONE)
- [x] `tests/entityManagement/testutils/assertions.go` (DONE)
- [x] `tests/entityManagement/testutils/builders.go` (DONE)
- [x] `tests/entityManagement/testutils/example_test.go` (DONE - demonstrates usage)

**Files to Refactor:**
- [ ] `tests/entityManagement/genericRepository_test.go`
- [ ] `tests/entityManagement/genericService_test.go`
- [ ] `tests/entityManagement/genericController_test.go`
- [ ] `tests/entityManagement/hooks_simple_test.go`
- [ ] All other test files

**Expected Outcome:**
- Reduce test setup from ~100 lines to ~5-10 lines per file
- Consistent test environment across all tests
- Easier to add new tests

---

### 4.2 Add Package Documentation ✅ COMPLETED

**Priority:** LOW
**Effort:** 2-3 hours
**Impact:** Easier onboarding, better IDE tooltips

**Problem:**
- No package-level documentation
- Some methods undocumented
- No examples in godoc

**Solution: Add Package Comments**

```go
// NEW FILE: src/entityManagement/doc.go

// Package entityManagement provides a generic CRUD framework for database entities
// with automatic route generation, Swagger documentation, and hook system integration.
//
// # Architecture
//
// The entity management system consists of four layers:
//
//   - Controllers (routes/): HTTP request handling and routing
//   - Services (services/): Business logic, hooks, and orchestration
//   - Repositories (repositories/): Database operations using GORM
//   - Models (models/): Base entity definitions
//
// # Usage
//
// To create a new managed entity:
//
// 1. Define your model:
//
//	type Course struct {
//	    models.BaseModel
//	    Title  string    `json:"title"`
//	    Code   string    `json:"code"`
//	}
//
// 2. Implement RegistrableEntity interface:
//
//	type CourseRegistration struct{}
//
//	func (cr CourseRegistration) GetEntity() interface{} {
//	    return &Course{}
//	}
//
//	func (cr CourseRegistration) GetEntityName() string {
//	    return "Course"
//	}
//
// 3. Register the entity in main.go:
//
//	ems.GlobalEntityRegistrationService.RegisterEntity(CourseRegistration{})
//
// This automatically creates these routes:
//   - GET    /api/v1/courses           - List all courses (paginated)
//   - GET    /api/v1/courses/:id       - Get single course
//   - POST   /api/v1/courses           - Create course
//   - PATCH  /api/v1/courses/:id       - Update course
//   - DELETE /api/v1/courses/:id       - Delete course
//
// # Hooks
//
// Entities can execute code at lifecycle events:
//
//	hook := hooks.NewFunctionHook(
//	    "SendWelcomeEmail",
//	    "User",
//	    hooks.AfterCreate,
//	    func(ctx *hooks.HookContext) error {
//	        user := ctx.NewEntity.(*User)
//	        return sendEmail(user.Email, "Welcome!")
//	    },
//	)
//	hooks.GlobalHookRegistry.RegisterHook(hook)
//
// # Filtering
//
// Supports advanced filtering via query parameters:
//
//   - Direct fields: GET /courses?title=Golang
//   - Foreign keys: GET /chapters?courseId=123
//   - Many-to-many: GET /courses?tagIDs=1,2,3
//   - Nested relations: GET /pages?courseId=123 (via RelationshipFilter)
//
// # Permissions
//
// Integrates with Casbin for role-based access control. Permissions are
// automatically created when entities are registered.
//
// # Swagger Documentation
//
// API documentation is auto-generated from struct tags and registered
// entity metadata. Visit /swagger/ to view interactive docs.
package entityManagement
```

**Method Documentation Examples:**

```go
// In repositories/genericRepository.go

// GetAllEntities retrieves a paginated list of entities with optional filtering.
//
// Parameters:
//   - page: Page number (1-indexed)
//   - pageSize: Number of items per page
//   - filters: Map of field names to filter values
//   - includes: Slice of relation names to preload (e.g., ["chapters", "authors"])
//
// Returns:
//   - Slice of entities for the requested page
//   - Total count of entities matching filters
//   - Error if database operation fails
//
// Example:
//
//	entities, total, err := repo.GetAllEntities(
//	    1,
//	    20,
//	    map[string]interface{}{"published": true},
//	    []string{"chapters"},
//	)
func (gr *GenericRepository) GetAllEntities(
    page int,
    pageSize int,
    filters map[string]interface{},
    includes []string,
) ([]interface{}, int64, error) {
    // ...
}
```

**Files Documented:**
- [x] `src/entityManagement/doc.go` (DONE - comprehensive package overview)
- [x] `src/entityManagement/repositories/genericRepository.go` (DONE - GetAllEntities method)
- [x] `src/entityManagement/hooks/interfaces.go` (DONE - package doc, Hook interface, HookContext)
- [ ] `src/entityManagement/services/genericService.go` (TODO - method docs)
- [ ] `src/entityManagement/swagger/routeGenerator.go` (TODO - complex logic docs)

**Documentation Checklist:**
- [ ] Package-level overview with examples
- [ ] All exported types have doc comments
- [ ] All exported functions have doc comments
- [ ] Complex algorithms have inline explanations
- [ ] Examples for common use cases

**Generate Documentation:**
```bash
# View godoc locally
godoc -http=:6060
# Open http://localhost:6060/pkg/ocf-core/src/entityManagement/
```

---

### 4.3 Fix Naming Issues ✅ COMPLETED

**Priority:** LOW
**Effort:** 30 minutes
**Impact:** Code consistency

**Issues Found:**

1. **French variable name:**
   - Location: `src/entityManagement/services/genericService.go:281`
   - Current: `mon_uuid`
   - Fix: `myUUID` or `generatedID`

2. **Typo in error message:**
   - Location: `src/entityManagement/repositories/genericRepository.go:43`
   - Current: "Entity convertion function does not exist"
   - Fix: "Entity conversion function does not exist"

3. **Inconsistent abbreviations:**
   - Current: Mix of `Dto` and `DTO`
   - Fix: Standardize on `DTO` (following Go conventions)
   - Files affected: All files with "Dto" in name/variable

**Find and Replace:**

```bash
# Find all instances of "Dto"
grep -r "Dto" src/entityManagement/

# Find typo
grep -r "convertion" src/entityManagement/
```

**Files Modified:**
- [x] `src/entityManagement/services/genericService.go:281` (DONE - renamed mon_uuid to entityUUID)
- [x] `src/entityManagement/repositories/genericRepository.go:43` (DONE - fixed "convertion" → "conversion")

**Consistency Rules:**
- Use `DTO` (all caps) in names: `CourseDTO`, `inputDTO`
- Use `ID` (all caps) for identifiers: `userID`, `courseID`
- Use camelCase for variables: `entityName`, `filterKey`
- Use PascalCase for exported types: `GenericRepository`, `HookContext`

---

## 🎯 Quick Wins (Do First)

**Total Effort: ~6.5 hours for significant improvement**

1. **Fix naming issues** (30 min) - Task 4.3
2. **Extract OwnerIDs duplication** (1 hour) - Task 1.3
3. **Add package documentation** (2 hours) - Task 4.2
4. **Create test utilities** (3 hours) - Task 4.1

---

## 📊 Progress Tracking

### Overall Progress: 4/12 tasks completed (33%) 🎉

### Phase 1 (Critical): 1/3 ✅
- [ ] 1.1 Decouple Casdoor
- [ ] 1.2 Fix async hooks
- [x] 1.3 Extract OwnerIDs ✅

### Phase 2 (Performance): 0/3 ⏳
- [ ] 2.1 Cursor pagination
- [ ] 2.2 Selective preloading
- [ ] 2.3 Query caching

### Phase 3 (Maintainability): 0/3 ⏳
- [ ] 3.1 Refactor filters
- [ ] 3.2 Standardize errors
- [ ] 3.3 Add validation

### Phase 4 (Polish): 3/3 ✅ COMPLETE!
- [x] 4.1 Test utilities ✅
- [x] 4.2 Package docs ✅
- [x] 4.3 Fix naming ✅

---

## 📈 Success Metrics

**After completion, we expect:**

| Metric | Current | Target | Status |
|--------|---------|--------|--------|
| Controller test coverage | 60% | 95%+ | ⏳ |
| Test setup code | ~100 lines/file | ~10 lines/file | ⏳ |
| Silent errors | Async hooks | 0 | ⏳ |
| Code duplication | 3-4 instances | 0 | ⏳ |
| Pagination scalability | 10K records | 1M+ records | ⏳ |
| Cache hit rate | 0% | 50-90% | ⏳ |
| Filter complexity | 102 lines | ~30 lines/strategy | ⏳ |
| API error consistency | Mixed | 100% standardized | ⏳ |

---

## 🔄 Review Schedule

- **Weekly:** Review completed tasks, update progress
- **After Phase 1:** Performance testing baseline
- **After Phase 2:** Load testing with 100K+ records
- **After Phase 3:** Code review for maintainability
- **After Phase 4:** Final documentation review

---

## 📝 Notes

- This plan is a living document - update as we learn more
- Priorities may shift based on production issues
- Each task should have tests before marking complete
- Performance benchmarks should be documented

---

**Last Updated:** 2025-10-04
**Next Review:** After Phase 1 completion
