# Authentication Security Integration Tests

## Overview

This directory contains comprehensive integration tests for the authentication security fixes implemented to address critical vulnerabilities in the login flow.

## Security Issues Addressed

### 1. URL Parameter Injection Vulnerability (CRITICAL)
**Problem**: Username and password parameters were not URL-encoded when building OAuth requests to Casdoor, causing:
- Authentication failures with special characters
- Potential URL injection attacks
- Unpredictable/cached token responses

**Fix**: Added proper URL encoding using `url.QueryEscape()` for all parameters
**Test Coverage**: `TestLoginURLEncoding`, `TestLoginToCasdoorURLEncoding`

### 2. Missing Token Validation (CRITICAL)
**Problem**: No validation that the JWT token returned by Casdoor actually belongs to the user who logged in
**Fix**: Added JWT token parsing and user ID validation before returning token
**Test Coverage**: `TestLoginTokenValidation`, `TestLoginSecurityAlerts`

### 3. JSON Binding Failure (HIGH)
**Problem**: Missing JSON tags caused the API to reject all login requests with lowercase field names
**Fix**: Added proper JSON tags to `LoginInput` DTO
**Test Coverage**: `TestLoginJSONBinding`

## Test Files

### authSecurity_test.go
Comprehensive integration tests for authentication security:

1. **TestLoginJSONBinding**
   - Tests JSON field binding with lowercase, uppercase, and mixed case
   - Verifies required field validation
   - Ensures API accepts standard lowercase JSON conventions

2. **TestLoginURLEncoding**
   - Creates test users with special characters in passwords
   - Tests login with passwords containing: `!@#$%^&*()`
   - Verifies no internal server errors occur
   - Confirms successful authentication with encoded parameters

3. **TestLoginTokenValidation**
   - Creates multiple test users
   - Verifies returned JWT token matches the user who logged in
   - Confirms token claims contain correct user ID
   - Tests the security layer that prevents token swap attacks

4. **TestLoginToCasdoorURLEncoding**
   - Unit tests for the `LoginToCasdoor` function
   - Tests various special characters: `!`, `&`, `=`, `+`, spaces
   - Verifies function doesn't crash with special characters
   - Tests both username and password encoding

5. **TestLoginWithWrongCredentials**
   - Verifies wrong passwords are rejected
   - Tests error handling for invalid credentials

6. **TestLoginSecurityAlerts**
   - Documents security logging patterns
   - Guides monitoring of security alerts
   - Verifies security alerting infrastructure

7. **BenchmarkLoginJSONParsing**
   - Performance benchmark for JSON parsing
   - Ensures security fixes don't impact performance

## Running the Tests

### Run All Auth Security Tests
```bash
go test ./tests/auth/authSecurity_test.go -v
```

### Run Specific Test
```bash
go test ./tests/auth/authSecurity_test.go -v -run TestLoginTokenValidation
```

### Run All Auth Tests (Including Security)
```bash
go test ./tests/auth/... -v
```

### Run with Coverage
```bash
go test ./tests/auth/authSecurity_test.go -v -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run Benchmarks
```bash
go test ./tests/auth/authSecurity_test.go -v -bench=. -benchmem
```

## Test Requirements

These integration tests require:
- Running Casdoor instance (configured via `.env.test`)
- Test database access
- Proper environment variables set

## Expected Test Behavior

### Successful Tests
- JSON binding tests should pass with lowercase fields
- URL encoding tests should handle special characters without errors
- Token validation tests should confirm user ID matches
- Wrong credentials should be properly rejected

### Test Cleanup
All tests properly clean up created test users using deferred cleanup functions:
```go
defer func() {
    user, _ := casdoorsdk.GetUserByEmail(testUser.Email)
    if user != nil {
        casdoorsdk.DeleteUser(user)
    }
}()
```

## Security Monitoring

### Log Patterns to Monitor

The security fixes include comprehensive logging. Monitor these patterns in production:

1. **Token Validation Success**:
   ```
   [SECURITY] Token validation passed for user {username} (ID: {user_id})
   ```

2. **Token Mismatch Alert** (CRITICAL):
   ```
   [SECURITY ALERT] Token user ID mismatch! Expected: {expected_id}, Got: {actual_id}
   ```

3. **Token Parse Error**:
   ```
   [SECURITY ERROR] Failed to parse JWT token during validation: {error}
   ```

4. **Debug Logging** (Development):
   ```
   [DEBUG LOGIN] User.Name={username}, User.Id={id}, User.Email={email}
   [DEBUG LOGIN] Casdoor response (truncated): {response}...
   ```

## Continuous Integration

Add these tests to your CI/CD pipeline:

```yaml
# Example GitHub Actions
- name: Run Security Tests
  run: go test ./tests/auth/authSecurity_test.go -v

- name: Check Test Coverage
  run: |
    go test ./tests/auth/... -coverprofile=coverage.out
    go tool cover -func=coverage.out
```

## Related Files

**Implementation**:
- `src/auth/authController.go` - Login endpoint with security fixes
- `src/auth/dto/loginDto.go` - DTO with JSON binding tags

**Original Issue**:
- User tsaquet7 received 1_sup's JWT token when logging in
- Root cause: URL parameter not encoded + no token validation

## Future Improvements

Consider adding:
1. Rate limiting tests for login endpoints
2. Session management security tests
3. Token refresh validation tests
4. Cross-Site Request Forgery (CSRF) protection tests
5. Brute force protection tests

## Questions or Issues?

If tests fail:
1. Check Casdoor service is running
2. Verify `.env.test` configuration
3. Check database connectivity
4. Review test logs for specific failure details

## Security Disclosure

If you discover security issues, please:
1. DO NOT open a public issue
2. Report to the security team
3. Provide test case demonstrating the issue
4. Allow time for patching before disclosure
