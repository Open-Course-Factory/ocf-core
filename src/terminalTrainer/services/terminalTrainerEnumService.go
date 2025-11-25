package services

import (
	"fmt"
	"log"
	"sync"
	"time"

	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/utils"
)

// TerminalTrainerEnumService manages enum definitions from Terminal Trainer
type TerminalTrainerEnumService interface {
	// GetEnumDescription returns the description for a specific enum value
	GetEnumDescription(enumName string, value int) string

	// GetEnumName returns the name for a specific enum value
	GetEnumName(enumName string, value int) string

	// GetStatus returns the current status of the enum service
	GetStatus() *dto.EnumServiceStatus

	// RefreshEnums forces a refresh of enums from the API
	RefreshEnums() error

	// FormatError formats an error message with enum context
	FormatError(enumName string, value int, context string) string
}

type terminalTrainerEnumService struct {
	baseURL    string
	apiVersion string
	enums      map[string]map[int]dto.EnumValue // enumName -> value -> EnumValue
	lastFetch  time.Time
	source     string // "local", "api"
	mu         sync.RWMutex
	mismatches []string
}

// NewTerminalTrainerEnumService creates a new enum service with local fallback definitions
func NewTerminalTrainerEnumService(baseURL, apiVersion string) TerminalTrainerEnumService {
	service := &terminalTrainerEnumService{
		baseURL:    baseURL,
		apiVersion: apiVersion,
		enums:      make(map[string]map[int]dto.EnumValue),
		source:     "local",
		mismatches: make([]string, 0),
	}

	// Initialize with local fallback definitions
	service.initializeLocalEnums()

	// Start async refresh if Terminal Trainer is configured
	if baseURL != "" {
		go service.asyncRefresh()
	}

	return service
}

// initializeLocalEnums sets up fallback enum definitions
func (s *terminalTrainerEnumService) initializeLocalEnums() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Session status enum - based on Terminal Trainer's definitions
	s.enums["session_status"] = map[int]dto.EnumValue{
		0: {Value: 0, Name: "active", Description: "Session is active and running"},
		1: {Value: 1, Name: "expired", Description: "Session has expired and is no longer accessible"},
		2: {Value: 2, Name: "failed", Description: "Session failed to start or encountered an error"},
		3: {Value: 3, Name: "quota_limit", Description: "API key has reached its concurrent session quota limit"},
		4: {Value: 4, Name: "system_limit", Description: "System has reached maximum concurrent sessions"},
		5: {Value: 5, Name: "invalid_terms", Description: "Invalid terms hash provided"},
		6: {Value: 6, Name: "terminated", Description: "Session was terminated by user or admin"},
	}

	// API key status enum
	s.enums["api_key_status"] = map[int]dto.EnumValue{
		0: {Value: 0, Name: "active", Description: "API key is active and can be used"},
		1: {Value: 1, Name: "inactive", Description: "API key is inactive and cannot be used"},
		2: {Value: 2, Name: "expired", Description: "API key has expired"},
		3: {Value: 3, Name: "revoked", Description: "API key has been revoked by admin"},
	}

	log.Printf("[TerminalTrainerEnumService] Initialized with %d local enum definitions", len(s.enums))
}

// asyncRefresh attempts to fetch enums from the API in the background
func (s *terminalTrainerEnumService) asyncRefresh() {
	// Wait a bit before first fetch to allow system to stabilize
	time.Sleep(5 * time.Second)

	// Try initial fetch
	if err := s.fetchEnumsFromAPI(); err != nil {
		log.Printf("[TerminalTrainerEnumService] Initial enum fetch failed (using local fallbacks): %v", err)
	}

	// Set up periodic refresh every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if err := s.fetchEnumsFromAPI(); err != nil {
			log.Printf("[TerminalTrainerEnumService] Periodic enum refresh failed: %v", err)
		}
	}
}

// fetchEnumsFromAPI fetches enum definitions from the Terminal Trainer API
func (s *terminalTrainerEnumService) fetchEnumsFromAPI() error {
	if s.baseURL == "" {
		return fmt.Errorf("terminal trainer not configured")
	}

	url := fmt.Sprintf("%s/%s/enums", s.baseURL, s.apiVersion)

	var response dto.TerminalTrainerEnumsResponse
	opts := utils.DefaultHTTPClientOptions()
	utils.ApplyOptions(&opts, utils.WithTimeout(10*time.Second))

	err := utils.MakeExternalAPIJSONRequest("Terminal Trainer", "GET", url, nil, &response, opts)
	if err != nil {
		return fmt.Errorf("failed to fetch enums from API: %w", err)
	}

	// Update enums with API data
	s.mu.Lock()
	defer s.mu.Unlock()

	newMismatches := make([]string, 0)

	for _, enumDef := range response.Enums {
		// Create map for this enum if it doesn't exist
		if s.enums[enumDef.Name] == nil {
			s.enums[enumDef.Name] = make(map[int]dto.EnumValue)
		}

		// Check for mismatches with local definitions
		localEnum := s.enums[enumDef.Name]
		for _, apiValue := range enumDef.Values {
			if localValue, exists := localEnum[apiValue.Value]; exists {
				if localValue.Name != apiValue.Name || localValue.Description != apiValue.Description {
					mismatch := fmt.Sprintf("Enum '%s' value %d: local='%s' api='%s'",
						enumDef.Name, apiValue.Value, localValue.Name, apiValue.Name)
					newMismatches = append(newMismatches, mismatch)
					log.Printf("[TerminalTrainerEnumService] Warning: %s", mismatch)
				}
			}

			// Update with API value (API is source of truth)
			s.enums[enumDef.Name][apiValue.Value] = apiValue
		}
	}

	s.mismatches = newMismatches
	s.lastFetch = time.Now()
	s.source = "api"

	log.Printf("[TerminalTrainerEnumService] Successfully fetched %d enum definitions from API", len(response.Enums))
	if len(newMismatches) > 0 {
		log.Printf("[TerminalTrainerEnumService] Warning: Found %d mismatches between local and API definitions", len(newMismatches))
	}

	return nil
}

// GetEnumDescription returns the description for a specific enum value
func (s *terminalTrainerEnumService) GetEnumDescription(enumName string, value int) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if enumMap, exists := s.enums[enumName]; exists {
		if enumValue, exists := enumMap[value]; exists {
			return enumValue.Description
		}
	}

	return fmt.Sprintf("Unknown %s value: %d", enumName, value)
}

// GetEnumName returns the name for a specific enum value
func (s *terminalTrainerEnumService) GetEnumName(enumName string, value int) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if enumMap, exists := s.enums[enumName]; exists {
		if enumValue, exists := enumMap[value]; exists {
			return enumValue.Name
		}
	}

	return fmt.Sprintf("unknown_%d", value)
}

// GetStatus returns the current status of the enum service
func (s *terminalTrainerEnumService) GetStatus() *dto.EnumServiceStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &dto.EnumServiceStatus{
		Initialized:   true,
		LastFetch:     s.lastFetch,
		Source:        s.source,
		EnumCount:     len(s.enums),
		HasMismatches: len(s.mismatches) > 0,
		Mismatches:    s.mismatches,
	}
}

// RefreshEnums forces a refresh of enums from the API
func (s *terminalTrainerEnumService) RefreshEnums() error {
	return s.fetchEnumsFromAPI()
}

// FormatError formats an error message with enum context
func (s *terminalTrainerEnumService) FormatError(enumName string, value int, context string) string {
	description := s.GetEnumDescription(enumName, value)
	name := s.GetEnumName(enumName, value)

	return fmt.Sprintf("%s: %s (status=%d, name=%s)", context, description, value, name)
}
