---
description: Systematic refactoring with pattern consistency
tags: [refactor, cleanup, patterns]
---

# Systematic Refactoring

Refactor code to follow project patterns and best practices.

## Refactoring Tasks

### 1. Permission Management Refactoring
**Pattern:** All direct Casbin calls → `utils` helpers

**Before:**
```go
casdoor.Enforcer.AddPolicy(userID, route, "POST")
```

**After:**
```go
opts := utils.DefaultPermissionOptions()
opts.LoadPolicyFirst = true
utils.AddPolicy(casdoor.Enforcer, userID, route, "POST", opts)
```

### 2. Error Handling Refactoring
**Pattern:** Custom errors → `utils.Err*` constructors

**Before:**
```go
return errors.New("group not found")
```

**After:**
```go
return utils.ErrEntityNotFound("Group", groupID)
```

### 3. Validation Refactoring
**Pattern:** Inline checks → `utils.ChainValidators`

**Before:**
```go
if name == "" {
    return errors.New("name required")
}
if len(name) < 3 {
    return errors.New("name too short")
}
```

**After:**
```go
if err := utils.ChainValidators(
    utils.ValidateStringNotEmpty(name, "name"),
    utils.ValidateStringLength(name, 3, 100, "name"),
); err != nil {
    return err
}
```

### 4. Converter Refactoring
**Pattern:** Reflection code → `converters.GenericModelToOutput`

### 5. DTO Refactoring
**Pattern:** Add missing `mapstructure` tags

## Process

1. **Ask what to refactor:**
   - Specific file/function
   - Pattern across codebase
   - Module-wide refactoring

2. **Use Explore agent** to find all occurrences

3. **Show refactoring plan** with:
   - Number of files affected
   - Risk assessment
   - Breaking changes (if any)

4. **Execute systematically:**
   - One file at a time
   - Run tests after each change
   - Revert if tests fail

5. **Validate:**
   - Run `make lint`
   - Run `make test`
   - Check for regressions

**Safety:** I'll always show the plan before making bulk changes!
