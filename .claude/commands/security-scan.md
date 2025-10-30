---
description: Security vulnerability scanner and best practices enforcer
tags: [security, vulnerabilities, audit, safety]
---

# Security Audit Agent

Comprehensive security scan for vulnerabilities and best practices.

## Security Checks

### 1. Authentication & Authorization

#### A. JWT Token Handling
- [ ] Tokens validated using proper certificate
- [ ] Token expiration checked
- [ ] Refresh token rotation implemented
- [ ] No tokens in logs
- [ ] Secure token storage

**Scan for:**
```go
// ❌ VULNERABILITIES:
fmt.Println("Token:", token) // Token in logs
token := ctx.GetHeader("Authorization") // No validation
// Missing token expiration check
```

#### B. Permission Checks
- [ ] Every route has permission middleware
- [ ] Resource ownership verified
- [ ] No permission bypasses
- [ ] Proper role hierarchy

**Scan for:**
```go
// ❌ VULNERABILITIES:
// Handler without AuthManagement() middleware
// Direct resource access without ownership check
// Hardcoded admin checks
```

### 2. SQL Injection Prevention

#### A. GORM Usage
- [ ] All queries use GORM methods (not raw SQL)
- [ ] Raw SQL uses parameterized queries
- [ ] No string concatenation in queries
- [ ] User input sanitized

**Scan for:**
```go
// ❌ VULNERABILITIES:
db.Raw("SELECT * FROM users WHERE id = " + userID) // Injection!
db.Exec(fmt.Sprintf("DELETE FROM %s WHERE...", table)) // Injection!
```

**Should be:**
```go
db.Raw("SELECT * FROM users WHERE id = ?", userID)
```

#### B. Dynamic Queries
- [ ] Filter fields whitelisted
- [ ] Column names validated
- [ ] No user input in table/column names

### 3. Secrets Management

#### A. Environment Variables
- [ ] All secrets in .env (not hardcoded)
- [ ] .env in .gitignore
- [ ] No secrets in code comments
- [ ] No secrets in logs

**Scan for:**
```go
// ❌ VULNERABILITIES:
apiKey := "sk_live_abcd1234..." // Hardcoded!
password := "admin123" // Hardcoded!
// TODO: Change password from "temp123" // Exposed!
```

#### B. Sensitive Data Logging
**Scan for:**
```go
// ❌ VULNERABILITIES:
utils.Info("User logged in: %+v", user) // May log password hash
log.Printf("Request: %v", request) // May log tokens
fmt.Println("DB Config:", config) // May log credentials
```

### 4. Input Validation

#### A. Type Safety
- [ ] All inputs validated before use
- [ ] UUIDs validated as UUIDs
- [ ] Enums validated against allowed values
- [ ] Lengths checked

**Scan for:**
```go
// ❌ VULNERABILITIES:
// Using input without validation
name := input.Name // No length check
userID := ctx.Param("id") // No UUID validation
```

#### B. File Uploads
- [ ] File type validation
- [ ] File size limits
- [ ] Filename sanitization
- [ ] Stored outside webroot

### 5. API Security

#### A. Rate Limiting
- [ ] Rate limiting on auth endpoints
- [ ] Rate limiting on expensive operations
- [ ] Protection against brute force

#### B. CORS Configuration
- [ ] CORS properly configured
- [ ] No wildcard origins in production
- [ ] Credentials handled securely

**Check:**
```go
// ⚠️ REVIEW NEEDED:
AllowOrigins: []string{"*"} // Too permissive?
```

### 6. Cryptography

#### A. Password Handling
- [ ] Passwords hashed (bcrypt/argon2)
- [ ] No plain text passwords stored
- [ ] Proper salt usage
- [ ] Password complexity enforced

#### B. Encryption
- [ ] Sensitive data encrypted at rest
- [ ] TLS for data in transit
- [ ] Strong encryption algorithms

### 7. Error Handling

#### A. Information Disclosure
- [ ] Error messages don't expose internals
- [ ] Stack traces not sent to client
- [ ] Database errors sanitized

**Scan for:**
```go
// ❌ VULNERABILITIES:
return ctx.JSON(500, err.Error()) // May expose DB structure
panic(err) // Stack trace to client
```

### 8. Dependencies

#### A. Outdated Packages
- [ ] No known vulnerabilities in dependencies
- [ ] Regular dependency updates
- [ ] go.mod audit

**Run:**
```bash
go list -m all | nancy sleuth
```

### 9. Business Logic Vulnerabilities

#### A. Resource Access
- [ ] Users can only access their own resources
- [ ] Organization isolation enforced
- [ ] Group permissions cascading correctly

#### B. Financial Operations
- [ ] Subscription changes validated
- [ ] Payment amounts verified server-side
- [ ] No client-side price manipulation
- [ ] Idempotency for payment operations

### 10. Terminal/SSH Security

#### A. Key Management
- [ ] SSH keys properly generated
- [ ] Private keys never exposed
- [ ] Key rotation supported

#### B. Session Security
- [ ] Terminal sessions isolated
- [ ] Session timeouts enforced
- [ ] Proper cleanup on disconnect

## Execution

### Full Security Audit
```
/security-scan
```

Output:
```markdown
🔒 Security Audit Report

## Critical Issues (Fix Immediately)
1. ❌ SQL Injection risk in src/courses/repository.go:45
   - Severity: CRITICAL
   - Fix: Use parameterized query

## High Priority
2. ⚠️  Hardcoded API key in src/payment/stripe.go:12
   - Severity: HIGH
   - Fix: Move to environment variable

## Medium Priority
3. ⚠️  Missing rate limiting on /api/v1/auth/login
   - Severity: MEDIUM
   - Fix: Add rate limiting middleware

## Low Priority / Informational
4. ℹ️  Consider adding CSRF protection
   - Severity: LOW
   - Recommendation: Add CSRF tokens for state-changing operations

## Statistics
- Critical: 1
- High: 1
- Medium: 3
- Low: 5
- Total Score: 75/100

## Compliance Checklist
✅ Authentication: Secure
✅ Authorization: Secure
❌ SQL Injection Prevention: Issues found
✅ Secrets Management: Secure
⚠️  Input Validation: Needs improvement
✅ API Security: Secure
✅ Cryptography: Secure
⚠️  Error Handling: Needs improvement
✅ Dependencies: Up to date
✅ Business Logic: Secure
```

### Focused Scan
```
/security-scan
→ "Check SQL injection vulnerabilities only"
```

### File/Module Scan
```
/security-scan
→ "Audit src/payment/ for security issues"
```

## Automated Fixes

For each issue, provide:
1. Exact location (file:line)
2. Description of vulnerability
3. Exploit scenario (how it could be used)
4. Fix code
5. Prevention guidance

## Continuous Security

**Best Practice:** Run before:
- Each commit (critical issues)
- Each PR (full audit)
- Each release (comprehensive review)

This agent is your security expert!
