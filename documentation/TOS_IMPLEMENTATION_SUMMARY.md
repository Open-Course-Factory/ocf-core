# Terms of Service - Backend Implementation Summary

## Overview

The backend now supports GDPR-compliant Terms of Service acceptance during user registration. This implementation stores ToS acceptance data in Casdoor's user properties and validates all ToS fields during registration.

## Changes Made

### 1. Data Transfer Objects (DTOs) - `/src/auth/dto/userDto.go`

**Updated `CreateUserInput`:**
- Added `TosAcceptedAt` field (required): ISO 8601 timestamp of when user accepted ToS
- Added `TosVersion` field (required): Version identifier of the accepted ToS (format: YYYY-MM-DD)

**Updated `UserOutput`:**
- Added `TosAcceptedAt` field (optional): Returned in API responses
- Added `TosVersion` field (optional): Returned in API responses

**Updated `UserModelToUserOutput` function:**
- Extracts ToS data from Casdoor user Properties map
- Includes ToS fields in the output DTO

### 2. User Service - `/src/auth/services/userService.go`

**New `validateTosAcceptance` function:**
- Validates that ToS fields are present and non-empty
- Validates timestamp is in ISO 8601 format (RFC3339)
- Validates timestamp is not in the future
- Validates timestamp is within last 24 hours (prevents replay attacks)
- Validates version is in YYYY-MM-DD format
- Returns descriptive error messages for each validation failure

**Updated `createUserIntoCasdoor` function:**
- Stores `tos_accepted_at` in Casdoor user Properties
- Stores `tos_version` in Casdoor user Properties

**Updated `AddUser` method:**
- Calls `validateTosAcceptance` before creating user
- Returns validation errors to the client

### 3. API Documentation

**Swagger documentation updated:**
- `CreateUserInput` now shows `tosAcceptedAt` and `tosVersion` as required fields
- `UserOutput` includes ToS fields in responses
- API documentation available at `http://localhost:8080/swagger/`

## Validation Rules

### Required Fields
- `tosAcceptedAt`: MUST be present, non-empty
- `tosVersion`: MUST be present, non-empty

### Timestamp Validation
- MUST be in ISO 8601 format (e.g., `2025-10-11T19:25:00.000Z`)
- MUST NOT be in the future
- MUST be within the last 24 hours (security measure)

### Version Validation
- MUST be in YYYY-MM-DD format (e.g., `2025-10-11`)

## Error Responses

### Missing ToS Fields
```json
{
  "error_code": 400,
  "error_message": "Impossible de parser le json"
}
```

### Invalid Timestamp Format
```json
{
  "error_code": 400,
  "error_message": "INVALID_TOS_TIMESTAMP: Terms of Service acceptance timestamp must be in ISO 8601 format (e.g., 2025-10-11T14:23:45.123Z)"
}
```

### Future Timestamp
```json
{
  "error_code": 400,
  "error_message": "INVALID_TOS_TIMESTAMP: Terms of Service acceptance timestamp cannot be in the future"
}
```

### Old Timestamp (> 24 hours)
```json
{
  "error_code": 400,
  "error_message": "INVALID_TOS_TIMESTAMP: Terms of Service acceptance timestamp must be within the last 24 hours"
}
```

### Invalid Version Format
```json
{
  "error_code": 400,
  "error_message": "INVALID_TOS_VERSION: Terms of Service version must be in YYYY-MM-DD format"
}
```

## API Examples

### Successful Registration with ToS

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{
    "userName": "johndoe",
    "displayName": "John Doe",
    "email": "john.doe@example.com",
    "password": "SecurePass123!",
    "lastName": "Doe",
    "firstName": "John",
    "tosAcceptedAt": "2025-10-11T19:25:00.000Z",
    "tosVersion": "2025-10-11"
  }'
```

**Response (201 Created):**
```json
{
  "id": "a540be9c-2bae-4a3a-8694-2591b3236eb0",
  "name": "johndoe",
  "email": "john.doe@example.com",
  "created_at": "2025-10-11T19:30:57Z",
  "tos_accepted_at": "2025-10-11T19:25:00.000Z",
  "tos_version": "2025-10-11"
}
```

### Failed Registration (Missing ToS)

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{
    "userName": "johndoe",
    "displayName": "John Doe",
    "email": "john.doe@example.com",
    "password": "SecurePass123!",
    "lastName": "Doe",
    "firstName": "John"
  }'
```

**Response (400 Bad Request):**
```json
{
  "error_code": 400,
  "error_message": "Impossible de parser le json"
}
```

## Storage Architecture

### Casdoor User Properties
Since this system uses Casdoor for user management, ToS data is stored in the `Properties` map of the Casdoor user object:

```go
user.Properties = map[string]string{
    "username": "generated_username",
    "tos_accepted_at": "2025-10-11T19:25:00.000Z",
    "tos_version": "2025-10-11"
}
```

This approach:
- ✅ Requires no database schema changes
- ✅ Works with existing Casdoor infrastructure
- ✅ Persists data across user sessions
- ✅ Is included in Casdoor's user data exports
- ✅ Is GDPR compliant (data is tied to user account)

## Testing

### Automated Test Script
Run `./test_tos_registration.sh` to execute automated tests:

**Test Coverage:**
1. ✅ Registration without ToS fields (should fail)
2. ✅ Registration with invalid timestamp (should fail)
3. ✅ Registration with valid ToS acceptance (should succeed)

### Manual Testing
See test script for curl examples of each scenario.

## GDPR Compliance

### Article 7 - Conditions for Consent
✅ **Records of consent:**
- Timestamp of acceptance stored (`tos_accepted_at`)
- Version of ToS accepted stored (`tos_version`)
- Data is retrievable via user API endpoints

✅ **Burden of proof:**
- System can demonstrate when and which version was accepted
- User cannot register without explicit ToS acceptance

### Article 20 - Right to Data Portability
✅ **Data export includes ToS data:**
- `GET /api/v1/users/{id}` returns `tos_accepted_at` and `tos_version`
- Data is in machine-readable JSON format

### Article 17 - Right to Erasure
✅ **Account deletion:**
- When user is deleted from Casdoor, all Properties (including ToS data) are removed
- Handled by existing `DELETE /api/v1/users/{id}` endpoint

## Security Features

### Replay Attack Prevention
- Timestamp must be within last 24 hours
- Prevents reuse of old ToS acceptance data

### Timestamp Integrity
- Server validates timestamp format and logic
- Cannot accept future timestamps
- Cannot accept very old timestamps

### Version Tracking
- Enforces consistent version format (YYYY-MM-DD)
- Enables future version enforcement
- Allows identification of users with outdated ToS acceptance

## Future Enhancements

### Recommended (Not Implemented)

1. **ToS Version Management Table**
   - Create `terms_of_service` table to track versions
   - Store full ToS content for each version
   - Track effective dates

2. **Re-acceptance Flow**
   - Middleware to check ToS version on login
   - Redirect to ToS acceptance page if outdated
   - Update user's ToS data on re-acceptance

3. **Admin Dashboard**
   - View users by ToS version
   - Generate compliance reports
   - Force re-acceptance for all users

4. **Audit Logging**
   - Create `tos_acceptance_log` table
   - Log every ToS acceptance/update event
   - Include IP address and user agent

5. **Migration for Existing Users**
   - Implement one of these strategies:
     - Implicit acceptance (set ToS data to registration date)
     - Forced re-acceptance (null ToS data, prompt on login)
     - Grandfather clause (mark existing users as exempt)

## Files Modified

1. `/src/auth/dto/userDto.go` - DTOs updated
2. `/src/auth/services/userService.go` - Validation and storage logic
3. `/docs/` - Swagger documentation regenerated (auto-generated)

## Files Created

1. `/workspaces/ocf-core/test_tos_registration.sh` - Automated test script
2. `/workspaces/ocf-core/TOS_IMPLEMENTATION_SUMMARY.md` - This file

## Next Steps for Frontend Integration

The frontend should:
1. ✅ Already implemented: Collect ToS acceptance via checkbox
2. ✅ Already implemented: Record timestamp when checkbox is clicked
3. ✅ Already implemented: Send `tosAcceptedAt` and `tosVersion` in registration request
4. Handle error responses (display validation errors to user)
5. Ensure error messages are user-friendly (translate technical errors)

## Verification

To verify the implementation:

1. **Run automated tests:**
   ```bash
   ./test_tos_registration.sh
   ```

2. **Check Swagger documentation:**
   - Visit `http://localhost:8080/swagger/`
   - Navigate to `POST /api/v1/users`
   - Verify `tosAcceptedAt` and `tosVersion` are marked as required

3. **Test with frontend:**
   - Register a new user with ToS checkbox checked
   - Verify user creation succeeds
   - Verify ToS data appears in user profile API response

## Conclusion

The backend is now fully compliant with the frontend's ToS acceptance flow and meets GDPR requirements for recording explicit user consent. All validation is in place to ensure data integrity and prevent abuse.
