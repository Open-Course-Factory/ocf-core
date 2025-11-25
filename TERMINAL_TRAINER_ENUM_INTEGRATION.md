# Terminal Trainer Enum Integration

## Overview

OCF Core now integrates with Terminal Trainer's `/1.0/enums` endpoint to provide better error messages and automatic discovery of status values. This implementation uses a **hybrid approach** that ensures OCF Core always starts successfully, even when Terminal Trainer is unavailable.

## Architecture

### Hybrid Approach (Option 3)

The implementation combines the best of both local and dynamic approaches:

1. **Local Fallback Definitions**: Service initializes immediately with hardcoded enum definitions
2. **Async API Fetch**: Background goroutine attempts to fetch latest enums from Terminal Trainer
3. **Graceful Degradation**: If Terminal Trainer is down, local definitions are used
4. **Automatic Refresh**: Enums are refreshed every 5 minutes from the API
5. **Mismatch Detection**: Logs warnings if local definitions don't match API values

## Key Features

### 1. Better Error Messages ✅ (Primary Goal)

**Before:**
```
failed to start session response status: 3
```

**After:**
```
Failed to start session: API key has reached its concurrent session quota limit (status=3, name=quota_limit)
```

### 2. Always Starts Successfully ✅ (Critical Requirement)

- OCF Core starts immediately with local enum definitions
- No blocking on Terminal Trainer availability
- Async refresh happens in background

### 3. Automatic Discovery ✅ (Nice to Have)

- Fetches latest enum definitions from `/1.0/enums`
- Detects mismatches between local and API definitions
- Logs warnings for manual review

### 4. Future-Proof ✅ (Urgent Need)

- New status codes from Terminal Trainer are automatically discovered
- Admin endpoints allow inspection and forced refresh
- Easy to add new enum types as Terminal Trainer evolves

## Components Created

### 1. DTOs (`src/terminalTrainer/dto/terminalDto.go`)

```go
type EnumValue struct {
    Value       int    `json:"value"`
    Name        string `json:"name"`
    Description string `json:"description"`
}

type EnumDefinition struct {
    Name        string      `json:"name"`
    Description string      `json:"description"`
    Values      []EnumValue `json:"values"`
}

type TerminalTrainerEnumsResponse struct {
    Enums []EnumDefinition `json:"enums"`
}

type EnumServiceStatus struct {
    Initialized   bool      `json:"initialized"`
    LastFetch     time.Time `json:"last_fetch,omitempty"`
    Source        string    `json:"source"` // "local", "api"
    EnumCount     int       `json:"enum_count"`
    HasMismatches bool      `json:"has_mismatches"`
    Mismatches    []string  `json:"mismatches,omitempty"`
}
```

### 2. Enum Service (`src/terminalTrainer/services/terminalTrainerEnumService.go`)

```go
type TerminalTrainerEnumService interface {
    GetEnumDescription(enumName string, value int) string
    GetEnumName(enumName string, value int) string
    GetStatus() *dto.EnumServiceStatus
    RefreshEnums() error
    FormatError(enumName string, value int, context string) string
}
```

**Key Methods:**
- `GetEnumDescription()`: Returns human-readable description for status value
- `GetEnumName()`: Returns semantic name (e.g., "active", "expired")
- `FormatError()`: Formats error messages with full context
- `GetStatus()`: Returns current service status (for debugging)
- `RefreshEnums()`: Forces immediate refresh from API

### 3. Local Fallback Definitions

```go
// Session status enum - matches Terminal Trainer's definitions
session_status:
  0: active - "Session is active and running"
  1: expired - "Session has expired and is no longer accessible"
  2: failed - "Session failed to start or encountered an error"
  3: quota_limit - "API key has reached its concurrent session quota limit"
  4: system_limit - "System has reached maximum concurrent sessions"
  5: invalid_terms - "Invalid terms hash provided"
  6: terminated - "Session was terminated by user or admin"

// API key status enum
api_key_status:
  0: active - "API key is active and can be used"
  1: inactive - "API key is inactive and cannot be used"
  2: expired - "API key has expired"
  3: revoked - "API key has been revoked by admin"
```

## Usage

### In Service Code

```go
// Before
if sessionResp.Status != 0 {
    return nil, fmt.Errorf("failed to start session response status: %d", sessionResp.Status)
}

// After
if sessionResp.Status != 0 {
    errorMsg := tts.enumService.FormatError("session_status", int(sessionResp.Status), "Failed to start session")
    return nil, fmt.Errorf("%s", errorMsg)
}
```

### Admin Endpoints

#### Get Enum Status
```bash
GET /api/v1/terminals/enums/status
Authorization: Bearer <token>
```

**Response:**
```json
{
  "initialized": true,
  "last_fetch": "2025-11-25T10:30:00Z",
  "source": "api",
  "enum_count": 2,
  "has_mismatches": false,
  "mismatches": []
}
```

#### Force Enum Refresh
```bash
POST /api/v1/terminals/enums/refresh
Authorization: Bearer <token>
```

**Response:**
```json
{
  "initialized": true,
  "last_fetch": "2025-11-25T10:35:00Z",
  "source": "api",
  "enum_count": 2,
  "has_mismatches": false,
  "mismatches": []
}
```

## Behavior

### Startup Sequence

1. **Immediate Initialization (< 1ms)**
   - Load local enum definitions
   - Service becomes available instantly
   - Log: `[TerminalTrainerEnumService] Initialized with 2 local enum definitions`

2. **Background Fetch (5 seconds after startup)**
   - Attempt to fetch from `/1.0/enums`
   - If successful: Update definitions, log success
   - If failed: Log warning, continue with local definitions
   - Log: `[TerminalTrainerEnumService] Successfully fetched 2 enum definitions from API`

3. **Periodic Refresh (every 5 minutes)**
   - Automatically refresh from API
   - Detect and log any mismatches
   - Update service status

### Mismatch Detection

If local definitions don't match API values:

```log
[TerminalTrainerEnumService] Warning: Enum 'session_status' value 3: local='quota_limit' api='quota_reached'
```

This helps developers identify when local definitions need updating.

## Testing

### Unit Tests

```bash
go test -v ./src/terminalTrainer/services/ -run TestEnumService
```

**Test Coverage:**
- ✅ Service works without API availability
- ✅ Local fallback definitions are correct
- ✅ Unknown enum values handled gracefully
- ✅ Thread-safe concurrent access
- ✅ Format error creates useful messages

### Integration Testing

```bash
# 1. Start OCF Core without Terminal Trainer
# Service should initialize with local definitions

# 2. Check enum status
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/terminals/enums/status

# 3. Start Terminal Trainer

# 4. Force refresh
curl -X POST -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/terminals/enums/refresh

# 5. Verify source changed from "local" to "api"
```

## Maintenance

### Adding New Enum Types

When Terminal Trainer adds a new enum type:

1. **No immediate action required** - API will provide new enum automatically
2. **Optional**: Add to local fallbacks in `initializeLocalEnums()` for offline support
3. **Use in code**: Call `GetEnumDescription()` with new enum name

Example:
```go
// Terminal Trainer adds "machine_status" enum
desc := enumService.GetEnumDescription("machine_status", 2)
// Works automatically once API returns it
```

### Updating Local Definitions

When Terminal Trainer changes existing enum values:

1. Service logs mismatch warnings
2. Update `initializeLocalEnums()` in `terminalTrainerEnumService.go`
3. Test with `go test ./src/terminalTrainer/services/`
4. API definitions take precedence once fetched

## Benefits Summary

| Benefit | Implementation | Status |
|---------|---------------|--------|
| Better error messages | `FormatError()` with descriptions | ✅ Complete |
| Always starts successfully | Local fallbacks + async fetch | ✅ Complete |
| No blocking dependencies | 5-second delayed first fetch | ✅ Complete |
| Automatic discovery | Background refresh every 5 min | ✅ Complete |
| Mismatch detection | Logging + status endpoint | ✅ Complete |
| Future-proof | Dynamic enum support | ✅ Complete |
| Admin diagnostics | Status + refresh endpoints | ✅ Complete |
| Thread-safe | RWMutex on enum maps | ✅ Complete |
| Tested | Comprehensive unit tests | ✅ Complete |

## Next Steps

### Short Term (If Needed)
1. Add more enum types as Terminal Trainer expands (machine_status, etc.)
2. Enhance admin UI to display enum status and mismatches
3. Add Prometheus metrics for enum fetch success/failure

### Long Term (Future Enhancements)
1. Auto-generate API documentation from enum descriptions
2. Create TypeScript types from enum definitions for frontend
3. Add webhook support for real-time enum updates

## Comparison: Before vs After

### Error Message Quality

**Before:**
```
Error creating terminal: failed to start session response status: 3
```

**After:**
```
Error creating terminal: Failed to start session: API key has reached its concurrent session quota limit (status=3, name=quota_limit)
```

### Maintenance Burden

**Before:**
- Manual updates needed when Terminal Trainer changes statuses
- Hard to keep in sync across environments
- No visibility into status meaning

**After:**
- Automatic discovery of new statuses
- Self-documenting via descriptions
- Admin endpoints for diagnostics
- Mismatch warnings guide updates

### Reliability

**Before:**
- ❌ Would fail if enum endpoint required
- ❌ Single point of failure

**After:**
- ✅ Always starts with local fallbacks
- ✅ Degrades gracefully when API unavailable
- ✅ Non-blocking async updates

## Conclusion

This hybrid implementation achieves all goals:
1. ✅ **Better error messages** - Primary goal achieved
2. ✅ **Always starts** - Critical requirement met
3. ✅ **Automatic discovery** - Nice to have implemented
4. ✅ **Future-proof** - Ready for Terminal Trainer changes

The service is production-ready and has been thoroughly tested for graceful degradation.
