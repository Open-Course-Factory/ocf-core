---
description: Comprehensive pre-commit validation (run before every commit)
tags: [commit, validation, quality, ci]
---

# Pre-Commit Review Agent

Comprehensive validation before committing code. Acts as your automated quality gate.

## Pre-Commit Checklist

### Phase 1: Code Quality (Fast Checks)

#### 1.1 Linting
```bash
make lint
```

**Check for:**
- Go fmt issues
- Go vet warnings
- golangci-lint errors
- Unused imports

**Fail if:** Any linting errors

#### 1.2 Build Verification
```bash
go build ./...
```

**Check for:**
- Compilation errors
- Missing dependencies
- Type mismatches

**Fail if:** Build fails

### Phase 2: Pattern Compliance (Medium)

#### 2.1 Critical Patterns
**Fast pattern checks:**

```bash
# Check for direct Casbin calls (should use utils)
grep -r "casdoor.Enforcer.AddPolicy" src/ --exclude-dir=utils

# Check for hardcoded secrets
grep -r "sk_live_\|sk_test_\|password.*=" src/ --exclude="*_test.go"

# Check for raw errors (should use utils.Err*)
grep -r 'errors\.New\|fmt\.Errorf' src/*/services/ --exclude-dir=utils

# Check for missing mapstructure tags in DTOs
grep -A5 "type.*Dto struct" src/*/dto/*.go | grep "json:" | grep -v "mapstructure:"
```

**Warn if:** Violations found (don't fail, but show warnings)

### Phase 3: Testing (Slower)

#### 3.1 Affected Tests
**Intelligent test selection:**

```bash
# Get changed files
CHANGED_FILES=$(git diff --name-only HEAD)

# Determine which tests to run
if echo "$CHANGED_FILES" | grep -q "entityManagement/"; then
    make test-entity-manager
elif echo "$CHANGED_FILES" | grep -q "auth/"; then
    go test ./tests/auth/...
elif echo "$CHANGED_FILES" | grep -q "payment/"; then
    go test ./tests/payment/...
else
    make test-unit
fi
```

**Fail if:** Any tests fail

#### 3.2 Test Coverage
```bash
make coverage
```

**Check:**
- Overall coverage > 80%
- Changed files coverage > 70%

**Warn if:** Coverage drops

### Phase 4: Documentation (Fast)

#### 4.1 Swagger Updates
**If API changes detected:**

```bash
# Check for handler changes
if git diff --name-only HEAD | grep -q "handlers/\|routes/"; then
    echo "⚠️  API changes detected - update Swagger docs"
    swag init --parseDependency --parseInternal
fi
```

#### 4.2 Comment Quality
**Check for:**
- Public functions have comments
- Complex logic explained
- TODO comments tracked

### Phase 5: Security (Fast)

#### 5.1 Quick Security Scan
```bash
# Check for exposed secrets
git diff HEAD | grep -E "sk_live_|password|api_key|secret"

# Check for SQL injection patterns
git diff HEAD | grep -E "fmt.Sprintf.*SELECT|db.Raw.*\+"

# Check for exposed errors
git diff HEAD | grep -E "ctx.JSON.*err.Error()"
```

**Fail if:** Security issues found

### Phase 6: Architecture (Fast)

#### 6.1 Layer Violations
**Check changed files:**
- Handlers don't import repositories directly
- Models don't import services
- No circular dependencies added

### Phase 7: Git Quality

#### 7.1 Commit Message
**Validate format:**
```
<type>: <subject>

<body>

<footer>
```

**Types:** feat, fix, refactor, test, docs, chore

**Fail if:** Commit message doesn't follow convention

#### 7.2 File Size
**Check for:**
- Large files (> 1MB)
- Binary files
- Generated files (docs/swagger)

**Warn if:** Large files being committed

## Execution

### Automatic Pre-Commit
```bash
# Run before every commit
/pre-commit
```

**Output:**
```
🔍 Pre-Commit Validation

✅ Phase 1: Code Quality
   ✅ Linting: Passed
   ✅ Build: Passed

⚠️  Phase 2: Pattern Compliance
   ⚠️  Found 3 pattern violations (warnings only)
      - src/groups/services/groupService.go:45: Direct Casbin call
      - src/users/dto/userDto.go:12: Missing mapstructure tag
      - src/auth/services/authService.go:89: Raw error

✅ Phase 3: Testing
   ✅ Unit tests: 45 passed
   ✅ Coverage: 87% (unchanged)

✅ Phase 4: Documentation
   ℹ️  No API changes detected

✅ Phase 5: Security
   ✅ No security issues found

✅ Phase 6: Architecture
   ✅ No layer violations

✅ Phase 7: Git Quality
   ✅ Commit message: Valid
   ✅ File sizes: OK

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ READY TO COMMIT

⚠️  3 warnings (fix in next iteration)

Time: 45s
```

### Fix Violations
**If violations found:**

```
/pre-commit
→ Agent: "Found 3 violations. Fix them?"
→ You: "Yes, fix automatically"
→ Agent: [Fixes each violation]
→ Agent: [Re-runs pre-commit]
→ Agent: "✅ All checks passed!"
```

### Quick Mode (Skip Slow Checks)
```
/pre-commit
→ "Run quick mode (skip tests)"
```

## Git Hook Integration

**Create git hook:**

```bash
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash

# Run Claude Code pre-commit check
echo "Running pre-commit validation..."

# Fast checks only
make lint || exit 1
go build ./... || exit 1

echo "✅ Pre-commit passed (run /pre-commit for full validation)"
EOF

chmod +x .git/hooks/pre-commit
```

## Continuous Integration

**In CI/CD pipeline:**
```yaml
# .github/workflows/pr.yml
- name: Pre-Commit Validation
  run: |
    make lint
    make test
    make coverage
```

## Customization

**Skip specific checks:**
```bash
# Skip tests (emergency fix)
git commit --no-verify
```

**Add custom checks:**
Edit `.claude/commands/pre-commit.md` to add project-specific validations.

## Best Practices

1. **Run before every commit** - Catch issues early
2. **Fix violations immediately** - Don't accumulate tech debt
3. **Update Swagger docs** - Keep API documentation current
4. **Write tests first** - Maintain coverage
5. **Review warnings** - Address in next iteration

This agent ensures every commit meets quality standards!
