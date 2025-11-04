package entityManagement_tests

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/entityManagement/repositories"
	controller "soli/formations/src/entityManagement/routes"
)

// Test Entity for cursor pagination tests
type CursorTestEntity struct {
	ID    uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Name  string    `json:"name"`
	Value int       `json:"value"`
}

func setupCursorTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&CursorTestEntity{})
	require.NoError(t, err)

	return db
}

// Test: GetAllEntitiesCursor - First page with no cursor
func TestCursorPagination_FirstPage(t *testing.T) {
	db := setupCursorTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Create 10 test entities
	for i := 0; i < 10; i++ {
		entity := &CursorTestEntity{
			ID:    uuid.New(),
			Name:  "Entity " + string(rune('A'+i)),
			Value: i,
		}
		db.Create(entity)
	}

	// Request first page with limit 5
	results, nextCursor, hasMore, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, "", 5, map[string]any{}, nil)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, results)
	assert.Len(t, results, 1) // Results wrapped in array

	// Extract actual entities
	entitiesSlice := results[0].([]CursorTestEntity)
	assert.Len(t, entitiesSlice, 5)
	assert.True(t, hasMore)
	assert.NotEmpty(t, nextCursor)

	// Verify cursor is valid base64
	_, err = base64.StdEncoding.DecodeString(nextCursor)
	assert.NoError(t, err)
}

// Test: GetAllEntitiesCursor - Second page using cursor
func TestCursorPagination_SecondPage(t *testing.T) {
	db := setupCursorTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Create 10 test entities
	createdIDs := make([]uuid.UUID, 10)
	for i := 0; i < 10; i++ {
		entity := &CursorTestEntity{
			ID:    uuid.New(),
			Name:  "Entity " + string(rune('A'+i)),
			Value: i,
		}
		db.Create(entity)
		createdIDs[i] = entity.ID
	}

	// Get first page
	_, firstCursor, firstHasMore, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, "", 5, map[string]any{}, nil)
	require.NoError(t, err)
	require.True(t, firstHasMore)

	// Get second page using cursor
	results, secondCursor, secondHasMore, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, firstCursor, 5, map[string]any{}, nil)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, results)

	entitiesSlice := results[0].([]CursorTestEntity)
	assert.Len(t, entitiesSlice, 5)
	assert.False(t, secondHasMore) // No more pages (exactly 10 entities)
	assert.Empty(t, secondCursor)  // No next cursor
}

// Test: GetAllEntitiesCursor - Last page (incomplete)
func TestCursorPagination_LastPageIncomplete(t *testing.T) {
	db := setupCursorTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Create 7 test entities
	for i := 0; i < 7; i++ {
		entity := &CursorTestEntity{
			ID:    uuid.New(),
			Name:  "Entity " + string(rune('A'+i)),
			Value: i,
		}
		db.Create(entity)
	}

	// Get first page (5 items)
	_, firstCursor, firstHasMore, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, "", 5, map[string]any{}, nil)
	require.NoError(t, err)
	require.True(t, firstHasMore)

	// Get second page (should have only 2 items)
	results, nextCursor, hasMore, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, firstCursor, 5, map[string]any{}, nil)

	// Assertions
	assert.NoError(t, err)
	entitiesSlice := results[0].([]CursorTestEntity)
	assert.Len(t, entitiesSlice, 2) // Only 2 remaining
	assert.False(t, hasMore)
	assert.Empty(t, nextCursor)
}

// Test: GetAllEntitiesCursor - Invalid cursor
func TestCursorPagination_InvalidCursor(t *testing.T) {
	db := setupCursorTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Try with invalid cursor
	_, _, _, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, "invalid-cursor", 5, map[string]any{}, nil)

	// Should return error with ENT010 code
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid cursor")
}

// Test: GetAllEntitiesCursor - With filters
func TestCursorPagination_WithFilters(t *testing.T) {
	db := setupCursorTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// Create entities with different values
	for i := 0; i < 10; i++ {
		entity := &CursorTestEntity{
			ID:    uuid.New(),
			Name:  "Entity " + string(rune('A'+i)),
			Value: i % 2, // Alternating 0 and 1
		}
		db.Create(entity)
	}

	// Filter by value = 0 (should return 5 entities)
	filters := map[string]any{"value": 0}
	results, nextCursor, hasMore, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, "", 3, filters, nil)

	// Assertions
	assert.NoError(t, err)
	entitiesSlice := results[0].([]CursorTestEntity)
	assert.Len(t, entitiesSlice, 3)
	assert.True(t, hasMore) // 5 total, 3 fetched, 2 remaining

	// Verify all have value = 0
	for _, entity := range entitiesSlice {
		assert.Equal(t, 0, entity.Value)
	}

	// Get next page
	results2, _, hasMore2, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, nextCursor, 3, filters, nil)
	assert.NoError(t, err)
	entitiesSlice2 := results2[0].([]CursorTestEntity)
	assert.Len(t, entitiesSlice2, 2) // Remaining 2
	assert.False(t, hasMore2)
}

// Test: GetAllEntitiesCursor - Empty result set
func TestCursorPagination_EmptyResults(t *testing.T) {
	db := setupCursorTestDB(t)
	repo := repositories.NewGenericRepository(db)

	// No entities created
	results, nextCursor, hasMore, _, err := repo.GetAllEntitiesCursor(CursorTestEntity{}, "", 5, map[string]any{}, nil)

	// Assertions
	assert.NoError(t, err)
	entitiesSlice := results[0].([]CursorTestEntity)
	assert.Len(t, entitiesSlice, 0)
	assert.False(t, hasMore)
	assert.Empty(t, nextCursor)
}

// Integration test: Full HTTP layer cursor pagination
func TestIntegration_CursorPagination_HTTP(t *testing.T) {
	suite := setupIntegrationTest(t)

	// Create 15 test entities
	createdIDs := make([]string, 15)
	for i := 0; i < 15; i++ {
		input := IntegrationTestEntityInput{
			Name:     "Cursor Entity " + string(rune('A'+i)),
			Value:    i,
			IsActive: true,
			Tags:     []string{"cursor-test"},
		}
		body, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/integration-test-entities", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		suite.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)

		var output IntegrationTestEntityOutput
		err := json.Unmarshal(w.Body.Bytes(), &output)
		require.NoError(t, err, "Failed to unmarshal created entity")
		createdIDs[i] = output.ID
	}

	t.Logf("✅ Created %d test entities for cursor pagination", len(createdIDs))

	// Test: First page with cursor parameter
	t.Run("FirstPage", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/integration-test-entities?cursor=&limit=5", nil)
		w := httptest.NewRecorder()

		suite.router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response controller.CursorPaginationResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Len(t, response.Data, 5)
		assert.True(t, response.HasMore)
		assert.NotEmpty(t, response.NextCursor)
		assert.Equal(t, 5, response.Limit)

		t.Logf("✅ Cursor pagination first page works: 5 items, hasMore=%v", response.HasMore)
	})

	// Test: Traversing all pages
	t.Run("TraverseAllPages", func(t *testing.T) {
		cursor := ""
		totalFetched := 0
		pageCount := 0

		for {
			requestURL := "/api/v1/integration-test-entities?cursor=" + url.QueryEscape(cursor) + "&limit=4"
			req := httptest.NewRequest(http.MethodGet, requestURL, nil)
			w := httptest.NewRecorder()

			suite.router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)

			var response controller.CursorPaginationResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			totalFetched += len(response.Data)
			pageCount++

			t.Logf("Page %d: fetched %d items, hasMore=%v", pageCount, len(response.Data), response.HasMore)

			if !response.HasMore {
				break
			}

			cursor = response.NextCursor
			assert.NotEmpty(t, cursor)
		}

		assert.Equal(t, 15, totalFetched)
		assert.Equal(t, 4, pageCount) // 4 pages (4 + 4 + 4 + 3)

		t.Logf("✅ Successfully traversed all pages: %d total entities in %d pages", totalFetched, pageCount)
	})
}
