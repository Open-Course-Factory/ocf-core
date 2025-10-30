# Bulk Import - Frontend Implementation Specification

## Security & Authorization

### ✅ Endpoint Security Verified

**Endpoint**: `POST /api/v1/organizations/{organization_id}/import`

**Security Layers**:

1. **Authentication Required** (BearerAuth)
   - User must be logged in with valid JWT token
   - Token must be included in Authorization header: `Bearer {token}`

2. **Organization-Level Authorization**
   - User must be a member of the target organization
   - User must have one of these roles within the organization:
     - **Owner** - Organization creator (full control)
     - **Manager** - Can manage all groups and members

3. **Automatic Permission Checks**
   - System automatically verifies user can manage the organization
   - Returns `403 Forbidden` if user lacks permissions
   - Returns `404 Not Found` if organization doesn't exist

### Who Can Import?

✅ **Can Import:**
- Organization owners
- Organization managers
- System administrators (through admin panel)

❌ **Cannot Import:**
- Regular organization members (role: "member")
- Users not in the organization
- Unauthenticated users

---

## Frontend Implementation Prompt

### Overview

Create a bulk import interface for organizations that allows authorized users (owners/managers) to upload CSV files containing users, groups, and memberships. The interface should provide validation, progress tracking, error reporting, and support for recurring imports.

### User Interface Requirements

#### 1. **Import Page/Modal Layout**

```
┌─────────────────────────────────────────────────────────┐
│  Bulk Import Users & Groups                              │
│  Organization: {Organization Name}                       │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  📥 Step 1: Upload CSV Files                             │
│  ┌──────────────────────────────────────────┐           │
│  │ 📄 Users CSV (Required)                  │           │
│  │ [Choose File] users.csv                   │ [Preview]│
│  │ 9 users found                             │           │
│  └──────────────────────────────────────────┘           │
│                                                           │
│  ┌──────────────────────────────────────────┐           │
│  │ 📄 Groups CSV (Optional)                 │           │
│  │ [Choose File] groups.csv                  │ [Preview]│
│  │ 9 groups found (3 nested)                 │           │
│  └──────────────────────────────────────────┘           │
│                                                           │
│  ┌──────────────────────────────────────────┐           │
│  │ 📄 Memberships CSV (Optional)            │           │
│  │ [Choose File] memberships.csv             │ [Preview]│
│  │ 15 memberships found                      │           │
│  └──────────────────────────────────────────┘           │
│                                                           │
│  ⚙️ Step 2: Import Options                               │
│  ☐ Dry Run (validate only, don't import)                │
│  ☐ Update existing users if found                       │
│                                                           │
│  💡 Tips:                                                │
│  • Always test with dry run first                        │
│  • Check organization limits before importing            │
│  • Download example CSV files if needed                  │
│                                                           │
│  [Download Examples] [Cancel] [Validate & Import]       │
└─────────────────────────────────────────────────────────┘
```

#### 2. **File Upload Component**

**Features:**
- Drag & drop support for CSV files
- File validation (must be .csv)
- Client-side CSV parsing to show row counts
- Preview modal showing first 5-10 rows
- Clear/replace file functionality

**Validation Messages:**
```javascript
const validationStates = {
  empty: "No file selected",
  invalid: "File must be a CSV (.csv)",
  tooLarge: "File exceeds 10MB limit",
  valid: "✓ {rowCount} rows detected",
  parsing: "Parsing CSV...",
  error: "Failed to parse CSV"
}
```

#### 3. **Preview Modal**

Show first 10 rows of CSV with proper column headers:

```
┌─────────────────────────────────────────────────┐
│  Preview: users.csv                             │
├─────────────────────────────────────────────────┤
│  Email              | First Name | Last Name ... │
│  john.doe@sch...    | John       | Doe       ... │
│  jane.smith@sch...  | Jane       | Smith     ... │
│  ...                                              │
│                                                   │
│  Total: 9 rows                                   │
│  [Close]                                         │
└─────────────────────────────────────────────────┘
```

#### 4. **Validation Results Screen**

After clicking "Validate & Import" with dry_run=true:

```
┌─────────────────────────────────────────────────────────┐
│  ✓ Validation Complete                                   │
├─────────────────────────────────────────────────────────┤
│  Summary:                                                │
│  • 9 users will be created                               │
│  • 0 users will be skipped                               │
│  • 9 groups will be created                              │
│  • 15 memberships will be created                        │
│                                                           │
│  ⚠️ Warnings: 2                                          │
│  • Row 5 (users): User john@example.com already exists   │
│  • Row 8 (groups): Group "M2_cloud_c" exceeds capacity   │
│                                                           │
│  ❌ Errors: 0                                             │
│                                                           │
│  Organization Limits:                                    │
│  • Members: 18/50 → 27/50 after import ✓                │
│  • Groups: 5/20 → 14/20 after import ✓                  │
│                                                           │
│  [Back] [Proceed with Import]                            │
└─────────────────────────────────────────────────────────┘
```

#### 5. **Import Progress Screen**

During actual import (dry_run=false):

```
┌─────────────────────────────────────────────────────────┐
│  Importing...                                            │
├─────────────────────────────────────────────────────────┤
│  [████████████████░░░░░░░░░░] 65%                       │
│                                                           │
│  Processing users.csv...                                 │
│  • Created: 6/9                                          │
│  • Skipped: 1                                            │
│  • Errors: 0                                             │
│                                                           │
│  Elapsed: 12s                                            │
│                                                           │
│  Please wait, do not close this window.                  │
└─────────────────────────────────────────────────────────┘
```

#### 6. **Success Screen**

```
┌─────────────────────────────────────────────────────────┐
│  ✅ Import Complete!                                     │
├─────────────────────────────────────────────────────────┤
│  Successfully imported data in 24.5s                     │
│                                                           │
│  Results:                                                │
│  • Users created: 9                                      │
│  • Users updated: 0                                      │
│  • Groups created: 9                                     │
│  • Memberships created: 15                               │
│                                                           │
│  ⚠️ Warnings: 2 (view details)                           │
│  ❌ Errors: 0                                             │
│                                                           │
│  [View Organization] [Import More] [Close]               │
└─────────────────────────────────────────────────────────┘
```

#### 7. **Error Screen**

```
┌─────────────────────────────────────────────────────────┐
│  ❌ Import Failed                                        │
├─────────────────────────────────────────────────────────┤
│  Found 3 critical errors. No data was imported.          │
│                                                           │
│  Errors:                                                 │
│  1. Row 3 (users): Invalid email format                  │
│     Field: email                                          │
│     Value: "not-an-email"                                │
│                                                           │
│  2. Row 7 (users): Missing required field                │
│     Field: password                                       │
│                                                           │
│  3. Row 12 (groups): Parent group not found              │
│     Field: parent_group                                   │
│     Value: "non_existent_group"                          │
│                                                           │
│  [Download Error Report] [Fix & Retry] [Close]           │
└─────────────────────────────────────────────────────────┘
```

---

## API Integration

### Authentication

```typescript
// Get auth token from your auth system
const token = localStorage.getItem('auth_token') || sessionStorage.getItem('auth_token');

// Include in all requests
const headers = {
  'Authorization': `Bearer ${token}`
};
```

### API Endpoints

#### 1. **Dry Run Validation**

```typescript
async function validateImport(
  organizationId: string,
  usersFile: File,
  groupsFile?: File,
  membershipsFile?: File
) {
  const formData = new FormData();
  formData.append('users', usersFile);
  if (groupsFile) formData.append('groups', groupsFile);
  if (membershipsFile) formData.append('memberships', membershipsFile);
  formData.append('dry_run', 'true');
  formData.append('update_existing', 'false');

  const response = await fetch(
    `${API_BASE_URL}/api/v1/organizations/${organizationId}/import`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`
      },
      body: formData
    }
  );

  return await response.json();
}
```

#### 2. **Actual Import**

```typescript
async function performImport(
  organizationId: string,
  usersFile: File,
  groupsFile?: File,
  membershipsFile?: File,
  updateExisting: boolean = false
) {
  const formData = new FormData();
  formData.append('users', usersFile);
  if (groupsFile) formData.append('groups', groupsFile);
  if (membershipsFile) formData.append('memberships', membershipsFile);
  formData.append('dry_run', 'false');
  formData.append('update_existing', updateExisting.toString());

  const response = await fetch(
    `${API_BASE_URL}/api/v1/organizations/${organizationId}/import`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`
      },
      body: formData
    }
  );

  return await response.json();
}
```

### Response Format

```typescript
interface ImportResponse {
  success: boolean;
  dry_run: boolean;
  summary: {
    users_created: number;
    users_updated: number;
    users_skipped: number;
    groups_created: number;
    groups_updated: number;
    groups_skipped: number;
    memberships_created: number;
    memberships_skipped: number;
    total_processed: number;
    processing_time: string;
  };
  errors: ImportError[];
  warnings: ImportWarning[];
}

interface ImportError {
  row: number;
  file: string;  // "users", "groups", "memberships"
  field?: string;
  message: string;
  code: string;  // Error code constant
}

interface ImportWarning {
  row: number;
  file: string;
  message: string;
}
```

### Error Codes

```typescript
const ERROR_CODES = {
  VALIDATION_ERROR: 'Field validation failed',
  DUPLICATE: 'Duplicate entry found',
  LIMIT_EXCEEDED: 'Organization limit exceeded',
  NOT_FOUND: 'Referenced entity not found',
  INVALID_ROLE: 'Invalid role specified',
  INVALID_EMAIL: 'Invalid email format',
  INVALID_DATE: 'Invalid date format',
  CIRCULAR_REFERENCE: 'Circular parent-child reference',
  ORPHANED_GROUP: 'Group references non-existent parent'
};
```

---

## Component Structure (React Example)

```
src/
├── features/
│   └── organization/
│       └── import/
│           ├── BulkImportModal.tsx        # Main modal wrapper
│           ├── FileUploadStep.tsx          # Step 1: File uploads
│           ├── CSVFileUpload.tsx           # Reusable CSV uploader
│           ├── CSVPreviewModal.tsx         # CSV preview
│           ├── ValidationResults.tsx       # Dry-run results
│           ├── ImportProgress.tsx          # Progress indicator
│           ├── ImportSuccess.tsx           # Success screen
│           ├── ImportErrors.tsx            # Error display
│           ├── hooks/
│           │   ├── useCSVParser.ts         # Client-side CSV parsing
│           │   ├── useBulkImport.ts        # API integration
│           │   └── useImportValidation.ts  # Validation logic
│           ├── utils/
│           │   ├── csvValidator.ts         # CSV format validation
│           │   └── errorFormatter.ts       # Error message formatting
│           └── types.ts                    # TypeScript interfaces
```

---

## State Management

```typescript
interface ImportState {
  // Files
  usersFile: File | null;
  groupsFile: File | null;
  membershipsFile: File | null;

  // Options
  dryRun: boolean;
  updateExisting: boolean;

  // UI State
  step: 'upload' | 'validating' | 'validation-results' | 'importing' | 'success' | 'error';
  isUploading: boolean;
  isImporting: boolean;

  // Results
  validationResults: ImportResponse | null;
  importResults: ImportResponse | null;

  // Errors
  uploadErrors: Record<string, string>;
}
```

---

## User Experience Guidelines

### 1. **Progressive Disclosure**

- Start with simple file upload
- Show options only when files are selected
- Guide users through dry-run before actual import

### 2. **Fail-Fast Validation**

- Validate file format on selection (client-side)
- Parse CSV headers to check column names
- Show row counts immediately
- Run server-side validation before allowing import

### 3. **Clear Feedback**

- Use color coding: green (success), yellow (warning), red (error)
- Show specific row numbers for errors
- Provide actionable error messages
- Explain what will happen before it happens

### 4. **Safety Mechanisms**

- Always default to dry_run=true
- Require explicit confirmation for actual import
- Show summary of changes before committing
- Prevent closing during import
- Provide rollback information if needed

### 5. **Error Recovery**

- Allow editing/replacing files after validation errors
- Provide downloadable error report (CSV with error column)
- Suggest fixes for common errors
- Allow partial retry (failed rows only)

### 6. **Performance Considerations**

- Show progress for large imports (>100 rows)
- Support file size up to 10MB
- Timeout after 5 minutes with clear message
- Chunk large uploads if needed

---

## Example CSV Templates

Provide downloadable example CSV files:

```typescript
const EXAMPLE_CSVS = {
  users: {
    filename: 'users_template.csv',
    content: `email,first_name,last_name,password,role,external_id,force_reset,update_existing
john.doe@school.fr,John,Doe,TempPass123!,member,student_001,true,false
jane.smith@school.fr,Jane,Smith,TempPass456!,supervisor,teacher_001,true,false`
  },
  groups: {
    filename: 'groups_template.csv',
    content: `group_name,display_name,description,parent_group,max_members,expires_at,external_id
m1_devops,M1 DevOps,Master 1 DevOps - All classes,,150,,dept_devops
m1_devops_a,M1 DevOps A,Master 1 DevOps - Class A,m1_devops,50,2026-06-30T23:59:59Z,class_a`
  },
  memberships: {
    filename: 'memberships_template.csv',
    content: `user_email,group_name,role
john.doe@school.fr,m1_devops_a,member
jane.smith@school.fr,m1_devops_a,admin`
  }
};

function downloadTemplate(type: keyof typeof EXAMPLE_CSVS) {
  const template = EXAMPLE_CSVS[type];
  const blob = new Blob([template.content], { type: 'text/csv' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = template.filename;
  a.click();
  URL.revokeObjectURL(url);
}
```

---

## Recurring Import Support

For schools importing updates each semester:

```typescript
interface RecurringImportSettings {
  // Remember last import settings
  updateExisting: boolean;
  lastImportDate: Date;
  importHistory: {
    date: Date;
    usersCreated: number;
    groupsCreated: number;
    errors: number;
  }[];
}

// Show helpful messages for recurring imports
function getRecurringImportHelp(lastImport: Date) {
  const daysSince = Math.floor((Date.now() - lastImport.getTime()) / (1000 * 60 * 60 * 24));

  if (daysSince < 30) {
    return "Recent import detected. Consider enabling 'Update existing users' to sync changes.";
  } else if (daysSince < 180) {
    return "New semester? Review organization limits before importing new students.";
  }

  return null;
}
```

---

## Testing Checklist

Frontend QA must verify:

- [ ] Only organization owners/managers can access import feature
- [ ] Non-authorized users see proper permission error
- [ ] File upload accepts only .csv files
- [ ] CSV preview shows correct data
- [ ] Dry run validation shows accurate results
- [ ] Errors display with row numbers and fields
- [ ] Progress indicator updates during import
- [ ] Success screen shows correct counts
- [ ] Browser back button doesn't interrupt import
- [ ] Page refresh warning during import
- [ ] Error messages are user-friendly
- [ ] Download example templates works
- [ ] Large files (1000+ rows) handle well
- [ ] Network errors show helpful messages
- [ ] Token expiration handled gracefully

---

## Accessibility Requirements

- Use semantic HTML (form, fieldset, legend)
- Provide clear labels for file inputs
- Use aria-live regions for status updates
- Support keyboard navigation throughout
- Ensure color is not the only indicator of status
- Provide text alternatives for icons
- Make modals focusable and escapable (ESC key)
- Announce progress updates to screen readers

---

## Mobile Considerations

- File upload via device file picker
- Simplified layout for small screens
- Preview modal scrolls horizontally for wide tables
- Long error messages truncate with "show more"
- Prevent accidental navigation during import

---

## Integration with Existing UI

### Add Import Button to Organization Page

```tsx
// In OrganizationDashboard.tsx
<PageHeader>
  <h1>{organization.name}</h1>
  <div className="actions">
    <Button variant="secondary" onClick={handleAddMember}>
      Add Member
    </Button>
    <Button variant="primary" onClick={handleBulkImport}>
      Bulk Import
    </Button>
  </div>
</PageHeader>
```

### Permission Check Before Showing Button

```typescript
function canImport(organization: Organization, currentUser: User): boolean {
  const membership = organization.members.find(m => m.user_id === currentUser.id);

  if (!membership) return false;

  return membership.role === 'owner' || membership.role === 'manager';
}

// In component
{canImport(organization, currentUser) && (
  <Button onClick={handleBulkImport}>
    Bulk Import
  </Button>
)}
```

---

## Documentation Links

Provide in-app help links to:
- CSV format documentation: `/docs/import-format`
- Example files download: Built-in to the UI
- Hyperplanning/Pronote conversion guide: `/docs/school-systems-integration`
- Video tutorial: Link to video walkthrough
- Support: `/support` with import-specific category

---

## Success Metrics

Track these analytics:
- Number of imports per organization
- Average import size (rows)
- Success rate (imports with 0 errors)
- Most common error codes
- Time to complete import
- Dry run usage rate (should be high)
- Error recovery rate (users who fix and retry)

---

## Future Enhancements

Consider for v2:
- Scheduled imports (cron-style)
- API integration with Hyperplanning/Pronote
- Automatic conflict resolution suggestions
- Import templates saved per organization
- Batch operations (import to multiple orgs)
- Undo last import feature
- Import from Excel (.xlsx) files
- Column mapping tool for custom CSV formats
