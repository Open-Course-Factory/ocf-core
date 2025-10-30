---
description: Handle database migrations and schema changes
tags: [database, migration, schema]
---

# Database Migration Helper

Handle database schema changes safely.

## Tasks

### 1. Adding New Fields
- Add field to model with proper GORM tags
- Update relevant DTOs (Input/Output/Edit)
- Add to EntityDtoToMap if needed
- Run `go run main.go` to auto-migrate
- Test CRUD operations work

### 2. Changing Field Types
**Warning:** May lose data! Consider:
- Create new field with new type
- Migrate data from old to new field
- Drop old field
- Or: Manual SQL migration

### 3. Adding Relationships
- Add foreign key field to model
- Add relationship tag to both models
- Update DTOs to include related data
- Add relationship filter if needed
- Update SwaggerConfig for preloading

### 4. Database Reset (Development)
```bash
# Stop containers
docker compose down

# Remove volumes
docker volume rm ocf-core_postgres_data

# Restart
docker compose up -d

# Restart server (auto-migrates)
go run main.go
```

### 5. Check Migration Status
I'll:
- Read current models
- Check main.go AutoMigrate list
- Verify all entities registered
- Look for orphaned migrations

### 6. Production Considerations
- GORM AutoMigrate is safe (adds only, doesn't drop)
- For destructive changes, use manual migrations
- Always backup before major schema changes
- Test in dev environment first

**Current DB Config:**
- Dev: `postgres:5432` / database: `ocf`
- Test: `postgres:5432` / database: `ocf_test`

Tell me what migration you need!
