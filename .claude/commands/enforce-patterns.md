---
description: Scan codebase for pattern violations and enforce consistency
tags: [patterns, consistency, quality, refactor]
---

# Pattern Enforcement Agent

Systematically scan and enforce OCF Core patterns across the entire codebase.

## Patterns to Enforce

### 1. Permission Management Pattern
**Rule:** ALL Casbin operations use `utils` helpers

**Scan for violations:**
```go
// ❌ VIOLATIONS - Find these patterns:
casdoor.Enforcer.AddPolicy(...)
casdoor.Enforcer.RemovePolicy(...)
enforcer.AddPolicy(...)
.LoadPolicy()
.SavePolicy()
```

**Enforcement:**
- Use Grep to find all direct Casbin calls
- Show file:line for each violation
- Provide exact replacement code
- Auto-fix if approved

### 2. Error Handling Pattern
**Rule:** Use `utils.Err*` constructors, not raw errors

**Scan for violations:**
```go
// ❌ VIOLATIONS:
errors.New("entity not found")
fmt.Errorf("cannot delete")
errors.New("permission denied")
```

**Enforcement:**
- Find all `errors.New` and `fmt.Errorf` in services
- Categorize by error type
- Replace with appropriate constructor:
  - "not found" → `utils.ErrEntityNotFound`
  - "already exists" → `utils.ErrEntityAlreadyExists`
  - "permission" → `utils.ErrPermissionDenied`
  - etc.

### 3. Validation Pattern
**Rule:** Use `utils.ChainValidators` for input validation

**Scan for violations:**
```go
// ❌ VIOLATIONS:
if name == "" { return errors.New(...) }
if len(name) < 3 { return errors.New(...) }
if !regexp.Match(...) { return errors.New(...) }
```

**Enforcement:**
- Find inline validation checks
- Group related validations
- Replace with ChainValidators

### 4. DTO Pattern
**Rule:** All DTOs have `json` AND `mapstructure` tags

**Scan for violations:**
```go
// ❌ VIOLATIONS:
type SomeDto struct {
    Name string `json:"name"` // Missing mapstructure!
    ID   string `mapstructure:"id"` // Missing json!
}
```

**Enforcement:**
- Scan all files in `dto/` folders
- Check each struct field
- Add missing tags

### 5. Converter Pattern
**Rule:** Use `converters.GenericModelToOutput` in EntityModelToEntityOutput

**Scan for violations:**
```go
// ❌ VIOLATIONS:
func EntityModelToEntityOutput(input any) (any, error) {
    model := input.(*models.Entity)
    // Direct conversion without GenericModelToOutput
    return dto.EntityOutput{...}, nil
}
```

**Enforcement:**
- Check all entityRegistration files
- Find EntityModelToEntityOutput implementations
- Verify GenericModelToOutput usage

### 6. Logging Pattern
**Rule:** Use `utils.Debug/Info/Warn/Error`, not fmt.Println

**Scan for violations:**
```go
// ❌ VIOLATIONS:
fmt.Println(...)
log.Println(...)
println(...)
```

**Enforcement:**
- Find all print statements
- Categorize by log level needed
- Replace with appropriate utils function

### 7. Database Pattern
**Rule:** Use `postgres` hostname, not `localhost` in tests

**Scan for violations:**
```go
// ❌ VIOLATIONS in test setup:
"localhost:5432"
"127.0.0.1:5432"
```

**Enforcement:**
- Scan test files
- Find hardcoded localhost
- Replace with "postgres"

### 8. SQLite Pattern
**Rule:** Use shared cache for in-memory SQLite

**Scan for violations:**
```go
// ❌ VIOLATIONS:
sqlite.Open(":memory:")
```

**Enforcement:**
- Find all SQLite opens in tests
- Replace with `file::memory:?cache=shared`

### 9. EditDto Pattern
**Rule:** EditDto fields are pointers for partial updates

**Scan for violations:**
```go
// ❌ VIOLATIONS:
type EditEntityInput struct {
    Name string `json:"name"` // Should be *string
    Age  int    `json:"age"`  // Should be *int
}
```

**Enforcement:**
- Scan all EditDto structs
- Check field types
- Flag non-pointer fields (except nested structs)

### 10. Relationship Pattern
**Rule:** Foreign key fields end with "ID" and have indexes

**Scan for violations:**
```go
// ❌ VIOLATIONS:
UserUUID string `gorm:"type:uuid"` // Should be UserID
CourseId string // Wrong casing
UserID   string // Missing index tag
```

**Enforcement:**
- Scan all models
- Check foreign key naming
- Verify index tags exist

## Execution Modes

### Mode 1: Full Scan
Scan entire codebase for ALL patterns:
```
/enforce-patterns
→ "Run full pattern scan"
```

Output:
```
📊 Pattern Compliance Report

✅ Permission Management: 100% (0 violations)
⚠️  Error Handling: 85% (23 violations in 8 files)
⚠️  Validation: 78% (15 violations in 6 files)
✅ DTO Tags: 100% (0 violations)
...

Total Score: 87% compliant
```

### Mode 2: Specific Pattern
Focus on one pattern:
```
/enforce-patterns
→ "Check error handling pattern"
```

### Mode 3: File/Module
Scan specific area:
```
/enforce-patterns
→ "Scan src/organizations/ for violations"
```

### Mode 4: Auto-Fix Mode
Automatically fix violations:
```
/enforce-patterns
→ "Fix all DTO tag violations automatically"
```

## Report Format

For each violation:
```markdown
### ❌ Violation #1: Error Handling Pattern
**File:** src/groups/services/groupService.go:45
**Severity:** Important

**Current Code:**
```go
if group == nil {
    return errors.New("group not found")
}
```

**Should Be:**
```go
if group == nil {
    return utils.ErrEntityNotFound("Group", groupID)
}
```

**Why:** Consistent error handling improves debugging and provides
standardized error messages across the API.

**Auto-fix available:** Yes
```

## Progress Tracking

Use TodoWrite to track:
- [ ] Scan pattern X
- [ ] Fix violations in file Y
- [ ] Re-scan to verify
- [ ] Move to next pattern

## Safety

- Always show plan before bulk changes
- Run tests after each fix
- Revert if tests fail
- One pattern at a time for safety

This agent maintains code consistency at scale!
