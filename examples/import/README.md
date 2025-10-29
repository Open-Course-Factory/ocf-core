# Organization Bulk Import

This directory contains example CSV files for the bulk import feature. You can import multiple users, groups, and memberships into an organization in a single operation.

## CSV File Formats

### 1. users.csv (Required)

Import users into the organization.

**Columns:**
- `email` (required) - User email address (must be unique)
- `first_name` (required) - User's first name
- `last_name` (required) - User's last name
- `password` (required) - Temporary password
- `role` (required) - User role: `member`, `supervisor`, `admin`, or `trainer`
- `external_id` (optional) - Reference ID from external system (e.g., student ID)
- `force_reset` (optional) - "true" or "false" - Force password reset on first login
- `update_existing` (optional) - "true" or "false" - Update existing user if found

**Example:**
```csv
email,first_name,last_name,password,role,external_id,force_reset,update_existing
john.doe@school.fr,John,Doe,TempPass123!,member,student_001,true,false
```

### 2. groups.csv (Optional)

Create groups (classes, departments) with support for nested hierarchies.

**Columns:**
- `group_name` (required) - Unique identifier for the group
- `display_name` (required) - Human-readable name
- `description` (optional) - Group description
- `parent_group` (optional) - Parent group name for nested structure
- `max_members` (optional) - Maximum number of members (default: 50)
- `expires_at` (optional) - Expiration date (ISO8601 format: `2026-06-30T23:59:59Z`)
- `external_id` (optional) - Reference ID from external system

**Example (Nested Groups):**
```csv
group_name,display_name,description,parent_group,max_members,expires_at,external_id
m1_devops,M1 DevOps,Master 1 DevOps,,150,,dept_devops
m1_devops_a,M1 DevOps A,Class A,m1_devops,50,2026-06-30T23:59:59Z,class_a
m1_devops_b,M1 DevOps B,Class B,m1_devops,50,2026-06-30T23:59:59Z,class_b
```

This creates a hierarchy:
```
M1 DevOps (parent)
├── M1 DevOps A (child)
└── M1 DevOps B (child)
```

### 3. memberships.csv (Optional)

Assign users to groups with specific roles.

**Columns:**
- `user_email` (required) - User email (must exist in users.csv or organization)
- `group_name` (required) - Group name (must exist in groups.csv or organization)
- `role` (required) - Group role: `member`, `admin`, `assistant`, or `owner`

**Example:**
```csv
user_email,group_name,role
john.doe@school.fr,m1_devops_a,member
pierre.petit@school.fr,m1_devops_a,admin
```

## API Usage

### Endpoint
```
POST /api/v1/organizations/{organization_id}/import
```

### Authentication
Requires Bearer token with organization manager permissions.

### Parameters
- `users` (file, required) - Users CSV file
- `groups` (file, optional) - Groups CSV file
- `memberships` (file, optional) - Memberships CSV file
- `dry_run` (boolean, optional) - Validate without persisting (default: false)
- `update_existing` (boolean, optional) - Update existing users/groups (default: false)

### Example with curl

```bash
# Get authentication token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"1.supervisor@test.com","password":"test"}' | jq -r '.access_token')

# Dry-run import (validation only)
curl -X POST "http://localhost:8080/api/v1/organizations/{org_id}/import" \
  -H "Authorization: Bearer $TOKEN" \
  -F "users=@users.csv" \
  -F "groups=@groups.csv" \
  -F "memberships=@memberships.csv" \
  -F "dry_run=true" \
  -F "update_existing=false"

# Actual import
curl -X POST "http://localhost:8080/api/v1/organizations/{org_id}/import" \
  -H "Authorization: Bearer $TOKEN" \
  -F "users=@users.csv" \
  -F "groups=@groups.csv" \
  -F "memberships=@memberships.csv" \
  -F "dry_run=false" \
  -F "update_existing=true"
```

## Response Format

```json
{
  "success": true,
  "dry_run": false,
  "summary": {
    "users_created": 9,
    "users_updated": 0,
    "users_skipped": 0,
    "groups_created": 9,
    "groups_updated": 0,
    "groups_skipped": 0,
    "memberships_created": 15,
    "memberships_skipped": 0,
    "total_processed": 33,
    "processing_time": "2.5s"
  },
  "errors": [],
  "warnings": []
}
```

## Error Handling

If validation fails, the response includes detailed error information:

```json
{
  "success": false,
  "errors": [
    {
      "row": 5,
      "file": "users",
      "field": "email",
      "message": "Invalid email format",
      "code": "INVALID_EMAIL"
    }
  ]
}
```

### Error Codes
- `VALIDATION_ERROR` - Missing or invalid field
- `DUPLICATE` - Duplicate entry
- `LIMIT_EXCEEDED` - Organization limit exceeded
- `NOT_FOUND` - Referenced entity not found
- `INVALID_ROLE` - Invalid role specified
- `INVALID_EMAIL` - Invalid email format
- `INVALID_DATE` - Invalid date format

## Best Practices

1. **Always test with dry_run first** - Validate your CSV files before importing
2. **Use strong passwords** - Import sets temporary passwords, users should change them on first login
3. **Enable force_reset** - Require password change on first login for security
4. **Check organization limits** - Ensure your CSV doesn't exceed max_groups and max_members
5. **Handle nested groups carefully** - Parent groups must be defined before child groups
6. **Use external_id** - Track references to external systems (Hyperplanning, Pronote, etc.)

## Integration with School Systems

### Hyperplanning Export
Hyperplanning exports typically look like:
```csv
Nom,Prénom,Email,Classe,Statut
Doe,John,john.doe@school.fr,1ère A,Élève
```

You'll need to convert this to OCF format using a mapping script or manually.

### Pronote Export
Similar format, may require conversion. A conversion script example:

```bash
# Convert Hyperplanning/Pronote export to OCF format
python scripts/convert_school_export.py input.csv --output-dir ./import/
```

## Recurring Imports

For recurring imports (new semester, new students):
1. Export updated data from your school system
2. Convert to OCF CSV format
3. Set `update_existing=true` to update existing users
4. Run import with `dry_run=true` first
5. Review the summary and errors
6. Run actual import

This allows you to sync your organization with external systems regularly.

## Support

For issues or questions about bulk import:
- Check API documentation: http://localhost:8080/swagger/
- Review error codes and messages in the response
- Contact support with your dry-run results
