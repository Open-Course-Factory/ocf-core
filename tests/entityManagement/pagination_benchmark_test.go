package entityManagement_tests

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/entityManagement/repositories"
)

// Use existing BenchmarkEntity from benchmarks_test.go

func setupPaginationBenchmarkDB(b *testing.B, entityCount int) (*gorm.DB, repositories.GenericRepository) {
	b.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatalf("Failed to setup benchmark DB: %v", err)
	}

	err = db.AutoMigrate(&BenchmarkEntity{})
	if err != nil {
		b.Fatalf("Failed to migrate: %v", err)
	}

	// Create entities
	for i := 0; i < entityCount; i++ {
		entity := &BenchmarkEntity{
			Name:  "Entity " + string(rune('A'+(i%26))),
			Value: i,
		}
		entity.ID = uuid.New()
		db.Create(entity)
	}

	repo := repositories.NewGenericRepository(db)
	return db, repo
}

// Benchmark offset pagination - First page (fast)
func BenchmarkOffsetPagination_FirstPage(b *testing.B) {
	_, repo := setupPaginationBenchmarkDB(b, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.GetAllEntities(BenchmarkEntity{}, 1, 20, map[string]interface{}{})
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

// Benchmark offset pagination - Middle page (slow)
func BenchmarkOffsetPagination_Page250(b *testing.B) {
	_, repo := setupPaginationBenchmarkDB(b, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Page 250 means OFFSET 4980 (scanning 4,980 rows!)
		_, _, err := repo.GetAllEntities(BenchmarkEntity{}, 250, 20, map[string]interface{}{})
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

// Benchmark offset pagination - Deep page (very slow)
func BenchmarkOffsetPagination_Page500(b *testing.B) {
	_, repo := setupPaginationBenchmarkDB(b, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Page 500 means OFFSET 9980 (scanning 9,980 rows!)
		_, _, err := repo.GetAllEntities(BenchmarkEntity{}, 500, 20, map[string]interface{}{})
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

// Benchmark cursor pagination - First page
func BenchmarkCursorPagination_FirstPage(b *testing.B) {
	_, repo := setupPaginationBenchmarkDB(b, 10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, err := repo.GetAllEntitiesCursor(BenchmarkEntity{}, "", 20, map[string]interface{}{})
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

// Benchmark cursor pagination - 250th "page" (still fast!)
func BenchmarkCursorPagination_Page250(b *testing.B) {
	_, repo := setupPaginationBenchmarkDB(b, 10000)

	// Navigate to page 250 to get the cursor
	cursor := ""
	for page := 0; page < 250; page++ {
		_, nextCursor, _, err := repo.GetAllEntitiesCursor(BenchmarkEntity{}, cursor, 20, map[string]interface{}{})
		if err != nil {
			b.Fatalf("Setup error: %v", err)
		}
		cursor = nextCursor
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Fetching with cursor only scans 20 rows, not 4,980!
		_, _, _, err := repo.GetAllEntitiesCursor(BenchmarkEntity{}, cursor, 20, map[string]interface{}{})
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

// Benchmark cursor pagination - 500th "page" (still fast!)
func BenchmarkCursorPagination_Page500(b *testing.B) {
	_, repo := setupPaginationBenchmarkDB(b, 10000)

	// Navigate to page 500 to get the cursor
	cursor := ""
	for page := 0; page < 500; page++ {
		_, nextCursor, _, err := repo.GetAllEntitiesCursor(BenchmarkEntity{}, cursor, 20, map[string]interface{}{})
		if err != nil {
			b.Fatalf("Setup error: %v", err)
		}
		cursor = nextCursor
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Fetching with cursor only scans 20 rows, not 9,980!
		_, _, _, err := repo.GetAllEntitiesCursor(BenchmarkEntity{}, cursor, 20, map[string]interface{}{})
		if err != nil {
			b.Fatalf("Error: %v", err)
		}
	}
}

// Benchmark: Sequential traversal comparison
func BenchmarkSequentialTraversal_Offset(b *testing.B) {
	_, repo := setupPaginationBenchmarkDB(b, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Traverse all 50 pages
		for page := 1; page <= 50; page++ {
			_, _, err := repo.GetAllEntities(BenchmarkEntity{}, page, 20, map[string]interface{}{})
			if err != nil {
				b.Fatalf("Error at page %d: %v", page, err)
			}
		}
	}
}

func BenchmarkSequentialTraversal_Cursor(b *testing.B) {
	_, repo := setupPaginationBenchmarkDB(b, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Traverse all pages using cursor
		cursor := ""
		for {
			_, nextCursor, hasMore, err := repo.GetAllEntitiesCursor(BenchmarkEntity{}, cursor, 20, map[string]interface{}{})
			if err != nil {
				b.Fatalf("Error: %v", err)
			}
			if !hasMore {
				break
			}
			cursor = nextCursor
		}
	}
}
