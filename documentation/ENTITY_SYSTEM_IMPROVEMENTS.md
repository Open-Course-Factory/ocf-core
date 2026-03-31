# Entity Framework Evolution Plan

**Created:** 2025-10-04
**Updated:** 2026-03-31
**Status:** Framework Extraction Phase
**Completed:** 8/12 original tasks (67%)
**Remaining + New:** 4 original tasks + 7 framework phases
**Estimated Total Effort:** 12-16 weeks (remaining)

## Overview

This document tracks the evolution of ocf-core's generic entity management system (`src/entityManagement/`) from an internal CRUD framework into a reusable, full-stack Go+Vue framework for building entity-driven applications.

### Current State (v1 — Internal Framework)

The system powers **38 entities** across 8 modules with ~3,660 lines of core code and 3,000+ lines of tests. It provides:

- Auto-generated CRUD routes from a single entity registration file
- Cursor-based + offset pagination (UUIDv7 time-ordered)
- Strategy-based filtering (direct, FK, M2M, relationship paths, membership)
- Lifecycle hooks (6 events, priority-based, async error tracking)
- Casbin RBAC auto-policy generation per entity
- Swagger auto-documentation per operation
- Typed DTO pipeline (Create/Edit/Output with converters)
- Modular feature flag system
- Standardized error handling (EntityError with codes ENT001-ENT010)

The frontend counterpart (ocf-front) mirrors this with **27 stores** on `useBaseStore()`, `Entity.vue` for universal CRUD UI, `FieldBuilder` (40+ fluent methods, 13 field types) for declarative field definitions, and bilingual i18n.

### Target State (v2 — Reusable Full-Stack Framework)

A framework any Go+Vue team can adopt to build entity-driven applications, adding:

- **CLI scaffolding** — `ocf generate entity Tag --fields name:string,color:string` produces model, DTOs, registration, store, page, route
- **Declarative validation** — struct tags + custom validators + cross-field rules, integrated at service layer
- **Transaction support** — atomic multi-entity operations with rollback, hook-aware
- **Soft deletes** — `deleted_at` with automatic query filtering and restore operations
- **Audit trail** — `created_by`, `updated_by` tracking + optional change history table
- **Bulk operations** — batch create/update/delete with validation and hooks
- **Full-text search** — generic search endpoint with configurable indexed fields
- **Pluggable caching** — Redis or in-memory, per-entity TTL, automatic invalidation
- **Frontend parity** — TypeScript entity schemas, advanced field types, test factories

### Maturity Scorecard

| Capability | Backend | Frontend | Target |
|---|---|---|---|
| CRUD generation | 5/5 | 5/5 | Done |
| Filtering & pagination | 5/5 | 4/5 | Done |
| Hook/lifecycle system | 5/5 | 4/5 | Done |
| Error handling | 4/5 | 3/5 | Done |
| Permissions (RBAC) | 4/5 | 4/5 | Done |
| i18n | N/A | 5/5 | Done |
| Validation | 1/5 | 2/5 | Phase 3.3 |
| Transactions | 0/5 | N/A | Phase 5 |
| Code generation / CLI | 0/5 | 0/5 | Phase 6 |
| Soft deletes | 0/5 | N/A | Phase 7 |
| Bulk operations | 1/5 | 0/5 | Phase 8 |
| Audit trail | 0/5 | N/A | Phase 7 |
| Full-text search | 0/5 | N/A | Phase 9 |
| Caching | 0/5 | 3/5 | Phase 2.3 |
| Frontend TypeScript strictness | N/A | 2/5 | Phase 10 |

---

## Phases 1-4: Foundation (Original Plan)

Phases 1-4 addressed critical testing blockers, performance, maintainability, and polish. **8 of 12 tasks completed.** Detailed implementation notes for each completed task are preserved below.

---

## 🔴 Phase 1: Unblock Testing (Week 1)

**Goal:** Enable all tests to run, eliminate silent failures

### 1.1 Decouple Casdoor Dependency ✅ COMPLETED

**Priority:** CRITICAL
**Effort:** 4-6 hours (Actual: 5 hours)
**Impact:** Unlocked all controller and security tests

**Problem:**
- Tests in `genericController_test.go` commented out due to tight coupling to global `casdoor.Enforcer`
- Lines affected: 192-236, 442-472

**Solution Implemented:**
- Used existing `EnforcerInterface` in `src/auth/interfaces/casdoorEnforcerInterface.go`
- Injected enforcer into `GenericController` and `GenericService` via dependency injection
- Updated all production code to pass `casdoor.Enforcer`
- Updated all test code to pass `nil` or mock enforcers

**Files Modified:**
- [x] `src/entityManagement/routes/genericController.go` - Added enforcer parameter
- [x] `src/entityManagement/services/genericService.go` - Added enforcer parameter
- [x] `src/entityManagement/routes/deleteEntity.go` - Use injected enforcer
- [x] 14+ production files - Pass `casdoor.Enforcer` to constructors
- [x] All test files - Pass `nil` or mock enforcers
- [x] `tests/entityManagement/security_test.go` - Fixed to pass mock enforcer

**Test Coverage:**
- [x] All 85 entity management tests passing
- [x] Security tests properly verify permission behavior with mock enforcer
- [x] Integration tests work with real Casdoor

---

### 1.2 Fix Async Hook Error Handling ✅ COMPLETED

**Priority:** CRITICAL
**Effort:** 2-4 hours (Actual: 3 hours)
**Impact:** Prevents silent failures in production

**Problem:**
- `AfterCreate/Update/Delete` hooks run in goroutines with no error propagation
- Location: `src/entityManagement/services/genericService.go:76-82, 174-185, 230-241`
- Errors only logged, never surfaced to caller

**Solution Implemented:**

Implemented a comprehensive error tracking system with:
- Circular buffer storing last 100 async hook errors
- `HookError` struct capturing full error context (hook name, entity name, entity ID, timestamp)
- `GetRecentErrors(maxErrors int)` method to retrieve errors
- `ClearErrors()` method for maintenance
- `SetErrorCallback(callback)` for custom error handling/alerting
- Automatic error recording in `ExecuteHooks()` for After* hook types
- EntityID now properly set in all hook contexts

**Files Modified:**
- [x] `src/entityManagement/hooks/interfaces.go` - Added HookError struct and new registry methods
- [x] `src/entityManagement/hooks/registry.go` - Implemented error tracking with circular buffer
- [x] `src/entityManagement/services/genericService.go` - Added EntityID to AfterCreate hook context
- [x] `tests/entityManagement/hooks_simple_test.go` - Added 8 comprehensive error tracking tests

**Test Coverage:**
- [x] Test async hook failure is recorded
- [x] Test multiple errors tracked correctly
- [x] Test GetRecentErrors with different limits
- [x] Test circular buffer (100 error limit)
- [x] Test ClearErrors functionality
- [x] Test error callback mechanism
- [x] Test Before* hooks NOT tracked (they return errors synchronously)
- [x] All 93 entity management tests passing

**Key Features:**
- Thread-safe error recording with mutex
- No deadlocks (fixed RLock/Lock conflict)
- Optional callback for real-time alerting
- Circular buffer prevents memory leaks

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

### 2.1 Implement Cursor-Based Pagination ✅ COMPLETED

**Priority:** HIGH
**Effort:** 6-8 hours (Actual: 6 hours)
**Impact:** Enables scaling to millions of records

**Problem:**
- Offset pagination scans all skipped rows: `OFFSET 20000` scans 20,000 rows
- Location: `src/entityManagement/repositories/genericRepository.go:142-146`
- Performance degrades linearly with page number

**Solution Implemented:**

Cursor-based pagination using base64-encoded UUIDs with automatic detection:
```
# Offset pagination (backward compatible)
GET /api/v1/courses?page=1&size=20

# Cursor pagination (new, efficient)
GET /api/v1/courses?cursor=&limit=20              # First page
GET /api/v1/courses?cursor=<nextCursor>&limit=20  # Next page
```

**Key Features:**
- **Automatic detection**: Presence of `cursor` param triggers cursor pagination
- **Backward compatible**: Existing offset pagination still works
- **O(1) performance**: Constant time regardless of page depth
- **Filter support**: Works with all existing filter types
- **Base64-encoded cursors**: Clean, URL-safe cursor format

**Implementation Details:**

```go
// New response type
type CursorPaginationResponse struct {
    Data       []interface{} `json:"data"`
    NextCursor string        `json:"nextCursor,omitempty"` // Base64-encoded cursor
    HasMore    bool          `json:"hasMore"`              // More results available
    Limit      int           `json:"limit"`                // Items per page
}

// Repository method signature
func (o *genericRepository) GetAllEntitiesCursor(
    data any,
    cursor string,
    limit int,
    filters map[string]interface{}
) ([]any, string, bool, error)

// Cursor encoding/decoding
- Cursor = base64(UUID bytes)
- WHERE id > cursor for efficient filtering
- Fetch limit+1 to determine hasMore
```

**Files Modified:**
- [x] `src/entityManagement/repositories/genericRepository.go` (added GetAllEntitiesCursor method)
- [x] `src/entityManagement/services/genericService.go` (added GetEntitiesCursor method)
- [x] `src/entityManagement/routes/getEntities.go` (dual pagination support with auto-detection)
- [x] `tests/entityManagement/genericService_test.go` (updated mocks)
- [x] `tests/entityManagement/cursor_pagination_test.go` (NEW - 8 comprehensive tests)
- [x] `tests/entityManagement/pagination_benchmark_test.go` (NEW - performance benchmarks)

**Test Coverage:**
- [x] First page (no cursor)
- [x] Second page (with cursor)
- [x] Incomplete last page
- [x] Invalid cursor handling
- [x] Cursor pagination with filters
- [x] Empty result sets
- [x] HTTP integration tests (full traversal)
- [x] Benchmarks comparing offset vs cursor performance

**Test Results:**
- ✅ All 8 cursor pagination tests passing
- ✅ All existing pagination tests still passing (backward compatibility verified)
- ✅ Integration tests: Successfully traverse all pages using cursors

**Performance Benchmarks:**
```
BenchmarkOffsetPagination_FirstPage   668,904 ns/op  (baseline)
BenchmarkOffsetPagination_Page250   1,411,412 ns/op  (2.1x slower - scans 4,980 rows)
BenchmarkCursorPagination_FirstPage 1,802,667 ns/op  (setup overhead)
BenchmarkCursorPagination_Page250   4,498,517 ns/op  (includes 250-page navigation in setup)
```

**Note on Benchmarks**: The cursor Page250 benchmark includes the setup cost of navigating through 250 pages to get the cursor. In production, clients maintain cursors between requests, so the actual per-request cost is similar to FirstPage (~1.8ms).

**CRITICAL FIX - UUIDv7 for Sortable IDs:**
- ✅ **FIXED**: Switched from UUIDv4 (random) to UUIDv7 (time-ordered) in `baseModel.go:22`
- **Why**: Cursor pagination uses `WHERE id > cursor ORDER BY id ASC`, which requires sortable IDs
- **Before**: UUIDv4 is random - cursor pagination would return unpredictable results
- **After**: UUIDv7 contains timestamp in first 48 bits - naturally sortable and time-ordered
- **Tests**: Added `uuid_ordering_test.go` demonstrating UUIDv7 is 100% time-ordered vs UUIDv4 ~50%

**Migration Strategy:**
1. ✅ Cursor support added alongside existing offset pagination
2. ✅ Fixed UUID generation to use UUIDv7 (required for cursor pagination)
3. ⏳ Mark offset pagination as deprecated in Swagger (Phase 4)
4. ⏳ Update client libraries to use cursors (post-Phase 4)
5. ⏳ Remove offset after 2-3 releases (future consideration)

---

### 2.2 Add Selective Preloading ✅ COMPLETED

**Priority:** HIGH
**Effort:** 4-6 hours (Actual: 5 hours)
**Impact:** Reduces query count by 10-100x, eliminates N+1 problems

**Problem:**
- Recursive preloading loads ALL nested entities, even when not needed
- Previously: `.Preload(clause.Associations)` loaded everything recursively
- Location: `src/entityManagement/repositories/genericRepository.go`

**Solution Implemented:**

Selective preloading with `include` query parameter supporting:
```
# No preloading (fastest)
GET /api/v1/courses

# Specific relations
GET /api/v1/courses?include=Chapters,Authors

# Nested relations with dot notation
GET /api/v1/courses?include=Chapters.Sections,Authors

# Deep nesting
GET /api/v1/courses?include=Chapters.Sections.Pages

# All relations (backward compatible)
GET /api/v1/courses?include=*
```

**Key Features:**
- **Generic implementation**: Works for all entities automatically
- **Dot notation support**: Load nested relations (e.g., `Chapters.Sections.Pages`)
- **Wildcard support**: `?include=*` loads all associations (backward compatible)
- **Whitespace handling**: Trims spaces from relation names
- **Backward compatible**: `nil` or empty includes = no preloading (default)

**Implementation Details:**

```go
// applyIncludes applies selective preloading to a GORM query
func applyIncludes(query *gorm.DB, includes []string) *gorm.DB {
    if includes == nil || len(includes) == 0 {
        return query  // No preloading
    }

    // Check for wildcard
    for _, include := range includes {
        if include == "*" {
            return query.Preload(clause.Associations)
        }
    }

    // Apply selective preloading
    for _, include := range includes {
        include = strings.TrimSpace(include)
        if include != "" {
            query = query.Preload(include)  // GORM handles dot notation
        }
    }

    return query
}

```

**Files Modified:**
- [x] `src/entityManagement/repositories/genericRepository.go` (DONE - interface & implementation with `applyIncludes()`)
- [x] `src/entityManagement/services/genericService.go` (DONE - pass includes through all methods)
- [x] `src/entityManagement/routes/getEntities.go` (DONE - parse include param, supports both pagination modes)
- [x] `src/entityManagement/routes/getEntity.go` (DONE - parse include param for single entity)
- [x] `tests/entityManagement/selective_preloading_test.go` (DONE - comprehensive 11-test suite)
- [x] `docs/swagger.json` (DONE - regenerated with include parameter documentation)

**Updated Method Signatures:**
```go
// Repository layer
GetEntity(id uuid.UUID, data any, entityName string, includes []string) (any, error)
GetAllEntities(data any, page int, pageSize int, filters map[string]interface{}, includes []string) ([]any, int64, error)
GetAllEntitiesCursor(data any, cursor string, limit int, filters map[string]interface{}, includes []string) ([]any, string, bool, error)

// Service layer (mirrors repository)
GetEntity(id uuid.UUID, data interface{}, entityName string, includes []string) (interface{}, error)
GetEntities(data interface{}, page int, pageSize int, filters map[string]interface{}, includes []string) ([]interface{}, int64, error)
GetEntitiesCursor(data interface{}, cursor string, limit int, filters map[string]interface{}, includes []string) ([]interface{}, string, bool, error)
```

**Test Coverage:**
- [x] No includes - basic entity loading works (2 tests)
- [x] Wildcard "*" - loads all associations
- [x] Specific relation - loads only requested (e.g., `Chapters`)
- [x] Multiple relations - loads all specified
- [x] Nested relations - dot notation support (e.g., `Chapters.Sections`)
- [x] Deep nesting - three levels (e.g., `Chapters.Sections.Pages`)
- [x] GetEntities with includes - list endpoint
- [x] GetEntitiesCursor with includes - cursor pagination
- [x] Whitespace trimming - handles ` Chapters ` correctly
- [x] Invalid relation - GORM returns error

**Total Tests:** 11/11 passing ✅

**Swagger Documentation:**
```
@Param include query string false "Comma-separated list of relations to preload (e.g., 'Chapters,Authors' or 'Chapters.Sections' for nested, use '*' for all relations)"
```


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

### 3.1 Refactor Filter Logic with Strategy Pattern ✅ COMPLETED

**Priority:** MEDIUM
**Effort:** 8-12 hours (Actual: 6 hours)
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

**Files Modified:**
- [x] `src/entityManagement/repositories/filters/filterStrategy.go` (DONE - base interface & docs)
- [x] `src/entityManagement/repositories/filters/directFieldFilter.go` (DONE - priority 10)
- [x] `src/entityManagement/repositories/filters/foreignKeyFilter.go` (DONE - priority 20)
- [x] `src/entityManagement/repositories/filters/manyToManyFilter.go` (DONE - priority 30)
- [x] `src/entityManagement/repositories/filters/relationshipPathFilter.go` (DONE - priority 5, fixed!)
- [x] `src/entityManagement/repositories/filters/filterManager.go` (DONE - orchestration)
- [x] `src/entityManagement/repositories/genericRepository.go` (DONE - removed 200+ lines of monolithic code)
- [x] `tests/entityManagement/filters/directFieldFilter_test.go` (DONE - 13 tests)
- [x] `tests/entityManagement/filters/foreignKeyFilter_test.go` (DONE - 11 tests)
- [x] `tests/entityManagement/filters/manyToManyFilter_test.go` (DONE - 11 tests)
- [x] `tests/entityManagement/filters/filterManager_test.go` (DONE - 15 integration tests)

**Test Coverage:**
- [x] Each strategy in isolation (100% passing)
- [x] Strategy priority ordering (verified RelationshipPath=5 takes precedence)
- [x] Multiple filters combined (AND logic verified)
- [x] Filter with valid syntax processed correctly
- [x] Edge cases (empty arrays, comma-separated values, whitespace)
- [x] All existing entity management tests pass (backward compatibility verified)

**Implementation Highlights:**
- ✅ **Priority Fix**: RelationshipPathFilter priority changed from 40→5 to ensure registered filters take precedence over pattern matching
- ✅ **First-match-wins**: Each filter is processed by the first matching strategy
- ✅ Reduced from 1 monolithic function (200+ lines) to 4 focused strategies (~30 lines each)
- ✅ 50 new unit tests + full integration test suite passing

**Benefits:**
- ✅ Each filter type is testable independently
- ✅ Easy to add new filter strategies (just implement interface & add to FilterManager)
- ✅ Clear separation of concerns (each strategy has single responsibility)
- ✅ Reduced complexity (each strategy ~30 lines vs 200+ lines monolithic)
- ✅ Better debugging (errors isolated to specific strategy)

---

### 3.2 Standardize Error Handling ✅ COMPLETED

**Priority:** MEDIUM
**Effort:** 3-4 hours (Actual: 3.5 hours)
**Impact:** Better API consistency, easier client error handling

**Problem:**
- Mix of error types: `APIError`, `fmt.Errorf`, silent failures
- No consistent error codes
- Hard for clients to programmatically handle errors

**Solution Implemented:**

Created a standardized `EntityError` type with:
- Consistent error codes (ENT001-ENT010)
- HTTP status codes
- Optional details map for context
- Error wrapping support (errors.Is/As compatible)
- Chainable WithDetails() method

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

**Files Modified:**
- [x] `src/entityManagement/errors/entityErrors.go` (DONE - NEW file with EntityError type)
- [x] `src/entityManagement/repositories/genericRepository.go` (DONE - all methods use new errors)
- [x] `src/entityManagement/services/genericService.go` (DONE - hook errors wrapped)
- [x] `src/entityManagement/routes/errorHandler.go` (DONE - NEW centralized error handler)
- [x] `src/entityManagement/routes/addEntity.go` (DONE - example migration to HandleEntityError)
- [x] `tests/entityManagement/errors/entityErrors_test.go` (DONE - 15 comprehensive tests)

**Test Coverage:** ✅ All 15 tests passing
- [x] Each error type returns correct HTTP status
- [x] Error details are populated correctly
- [x] Error wrapping preserves original error (errors.Is/As support)
- [x] Predefined errors are immutable (copied, not modified)
- [x] WithDetails() chainable method works
- [x] All 10 error constructor helpers tested

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

---

## Framework Phases (New)

The following phases transform the entity system from an OCF-internal tool into a reusable full-stack framework. They build on the completed foundation (Phases 1-4) and the remaining items (2.3, 3.3).

---

## 🔐 Phase 5: Transaction Support (Week 6-7)

**Goal:** Atomic multi-entity operations with rollback semantics

### 5.1 Transaction-Aware Service Layer

**Priority:** CRITICAL
**Effort:** 8-12 hours
**Impact:** Data integrity for any non-trivial use case

**Problem:**
- No transaction support in the generic service/repository layers
- Multi-step operations (create entity + create children + run hooks) can leave inconsistent state on failure
- AfterCreate/Update/Delete hooks run outside any transaction boundary
- No way to atomically create related entities (e.g., Course + Chapters + Sections in one call)

**Solution:**

```go
// NEW FILE: src/entityManagement/services/transactionService.go

// TransactionContext wraps a GORM transaction and passes it through the service layer
type TransactionContext struct {
    tx     *gorm.DB
    committed bool
}

// WithTransaction executes a function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// If the function panics, the transaction is rolled back and the panic is re-raised.
func (gs *genericService) WithTransaction(fn func(tx *TransactionContext) error) error {
    return gs.db.Transaction(func(gormTx *gorm.DB) error {
        txCtx := &TransactionContext{tx: gormTx}
        return fn(txCtx)
    })
}

// CreateEntityTx creates an entity within a transaction
func (gs *genericService) CreateEntityTx(tx *TransactionContext, dto interface{}, userId string) (interface{}, error) {
    // Same logic as CreateEntity but using tx.tx instead of gs.db
}
```

**Hook Integration:**
```go
// Before-hooks run INSIDE the transaction (can abort it)
// After-hooks receive a flag indicating whether they're in a transaction
type HookContext struct {
    // ... existing fields ...
    InTransaction bool     // NEW: true if running inside a transaction
    TxDB          *gorm.DB // NEW: transaction-scoped DB handle (nil if not in tx)
}
```

**Files to Create/Modify:**
- [ ] `src/entityManagement/services/transactionService.go` (NEW — transaction wrapper)
- [ ] `src/entityManagement/services/genericService.go` (add Tx variants of CRUD methods)
- [ ] `src/entityManagement/repositories/genericRepository.go` (accept `*gorm.DB` param for tx)
- [ ] `src/entityManagement/hooks/interfaces.go` (add InTransaction + TxDB to HookContext)
- [ ] `tests/entityManagement/transaction_test.go` (NEW)

**Test Coverage:**
- [ ] Successful multi-entity creation in transaction
- [ ] Rollback on service error
- [ ] Rollback on hook error (Before* hook fails mid-transaction)
- [ ] Nested transactions (savepoints)
- [ ] After-hooks fire only after commit
- [ ] Concurrent transaction isolation

---

## 🛠️ Phase 6: CLI Scaffolding & Code Generation (Week 7-8)

**Goal:** Zero-boilerplate entity creation with a single CLI command

### 6.1 Entity Generator CLI

**Priority:** HIGH
**Effort:** 12-16 hours
**Impact:** Reduces new entity setup from ~1 hour of copy-paste to ~30 seconds

**Problem:**
- 38 entities were created by manually copying and adapting existing registration files
- Each new entity requires creating 4-6 files (model, DTOs, registration, hooks) on backend + 3 files (store, page, route) on frontend
- Error-prone: field names, converter functions, and route paths must match perfectly
- No automation — the "framework" is really "copy-paste from an existing entity"

**Solution: `ocf-gen` CLI tool**

```bash
# Generate a full entity (backend only)
ocf-gen entity Tag --fields "name:string:required,color:string,description:text"

# Generate with relationships
ocf-gen entity Chapter --fields "title:string:required,order:int" --belongs-to Course

# Generate a full-stack entity (backend + frontend)
ocf-gen entity Tag --fields "name:string:required,color:string" --full-stack

# List registered entities
ocf-gen list

# Validate entity registration (check for missing converters, routes, etc.)
ocf-gen validate
```

**Generated Files (backend):**
```
src/{module}/models/tag.go              # Model with BaseModel + fields
src/{module}/dtos/tagDto.go             # CreateDTO, EditDTO, OutputDTO
src/{module}/entityRegistration/tagRegistration.go  # TypedEntityRegistration
src/{module}/hooks/tagHooks.go          # Hook stubs (BeforeCreate, etc.)
```

**Generated Files (frontend, with `--full-stack`):**
```
src/stores/tags.ts                      # Pinia store extending baseStore
src/components/Pages/Tags.vue           # Page component wrapping Entity.vue
# + route entry instruction in stdout
```

**Implementation Approach:**
```go
// cmd/ocf-gen/main.go — standalone CLI binary
// Uses Go text/template for file generation
// Reads existing entity registrations to validate naming conflicts
// Templates stored in cmd/ocf-gen/templates/

type EntitySpec struct {
    Name       string            // PascalCase: "Tag"
    Module     string            // lowercase: "courses"
    Fields     []FieldSpec       // parsed from --fields flag
    BelongsTo  []string          // parent entities
    HasMany    []string          // child entities
    FullStack  bool              // generate frontend files too
}

type FieldSpec struct {
    Name       string            // camelCase: "tagName"
    Type       string            // Go type: "string", "int", "uuid.UUID"
    GoType     string            // resolved Go type
    TSType     string            // resolved TypeScript type
    Required   bool
    Validation string            // validate tag: "required,min=3"
    JSONName   string            // json tag: "tag_name"
}
```

**Files to Create:**
- [ ] `cmd/ocf-gen/main.go` (CLI entry point)
- [ ] `cmd/ocf-gen/generator/entity.go` (entity generation logic)
- [ ] `cmd/ocf-gen/generator/fields.go` (field parsing and type mapping)
- [ ] `cmd/ocf-gen/generator/validator.go` (validate existing registrations)
- [ ] `cmd/ocf-gen/templates/model.go.tmpl`
- [ ] `cmd/ocf-gen/templates/dto.go.tmpl`
- [ ] `cmd/ocf-gen/templates/registration.go.tmpl`
- [ ] `cmd/ocf-gen/templates/hooks.go.tmpl`
- [ ] `cmd/ocf-gen/templates/store.ts.tmpl` (frontend)
- [ ] `cmd/ocf-gen/templates/page.vue.tmpl` (frontend)
- [ ] `tests/cmd/ocf_gen_test.go` (NEW)

**Test Coverage:**
- [ ] Generate entity with basic fields
- [ ] Generate entity with relationships (belongs-to, has-many)
- [ ] Generate full-stack (backend + frontend)
- [ ] Validate naming conflict detection
- [ ] Validate generated code compiles (`go build`)
- [ ] Validate generated store has correct field types
- [ ] `ocf-gen validate` catches missing converters
- [ ] `ocf-gen list` shows all registered entities

### 6.2 OpenAPI → Frontend Store Generation (Optional Enhancement)

**Priority:** LOW
**Effort:** 8-12 hours
**Impact:** Keeps frontend stores in sync with backend automatically

```bash
# Generate/update frontend stores from Swagger spec
ocf-gen sync-frontend --swagger docs/swagger.json --output ../ocf-front/src/stores/
```

This reads the Swagger spec and generates or updates Pinia stores with the correct field types, validation rules, and API paths. Useful for keeping backend and frontend in sync after schema changes.

---

## 🗑️ Phase 7: Soft Deletes & Audit Trail (Week 8-9)

**Goal:** Data preservation and change tracking for compliance and debugging

### 7.1 Soft Delete Support

**Priority:** HIGH
**Effort:** 4-6 hours
**Impact:** Required for any SaaS, compliance, or audit use case

**Problem:**
- Only hard deletes supported — data is permanently lost
- No `deleted_at` column or automatic filtering
- No restore operations
- Blocks compliance requirements (GDPR right to erasure with audit, Qualiopi traceability)

**Solution:**

GORM natively supports soft deletes via `gorm.DeletedAt`. The framework needs to wire this into the generic layers.

```go
// Updated BaseModel — add soft delete support
type BaseModel struct {
    ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
    CreatedAt time.Time      `json:"createdAt"`
    UpdatedAt time.Time      `json:"updatedAt"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"deletedAt,omitempty"` // NEW
}
```

```go
// Entity registration opt-in for soft deletes
type EntityConfig struct {
    // ... existing fields ...
    SoftDelete     bool  // If true, DELETE marks as deleted instead of removing
    IncludeDeleted bool  // If true, GET endpoints include soft-deleted records (admin use)
}
```

```go
// Restore endpoint (auto-generated when SoftDelete=true)
// PATCH /api/v1/courses/:id/restore
func (gc *genericController) RestoreEntity(ctx *gin.Context) {
    id := ctx.Param("id")
    // Use Unscoped() to find soft-deleted record
    result := gc.db.Unscoped().Model(gc.entity).Where("id = ?", id).Update("deleted_at", nil)
    // ...
}
```

**Files to Modify:**
- [ ] `src/entityManagement/models/baseModel.go` (add DeletedAt field)
- [ ] `src/entityManagement/interfaces/typedEntity.go` (add SoftDelete config)
- [ ] `src/entityManagement/routes/deleteEntity.go` (soft delete logic)
- [ ] `src/entityManagement/routes/restoreEntity.go` (NEW — restore endpoint)
- [ ] `src/entityManagement/routes/getEntities.go` (filter deleted by default, `?includeDeleted=true` for admin)
- [ ] `src/entityManagement/swagger/routeGenerator.go` (generate restore endpoint docs)
- [ ] `tests/entityManagement/soft_delete_test.go` (NEW)

**Test Coverage:**
- [ ] Soft delete marks record (deleted_at set, not removed from DB)
- [ ] GET excludes soft-deleted records by default
- [ ] GET with `?includeDeleted=true` returns soft-deleted records
- [ ] Restore endpoint clears deleted_at
- [ ] Restore non-existent record returns 404
- [ ] Hard delete still available via config or `?permanent=true`
- [ ] Hooks fire on soft delete (BeforeDelete, AfterDelete)
- [ ] Unique constraints handle soft-deleted records (e.g., allow re-creating same name)
- [ ] Cursor/offset pagination respects soft delete filter

### 7.2 Audit Trail

**Priority:** MEDIUM
**Effort:** 6-8 hours
**Impact:** Change tracking for compliance, debugging, and accountability

**Problem:**
- No `created_by` / `updated_by` tracking on entities
- No change history — impossible to answer "who changed what, when?"
- Manual implementation required per entity

**Solution:**

```go
// Updated BaseModel — add audit fields
type AuditableModel struct {
    BaseModel
    CreatedBy string `gorm:"type:varchar(255)" json:"createdBy,omitempty"`
    UpdatedBy string `gorm:"type:varchar(255)" json:"updatedBy,omitempty"`
}

// Audit log table (optional, per-entity)
type AuditLog struct {
    ID         uuid.UUID              `gorm:"type:uuid;primaryKey"`
    EntityName string                 `gorm:"type:varchar(100);index"`
    EntityID   uuid.UUID              `gorm:"type:uuid;index"`
    Action     string                 `gorm:"type:varchar(20)"` // create, update, delete, restore
    UserID     string                 `gorm:"type:varchar(255)"`
    Changes    datatypes.JSON         `gorm:"type:jsonb"`       // {"field": {"old": x, "new": y}}
    CreatedAt  time.Time
}
```

```go
// Entity registration opt-in for audit
type EntityConfig struct {
    // ... existing fields ...
    Auditable   bool  // If true, populate CreatedBy/UpdatedBy automatically
    AuditLog    bool  // If true, write to audit_logs table on every change
}
```

**Implementation:**
- `CreatedBy`/`UpdatedBy` populated automatically via hooks using `HookContext.UserID`
- Audit log entries created in AfterCreate/AfterUpdate/AfterDelete hooks
- Change diff computed by comparing old vs new entity state
- Query endpoint: `GET /api/v1/audit-logs?entityName=Course&entityId=<id>`

**Files to Create/Modify:**
- [ ] `src/entityManagement/models/auditableModel.go` (NEW)
- [ ] `src/entityManagement/models/auditLog.go` (NEW)
- [ ] `src/entityManagement/hooks/auditHooks.go` (NEW — auto-register audit hooks)
- [ ] `src/entityManagement/routes/auditRoutes.go` (NEW — query endpoint)
- [ ] `src/entityManagement/services/genericService.go` (populate CreatedBy/UpdatedBy)
- [ ] `tests/entityManagement/audit_test.go` (NEW)

**Test Coverage:**
- [ ] CreatedBy set on create
- [ ] UpdatedBy set on update
- [ ] Audit log entry created on each operation
- [ ] Change diff captures old and new values
- [ ] Query audit logs by entity name + ID
- [ ] Audit log not created for read operations
- [ ] Audit works with soft deletes

---

## 📦 Phase 8: Bulk Operations (Week 9-10)

**Goal:** Efficient batch processing for admin workflows and data imports

### 8.1 Batch CRUD Operations

**Priority:** HIGH
**Effort:** 8-12 hours
**Impact:** Enables admin workflows, data migration, import/export

**Problem:**
- No bulk create/update/delete — each operation is a separate HTTP request
- Importing 100 entities requires 100 API calls
- No transaction semantics for batch operations
- Frontend Entity.vue has import (JSON) but backend processes one-by-one

**Solution:**

```go
// Batch create
// POST /api/v1/courses/batch
// Body: [{"title": "Course 1", ...}, {"title": "Course 2", ...}]
type BatchCreateRequest struct {
    Items []interface{} `json:"items" validate:"required,min=1,max=100"`
}

type BatchResponse struct {
    Succeeded []BatchItemResult `json:"succeeded"`
    Failed    []BatchItemResult `json:"failed"`
    Total     int               `json:"total"`
}

type BatchItemResult struct {
    Index  int         `json:"index"`
    ID     string      `json:"id,omitempty"`
    Error  string      `json:"error,omitempty"`
}
```

```go
// Batch update
// PATCH /api/v1/courses/batch
// Body: [{"id": "...", "title": "New Title"}, ...]

// Batch delete
// DELETE /api/v1/courses/batch
// Body: {"ids": ["id1", "id2", "id3"]}
```

**Transaction Modes:**
```go
type BatchConfig struct {
    Atomic bool // If true, all-or-nothing (single transaction). If false, partial success allowed.
    MaxSize int // Maximum batch size (default 100)
}
```

**Hook Integration:**
- Before/After hooks fire for each item in the batch
- In atomic mode, a single BeforeCreate failure rolls back the entire batch
- In non-atomic mode, failed items are collected and reported

**Files to Create/Modify:**
- [ ] `src/entityManagement/routes/batchEntity.go` (NEW — batch endpoints)
- [ ] `src/entityManagement/services/batchService.go` (NEW — batch logic with transaction modes)
- [ ] `src/entityManagement/interfaces/typedEntity.go` (add BatchConfig to registration)
- [ ] `src/entityManagement/swagger/routeGenerator.go` (generate batch endpoint docs)
- [ ] `tests/entityManagement/batch_test.go` (NEW)

**Test Coverage:**
- [ ] Batch create (all succeed)
- [ ] Batch create with validation failure (partial, non-atomic)
- [ ] Batch create atomic rollback on failure
- [ ] Batch update
- [ ] Batch delete
- [ ] Batch size limit enforced
- [ ] Hooks fire for each item
- [ ] Batch response format correct

---

## 🔍 Phase 9: Full-Text Search (Week 10-11)

**Goal:** Generic search endpoint with configurable indexed fields

### 9.1 Entity Search Endpoint

**Priority:** MEDIUM
**Effort:** 6-8 hours
**Impact:** Expected feature in any CRUD framework

**Problem:**
- No search functionality — only exact field filtering
- No way to search across multiple fields (e.g., "golang" matching title, description, code)
- No relevance ranking

**Solution:**

```go
// Entity registration — declare searchable fields
type EntityConfig struct {
    // ... existing fields ...
    SearchFields []string // Fields to include in full-text search (e.g., ["title", "description", "code"])
}
```

```go
// Search endpoint (auto-generated when SearchFields is non-empty)
// GET /api/v1/courses/search?q=golang&limit=20
func (gc *genericController) SearchEntities(ctx *gin.Context) {
    query := ctx.Query("q")
    limit := ctx.DefaultQuery("limit", "20")
    // ...
}
```

**PostgreSQL Implementation:**
```go
// Uses PostgreSQL tsvector for efficient full-text search
func (gr *genericRepository) SearchEntities(query string, fields []string, limit int) ([]any, error) {
    searchConditions := make([]string, len(fields))
    for i, field := range fields {
        col := toSnakeCase(field)
        searchConditions[i] = fmt.Sprintf("COALESCE(%s, '') ILIKE ?", col)
    }

    condition := strings.Join(searchConditions, " OR ")
    pattern := "%" + query + "%"

    args := make([]interface{}, len(fields))
    for i := range fields {
        args[i] = pattern
    }

    var results []any
    err := gr.db.Model(gr.entity).
        Where(condition, args...).
        Limit(limit).
        Find(&results).Error

    return results, err
}
```

**Advanced (Phase 9.2 — optional):**
- PostgreSQL `to_tsvector` / `to_tsquery` for proper full-text indexing
- Relevance ranking with `ts_rank`
- Search highlighting
- Faceted search (count by category)

**Files to Create/Modify:**
- [ ] `src/entityManagement/interfaces/typedEntity.go` (add SearchFields to config)
- [ ] `src/entityManagement/repositories/searchRepository.go` (NEW)
- [ ] `src/entityManagement/routes/searchEntity.go` (NEW)
- [ ] `src/entityManagement/swagger/routeGenerator.go` (generate search endpoint docs)
- [ ] `tests/entityManagement/search_test.go` (NEW)

**Test Coverage:**
- [ ] Search matches single field
- [ ] Search matches across multiple fields
- [ ] Search is case-insensitive
- [ ] Search with no results returns empty array
- [ ] Search respects soft delete filter
- [ ] Search respects Casbin permissions
- [ ] Search with pagination

---

## 🖥️ Phase 10: Frontend Framework Parity (Week 11-13)

**Goal:** Bring ocf-front's generic system to framework-grade quality

This phase lives primarily in the `ocf-front` repository but is tracked here for full-stack coherence.

### 10.1 TypeScript Entity Schemas

**Priority:** HIGH
**Effort:** 8-12 hours
**Impact:** Type safety, IDE autocompletion, catch errors at compile time

**Problem:**
- Entities are loosely typed — stores use `any` for entity data
- No compile-time guarantee that field names match backend schema
- FieldBuilder config is the only "schema" but it's runtime-only

**Solution:**
```typescript
// Auto-generated (or hand-written) entity interfaces
interface Course {
    id: string
    title: string
    code: string
    description?: string
    chapters?: Chapter[]
    createdAt: string
    updatedAt: string
}

// BaseStore becomes generic
function useBaseStore<T extends BaseEntity>() {
    const entities = ref<T[]>([])
    const currentEntity = ref<T | null>(null)
    // ... all methods typed with T
}

// Store usage
const useCourseStore = defineStore('courses', () => {
    const base = useBaseStore<Course>()
    // ...
})
```

### 10.2 Advanced Field Types

**Priority:** HIGH
**Effort:** 8-12 hours
**Impact:** Unblocks content-heavy entities

New field types for EntityModal:
- **`file`** — file upload with preview (images, PDFs)
- **`richtext`** — WYSIWYG editor (TipTap or similar)
- **`color`** — color picker
- **`json`** — JSON editor with validation
- **`conditional`** — show/hide based on other field values
- **`repeater`** — dynamic array of field groups

### 10.3 Frontend Test Utilities

**Priority:** MEDIUM
**Effort:** 4-6 hours
**Impact:** Enables testing of stores and components

```typescript
// Test factory for creating mock entities
function createMockEntity<T>(overrides?: Partial<T>): T { ... }

// Test wrapper for stores
function withMockStore<S>(store: S, options?: MockOptions): S { ... }

// Component test helper
function mountEntity(entityName: string, options?: MountOptions): VueWrapper { ... }
```

### 10.4 Bulk Actions in Entity.vue

**Priority:** MEDIUM
**Effort:** 4-6 hours
**Impact:** Completes the full-stack bulk operations story (Phase 8 backend)

- Checkbox selection for multiple entities
- Bulk delete, bulk export
- Integration with backend batch endpoints (Phase 8)

---

## 📊 Progress Tracking

### Overall Progress: 8/23 tasks completed (35%)

### Phase 1 — Unblock Testing: 3/3 ✅ COMPLETE
- [x] 1.1 Decouple Casdoor dependency
- [x] 1.2 Fix async hook error handling
- [x] 1.3 Extract OwnerIDs duplication

### Phase 2 — Performance Foundation: 2/3 ⏳
- [x] 2.1 Cursor-based pagination (+ UUIDv7 fix)
- [x] 2.2 Selective preloading
- [ ] 2.3 Query result caching

### Phase 3 — Maintainability: 2/3 ⏳
- [x] 3.1 Filter strategy pattern
- [x] 3.2 Standardized error handling
- [ ] 3.3 DTO validation layer

### Phase 4 — Polish: 3/3 ✅ COMPLETE
- [x] 4.1 Test utilities
- [x] 4.2 Package documentation
- [x] 4.3 Naming fixes

### Phase 5 — Transactions: 0/1 ⏳ NEW
- [ ] 5.1 Transaction-aware service layer

### Phase 6 — CLI Scaffolding: 0/2 ⏳ NEW
- [ ] 6.1 Entity generator CLI (`ocf-gen`)
- [ ] 6.2 OpenAPI → frontend store sync (optional)

### Phase 7 — Soft Deletes & Audit: 0/2 ⏳ NEW
- [ ] 7.1 Soft delete support
- [ ] 7.2 Audit trail

### Phase 8 — Bulk Operations: 0/1 ⏳ NEW
- [ ] 8.1 Batch CRUD operations

### Phase 9 — Full-Text Search: 0/1 ⏳ NEW
- [ ] 9.1 Entity search endpoint

### Phase 10 — Frontend Parity: 0/4 ⏳ NEW
- [ ] 10.1 TypeScript entity schemas
- [ ] 10.2 Advanced field types
- [ ] 10.3 Frontend test utilities
- [ ] 10.4 Bulk actions in Entity.vue

---

## 📈 Success Metrics

### Foundation Metrics (Phases 1-4)

| Metric | Before | After | Status |
|--------|--------|-------|--------|
| Controller test coverage | 60% | 95%+ | ✅ |
| Test setup code | ~100 lines/file | ~10 lines/file | ✅ |
| Silent async errors | Untracked | Circular buffer + callback | ✅ |
| Code duplication | 3-4 instances | 0 | ✅ |
| Pagination scalability | 10K records | 1M+ (cursor + UUIDv7) | ✅ |
| Filter complexity | 200+ lines monolithic | ~30 lines/strategy | ✅ |
| API error consistency | Mixed formats | ENT001-ENT010 codes | ✅ |

### Framework Metrics (Phases 5-10)

| Metric | Current | Target | Phase |
|--------|---------|--------|-------|
| New entity setup time | ~1 hour (copy-paste) | ~30 seconds (CLI) | 6 |
| Boilerplate per entity | 4-6 files, ~300 lines | 1 command | 6 |
| Validation coverage | 0% of DTOs | 100% of DTOs | 3.3 |
| Data loss on delete | Permanent | Recoverable (soft delete) | 7 |
| Change traceability | None | Full audit log | 7 |
| Bulk import speed | N requests for N items | 1 request per 100 items | 8 |
| Search capability | Exact field match only | Multi-field full-text | 9 |
| Frontend type safety | Loose (any) | Strict (generics) | 10 |
| Frontend field types | 13 | 19+ (file, richtext, etc.) | 10 |

---

## 🗓️ Recommended Execution Order

**Critical path** (must be done in order):
1. **3.3 Validation** — unblocks everything that needs input safety
2. **5.1 Transactions** — unblocks atomic bulk operations
3. **7.1 Soft deletes** — changes BaseModel, so do before bulk adoption
4. **8.1 Bulk operations** — depends on transactions + validation
5. **6.1 CLI scaffolding** — generates code using all the above features

**Independent tracks** (can be done in parallel with critical path):
- **2.3 Caching** — independent infrastructure concern
- **7.2 Audit trail** — independent, uses existing hook system
- **9.1 Search** — independent, additive feature
- **10.x Frontend** — can start 10.1 (TypeScript) and 10.3 (test utils) any time

---

## 📝 Notes

- This plan is a living document — update as we learn more
- Priorities may shift based on production needs or framework adoption interest
- Each task must follow TDD: failing test → implementation → verification
- Performance benchmarks should be documented for each phase
- Framework extraction should remain backward-compatible with existing 38 entities
- The CLI tool (Phase 6) should be designed to eventually support plugins for custom field types and templates

---

**Last Updated:** 2026-03-31
**Next Review:** After Phase 3.3 (Validation) + Phase 5 (Transactions) completion
