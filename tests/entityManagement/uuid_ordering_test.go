package entityManagement_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestUUIDv7_TimeOrdering verifies that UUIDv7 generates time-ordered IDs
// This is critical for cursor pagination which relies on: WHERE id > cursor ORDER BY id ASC
func TestUUIDv7_TimeOrdering(t *testing.T) {
	// Generate 10 UUIDs with small time delays
	uuids := make([]uuid.UUID, 10)
	for i := range 10 {
		var err error
		uuids[i], err = uuid.NewV7()
		assert.NoError(t, err, "UUIDv7 generation should not fail")
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// Verify each UUID is greater than the previous (time-ordered)
	for i := 1; i < len(uuids); i++ {
		previous := uuids[i-1]
		current := uuids[i]

		// Compare as strings (lexicographic order)
		assert.Greater(t, current.String(), previous.String(),
			"UUIDv7 #%d should be greater than #%d (time-ordered)", i, i-1)

		// Also verify bytewise comparison (what database uses)
		assert.Equal(t, 1, compareUUIDBytes(current, previous),
			"UUIDv7 #%d should be bytewise greater than #%d", i, i-1)
	}

	t.Logf("✅ Verified %d UUIDs are in strict time-ordered sequence", len(uuids))
}

// TestUUIDv4_NotTimeOrdered demonstrates that UUIDv4 is NOT sortable (for comparison)
func TestUUIDv4_NotTimeOrdered(t *testing.T) {
	// Generate 100 random UUIDv4
	uuids := make([]uuid.UUID, 100)
	for i := range 100 {
		uuids[i] = uuid.New() // Uses v4 (random)
		time.Sleep(1 * time.Millisecond)
	}

	// Count how many times UUID[i] > UUID[i-1]
	orderedCount := 0
	for i := 1; i < len(uuids); i++ {
		if compareUUIDBytes(uuids[i], uuids[i-1]) > 0 {
			orderedCount++
		}
	}

	// With random UUIDs, roughly 50% should be "greater" than previous
	// This demonstrates they are NOT time-ordered
	percentage := float64(orderedCount) / float64(len(uuids)-1) * 100

	t.Logf("ℹ️  UUIDv4 ordering: %d/%d (%.1f%%) in sequence - NOT time-ordered",
		orderedCount, len(uuids)-1, percentage)

	// We expect roughly 50% ± 20% due to randomness
	assert.Greater(t, percentage, 30.0, "Should be roughly 50%% (random)")
	assert.Less(t, percentage, 70.0, "Should be roughly 50%% (random)")
}

// compareUUIDBytes compares two UUIDs bytewise (same as database ORDER BY)
// Returns: -1 if a < b, 0 if a == b, 1 if a > b
func compareUUIDBytes(a, b uuid.UUID) int {
	for i := range 16 {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// TestCursorPagination_WithUUIDv7_CorrectOrdering integration test
func TestCursorPagination_WithUUIDv7_CorrectOrdering(t *testing.T) {
	// This test verifies that with UUIDv7, cursor pagination correctly
	// returns records in creation order even when using id > cursor

	t.Run("Simulated database ordering", func(t *testing.T) {
		// Create 5 records with UUIDv7 IDs
		type Record struct {
			ID        uuid.UUID
			CreatedAt time.Time
		}

		records := make([]Record, 5)
		for i := range 5 {
			id, err := uuid.NewV7()
			assert.NoError(t, err)
			records[i] = Record{
				ID:        id,
				CreatedAt: time.Now(),
			}
			time.Sleep(2 * time.Millisecond)
		}

		// Simulate cursor pagination: get records where id > records[1].ID
		cursor := records[1].ID
		var remaining []Record
		for _, r := range records {
			if compareUUIDBytes(r.ID, cursor) > 0 {
				remaining = append(remaining, r)
			}
		}

		// Should get records 2, 3, 4 (3 records after index 1)
		assert.Equal(t, 3, len(remaining), "Should get 3 remaining records")

		// Verify they're in the correct order
		for i := 0; i < len(remaining)-1; i++ {
			assert.Equal(t, 1, compareUUIDBytes(remaining[i+1].ID, remaining[i].ID),
				"Records should be in ascending ID order")
		}

		t.Logf("✅ Cursor pagination with UUIDv7 returns correct sequence")
	})
}
