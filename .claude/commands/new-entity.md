---
description: Create a new entity with full CRUD operations, DTOs, registration, and tests
tags: [entity, scaffold, crud]
---

# Create New Entity

I need to create a new entity with complete implementation following the Entity Management System.

## Requirements

1. **Ask for entity details:**
   - Entity name (singular, PascalCase, e.g., "License")
   - Module name (e.g., "licenses")
   - Fields and their types
   - Relationships (belongsTo, hasMany, etc.)
   - Required permissions (which roles can do what)

2. **Create all required files:**
   - `src/{module}/models/{entity}.go` - GORM model
   - `src/{module}/dto/{entity}Dto.go` - Input/Output DTOs (with mapstructure tags!)
   - `src/{module}/entityRegistration/{entity}Registration.go` - Registration
   - `src/{module}/services/{entity}Service.go` - Custom service (if needed)
   - `tests/{module}/{entity}_test.go` - Test suite

3. **Follow these patterns:**
   - EditDto fields MUST be pointers for partial updates
   - ALL DTOs need both `json` and `mapstructure` tags
   - Use `converters.GenericModelToOutput` in registration
   - Implement custom `EntityDtoToMap` for pointer fields
   - Add BaseEntityDto embedding for timestamps
   - Use validation helpers from `utils` package

4. **Register the entity:**
   - Add migration to `main.go` AutoMigrate
   - Register with `ems.GlobalEntityRegistrationService.RegisterEntity()`
   - Run `swag init --parseDependency --parseInternal`

5. **Create tests:**
   - Test CRUD operations (Create, GetAll, GetOne, Update, Delete)
   - Test relationships
   - Test permissions
   - Use shared cache SQLite: `file::memory:?cache=shared`

6. **Validate:**
   - Run `make lint`
   - Run `make test`
   - Check Swagger docs at http://localhost:8080/swagger/

Show me the implementation plan first, then create all files systematically.
