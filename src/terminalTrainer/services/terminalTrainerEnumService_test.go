package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestEnumServiceWithoutAPI tests that the enum service works with local fallbacks
// even when Terminal Trainer API is not available
func TestEnumServiceWithoutAPI(t *testing.T) {
	// Create service without valid API URL (simulating TT being down)
	service := NewTerminalTrainerEnumService("", "1.0")

	// Give it a moment to initialize
	time.Sleep(100 * time.Millisecond)

	// Test that we can get status
	status := service.GetStatus()
	assert.NotNil(t, status)
	assert.True(t, status.Initialized)
	assert.Equal(t, "local", status.Source)
	assert.Greater(t, status.EnumCount, 0)

	// Test that we can get enum descriptions (using local fallbacks)
	desc0 := service.GetEnumDescription("session_status", 0)
	assert.Equal(t, "Session is active and running", desc0)

	desc1 := service.GetEnumDescription("session_status", 1)
	assert.Equal(t, "Session has expired and is no longer accessible", desc1)

	desc3 := service.GetEnumDescription("session_status", 3)
	assert.Equal(t, "API key has reached its concurrent session quota limit", desc3)

	// Test that we can get enum names
	name0 := service.GetEnumName("session_status", 0)
	assert.Equal(t, "active", name0)

	name1 := service.GetEnumName("session_status", 1)
	assert.Equal(t, "expired", name1)

	name6 := service.GetEnumName("session_status", 6)
	assert.Equal(t, "terminated", name6)

	// Test format error functionality
	errorMsg := service.FormatError("session_status", 3, "Failed to start session")
	assert.Contains(t, errorMsg, "Failed to start session")
	assert.Contains(t, errorMsg, "quota limit")
	assert.Contains(t, errorMsg, "status=3")
	assert.Contains(t, errorMsg, "name=quota_limit")
}

// TestEnumServiceUnknownValue tests handling of unknown enum values
func TestEnumServiceUnknownValue(t *testing.T) {
	service := NewTerminalTrainerEnumService("", "1.0")

	// Test unknown enum name
	desc := service.GetEnumDescription("unknown_enum", 0)
	assert.Contains(t, desc, "Unknown")

	// Test unknown enum value
	desc = service.GetEnumDescription("session_status", 999)
	assert.Contains(t, desc, "Unknown")
	assert.Contains(t, desc, "999")

	// Test unknown enum name
	name := service.GetEnumName("session_status", 999)
	assert.Contains(t, name, "unknown")
	assert.Contains(t, name, "999")
}

// TestEnumServiceAPIKeyStatus tests API key status enums
func TestEnumServiceAPIKeyStatus(t *testing.T) {
	service := NewTerminalTrainerEnumService("", "1.0")

	// Test API key status descriptions
	desc0 := service.GetEnumDescription("api_key_status", 0)
	assert.Equal(t, "API key is active and can be used", desc0)

	desc1 := service.GetEnumDescription("api_key_status", 1)
	assert.Equal(t, "API key is inactive and cannot be used", desc1)

	desc3 := service.GetEnumDescription("api_key_status", 3)
	assert.Equal(t, "API key has been revoked by admin", desc3)
}

// TestEnumServiceConcurrency tests that the service is thread-safe
func TestEnumServiceConcurrency(t *testing.T) {
	service := NewTerminalTrainerEnumService("", "1.0")

	// Run multiple goroutines concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = service.GetEnumDescription("session_status", j%7)
				_ = service.GetEnumName("session_status", j%7)
				_ = service.GetStatus()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to finish
	for i := 0; i < 10; i++ {
		<-done
	}
}
