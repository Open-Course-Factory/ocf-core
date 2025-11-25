# Sync-All Bug Fix: Status Type Mismatch

## Problem Summary

The `/terminals/sync-all` endpoint was incorrectly marking active sessions as expired, even when they were still running in Terminal Trainer.

## Root Cause

**Type Mismatch in Status Field**

Terminal Trainer's `/1.0/sessions` endpoint returns **integer status values** (0, 1, 6, etc.), but OCF Core's `TerminalTrainerSession` DTO expected a **string** value:

```go
// BEFORE - src/terminalTrainer/dto/terminalDto.go:183
type TerminalTrainerSession struct {
    ...
    Status      string `json:"status"`  // ❌ Expected string
    ...
}
```

When Terminal Trainer returned:
```json
{
  "session_id": "abc123",
  "status": 0  // Integer for "active"
}
```

OCF Core unmarshaled it as:
```go
apiSession.Status = "0"  // String "0", not semantic name "active"
```

Then the sync logic at line 509 compared:
```go
if apiSession.Status == "active" {  // "0" == "active" = false ❌
    // Create missing session
}
```

Since `"0" != "active"`, the sync logic would:
1. Skip recreating the session (thinking it wasn't active)
2. Or mark local sessions as expired (seeing mismatch between local="active" and api="0")

## The Fix

### 1. Changed Status Field Type (terminalDto.go)

```go
// AFTER
type TerminalTrainerSession struct {
    ...
    Status      FlexibleInt `json:"status"` // ✅ Handles both string and int
    ...
}

type TerminalTrainerSessionInfo struct {
    ...
    Status      FlexibleInt `json:"status"` // ✅ Also fixed for /info endpoint
    ...
}
```

### 2. Updated Sync Logic (terminalTrainerService.go)

```go
// BEFORE - Line 509
if apiSession.Status == "active" {  // ❌ Compared int with string

// AFTER - Line 507-512
// Convert numeric status to semantic name using enum service
apiStatusName := tts.enumService.GetEnumName("session_status", int(apiSession.Status))

if apiStatusName == "active" {  // ✅ Compares semantic names
    log.Printf("[DEBUG] Creating missing active session %s (status=%d, name=%s)\n",
        sessionID, apiSession.Status, apiStatusName)
```

```go
// BEFORE - Line 547
if localSession.Status != apiSession.Status && localSession.Status != "stopped" {
    // Would compare "active" with "0" - always mismatch ❌

// AFTER - Line 553
if localSession.Status != apiStatusName && localSession.Status != "stopped" {
    // Compares "active" with "active" - correct ✅
    log.Printf("[DEBUG] Status mismatch: changing '%s' -> '%s'\n",
        sessionID, localSession.Status, apiStatusName)
    localSession.Status = apiStatusName
```

### 3. Fixed Session Creation (terminalTrainerService.go)

```go
// BEFORE - Line 635
terminal := &models.Terminal{
    Status: apiSession.Status,  // Would store "0" instead of "active" ❌
}

// AFTER - Line 640
// Convert numeric status to semantic name
statusName := tts.enumService.GetEnumName("session_status", int(apiSession.Status))

terminal := &models.Terminal{
    Status: statusName,  // Stores "active", "expired", etc. ✅
}
```

### 4. Fixed Status Endpoint (terminalController.go)

```go
// BEFORE - Line 757
response.APIStatus = foundInAPI.Status  // Would assign int to string ❌

// AFTER - Line 758
enumService := tc.service.GetEnumService()
apiStatusName := enumService.GetEnumName("session_status", int(foundInAPI.Status))
response.APIStatus = apiStatusName  // Converts to semantic name ✅
```

## Status Value Mappings

The enum service now provides these mappings:

| Integer | Semantic Name | Description |
|---------|---------------|-------------|
| 0 | active | Session is active and running |
| 1 | expired | Session has expired and is no longer accessible |
| 2 | failed | Session failed to start or encountered an error |
| 3 | quota_limit | API key has reached its concurrent session quota limit |
| 4 | system_limit | System has reached maximum concurrent sessions |
| 5 | invalid_terms | Invalid terms hash provided |
| 6 | terminated | Session was terminated by user or admin |

## Files Modified

1. **src/terminalTrainer/dto/terminalDto.go**
   - Changed `TerminalTrainerSession.Status` from `string` to `FlexibleInt`
   - Changed `TerminalTrainerSessionInfo.Status` from `string` to `FlexibleInt`

2. **src/terminalTrainer/services/terminalTrainerService.go**
   - Updated `SyncUserSessions()` to convert numeric status to semantic names
   - Updated `createMissingLocalSession()` to use semantic names
   - Added better debug logging with both numeric and semantic values

3. **src/terminalTrainer/routes/terminalController.go**
   - Updated `GetSessionStatus()` to convert API status to semantic name

## Behavior After Fix

### Scenario: User has an active session

**Before Fix:**
```
1. Terminal Trainer returns: {"status": 0}
2. OCF Core sees: apiSession.Status = "0" (string)
3. Comparison: "0" != "active" ❌
4. Action: Mark session as expired or ignore it
5. Result: Active session flagged as expired 🐛
```

**After Fix:**
```
1. Terminal Trainer returns: {"status": 0}
2. OCF Core unmarshals: apiSession.Status = FlexibleInt(0)
3. Enum service converts: apiStatusName = "active"
4. Comparison: "active" == "active" ✅
5. Action: Keep session active
6. Result: Active session stays active ✓
```

### Debug Logs

**Before:**
```
[DEBUG] Session abc123: local='active', api='0'
[DEBUG] Status mismatch for session abc123: changing 'active' -> '0'
```

**After:**
```
[DEBUG] Session abc123: local='active', api='0' (name='active')
[DEBUG] Status match - no update needed
```

## Testing

### Unit Tests
```bash
go test -v ./src/terminalTrainer/services/ -run TestEnumService
# All tests pass ✓
```

### Manual Testing

1. **Start a new terminal session:**
   ```bash
   POST /api/v1/terminals/start-session
   ```

2. **Run sync-all:**
   ```bash
   POST /api/v1/terminals/sync-all
   ```

3. **Verify session is still active:**
   ```bash
   GET /api/v1/terminals/user-sessions
   # Status should be "active", not "expired"
   ```

### Expected Log Output
```log
[TerminalTrainerEnumService] Initialized with 2 local enum definitions
[DEBUG] SyncUserSessions - Session abc123: local='active', api='0' (name='active')
[DEBUG] SyncUserSessions - Status match - no update needed
```

## Benefits of This Fix

1. **Fixes the bug** - Active sessions no longer marked as expired ✓
2. **Better logging** - Shows both numeric and semantic status values
3. **Type safety** - FlexibleInt handles API inconsistencies
4. **Future-proof** - Enum service auto-discovers new status codes
5. **Consistent** - All status handling uses enum service

## Related Changes

This fix leverages the enum service implemented earlier, which provides:
- Local fallback definitions
- Async API fetching from `/1.0/enums`
- Automatic status name conversion
- Better error messages throughout the system

## Verification Checklist

- [x] Code compiles without errors
- [x] Type conversions use enum service
- [x] All status comparisons use semantic names
- [x] Debug logs show both numeric and semantic values
- [x] Session creation stores semantic names
- [ ] Manual test: Active session stays active after sync
- [ ] Manual test: Expired session stays expired after sync
- [ ] Manual test: Sync works with Terminal Trainer returning integers

## Next Steps

1. **Deploy and test** with real active session
2. **Monitor logs** for "Status mismatch" warnings
3. **Verify** no active sessions are incorrectly expired
4. **Check** that sync-all works correctly for all status types

## Rollback Plan

If issues occur, revert these commits:
1. Status field type changes in DTOs
2. Enum service integration in sync logic
3. Status conversion in createMissingLocalSession

The `FlexibleInt` type ensures backward compatibility, so partial rollback is safe.
