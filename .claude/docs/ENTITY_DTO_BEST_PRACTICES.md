# Entity DTO Best Practices

## The Order Field Problem

### Issue
When using GORM's `Updates()` method with non-pointer fields in Edit DTOs, zero values (like `0` for `int`, `""` for `string`, `false` for `bool`) are included in the update, even if they weren't sent in the PATCH request. This corrupts important fields like `Order` in join tables.

### Example of the Problem

**Bad DTO:**
```go
type EditSectionInput struct {
    Title  string `json:"title"`
    Number int    `json:"number"`  // ❌ Will be 0 if not provided
}
```

**What happens:**
1. Frontend sends: `{"title": "New Title"}` (no `number` field)
2. Go decodes into struct: `EditSectionInput{Title: "New Title", Number: 0}`
3. Default `EntityDtoToMap` includes: `{"title": "New Title", "number": 0}`
4. GORM updates: Sets `number = 0` in database (corrupts Order!)

### Solution: Use Pointer Types

**Good DTO:**
```go
type EditSectionInput struct {
    Title  *string `json:"title,omitempty" mapstructure:"title"`
    Number *int    `json:"number,omitempty" mapstructure:"number"`  // ✅ Will be nil if not provided
}
```

**With custom EntityDtoToMap:**
```go
func (s SectionRegistration) EntityDtoToMap(input any) map[string]any {
    editDto := input.(dto.EditSectionInput)
    updates := make(map[string]any)

    // Only include non-nil fields
    if editDto.Title != nil {
        updates["title"] = *editDto.Title
    }
    if editDto.Number != nil {
        updates["number"] = *editDto.Number
    }

    return updates
}
```

**Result:**
1. Frontend sends: `{"title": "New Title"}` (no `number` field)
2. Go decodes into struct: `EditSectionInput{Title: &"New Title", Number: nil}`
3. Custom `EntityDtoToMap` includes: `{"title": "New Title"}` (skips nil)
4. GORM updates: Only sets `title`, leaves `number` unchanged ✅

## Required Changes for All Edit DTOs

### 1. Update DTO Definition

**All EditDto fields should use pointers for optional fields:**

```go
type EditEntityInput struct {
    // Pointer types for all updatable fields
    Name        *string    `json:"name,omitempty" mapstructure:"name"`
    DisplayName *string    `json:"display_name,omitempty" mapstructure:"display_name"`
    IsActive    *bool      `json:"is_active,omitempty" mapstructure:"is_active"`
    Order       *int       `json:"order,omitempty" mapstructure:"order"`
    ExpiresAt   *time.Time `json:"expires_at,omitempty" mapstructure:"expires_at"`

    // Slices can stay as-is (nil is their zero value)
    Tags        []string   `json:"tags,omitempty" mapstructure:"tags"`
}
```

**Tag Requirements:**
- `json:"field_name,omitempty"` - For JSON parsing (Gin's BindJSON)
- `mapstructure:"field_name"` - For map-to-struct decoding (used in PATCH handler)

### 2. Add Custom EntityDtoToMap

**Pattern for registration file:**

```go
func (r EntityRegistration) EntityDtoToMap(input any) map[string]any {
    editDto, ok := input.(dto.EditEntityInput)
    if !ok {
        // Fallback to default behavior
        return r.AbstractRegistrableInterface.EntityDtoToMap(input)
    }

    updates := make(map[string]any)

    // Check each pointer field and only include non-nil values
    if editDto.Name != nil {
        updates["name"] = *editDto.Name
    }
    if editDto.DisplayName != nil {
        updates["display_name"] = *editDto.DisplayName
    }
    if editDto.IsActive != nil {
        updates["is_active"] = *editDto.IsActive
    }
    if editDto.Order != nil {
        updates["order"] = *editDto.Order
    }
    if editDto.ExpiresAt != nil {
        updates["expires_at"] = *editDto.ExpiresAt
    }

    // Slices: check for nil and length
    if editDto.Tags != nil && len(editDto.Tags) > 0 {
        updates["tags"] = editDto.Tags
    }

    return updates
}
```

### 3. Register the Converter

**In GetEntityRegistrationInput():**

```go
func (r EntityRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
    return entityManagementInterfaces.EntityRegistrationInput{
        EntityInterface: models.Entity{},
        EntityConverters: entityManagementInterfaces.EntityConverters{
            ModelToDto: r.EntityModelToEntityOutput,
            DtoToModel: r.EntityInputDtoToEntityModel,
            DtoToMap:   r.EntityDtoToMap,  // ✅ Register custom converter
        },
        // ...
    }
}
```

## Entities with Order Fields (Critical)

The following entities MUST use pointer types in Edit DTOs because they have `Order` fields used in join tables:

1. **Section** (`section.go:26`) - Order in `chapter_sections`
2. **Chapter** (`chapter.go:21`) - Order in `course_chapters`
3. **Page** (`page.go:17`) - Order in `section_pages`

### Status

✅ **Section** - Fixed (pointer types + custom EntityDtoToMap)
✅ **Chapter** - Fixed (pointer types + custom EntityDtoToMap)
✅ **Page** - Fixed (pointer types + custom EntityDtoToMap)

## Checklist for New Entities

When creating a new entity with PATCH support:

- [ ] **Edit DTO uses pointer types** for all optional fields
- [ ] **Both `json` and `mapstructure` tags** are present on all fields
- [ ] **Custom `EntityDtoToMap` method** implemented in registration
- [ ] **EntityDtoToMap registered** in `GetEntityRegistrationInput()`
- [ ] **Test PATCH endpoint** with partial updates to verify fields are preserved

## Testing PATCH Behavior

**Good test pattern:**

```bash
# 1. Get entity current state
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/entities/123

# 2. Update ONLY one field
curl -X PATCH -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title": "New Title"}' \
  http://localhost:8080/api/v1/entities/123

# 3. Verify other fields unchanged
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/entities/123

# Expected: Only 'title' changed, all other fields (including Order) preserved
```

## Framework Improvement (Future)

Consider adding validation in the entity registration system to:

1. **Detect non-pointer fields** in Edit DTOs and warn/error
2. **Require EntityDtoToMap** for entities with Order fields
3. **Auto-generate EntityDtoToMap** from Edit DTO struct tags

This would prevent developers from accidentally creating DTOs that corrupt Order fields.

## Related Files

- `/workspaces/ocf-core/CLAUDE.md` - Project-wide development guidelines
- `/workspaces/ocf-core/src/entityManagement/routes/editEntity.go` - PATCH handler implementation
- `/workspaces/ocf-core/src/entityManagement/interfaces/registrableInterface.go` - Default EntityDtoToMap
