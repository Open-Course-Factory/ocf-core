---
description: Review entity implementation for completeness and best practices
tags: [review, entity, quality]
---

# Entity Implementation Reviewer

Review an entity's implementation for completeness and adherence to best practices.

## What to Check

Ask me which entity to review, then I'll systematically check:

### 1. Model (`src/{module}/models/{entity}.go`)
- [ ] Proper GORM tags (`gorm:"..."`)
- [ ] UUID primary key
- [ ] Timestamps (CreatedAt, UpdatedAt, DeletedAt)
- [ ] Relationships defined correctly
- [ ] Indexes on foreign keys
- [ ] Validation tags if needed

### 2. DTOs (`src/{module}/dto/{entity}Dto.go`)
- [ ] InputCreateDto exists with required fields
- [ ] InputEditDto exists with pointer fields
- [ ] OutputDto exists
- [ ] All DTOs have `json` AND `mapstructure` tags
- [ ] Proper field validations (`binding:"required"`)
- [ ] Embedding of base DTOs (BaseEntityDto, etc.)

### 3. Registration (`src/{module}/entityRegistration/{entity}Registration.go`)
- [ ] Implements RegistrableInterface
- [ ] EntityModelToEntityOutput implemented
- [ ] EntityInputDtoToEntityModel implemented
- [ ] EntityDtoToMap implemented (for pointer fields)
- [ ] Uses GenericModelToOutput converter
- [ ] GetEntityRoles defined correctly
- [ ] SwaggerConfig complete
- [ ] RelationshipFilters if needed

### 4. Integration (`main.go`)
- [ ] AutoMigrate called
- [ ] RegisterEntity called
- [ ] In correct order (dependencies first)

### 5. Tests (`tests/{module}/{entity}_test.go`)
- [ ] CRUD operations tested
- [ ] Relationships tested
- [ ] Permissions tested
- [ ] Uses shared cache SQLite
- [ ] Proper cleanup

### 6. Swagger Documentation
- [ ] All operations documented
- [ ] Parameters described
- [ ] Response schemas defined

### 7. Best Practices
- [ ] Uses utils validators
- [ ] Uses utils error constructors
- [ ] Uses converters.GenericModelToOutput
- [ ] Follows naming conventions
- [ ] No direct Casbin calls (uses utils helpers)

I'll check each category and report any issues with specific line numbers and suggested fixes.
