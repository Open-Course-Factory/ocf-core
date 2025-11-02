# Frontend Integration Guide

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
# Course Version Management - Frontend Integration Guide

## Overview

The OCF Core API now supports full version management for courses. This allows users to:
- Import/update courses via CLI with automatic version detection
- Query all versions of a course
- Retrieve a specific version
- Delete/purge specific versions

This document explains how the frontend should integrate these features into the course details UI.

---

## Backend Behavior Summary

### CLI Import Behavior

When importing a course via CLI, the system uses the `version` field from `course.json`:

```bash
go run main.go -c mycourse --course-repo=git@github.com:org/repo.git --user-id=USER_ID
```

**Version matching logic:**
- Matches by: `owner_id` + `course_name` + `version`
- **Same version** → Updates existing course (preserves course ID)
- **New version** → Creates new course record (new course ID)
- **Different user** → Separate courses (no conflict)

**Example:**
```json
// First import with course.json containing "version": "1.0"
→ Creates course "mycourse" v1.0 (ID: abc-123)

// Reimport with same version
→ Updates course "mycourse" v1.0 (ID: abc-123) ✓ Same ID

// Import with "version": "2.0" in course.json
→ Creates NEW course "mycourse" v2.0 (ID: def-456)

// Result: User has both v1.0 and v2.0 available
```

---

## API Endpoints

### 1. List All Versions of a Course

**Endpoint:** `GET /api/v1/courses/versions`

**Query Parameters:**
- `name` (required): Course name

**Headers:**
- `Authorization: Bearer {token}`

**Response:**
```json
[
  {
    "id": "0199b9df-ae56-7b83-bc2e-1ea8f2cbcfb9",
    "name": "GIT",
    "version": "v3.0",
    "title": "Git, mise en oeuvre",
    "subtitle": "Coder à plusieurs, mais pas que !",
    "description": "",
    "created_at": "2025-01-15T10:30:00Z",
    "updated_at": "2025-01-15T10:30:00Z",
    "chapters": null
  },
  {
    "id": "0199b9cd-9c61-7d1c-8d88-474786b87914",
    "name": "GIT",
    "version": "v2.0",
    "title": "Git basics",
    "subtitle": "Version control fundamentals",
    "description": "",
    "created_at": "2024-12-01T08:15:00Z",
    "updated_at": "2024-12-01T08:15:00Z",
    "chapters": null
  }
]
```

**Notes:**
- Results are ordered by version descending (newest first)
- Returns empty array `[]` if no courses found
- Only returns courses the user has access to

---

### 2. Get a Specific Course Version

**Endpoint:** `GET /api/v1/courses/by-version`

**Query Parameters:**
- `name` (required): Course name
- `version` (required): Course version

**Headers:**
- `Authorization: Bearer {token}`

**Response:**
```json
{
  "id": "0199b9df-ae56-7b83-bc2e-1ea8f2cbcfb9",
  "name": "GIT",
  "version": "v3.0",
  "title": "Git, mise en oeuvre",
  "subtitle": "Coder à plusieurs, mais pas que !",
  "header": "Git, mise en œuvre",
  "footer": "2024 - Git v3.0 - Author Name - email@example.com",
  "description": "",
  "learning_objectives": "",
  "chapters": [...],
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-15T10:30:00Z"
}
```

**Error Response (404):**
```json
{
  "error": "Course version not found",
  "details": "course 'GIT' version 'v1.0' not found"
}
```

---

### 3. Delete a Specific Course Version

**Endpoint:** `DELETE /api/v1/courses/{courseId}`

**Headers:**
- `Authorization: Bearer {token}`

**Response (204):**
```
No Content
```

**Error Response (404):**
```json
{
  "error": "Course not found"
}
```

**Notes:**
- This is the standard entity management DELETE endpoint
- Permanently deletes the course version
- Use with caution - this operation cannot be undone

---

## Frontend UI Requirements

### Course Details Page

When displaying a course, the frontend should:

#### 1. **Version Selector Component**

Show a dropdown/select component that:
- Displays all available versions of the course
- Shows version number and last updated date
- Allows switching between versions
- Highlights the currently selected version

**Example UI:**
```
┌─────────────────────────────────────────┐
│ Course: Git, mise en oeuvre             │
│                                          │
│ Version: [ v3.0 ▼ ]  ← Dropdown         │
│          • v3.0 (Latest - Jan 15, 2025) │
│          • v2.0 (Dec 1, 2024)           │
│          • v1.0 (Nov 5, 2024)           │
└─────────────────────────────────────────┘
```

**Implementation:**
```javascript
// Fetch all versions when loading course details
const fetchCourseVersions = async (courseName) => {
  const response = await fetch(
    `/api/v1/courses/versions?name=${encodeURIComponent(courseName)}`,
    {
      headers: {
        'Authorization': `Bearer ${token}`,
      },
    }
  );

  if (!response.ok) {
    throw new Error('Failed to fetch course versions');
  }

  return await response.json();
};

// Usage
const versions = await fetchCourseVersions('GIT');
```

#### 2. **Load Specific Version on Selection**

When user selects a different version from the dropdown:

```javascript
const loadCourseVersion = async (courseName, version) => {
  const response = await fetch(
    `/api/v1/courses/by-version?name=${encodeURIComponent(courseName)}&version=${encodeURIComponent(version)}`,
    {
      headers: {
        'Authorization': `Bearer ${token}`,
      },
    }
  );

  if (!response.ok) {
    throw new Error('Failed to load course version');
  }

  const courseData = await response.json();
  // Update UI with new course data
  updateCourseDisplay(courseData);
};
```

#### 3. **Version Management Actions**

Add action buttons for each version:

**Example UI:**
```
┌─────────────────────────────────────────────────┐
│ Version: v3.0                                   │
│ ┌─────────────────────────────────────────────┐ │
│ │ • v3.0 (Latest)         [View] [Delete]     │ │
│ │ • v2.0                  [View] [Delete]     │ │
│ │ • v1.0                  [View] [Delete]     │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

**Delete Version Implementation:**
```javascript
const deleteCourseVersion = async (courseId, courseName, version) => {
  // Confirmation dialog
  const confirmed = await showConfirmDialog(
    `Are you sure you want to delete ${courseName} v${version}?`,
    'This action cannot be undone.'
  );

  if (!confirmed) return;

  const response = await fetch(
    `/api/v1/courses/${courseId}`,
    {
      method: 'DELETE',
      headers: {
        'Authorization': `Bearer ${token}`,
      },
    }
  );

  if (!response.ok) {
    throw new Error('Failed to delete course version');
  }

  // Refresh the version list
  await refreshCourseVersions();

  // If deleted version was currently selected, switch to latest
  if (currentVersion === version) {
    await loadLatestVersion();
  }
};
```

#### 4. **Version Badges/Tags**

Display version information prominently:

```
┌─────────────────────────────────────────┐
│ Git, mise en oeuvre                     │
│ [v3.0] [LATEST] [Updated: Jan 15, 2025] │
└─────────────────────────────────────────┘
```

**Badge logic:**
- `[LATEST]` - Show on the highest version number
- `[UPDATED: date]` - Show last update timestamp
- `[IMPORTED: date]` - Show creation date for first import

---

## Recommended User Flows

### Flow 1: Viewing Course Versions

1. User navigates to course details page
2. Frontend fetches all versions: `GET /api/v1/courses/versions?name=CourseX`
3. Display version dropdown with all available versions
4. Load the latest version by default
5. User can switch versions via dropdown

### Flow 2: Deleting Old Versions

1. User opens version management panel
2. List shows all versions with "Delete" buttons
3. User clicks "Delete" on v1.0
4. Confirmation dialog appears
5. Upon confirmation: `DELETE /api/v1/courses/{courseId}`
6. Refresh version list
7. If current version was deleted, auto-switch to latest

### Flow 3: Comparing Versions (Future Enhancement)

1. User selects two versions from checkboxes
2. Click "Compare" button
3. Show side-by-side comparison of course metadata
4. Highlight differences in chapters, sections, content

---

## UI/UX Best Practices

### Version Display

1. **Always show version in course title**
   ```
   Git, mise en oeuvre (v3.0)
   ```

2. **Use semantic versioning when possible**
   - Major: Breaking changes (v1.0 → v2.0)
   - Minor: New features (v2.0 → v2.1)
   - Patch: Bug fixes (v2.1.0 → v2.1.1)

3. **Visual differentiation**
   - Latest version: Green badge
   - Older versions: Gray badge
   - Deprecated versions: Red badge (if you implement deprecation)

### Deletion Warnings

When deleting a version, warn the user if:
- It's the only version (suggest keeping at least one)
- There are active generations using this version
- Other users have access to this version (if sharing is enabled)

**Example warning:**
```
⚠️ Warning: This is the last version of this course.
Deleting it will remove the course entirely.

Are you sure you want to continue?
[Cancel] [Delete Anyway]
```

---

## Error Handling

### Common Errors

| Error | Status | Handling |
|-------|--------|----------|
| Course not found | 404 | Show "Course not found" message |
| Version not found | 404 | Show "Version not found, loading latest..." |
| Unauthorized | 401 | Redirect to login |
| Forbidden | 403 | Show "You don't have access to this course" |
| Server error | 500 | Show retry button with error message |

### Frontend Error Handling Example

```javascript
const handleCourseVersionError = (error, courseName, version) => {
  if (error.status === 404) {
    showNotification(
      `Version ${version} of ${courseName} not found. Loading latest version...`,
      'warning'
    );
    loadLatestVersion(courseName);
  } else if (error.status === 403) {
    showNotification(
      `You don't have permission to access this course.`,
      'error'
    );
    redirectToCourseCatalog();
  } else {
    showNotification(
      `Failed to load course version: ${error.message}`,
      'error'
    );
  }
};
```

---

## Testing Checklist

### Frontend Tests

- [ ] Display all versions in dropdown
- [ ] Load specific version on selection
- [ ] Show "Latest" badge on newest version
- [ ] Delete version with confirmation
- [ ] Refresh list after deletion
- [ ] Switch to latest when current version deleted
- [ ] Handle 404 errors gracefully
- [ ] Handle permission errors (403)
- [ ] Show loading states during API calls
- [ ] Empty state when no versions exist

### Integration Tests

- [ ] Create multiple versions via CLI
- [ ] Verify all versions appear in frontend
- [ ] Delete a version and verify removal
- [ ] Update a version and verify changes
- [ ] Test with multiple users (no cross-contamination)

---

## Example Complete Implementation

### React Component Example

```jsx
import React, { useState, useEffect } from 'react';

const CourseVersionManager = ({ courseName, initialVersion }) => {
  const [versions, setVersions] = useState([]);
  const [selectedVersion, setSelectedVersion] = useState(initialVersion);
  const [courseData, setCourseData] = useState(null);
  const [loading, setLoading] = useState(false);

  // Fetch all versions on mount
  useEffect(() => {
    fetchVersions();
  }, [courseName]);

  // Load selected version when it changes
  useEffect(() => {
    if (selectedVersion) {
      loadVersion(selectedVersion);
    }
  }, [selectedVersion]);

  const fetchVersions = async () => {
    try {
      const response = await fetch(
        `/api/v1/courses/versions?name=${encodeURIComponent(courseName)}`,
        {
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('token')}`,
          },
        }
      );

      if (!response.ok) throw new Error('Failed to fetch versions');

      const data = await response.json();
      setVersions(data);

      // Set latest version as default if none selected
      if (!selectedVersion && data.length > 0) {
        setSelectedVersion(data[0].version);
      }
    } catch (error) {
      console.error('Error fetching versions:', error);
    }
  };

  const loadVersion = async (version) => {
    setLoading(true);
    try {
      const response = await fetch(
        `/api/v1/courses/by-version?name=${encodeURIComponent(courseName)}&version=${encodeURIComponent(version)}`,
        {
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('token')}`,
          },
        }
      );

      if (!response.ok) throw new Error('Failed to load version');

      const data = await response.json();
      setCourseData(data);
    } catch (error) {
      console.error('Error loading version:', error);
    } finally {
      setLoading(false);
    }
  };

  const deleteVersion = async (courseId, version) => {
    if (!confirm(`Delete ${courseName} ${version}?`)) return;

    try {
      const response = await fetch(
        `/api/v1/courses/${courseId}`,
        {
          method: 'DELETE',
          headers: {
            'Authorization': `Bearer ${localStorage.getItem('token')}`,
          },
        }
      );

      if (!response.ok) throw new Error('Failed to delete version');

      // Refresh versions
      await fetchVersions();

      // Switch to latest if deleted current version
      if (selectedVersion === version) {
        const remainingVersions = versions.filter(v => v.version !== version);
        if (remainingVersions.length > 0) {
          setSelectedVersion(remainingVersions[0].version);
        }
      }
    } catch (error) {
      console.error('Error deleting version:', error);
    }
  };

  return (
    <div className="course-version-manager">
      <div className="version-selector">
        <label>Version:</label>
        <select
          value={selectedVersion}
          onChange={(e) => setSelectedVersion(e.target.value)}
        >
          {versions.map((v) => (
            <option key={v.id} value={v.version}>
              {v.version} {v === versions[0] ? '(Latest)' : ''}
            </option>
          ))}
        </select>
      </div>

      {loading ? (
        <div>Loading...</div>
      ) : courseData ? (
        <div className="course-details">
          <h1>{courseData.title} <span className="version-badge">{courseData.version}</span></h1>
          <p>{courseData.subtitle}</p>
          {/* Render course content */}
        </div>
      ) : null}

      <div className="version-list">
        <h3>All Versions</h3>
        {versions.map((v) => (
          <div key={v.id} className="version-item">
            <span>{v.version}</span>
            <span>{new Date(v.updated_at).toLocaleDateString()}</span>
            <button onClick={() => setSelectedVersion(v.version)}>View</button>
            <button onClick={() => deleteVersion(v.id, v.version)}>Delete</button>
          </div>
        ))}
      </div>
    </div>
  );
};

export default CourseVersionManager;
```

---

## Summary

### Key Points

1. **Version matching:** `owner_id` + `course_name` + `version`
2. **Three main endpoints:**
   - `GET /api/v1/courses/versions?name=X` - List all versions
   - `GET /api/v1/courses/by-version?name=X&version=Y` - Get specific version
   - `DELETE /api/v1/courses/{id}` - Delete version
3. **Frontend must:**
   - Show version selector on course details
   - Allow switching between versions
   - Provide delete functionality with confirmation
   - Handle errors gracefully

### Next Steps

1. Implement version selector component
2. Add version management panel to course details
3. Test with multiple course versions
4. Add user feedback (toasts, confirmations)
5. Consider future enhancements (version comparison, rollback, etc.)

---

## Support

For API issues or questions:
- Check Swagger docs: `http://localhost:8080/swagger/`
- Backend logs: `/tmp/server.log`
- Test endpoints with the scripts in `/tmp/test_versions.sh`
# Frontend Integration Prompt - Feature Flags System

## Context

The backend now has a **modular feature flag system** that allows enabling/disabling features (courses, labs, terminals) globally. This affects what users see in their subscription dashboard and what features are available.

## API Endpoints

### Get All Feature Flags
```http
GET /api/v1/features
```

**Response:**
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "key": "course_conception",
    "name": "Course Generation",
    "description": "Enable/disable course generation and management features including Marp and Slidev engines",
    "enabled": true,
    "category": "modules",
    "module": "courses",
    "created_at": "2025-01-10T12:00:00Z",
    "updated_at": "2025-01-10T12:00:00Z"
  },
  {
    "id": "660e8400-e29b-41d4-a716-446655440000",
    "key": "labs",
    "name": "Lab Sessions",
    "description": "Enable/disable lab environment and session management",
    "enabled": true,
    "category": "modules",
    "module": "labs",
    "created_at": "2025-01-10T12:00:00Z",
    "updated_at": "2025-01-10T12:00:00Z"
  },
  {
    "id": "770e8400-e29b-41d4-a716-446655440000",
    "key": "terminals",
    "name": "Terminal Trainer",
    "description": "Enable/disable interactive terminal training sessions",
    "enabled": true,
    "category": "modules",
    "module": "terminals",
    "created_at": "2025-01-10T12:00:00Z",
    "updated_at": "2025-01-10T12:00:00Z"
  }
]
```

### Get Single Feature
```http
GET /api/v1/features/{id}
```

### Update Feature (Toggle Enable/Disable)
```http
PATCH /api/v1/features/{id}
Content-Type: application/json

{
  "enabled": false
}
```

**Response:** Same as GET single feature

### Sync User Metrics (After Toggling)
```http
POST /api/v1/user-subscriptions/sync-usage-limits
```

This removes/creates usage metrics based on new feature states.

## User-Facing Frontend Changes

### 1. Subscription Dashboard - Hide Disabled Feature Limits

**Current behavior:** Shows all limits (courses, labs, terminals) regardless of global feature state

**Required change:** Only show limits for **enabled** features

**Implementation:**

```javascript
// Example: Vue/React component
async function loadSubscriptionLimits() {
  // 1. Fetch user's subscription
  const subscription = await fetch('/api/v1/user-subscriptions/current')
    .then(r => r.json())

  // 2. Fetch enabled features
  const features = await fetch('/api/v1/features')
    .then(r => r.json())

  // 3. Create a map of enabled features
  const enabledFeatures = features
    .filter(f => f.enabled)
    .reduce((acc, f) => {
      acc[f.key] = true
      return acc
    }, {})

  // 4. Filter subscription limits based on enabled features
  const visibleLimits = []

  if (enabledFeatures['course_conception']) {
    visibleLimits.push({
      name: 'Courses',
      current: subscription.courses_used,
      limit: subscription.max_courses,
      icon: '📚'
    })
  }

  if (enabledFeatures['labs']) {
    visibleLimits.push({
      name: 'Lab Sessions',
      current: subscription.labs_used,
      limit: subscription.max_lab_sessions,
      icon: '🧪'
    })
  }

  if (enabledFeatures['terminals']) {
    visibleLimits.push({
      name: 'Concurrent Terminals',
      current: subscription.terminals_active,
      limit: subscription.max_concurrent_terminals,
      icon: '💻'
    })
  }

  return visibleLimits
}
```

**UI Example:**

Before (all features shown):
```
Your Subscription - Pro Plan
─────────────────────────────
📚 Courses: 3 / 10
🧪 Lab Sessions: 5 / 20
💻 Terminals: 2 / 5
```

After (only enabled features shown, e.g., courses disabled):
```
Your Subscription - Pro Plan
─────────────────────────────
🧪 Lab Sessions: 5 / 20
💻 Terminals: 2 / 5
```

### 2. Navigation Menu - Hide Disabled Features

**Required change:** Hide menu items for disabled features

```javascript
async function buildNavigationMenu() {
  const features = await fetch('/api/v1/features').then(r => r.json())

  const enabledFeatures = features
    .filter(f => f.enabled)
    .reduce((acc, f) => ({ ...acc, [f.key]: true }), {})

  const menuItems = []

  if (enabledFeatures['course_conception']) {
    menuItems.push({
      label: 'Courses',
      icon: '📚',
      path: '/courses',
      children: [
        { label: 'My Courses', path: '/courses/mine' },
        { label: 'Create Course', path: '/courses/create' }
      ]
    })
  }

  if (enabledFeatures['labs']) {
    menuItems.push({
      label: 'Labs',
      icon: '🧪',
      path: '/labs'
    })
  }

  if (enabledFeatures['terminals']) {
    menuItems.push({
      label: 'Terminals',
      icon: '💻',
      path: '/terminals'
    })
  }

  return menuItems
}
```

### 3. Feature Availability Check (for Deep Links)

**Problem:** Users might have bookmarked disabled features

**Solution:** Check feature availability before rendering page

```javascript
// Example: Vue Router Guard
router.beforeEach(async (to, from, next) => {
  // Check if route requires a feature
  const requiredFeature = to.meta.requiredFeature // e.g., 'course_conception'

  if (requiredFeature) {
    const features = await fetch('/api/v1/features').then(r => r.json())
    const feature = features.find(f => f.key === requiredFeature)

    if (!feature || !feature.enabled) {
      // Feature disabled, redirect to home with message
      next({
        path: '/',
        query: {
          message: `Feature "${feature?.name || requiredFeature}" is currently unavailable`
        }
      })
      return
    }
  }

  next()
})
```

### 4. Caching Strategy

**Recommended:** Cache features in localStorage/sessionStorage with TTL

```javascript
class FeatureCache {
  static CACHE_KEY = 'feature_flags'
  static CACHE_TTL = 5 * 60 * 1000 // 5 minutes

  static async getFeatures() {
    const cached = localStorage.getItem(this.CACHE_KEY)

    if (cached) {
      const { features, timestamp } = JSON.parse(cached)
      const age = Date.now() - timestamp

      if (age < this.CACHE_TTL) {
        return features
      }
    }

    // Cache miss or expired, fetch fresh
    const features = await fetch('/api/v1/features').then(r => r.json())

    localStorage.setItem(this.CACHE_KEY, JSON.stringify({
      features,
      timestamp: Date.now()
    }))

    return features
  }

  static clearCache() {
    localStorage.removeItem(this.CACHE_KEY)
  }

  static isFeatureEnabled(key) {
    const cached = localStorage.getItem(this.CACHE_KEY)
    if (!cached) return null

    const { features } = JSON.parse(cached)
    const feature = features.find(f => f.key === key)
    return feature?.enabled ?? null
  }
}
```

### 5. Real-Time Updates (Optional)

**For admin dashboard:** Notify users when features are toggled

```javascript
// Using WebSocket or SSE
const eventSource = new EventSource('/api/v1/events')

eventSource.addEventListener('feature_updated', (event) => {
  const { key, enabled } = JSON.parse(event.data)

  // Clear cache and reload features
  FeatureCache.clearCache()

  // Show notification
  showNotification(
    `Feature "${key}" has been ${enabled ? 'enabled' : 'disabled'}.
     Please refresh the page to see changes.`
  )
})
```

## Admin Section Changes

### 1. Feature Management Page

**Create new page:** `/admin/features`

**Layout:**

```
┌─────────────────────────────────────────────────────────────┐
│  Feature Management                            [Sync Metrics]│
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  Courses Module                                               │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 📚 Course Generation                          [ON/OFF] │  │
│  │ Enable/disable course generation features              │  │
│  │ Module: courses | Category: modules                    │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                               │
│  Labs Module                                                  │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 🧪 Lab Sessions                               [ON/OFF] │  │
│  │ Enable/disable lab environment features                │  │
│  │ Module: labs | Category: modules                       │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                               │
│  Terminals Module                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 💻 Terminal Trainer                           [ON/OFF] │  │
│  │ Enable/disable terminal training sessions              │  │
│  │ Module: terminals | Category: modules                  │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

**Implementation (React example):**

```jsx
import { useState, useEffect } from 'react'

function FeatureManagementPage() {
  const [features, setFeatures] = useState([])
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)

  useEffect(() => {
    loadFeatures()
  }, [])

  async function loadFeatures() {
    setLoading(true)
    try {
      const data = await fetch('/api/v1/features').then(r => r.json())
      setFeatures(data)
    } catch (error) {
      console.error('Failed to load features:', error)
    } finally {
      setLoading(false)
    }
  }

  async function toggleFeature(featureId, currentState) {
    // Confirm before disabling
    if (currentState) {
      const confirmed = confirm(
        'Disabling this feature will hide it from all users. ' +
        'Their metrics will be removed. Continue?'
      )
      if (!confirmed) return
    }

    try {
      await fetch(`/api/v1/features/${featureId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: !currentState })
      })

      // Update local state
      setFeatures(prev => prev.map(f =>
        f.id === featureId ? { ...f, enabled: !currentState } : f
      ))

      // Show success notification
      showNotification('Feature updated successfully')
    } catch (error) {
      showNotification('Failed to update feature', 'error')
    }
  }

  async function syncMetrics() {
    setSyncing(true)
    try {
      await fetch('/api/v1/user-subscriptions/sync-usage-limits', {
        method: 'POST'
      })
      showNotification('Metrics synced successfully')
    } catch (error) {
      showNotification('Failed to sync metrics', 'error')
    } finally {
      setSyncing(false)
    }
  }

  // Group features by module
  const featuresByModule = features.reduce((acc, feature) => {
    const module = feature.module || 'other'
    if (!acc[module]) acc[module] = []
    acc[module].push(feature)
    return acc
  }, {})

  return (
    <div className="feature-management">
      <header>
        <h1>Feature Management</h1>
        <button onClick={syncMetrics} disabled={syncing}>
          {syncing ? 'Syncing...' : 'Sync User Metrics'}
        </button>
      </header>

      {loading ? (
        <div>Loading features...</div>
      ) : (
        Object.entries(featuresByModule).map(([module, moduleFeatures]) => (
          <section key={module} className="module-section">
            <h2>{module.charAt(0).toUpperCase() + module.slice(1)} Module</h2>
            {moduleFeatures.map(feature => (
              <div key={feature.id} className="feature-card">
                <div className="feature-info">
                  <h3>{feature.name}</h3>
                  <p>{feature.description}</p>
                  <div className="feature-meta">
                    <span>Module: {feature.module}</span>
                    <span>Category: {feature.category}</span>
                    <span>Key: {feature.key}</span>
                  </div>
                </div>
                <div className="feature-toggle">
                  <label className="switch">
                    <input
                      type="checkbox"
                      checked={feature.enabled}
                      onChange={() => toggleFeature(feature.id, feature.enabled)}
                    />
                    <span className="slider"></span>
                  </label>
                  <span className={feature.enabled ? 'status-on' : 'status-off'}>
                    {feature.enabled ? 'ON' : 'OFF'}
                  </span>
                </div>
              </div>
            ))}
          </section>
        ))
      )}
    </div>
  )
}
```

### 2. Feature Impact Dashboard

**Show what happens when toggling:**

```jsx
function FeatureImpactCard({ feature, affectedUsers }) {
  return (
    <div className="impact-card">
      <h4>Impact Analysis: {feature.name}</h4>
      <div className="impact-stats">
        <div className="stat">
          <span className="label">Affected Users:</span>
          <span className="value">{affectedUsers.total}</span>
        </div>
        <div className="stat">
          <span className="label">Active Usage:</span>
          <span className="value">{affectedUsers.activelyUsing}</span>
        </div>
        <div className="stat">
          <span className="label">Subscription Plans:</span>
          <span className="value">{affectedUsers.plans.length}</span>
        </div>
      </div>
      <div className="warning">
        ⚠️ Disabling this feature will:
        <ul>
          <li>Hide {feature.name} from navigation menu</li>
          <li>Remove {feature.name} limits from subscription dashboard</li>
          <li>Prevent new {feature.name} usage metrics creation</li>
          <li>Require metrics sync to take effect for existing users</li>
        </ul>
      </div>
    </div>
  )
}
```

### 3. Audit Log

**Track feature toggle history:**

```jsx
function FeatureAuditLog({ featureKey }) {
  const [logs, setLogs] = useState([])

  useEffect(() => {
    // Fetch audit logs (you may need to create this endpoint)
    fetch(`/api/v1/features/${featureKey}/audit-log`)
      .then(r => r.json())
      .then(setLogs)
  }, [featureKey])

  return (
    <div className="audit-log">
      <h3>Change History</h3>
      <table>
        <thead>
          <tr>
            <th>Date</th>
            <th>Action</th>
            <th>Changed By</th>
            <th>Old Value</th>
            <th>New Value</th>
          </tr>
        </thead>
        <tbody>
          {logs.map(log => (
            <tr key={log.id}>
              <td>{new Date(log.timestamp).toLocaleString()}</td>
              <td>{log.action}</td>
              <td>{log.user_email}</td>
              <td>{log.old_value ? 'ON' : 'OFF'}</td>
              <td>{log.new_value ? 'ON' : 'OFF'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
```

## Testing Checklist

### User Frontend
- [ ] Subscription dashboard only shows enabled features
- [ ] Navigation menu hides disabled features
- [ ] Bookmarked links to disabled features redirect with message
- [ ] Feature cache works (no excessive API calls)
- [ ] UI updates immediately after admin toggles feature (with cache clear)

### Admin Frontend
- [ ] Can view all features grouped by module
- [ ] Can toggle features ON/OFF
- [ ] Confirmation dialog before disabling
- [ ] "Sync Metrics" button works
- [ ] Success/error notifications appear
- [ ] UI shows loading states properly

## CSS Styling Suggestions

```css
/* Feature toggle switch */
.switch {
  position: relative;
  display: inline-block;
  width: 60px;
  height: 34px;
}

.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: #ccc;
  transition: 0.4s;
  border-radius: 34px;
}

.slider:before {
  position: absolute;
  content: "";
  height: 26px;
  width: 26px;
  left: 4px;
  bottom: 4px;
  background-color: white;
  transition: 0.4s;
  border-radius: 50%;
}

input:checked + .slider {
  background-color: #2196F3;
}

input:checked + .slider:before {
  transform: translateX(26px);
}

/* Feature card */
.feature-card {
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.feature-card:hover {
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}

.status-on {
  color: #4caf50;
  font-weight: bold;
}

.status-off {
  color: #f44336;
  font-weight: bold;
}
```

## Summary

**User Frontend:**
- Fetch features on load → Cache → Filter UI based on enabled features
- Hide navigation items + subscription limits for disabled features

**Admin Frontend:**
- Show all features grouped by module
- Toggle switch to enable/disable
- Sync metrics button after changes
- Show impact/warnings before disabling

**Key API calls:**
1. `GET /api/v1/features` - Get all features
2. `PATCH /api/v1/features/{id}` - Toggle feature
3. `POST /api/v1/user-subscriptions/sync-usage-limits` - Sync metrics after toggle
# Frontend Subscription System Integration Guide

**Last Updated**: 2025-10-25
**API Version**: v1
**Backend Status**: ✅ All 3 Phases Fully Implemented

---

## Table of Contents

1. [Overview](#overview)
2. [Role System (Phase 3 Simplification)](#role-system-phase-3-simplification)
3. [Authentication](#authentication)
4. [Phase 1: Individual User Subscriptions](#phase-1-individual-user-subscriptions)
5. [Phase 2: Organization Subscriptions](#phase-2-organization-subscriptions)
6. [Phase 3: Bulk License Management](#phase-3-bulk-license-management)
7. [Feature Detection & Limits](#feature-detection--limits)
8. [Stripe Integration](#stripe-integration)
9. [Error Handling](#error-handling)
10. [Complete Use Cases](#complete-use-cases)
11. [API Reference](#api-reference)

---

## Overview

The OCF Core platform supports **three distinct subscription models**:

### 1. Individual Subscriptions (Phase 1)
Users subscribe directly to a plan for personal use.
- **Use Case**: Individual learners, personal projects
- **Payment**: User pays for their own subscription
- **Features**: Assigned to individual user
- **Status**: ✅ Fully Implemented & Tested

### 2. Organization Subscriptions (Phase 2)
Organizations subscribe, all members inherit features.
- **Use Case**: Training companies, teams, schools
- **Payment**: Organization owner/manager handles billing
- **Features**: Shared across all organization members (aggregated if member of multiple orgs)
- **Status**: ✅ Fully Implemented (Backend Complete)

### 3. Bulk Licenses (Phase 3)
Purchase multiple licenses and assign them to specific users.
- **Use Case**: Corporate training, classroom management, license reselling
- **Payment**: Bulk purchaser pays upfront for all licenses
- **Features**: Assigned per license to specific users
- **Status**: ✅ Fully Implemented & Tested

---

## Role System (Phase 3 Simplification)

### Overview of Role Changes

**Phase 3 has simplified the role system from 7 roles to 2 system roles.** Business roles (trainer, manager, etc.) are now determined by organization and group membership, not system roles.

### System Roles (Only 2)

```typescript
type SystemRole = 'member' | 'administrator';
```

- **`member`**: Default role for all authenticated users
- **`administrator`**: System administrators (platform management)

### Business Roles (Context-Based)

Business capabilities are determined by **organization** and **group** membership:

#### Organization Roles
```typescript
type OrganizationRole = 'owner' | 'manager' | 'member';
```

- **`owner`**: Full organization control, manages billing
- **`manager`**: Full access to all org groups and resources
- **`member`**: Basic organization access

#### Group Roles
```typescript
type GroupRole = 'owner' | 'admin' | 'assistant' | 'member';
```

- **`owner`**: Full group control
- **`admin`**: Manages group settings and members
- **`assistant`**: Helper role (e.g., teaching assistant)
- **`member`**: Regular group member (e.g., student)

### Frontend Implementation

#### Check if User is System Administrator

```typescript
// Get current user
const getCurrentUser = async (token: string) => {
  const response = await fetch('http://localhost:8080/api/v1/users/me', {
    headers: { 'Authorization': `Bearer ${token}` }
  });
  return await response.json();
};

// Check if user is system admin
const isSystemAdmin = (user: User): boolean => {
  // Only 2 possible system roles now: 'member' or 'administrator'
  return user.roles?.some(role =>
    role.name === 'administrator' || role.name === 'admin'
  ) ?? false;
};
```

#### Check Organization Membership

```typescript
interface OrganizationMembership {
  organization_id: string;
  user_id: string;
  role: 'owner' | 'manager' | 'member';
  joined_at: string;
  is_active: boolean;
}

// Check if user can manage an organization
const canManageOrganization = (
  user: User,
  orgId: string
): boolean => {
  const membership = user.organization_memberships?.find(
    m => m.organization_id === orgId
  );
  return membership?.role === 'owner' || membership?.role === 'manager';
};

// Check if user owns an organization
const isOrganizationOwner = (
  user: User,
  orgId: string
): boolean => {
  const membership = user.organization_memberships?.find(
    m => m.organization_id === orgId
  );
  return membership?.role === 'owner';
};
```

#### Check Group Membership

```typescript
interface GroupMembership {
  group_id: string;
  user_id: string;
  role: 'owner' | 'admin' | 'assistant' | 'member';
  joined_at: string;
}

// Check if user can manage a group
const canManageGroup = (
  user: User,
  groupId: string
): boolean => {
  const membership = user.group_memberships?.find(
    m => m.group_id === groupId
  );
  return membership?.role === 'owner' || membership?.role === 'admin';
};
```

### Migrating from Old Role System

If your frontend currently checks for old roles (`member_pro`, `trainer`, `group_manager`, `organization`), here's how to migrate:

#### Old Code (Deprecated)
```typescript
// ❌ DON'T USE - Old role system
if (user.role === 'trainer' || user.role === 'organization') {
  // Show advanced features
}
```

#### New Code (Phase 3)
```typescript
// ✅ USE - Check organization membership instead
const hasAdvancedAccess = user.organization_memberships?.some(
  m => m.role === 'manager' || m.role === 'owner'
) ?? false;

if (hasAdvancedAccess) {
  // Show advanced features
}

// OR check effective features from subscriptions
const plan = await getUserEffectivePlan(token);
if (plan.can_create_advanced_labs) {
  // Show advanced features
}
```

### Feature Access Pattern

Features are now determined by **organization subscriptions**, not system roles:

```typescript
// Get user's effective features (aggregated across all organizations)
const getUserEffectivePlan = async (token: string) => {
  // Method 1: Get current user with subscriptions
  const user = await getCurrentUser(token);

  // The user object will have feature flags from their organizations
  // Backend aggregates features from all orgs user belongs to

  // Method 2: Check specific feature via API
  const canExport = await checkUserFeature(token, 'can_export_courses');

  return {
    can_create_advanced_labs: user.can_create_advanced_labs,
    can_export_courses: canExport,
    max_courses: user.max_courses,
    max_terminals: user.max_terminals
  };
};
```

### Important Notes

1. **All authenticated users are `member`** (system role)
2. **Business capabilities come from org/group membership**
3. **Features come from organization subscriptions**
4. **Personal organizations** are auto-created for backward compatibility
5. **Feature aggregation**: Users get MAX features across all their organizations

### Example: Checking Permissions

```typescript
// Complete permission check example
const checkUserPermissions = async (token: string) => {
  const user = await getCurrentUser(token);

  return {
    // System-level
    isSystemAdmin: isSystemAdmin(user),

    // Organization-level
    canCreateOrganizations: true, // All members can create orgs
    ownedOrganizations: user.organization_memberships?.filter(
      m => m.role === 'owner'
    ) ?? [],
    managedOrganizations: user.organization_memberships?.filter(
      m => m.role === 'owner' || m.role === 'manager'
    ) ?? [],

    // Group-level
    ownedGroups: user.group_memberships?.filter(
      m => m.role === 'owner'
    ) ?? [],
    managedGroups: user.group_memberships?.filter(
      m => m.role === 'owner' || m.role === 'admin'
    ) ?? [],

    // Feature-level (from subscriptions)
    features: {
      maxCourses: user.max_courses ?? 0,
      maxTerminals: user.max_terminals ?? 0,
      canExportCourses: user.can_export_courses ?? false,
      canUseAPI: user.can_use_api ?? false,
      // ... other features
    }
  };
};
```

---

## Authentication

All API requests require JWT authentication.

### Login Flow

```typescript
// 1. Login to get access token
const loginResponse = await fetch('http://localhost:8080/api/v1/auth/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    email: 'user@example.com',
    password: 'password123'
  })
});

const { access_token, token_type, expires_in } = await loginResponse.json();

// 2. Use token in subsequent requests
const headers = {
  'Authorization': `Bearer ${access_token}`,
  'Content-Type': 'application/json'
};
```

### Test Credentials (Development Only)
- **Email**: `1.supervisor@test.com`
- **Password**: `test`

---

## Phase 1: Individual User Subscriptions

### Overview

Individual users can subscribe to plans for personal use. Each subscription is tied directly to the user account.

### 1.1 List Available Plans

```typescript
// GET /api/v1/subscription-plans
const getSubscriptionPlans = async (token: string) => {
  const response = await fetch('http://localhost:8080/api/v1/subscription-plans', {
    headers: { 'Authorization': `Bearer ${token}` }
  });

  const data = await response.json();
  return data.data; // Array of SubscriptionPlanOutput
};
```

**Response Structure**:
```typescript
interface SubscriptionPlanOutput {
  id: string;
  name: string;
  description: string;
  priority: number; // Higher = better tier
  stripe_product_id: string;
  stripe_price_id: string;
  price_amount: number; // In cents (e.g., 900 = €9.00)
  currency: string;
  billing_interval: 'month' | 'year';
  trial_days: number;
  features: string[]; // Human-readable features
  max_concurrent_users: number;
  max_courses: number; // -1 = unlimited
  max_lab_sessions: number;
  is_active: boolean;
  required_role: string;

  // Terminal-specific limits
  max_session_duration_minutes: number;
  max_concurrent_terminals: number;
  allowed_machine_sizes: string[]; // ["XS", "S", "M", "L"]
  network_access_enabled: boolean;
  data_persistence_enabled: boolean;
  data_persistence_gb: number;
  allowed_templates: string[];

  // Tiered pricing (for bulk purchases)
  use_tiered_pricing: boolean;
  pricing_tiers?: PricingTier[];
}

interface PricingTier {
  min_quantity: number;
  max_quantity: number; // 0 = unlimited
  unit_amount: number; // Price per license in this tier (in cents)
  description?: string;
}
```

### 1.2 Get User's Current Subscription

```typescript
// GET /api/v1/user-subscriptions/current
const getCurrentSubscription = async (token: string) => {
  const response = await fetch('http://localhost:8080/api/v1/user-subscriptions/current', {
    headers: { 'Authorization': `Bearer ${token}` }
  });

  return await response.json(); // UserSubscriptionOutput
};
```

**Response Structure**:
```typescript
interface UserSubscriptionOutput {
  id: string;
  user_id: string;
  subscription_plan_id: string;
  subscription_plan: SubscriptionPlanOutput;
  stripe_subscription_id: string;
  stripe_customer_id: string;
  status: 'active' | 'trialing' | 'past_due' | 'canceled' | 'unpaid';
  subscription_type: 'personal' | 'assigned';
  is_primary: boolean; // True if this is the active subscription
  current_period_start: string; // ISO 8601
  current_period_end: string;
  trial_end?: string;
  cancel_at_period_end: boolean;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;

  // Bulk license info (if applicable)
  subscription_batch_id?: string;
  batch_owner_id?: string;
  batch_owner_name?: string;
  batch_owner_email?: string;
  assigned_at?: string;
}
```

### 1.3 Create Subscription (Stripe Checkout)

```typescript
// POST /api/v1/user-subscriptions/create-checkout-session
const createCheckoutSession = async (
  token: string,
  planId: string,
  successUrl: string,
  cancelUrl: string
) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/user-subscriptions/create-checkout-session',
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        subscription_plan_id: planId,
        success_url: successUrl,
        cancel_url: cancelUrl,
        allow_replace: true // Allow replacing free subscription with paid
      })
    }
  );

  const { url, session_id } = await response.json();

  // Redirect user to Stripe Checkout
  window.location.href = url;
};
```

### 1.4 Cancel Subscription

```typescript
// DELETE /api/v1/user-subscriptions/current
const cancelSubscription = async (token: string, cancelAtPeriodEnd: boolean = true) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/user-subscriptions/current',
    {
      method: 'DELETE',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        cancel_at_period_end: cancelAtPeriodEnd
      })
    }
  );

  return await response.json();
};
```

### 1.5 Upgrade/Downgrade Plan

```typescript
// POST /api/v1/user-subscriptions/current/upgrade
const upgradePlan = async (token: string, newPlanId: string) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/user-subscriptions/current/upgrade',
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        new_plan_id: newPlanId,
        proration_behavior: 'always_invoice' // 'always_invoice', 'create_prorations', or 'none'
      })
    }
  );

  return await response.json();
};
```

---

## Phase 2: Organization Subscriptions

### Overview

Organizations can subscribe to plans, and all members inherit the organization's features. Users can belong to multiple organizations and will inherit the **maximum** features across all organizations.

### 2.1 Create Organization

```typescript
// POST /api/v1/organizations
const createOrganization = async (
  token: string,
  name: string,
  displayName: string,
  description: string
) => {
  const response = await fetch('http://localhost:8080/api/v1/organizations', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      name,
      display_name: displayName,
      description,
      max_groups: 10,
      max_members: 50
    })
  });

  return await response.json(); // OrganizationOutput
};
```

**Response Structure**:
```typescript
interface OrganizationOutput {
  id: string;
  name: string;
  display_name: string;
  description: string;
  owner_user_id: string;
  subscription_plan_id?: string;
  is_personal: boolean;
  max_groups: number;
  max_members: number;
  is_active: boolean;
  metadata?: object;
  created_at: string;
  updated_at: string;

  // Counts (if preloaded)
  group_count?: number;
  member_count?: number;

  // Related data (if preloaded with ?includes=members,groups)
  members?: OrganizationMemberOutput[];
  groups?: GroupSummary[];
}

interface OrganizationMemberOutput {
  id: string;
  organization_id: string;
  user_id: string;
  role: 'owner' | 'manager' | 'member';
  invited_by: string;
  joined_at: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}
```

### 2.2 Subscribe Organization to Plan

```typescript
// POST /api/v1/organizations/:orgId/subscribe
const subscribeOrganization = async (
  token: string,
  orgId: string,
  planId: string,
  paymentMethodId?: string
) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/subscribe`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        subscription_plan_id: planId,
        payment_method_id: paymentMethodId,
        quantity: 1 // For organization subscriptions, always 1
      })
    }
  );

  return await response.json(); // OrganizationSubscriptionOutput
};
```

**Response Structure**:
```typescript
interface OrganizationSubscriptionOutput {
  id: string;
  organization_id: string;
  subscription_plan_id: string;
  subscription_plan: SubscriptionPlanOutput;
  stripe_subscription_id: string;
  stripe_customer_id: string;
  status: 'active' | 'trialing' | 'past_due' | 'canceled' | 'pending_payment';
  current_period_start: string;
  current_period_end: string;
  cancel_at_period_end: boolean;
  quantity: number; // Always 1 for org subscriptions
  created_at: string;
  updated_at: string;
}
```

### 2.3 Get Organization Subscription

```typescript
// GET /api/v1/organizations/:orgId/subscription
const getOrganizationSubscription = async (token: string, orgId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/subscription`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // OrganizationSubscriptionOutput
};
```

### 2.4 Get Organization Features

```typescript
// GET /api/v1/organizations/:orgId/features
const getOrganizationFeatures = async (token: string, orgId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/features`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json();
};
```

**Response Structure**:
```typescript
interface OrganizationFeaturesOutput {
  organization_id: string;
  organization_name: string;
  subscription_plan?: SubscriptionPlanOutput;
  has_active_subscription: boolean;
  features: string[];
  usage_limits: UsageLimits;
}

interface UsageLimits {
  max_concurrent_terminals: number;
  max_session_duration_minutes: number;
  max_courses: number;
  allowed_machine_sizes: string[];
  network_access_enabled: boolean;
  data_persistence_enabled: boolean;
  data_persistence_gb: number;
}
```

### 2.5 Get User's Effective Features (Aggregated)

**IMPORTANT**: Users can belong to multiple organizations. Their effective features are the **maximum** across all organizations.

```typescript
// GET /api/v1/users/me/features
const getUserEffectiveFeatures = async (token: string) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/users/me/features',
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json();
};
```

**Response Structure**:
```typescript
interface UserEffectiveFeaturesOutput {
  user_id: string;
  effective_features: SubscriptionPlanOutput; // Aggregated maximum features
  source_organizations: OrganizationFeatureSource[];
  has_personal_subscription: boolean;
  personal_subscription?: UserSubscriptionOutput;
}

interface OrganizationFeatureSource {
  organization_id: string;
  organization_name: string;
  role: 'owner' | 'manager' | 'member';
  contributing_features: string[];
}
```

### 2.6 Manage Organization Members

```typescript
// GET /api/v1/organizations/:orgId?includes=members
const getOrganizationMembers = async (token: string, orgId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}?includes=members`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  const org = await response.json();
  return org.members; // OrganizationMemberOutput[]
};

// To add/remove members, use organization management endpoints
// POST /api/v1/organizations/:orgId/members
// DELETE /api/v1/organizations/:orgId/members/:userId
```

### 2.7 Cancel Organization Subscription

```typescript
// DELETE /api/v1/organizations/:orgId/subscription
const cancelOrganizationSubscription = async (
  token: string,
  orgId: string,
  cancelAtPeriodEnd: boolean = true
) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/organizations/${orgId}/subscription`,
    {
      method: 'DELETE',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        cancel_at_period_end: cancelAtPeriodEnd
      })
    }
  );

  return await response.json();
};
```

---

## Phase 3: Bulk License Management

### Overview

Bulk licenses allow purchasing multiple subscriptions at once and assigning them to specific users. Perfect for corporate training, classroom management, or reselling licenses.

### 3.1 Purchase Bulk Licenses

**Option A: Direct Purchase with Payment Method**

```typescript
// POST /api/v1/user-subscriptions/purchase-bulk
const purchaseBulkLicenses = async (
  token: string,
  planId: string,
  quantity: number,
  paymentMethodId?: string,
  groupId?: string
) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/user-subscriptions/purchase-bulk',
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        subscription_plan_id: planId,
        quantity,
        payment_method_id: paymentMethodId, // Optional
        group_id: groupId // Optional: link to a group
      })
    }
  );

  return await response.json(); // SubscriptionBatchOutput
};
```

**Option B: Stripe Checkout Session (Recommended)**

```typescript
// POST /api/v1/subscription-batches/create-checkout-session
const createBulkCheckoutSession = async (
  token: string,
  planId: string,
  quantity: number,
  successUrl: string,
  cancelUrl: string,
  groupId?: string
) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/subscription-batches/create-checkout-session',
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        subscription_plan_id: planId,
        quantity,
        success_url: successUrl,
        cancel_url: cancelUrl,
        group_id: groupId
      })
    }
  );

  const { url, session_id } = await response.json();
  window.location.href = url;
};
```

**Response Structure**:
```typescript
interface SubscriptionBatchOutput {
  id: string;
  purchaser_user_id: string;
  subscription_plan_id: string;
  subscription_plan: SubscriptionPlanOutput;
  group_id?: string;
  stripe_subscription_id: string;
  stripe_subscription_item_id: string;
  total_quantity: number;
  assigned_quantity: number;
  available_quantity: number; // Calculated: total - assigned
  status: 'pending_payment' | 'active' | 'cancelled';
  current_period_start: string;
  current_period_end: string;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;
}
```

### 3.2 List User's Batches

```typescript
// GET /api/v1/subscription-batches
const getMyBatches = async (token: string) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/subscription-batches',
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // SubscriptionBatchOutput[]
};
```

### 3.3 Get Batch Details

```typescript
// GET /api/v1/subscription-batches/:batchId
const getBatchDetails = async (token: string, batchId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // SubscriptionBatchOutput
};
```

### 3.4 Get Licenses in Batch

```typescript
// GET /api/v1/subscription-batches/:batchId/licenses
const getBatchLicenses = async (token: string, batchId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/licenses`,
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // UserSubscriptionOutput[]
};
```

### 3.5 Assign License to User

```typescript
// POST /api/v1/subscription-batches/:batchId/assign
const assignLicense = async (token: string, batchId: string, userId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/assign`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        user_id: userId
      })
    }
  );

  return await response.json(); // UserSubscriptionOutput
};
```

### 3.6 Revoke License Assignment

**CRITICAL**: Revoking a license will **terminate all active terminals** for that user!

```typescript
// DELETE /api/v1/subscription-batches/:batchId/licenses/:licenseId/revoke
const revokeLicense = async (token: string, batchId: string, licenseId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/licenses/${licenseId}/revoke`,
    {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json();
};
```

### 3.7 Update Batch Quantity

**IMPORTANT**: You cannot reduce quantity below the number of assigned licenses!

```typescript
// PATCH /api/v1/subscription-batches/:batchId/quantity
const updateBatchQuantity = async (
  token: string,
  batchId: string,
  newQuantity: number
) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/quantity`,
    {
      method: 'PATCH',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        new_quantity: newQuantity
      })
    }
  );

  return await response.json();
};
```

### 3.8 Permanently Delete Batch

**CRITICAL**: This will:
1. Cancel Stripe subscription
2. Terminate all terminals for users with assigned licenses
3. Delete all licenses
4. Delete the batch record

```typescript
// DELETE /api/v1/subscription-batches/:batchId/permanent
const permanentlyDeleteBatch = async (token: string, batchId: string) => {
  const response = await fetch(
    `http://localhost:8080/api/v1/subscription-batches/${batchId}/permanent`,
    {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json();
};
```

---

## Feature Detection & Limits

### Checking User Features

```typescript
// Use the effective features endpoint for accurate feature detection
const canUserCreateTerminal = async (token: string) => {
  const features = await getUserEffectiveFeatures(token);

  // Check if user has terminal access
  return features.effective_features.max_concurrent_terminals > 0;
};

const getMaxConcurrentTerminals = async (token: string) => {
  const features = await getUserEffectiveFeatures(token);
  return features.effective_features.max_concurrent_terminals;
};
```

### Usage Metrics

```typescript
// GET /api/v1/usage-metrics
const getUserUsageMetrics = async (token: string) => {
  const response = await fetch(
    'http://localhost:8080/api/v1/usage-metrics',
    {
      headers: { 'Authorization': `Bearer ${token}` }
    }
  );

  return await response.json(); // UsageMetricsOutput[]
};
```

**Response Structure**:
```typescript
interface UsageMetricsOutput {
  id: string;
  user_id: string;
  metric_type: 'concurrent_terminals' | 'courses' | 'lab_sessions';
  current_value: number;
  limit_value: number;
  period_start: string;
  period_end: string;
  last_updated: string;
  usage_percent: number; // Calculated
}
```

---

## Stripe Integration

### Webhook Events

The backend handles these Stripe webhook events automatically:

#### Individual Subscriptions
- `customer.subscription.created` → Create user subscription
- `customer.subscription.updated` → Update subscription status
- `customer.subscription.deleted` → Cancel subscription, terminate terminals
- `invoice.payment_succeeded` → Activate subscription
- `invoice.payment_failed` → Suspend subscription

#### Organization Subscriptions
- `customer.subscription.created` (with `organization_id` metadata) → Create org subscription
- `customer.subscription.updated` → Update org subscription
- `customer.subscription.deleted` → Cancel org subscription

#### Bulk Licenses
- `customer.subscription.created` (with `bulk_purchase: "true"` metadata) → Create batch & licenses
- `customer.subscription.updated` → Handle quantity changes
- `customer.subscription.deleted` → Cancel batch, revoke all licenses, terminate all terminals
- `invoice.payment_succeeded` → Activate licenses (change from `pending_payment` to `unassigned`)

### Metadata Structure

**Individual Subscription**:
```json
{
  "user_id": "uuid",
  "subscription_plan_id": "uuid"
}
```

**Organization Subscription**:
```json
{
  "organization_id": "uuid",
  "subscription_plan_id": "uuid",
  "user_id": "uuid" (purchaser)
}
```

**Bulk License**:
```json
{
  "bulk_purchase": "true",
  "user_id": "uuid" (purchaser),
  "subscription_plan_id": "uuid",
  "quantity": "5",
  "group_id": "uuid" (optional)
}
```

---

## Error Handling

### Common Error Responses

```typescript
interface APIError {
  error_code: number;
  error_message: string;
  details?: {
    field?: string;
    operation?: string;
    original?: string;
  };
}
```

### Error Codes

| Code | Meaning | Common Causes |
|------|---------|---------------|
| 400  | Bad Request | Invalid input data, validation errors |
| 401  | Unauthorized | Missing or invalid token |
| 403  | Forbidden | Insufficient permissions, not organization member |
| 404  | Not Found | Resource doesn't exist |
| 409  | Conflict | Already subscribed, duplicate resource |
| 500  | Internal Server Error | Backend failure, database issues |

### Example Error Handling

```typescript
const handleSubscriptionError = (error: APIError) => {
  switch (error.error_code) {
    case 400:
      if (error.error_message.includes('validation')) {
        return 'Please check your input fields';
      }
      break;

    case 403:
      if (error.error_message.includes('not a member')) {
        return 'You must be a member of this organization';
      }
      break;

    case 409:
      if (error.error_message.includes('already subscribed')) {
        return 'You already have an active subscription';
      }
      break;

    default:
      return 'An unexpected error occurred. Please try again.';
  }
};
```

---

## Complete Use Cases

### Use Case 1: Individual User Subscribes

```typescript
const individualSubscribeFlow = async (token: string) => {
  // 1. Get available plans
  const plans = await getSubscriptionPlans(token);

  // 2. Display plans to user, let them choose
  const selectedPlan = plans.find(p => p.name === 'Pro');

  // 3. Create Stripe checkout session
  await createCheckoutSession(
    token,
    selectedPlan.id,
    'https://app.example.com/subscription/success',
    'https://app.example.com/subscription/cancel'
  );

  // User is redirected to Stripe, completes payment
  // Webhook activates subscription in background

  // 4. After redirect back, check subscription status
  const subscription = await getCurrentSubscription(token);
  console.log('Subscription status:', subscription.status);
};
```

### Use Case 2: Organization Subscribes, Members Inherit Features

```typescript
const organizationSubscribeFlow = async (token: string) => {
  // 1. Create organization
  const org = await createOrganization(
    token,
    'acme-corp',
    'ACME Corporation',
    'Training organization for ACME employees'
  );

  // 2. Wait for organization setup (owner membership created async)
  await new Promise(resolve => setTimeout(resolve, 2000));

  // 3. Subscribe organization to plan
  const orgSub = await subscribeOrganization(
    token,
    org.id,
    'organization-plan-id'
  );

  // 4. Add team members to organization
  // (Use organization member management endpoints)

  // 5. Members automatically inherit org features
  const memberFeatures = await getUserEffectiveFeatures(memberToken);
  console.log('Member can use:', memberFeatures.effective_features);
};
```

### Use Case 3: Bulk License Purchase & Assignment

```typescript
const bulkLicenseFlow = async (token: string) => {
  // 1. Create Stripe checkout for bulk purchase
  await createBulkCheckoutSession(
    token,
    'plan-id',
    10, // Buy 10 licenses
    'https://app.example.com/bulk/success',
    'https://app.example.com/bulk/cancel'
  );

  // Webhook creates batch and licenses after payment

  // 2. Get purchased batches
  const batches = await getMyBatches(token);
  const batch = batches[0]; // Most recent

  // 3. View available licenses
  const licenses = await getBatchLicenses(token, batch.id);
  const unassigned = licenses.filter(l => l.status === 'unassigned');

  console.log(`${unassigned.length} licenses available to assign`);

  // 4. Assign license to a user
  const assignedLicense = await assignLicense(
    token,
    batch.id,
    'target-user-id'
  );

  console.log('License assigned:', assignedLicense.id);

  // 5. Target user can now use the subscription
  const userSub = await getCurrentSubscription(targetUserToken);
  console.log('User subscription type:', userSub.subscription_type); // 'assigned'
};
```

### Use Case 4: Display User's Effective Features (Multi-Org)

```typescript
const displayUserFeatures = async (token: string) => {
  const userFeatures = await getUserEffectiveFeatures(token);

  // Show aggregated features
  console.log('Your effective features:');
  console.log('- Max terminals:', userFeatures.effective_features.max_concurrent_terminals);
  console.log('- Session duration:', userFeatures.effective_features.max_session_duration_minutes, 'minutes');
  console.log('- Allowed machines:', userFeatures.effective_features.allowed_machine_sizes);

  // Show source organizations
  console.log('\nYour organizations:');
  for (const org of userFeatures.source_organizations) {
    console.log(`- ${org.organization_name} (${org.role})`);
  }

  // Show personal subscription if exists
  if (userFeatures.has_personal_subscription) {
    console.log('\nPersonal subscription:', userFeatures.personal_subscription.subscription_plan.name);
  }
};
```

---

## API Reference

### Base URL
```
http://localhost:8080/api/v1
```

### Individual Subscriptions

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/subscription-plans` | List all plans |
| GET | `/user-subscriptions/current` | Get current subscription |
| POST | `/user-subscriptions/create-checkout-session` | Create Stripe checkout |
| POST | `/user-subscriptions/current/upgrade` | Upgrade/downgrade plan |
| DELETE | `/user-subscriptions/current` | Cancel subscription |
| GET | `/usage-metrics` | Get usage metrics |

### Organization Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/organizations` | Create organization |
| GET | `/organizations/:id` | Get organization details |
| GET | `/organizations/:id?includes=members,groups` | Get with relationships |
| PATCH | `/organizations/:id` | Update organization |
| DELETE | `/organizations/:id` | Delete organization |

### Organization Subscriptions

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/organizations/:id/subscribe` | Subscribe organization |
| GET | `/organizations/:id/subscription` | Get org subscription |
| DELETE | `/organizations/:id/subscription` | Cancel org subscription |
| GET | `/organizations/:id/features` | Get org features |
| GET | `/organizations/:id/usage-limits` | Get org usage limits |
| GET | `/users/me/features` | Get effective features (aggregated) |

### Bulk Licenses

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/subscription-batches/create-checkout-session` | Create bulk checkout |
| POST | `/user-subscriptions/purchase-bulk` | Direct bulk purchase |
| GET | `/subscription-batches` | List my batches |
| GET | `/subscription-batches/:id` | Get batch details |
| GET | `/subscription-batches/:id/licenses` | List licenses in batch |
| POST | `/subscription-batches/:id/assign` | Assign license to user |
| DELETE | `/subscription-batches/:id/licenses/:licenseId/revoke` | Revoke license |
| PATCH | `/subscription-batches/:id/quantity` | Update batch quantity |
| DELETE | `/subscription-batches/:id/permanent` | Permanently delete batch |

---

## Important Notes

### 1. Personal Organizations
Every user automatically gets a personal organization on first login/registration. This is used for backward compatibility with individual subscriptions.

### 2. Feature Aggregation
Users in multiple organizations inherit the **maximum** features across all organizations. For example:
- Org A: 5 concurrent terminals
- Org B: 10 concurrent terminals
- User's effective limit: **10 concurrent terminals**

### 3. License Status Flow
```
pending_payment → unassigned → (assign to user) → active → (revoke) → unassigned
                                                           → cancelled
```

### 4. Terminal Termination
When licenses are revoked or subscriptions cancelled, **all active terminals for affected users are automatically terminated** to enforce feature limits immediately.

### 5. Stripe Incomplete Status
You cannot modify subscription quantity when it's in `incomplete` status (payment pending). Wait for `invoice.payment_succeeded` webhook to activate the subscription first.

### 6. Authorization
- **Organization subscriptions**: Only owners and managers can manage billing
- **Bulk licenses**: Only the purchaser can assign/revoke licenses
- **Organization members**: All members can view features but not manage billing

---

## Testing & Development

### Test Script Locations
- **Individual subscriptions**: `/tmp/test_user_subscription.sh`
- **Organization subscriptions**: `/tmp/test_org_subscription.sh`
- **Bulk licenses**: `/tmp/test_bulk_licenses.sh`

### Swagger Documentation
Full API documentation with request/response schemas:
```
http://localhost:8080/swagger/index.html
```

### Test Credentials
```
Email: 1.supervisor@test.com
Password: test
```

---

## Questions & Support

For implementation questions or issues:
1. Check Swagger documentation at `/swagger/`
2. Review test scripts in `/tmp/`
3. Check server logs at `/tmp/server.log` or `/tmp/server_bulk_test.log`

---

**Document Version**: 1.0
**Last Updated**: 2025-10-25
**Backend Implementation Status**: ✅ All 3 Phases Complete
# Frontend Pricing Page Update - MVP Launch

## Overview
The backend API has been updated for the MVP launch. Only 2 plans are active (Trial + Solo), with 2 more marked as "Coming Soon".

## API Changes

### Endpoint
`GET /api/v1/subscription-plans` (no authentication required)

### New Fields Available
```typescript
interface SubscriptionPlan {
  // Existing fields
  id: string;
  name: string;
  description: string;
  price_amount: number; // in cents (€9.00 = 900)
  currency: string;
  billing_interval: string;
  features: string[];
  is_active: boolean; // ⭐ USE THIS TO SHOW/HIDE PLANS

  // NEW Terminal-specific fields
  max_session_duration_minutes: number;
  max_concurrent_terminals: number;
  allowed_machine_sizes: string[]; // ["XS"], ["S"], ["M"], ["L"]
  network_access_enabled: boolean;
  data_persistence_enabled: boolean;
  data_persistence_gb: number;
  allowed_templates: string[];

  // NEW: Planned features (announced but not yet available)
  planned_features: string[]; // Features coming soon (🔜 prefix)
}
```

**Note on Planned Features:**
- Use the `planned_features` array to show upcoming features for each plan
- These are features that will be added in the future but aren't available yet
- Display them with a "Coming Soon" badge or grayed out style
- Example: `["🔜 200MB persistent storage", "🔜 Web development with port forwarding"]`

## Required Changes

### 1. Filter Plans by Active Status
```typescript
// Fetch all plans
const response = await fetch('http://localhost:8080/api/v1/subscription-plans');
const { data: allPlans } = await response.json();

// Split active and coming soon
const activePlans = allPlans.filter(plan => plan.is_active);
const comingSoonPlans = allPlans.filter(plan => !plan.is_active);
```

### 2. Update Pricing Cards

**For Active Plans (Trial & Solo):**
- Show normal pricing card
- Enable "Subscribe" / "Get Started" button
- Display all features from `features` array
- Show machine size from `allowed_machine_sizes[0]`

**For Coming Soon Plans (Trainer & Organization):**
- Gray out or add opacity overlay
- Add "Coming Soon" badge
- **Disable** all purchase/subscribe buttons
- Keep pricing visible but mark as unavailable
- Optional: Add "Notify me" button for interest

### 3. Display Current and Planned Features

```typescript
// Example display logic
const displayPlan = (plan) => {
  const price = plan.price_amount / 100; // Convert cents to euros
  const machineSize = plan.allowed_machine_sizes[0]; // "XS", "S", "M", "L"
  const sessionHours = plan.max_session_duration_minutes / 60;
  const storage = plan.data_persistence_enabled
    ? `${plan.data_persistence_gb}GB`
    : 'Ephemeral only';

  return {
    title: plan.name,
    price: `€${price}`,
    isAvailable: plan.is_active,
    currentFeatures: [
      ...plan.features, // Use existing features array
      `${sessionHours}h max session`,
      `${plan.max_concurrent_terminals} concurrent terminal${plan.max_concurrent_terminals > 1 ? 's' : ''}`,
      `${machineSize} machine size`,
      storage,
    ],
    // NEW: Show planned features with special styling
    plannedFeatures: plan.planned_features || [], // Array of upcoming features
  };
};
```

**Displaying Planned Features:**
```jsx
{/* Current features - normal style */}
{plan.currentFeatures.map(feature => (
  <li key={feature}>{feature}</li>
))}

{/* Planned features - grayed out or with badge */}
{plan.plannedFeatures.length > 0 && (
  <div className="planned-features">
    <h4>Coming Soon</h4>
    {plan.plannedFeatures.map(feature => (
      <li key={feature} className="text-gray-400">
        {feature} {/* Already includes 🔜 emoji */}
      </li>
    ))}
  </div>
)}
```

### 4. Update Button Behavior

```jsx
<button
  disabled={!plan.is_active}
  onClick={() => plan.is_active && handleSubscribe(plan.id)}
  className={plan.is_active ? 'btn-primary' : 'btn-disabled'}
>
  {plan.is_active ? 'Subscribe Now' : 'Coming Soon'}
</button>
```

## Current Active Plans

### ✅ Trial (FREE)
- **Status**: ACTIVE - Ready to purchase
- **Machine**: XS (0.5 vCPU, 256MB RAM)
- **Session**: 1 hour max
- **Concurrent**: 1 terminal
- **Network**: ❌ No network access
- **Storage**: Ephemeral only

### ✅ Solo (€9/month)
- **Status**: ACTIVE - Ready to purchase
- **Machine**: S (1 vCPU, 1GB RAM)
- **Session**: 8 hours max
- **Concurrent**: 1 terminal
- **Network**: Outbound access
- **Storage**: Ephemeral only
- **Planned Features**:
  - 🔜 200MB persistent storage

### ❌ Trainer (€19/month) - COMING SOON
- **Status**: INACTIVE - Do not allow purchase
- **Machine**: M (2 vCPU, 2GB RAM)
- **Session**: 8 hours max
- **Concurrent**: 3 terminals
- **Network**: Outbound access
- **Storage**: Ephemeral only
- **Planned Features**:
  - 🔜 1GB persistent storage
  - 🔜 Web development with port forwarding
  - 🔜 Custom images
  - 🔜 Team collaboration features

### ❌ Organization (€49/month) - COMING SOON
- **Status**: INACTIVE - Do not allow purchase
- **Machine**: L (4 vCPU, 4GB RAM)
- **Session**: 8 hours max
- **Concurrent**: 10 terminals
- **Network**: Outbound access
- **Storage**: Ephemeral only
- **Planned Features**:
  - 🔜 5GB persistent storage
  - 🔜 Web development with port forwarding
  - 🔜 Custom images
  - 🔜 Team collaboration features
  - 🔜 Priority support

## Design Recommendations

1. **Active Plans**: Full color, normal opacity, clickable
2. **Coming Soon Plans**:
   - Grayscale or 50% opacity
   - "Coming Soon" badge in top-right corner
   - Price visible but grayed out
   - Disabled button with "Notify Me" option
3. **Machine Size Labels**: Display prominently (XS, S, M, L)
4. **Session Duration**: Show as "Xh sessions" for clarity

## Terminal Sessions - Machine Size Information

**Important Update**: Terminal sessions now include the **actual machine size used** from Terminal Trainer.

### Terminal Session Response
```typescript
interface TerminalSession {
  id: string;
  session_id: string;
  user_id: string;
  status: string; // "active", "stopped", "expired"
  expires_at: string;
  instance_type: string; // "ubuntu", "python", etc.
  machine_size: string; // ⭐ NEW: Actual size used (XS, S, M, L, XL)
  created_at: string;
}
```

### How It Works

1. **Subscription Plan** defines `allowed_machine_sizes: ["XS"]` - what sizes the user CAN use
2. **Terminal Trainer** decides the actual size and returns it when creating a session
3. **Terminal Session** stores and exposes `machine_size: "XS"` - what size is ACTUALLY being used

### Example API Call
```bash
# Get user's active terminals
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/terminal-sessions/user-sessions
```

**Response:**
```json
{
  "data": [
    {
      "id": "...",
      "session_id": "abc123",
      "status": "active",
      "instance_type": "ubuntu",
      "machine_size": "XS",  // ⭐ Actual size used
      "expires_at": "2025-10-08T14:00:00Z"
    }
  ]
}
```

### Display Recommendations

Show the **actual machine size** in the terminal list:
- "Ubuntu terminal (XS - 0.5 vCPU, 256MB)"
- Use the subscription plan's `allowed_machine_sizes` to show what sizes are available
- Use the terminal session's `machine_size` to show what size is currently running

## Testing

Test the subscription plans endpoint:
```bash
curl http://localhost:8080/api/v1/subscription-plans | jq '.data[] | {name, is_active, price_amount, allowed_machine_sizes}'
```

Expected output:
- Trial: `is_active: true, allowed_machine_sizes: ["XS"]`
- Solo: `is_active: true, allowed_machine_sizes: ["S"]`
- Trainer: `is_active: false, allowed_machine_sizes: ["M"]`
- Organization: `is_active: false, allowed_machine_sizes: ["L"]`

Test terminal sessions (requires authentication):
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/terminal-sessions/user-sessions
```

Expected: Each terminal includes `machine_size` field with actual size used.

## Questions?

Contact backend team or check API documentation at: http://localhost:8080/swagger/
# Bulk License Management - Frontend Integration Guide

## 📋 Table of Contents
1. [Overview](#overview)
2. [Key Concepts](#key-concepts)
3. [API Endpoints](#api-endpoints)
4. [Data Structures](#data-structures)
5. [User Workflows](#user-workflows)
6. [UI/UX Requirements](#uiux-requirements)
7. [Code Examples](#code-examples)
8. [Error Handling](#error-handling)
9. [Testing Guide](#testing-guide)

---

## 🎯 Overview

The bulk license management system allows users (trainers, group administrators, training centers) to:
- Purchase multiple licenses at once with **volume discounts**
- Assign licenses to individual users (e.g., students in a class)
- Manage license assignments (assign, revoke, reassign)
- Scale up/down the number of licenses mid-subscription
- Track license usage and availability

**Key Feature**: Tiered pricing automatically applies volume discounts based on quantity.

---

## 🔑 Key Concepts

### 1. **Subscription Plans with Tiered Pricing**

Subscription plans can now have **volume pricing tiers**:

```json
{
  "id": "uuid",
  "name": "Trainer Plan",
  "use_tiered_pricing": true,
  "pricing_tiers": [
    {
      "min_quantity": 1,
      "max_quantity": 5,
      "unit_amount": 1200,  // €12 per license
      "description": "1-5 licenses"
    },
    {
      "min_quantity": 6,
      "max_quantity": 15,
      "unit_amount": 1000,  // €10 per license
      "description": "6-15 licenses"
    },
    {
      "min_quantity": 16,
      "max_quantity": 30,
      "unit_amount": 800,   // €8 per license
      "description": "16-30 licenses"
    },
    {
      "min_quantity": 31,
      "max_quantity": 0,    // 0 = unlimited
      "unit_amount": 600,   // €6 per license
      "description": "31+ licenses"
    }
  ]
}
```

**Features Required for Bulk Purchase**:
- User's plan must include `"bulk_purchase"` or `"group_management"` in `features` array

### 2. **Subscription Batch**

A **batch** represents a bulk license purchase:
- One Stripe subscription with `quantity > 1`
- Contains multiple individual licenses (UserSubscription records)
- Tracks: Total licenses, Assigned licenses, Available licenses

### 3. **License States**

Individual licenses can be in different states:
- `unassigned` - Not yet assigned to anyone
- `active` - Assigned to a user and active
- `cancelled` - Subscription cancelled
- `past_due` - Payment issues

---

## 📡 API Endpoints

### Authentication Required
All endpoints require a valid JWT token in the `Authorization` header:
```
Authorization: Bearer <token>
```

---

### 0. **Stripe Plan Synchronization** (Admin Only)

#### Import Plans from Stripe
Import subscription plans from Stripe into the database. This is useful when plans are created/modified in the Stripe Dashboard.

**Endpoint**: `POST /api/v1/subscription-plans/import-stripe`

**Headers**:
- `Authorization: Bearer <admin-token>` ✅ Required (Admin only)

**Response** (200 OK):
```json
{
  "processed_plans": 4,
  "created_plans": 1,
  "updated_plans": 3,
  "skipped_plans": 0,
  "failed_plans": [],
  "created_details": [
    "Created plan: XS (Stripe price: price_1SJdyX2VDBCbFKoanstbeLH9, pricing: tiered (4 tiers))"
  ],
  "updated_details": [
    "Updated plan: Solo (Stripe price: price_1SFMxN2VDBCbFKoaRgQEsZ9I, pricing: 900 eur/month)",
    "Updated plan: Trainer (Stripe price: price_1SFMxO2VDBCbFKoaIE0gFxPi, pricing: 1900 eur/month)"
  ],
  "skipped_details": []
}
```

**Features**:
- ✅ Automatically detects **tiered pricing** (volume/graduated pricing in Stripe)
- ✅ Creates new plans that exist in Stripe but not in database
- ✅ Updates existing plans with current Stripe data
- ✅ Properly converts Stripe tier structure to database format
- ✅ Handles both flat-rate and volume-based pricing

**Important Notes**:
- This endpoint imports plans FROM Stripe TO your database (reverse sync)
- Tiered pricing is detected using Stripe's `tiers` field
- The API automatically expands tier data from Stripe using `priceParams.AddExpand("data.tiers")`
- For existing plans, this updates prices, tiers, and metadata

**Use Cases**:
- After creating a new plan in Stripe Dashboard
- After modifying pricing tiers in Stripe
- Initial setup to sync existing Stripe products

---

### 1. **Get Pricing Preview** (Public)

Get a detailed pricing breakdown BEFORE purchase.

**Endpoint**: `GET /api/v1/subscription-plans/pricing-preview`

**Query Parameters**:
- `subscription_plan_id` (string, required) - UUID of the plan
- `quantity` (int, required) - Number of licenses

**Response**:
```json
{
  "plan_name": "Trainer Plan",
  "total_quantity": 30,
  "tier_breakdown": [
    {
      "range": "1-5",
      "quantity": 5,
      "unit_price": 1200,
      "subtotal": 6000
    },
    {
      "range": "6-15",
      "quantity": 10,
      "unit_price": 1000,
      "subtotal": 10000
    },
    {
      "range": "16-30",
      "quantity": 15,
      "unit_price": 800,
      "subtotal": 12000
    }
  ],
  "total_monthly_cost": 28000,      // €280 total
  "average_per_license": 9.33,      // €9.33 average
  "savings_vs_individual": 8000,    // €80 saved vs individual pricing
  "currency": "eur"
}
```

**Use Cases**:
- Display pricing calculator on plan selection page
- Show savings in real-time as user adjusts quantity
- Preview before checkout

---

### 2. **Purchase Bulk Licenses**

Create a bulk license purchase.

**Endpoint**: `POST /api/v1/user-subscriptions/purchase-bulk`

**Headers**:
- `Authorization: Bearer <token>` ✅ Required
- `Content-Type: application/json`

**Request Body**:
```json
{
  "subscription_plan_id": "uuid",
  "quantity": 30,
  "group_id": "uuid",           // Optional: link to a group
  "payment_method_id": "pm_xxx", // Optional: Stripe payment method
  "coupon_code": "SAVE20"        // Optional: discount coupon
}
```

**Response** (201 Created):
```json
{
  "id": "batch-uuid",
  "purchaser_user_id": "user-123",
  "subscription_plan_id": "plan-uuid",
  "subscription_plan": { /* plan details */ },
  "group_id": "group-uuid",
  "stripe_subscription_id": "sub_xxx",
  "stripe_subscription_item_id": "si_xxx",
  "total_quantity": 30,
  "assigned_quantity": 0,
  "available_quantity": 30,
  "status": "active",
  "current_period_start": "2025-01-01T00:00:00Z",
  "current_period_end": "2025-02-01T00:00:00Z",
  "created_at": "2025-01-01T00:00:00Z",
  "updated_at": "2025-01-01T00:00:00Z"
}
```

**Errors**:
- `403 Forbidden` - User's plan doesn't include `bulk_purchase` feature
- `400 Bad Request` - Invalid plan ID or quantity
- `500 Internal Server Error` - Server/Stripe error

---

### 3. **List My Batches**

Get all bulk purchases made by the current user.

**Endpoint**: `GET /api/v1/subscription-batches`

**Response** (200 OK):
```json
[
  {
    "id": "batch-uuid-1",
    "subscription_plan": {
      "name": "Trainer Plan",
      "price_amount": 1200
    },
    "total_quantity": 30,
    "assigned_quantity": 25,
    "available_quantity": 5,
    "status": "active",
    "current_period_end": "2025-02-01T00:00:00Z"
  },
  {
    "id": "batch-uuid-2",
    "subscription_plan": {
      "name": "Basic Plan",
      "price_amount": 800
    },
    "total_quantity": 10,
    "assigned_quantity": 10,
    "available_quantity": 0,
    "status": "active",
    "current_period_end": "2025-03-01T00:00:00Z"
  }
]
```

**Use Cases**:
- Dashboard showing all purchased batches
- Overview of license pools

---

### 4. **Get Batch Licenses**

Get all licenses (assigned & unassigned) in a batch.

**Endpoint**: `GET /api/v1/subscription-batches/:batch_id/licenses`

**Response** (200 OK):
```json
[
  {
    "id": "license-uuid-1",
    "user_id": "student-123",
    "subscription_batch_id": "batch-uuid",
    "status": "active",
    "created_at": "2025-01-01T00:00:00Z"
  },
  {
    "id": "license-uuid-2",
    "user_id": "",               // Unassigned
    "subscription_batch_id": "batch-uuid",
    "status": "unassigned",
    "created_at": "2025-01-01T00:00:00Z"
  }
]
```

**Use Cases**:
- View all licenses in a batch
- See who has assignments
- Identify available licenses

---

### 5. **Assign License to User**

Assign an unassigned license to a specific user.

**Endpoint**: `POST /api/v1/subscription-batches/:batch_id/assign`

**Request Body**:
```json
{
  "user_id": "student-456"
}
```

**Response** (200 OK):
```json
{
  "id": "license-uuid",
  "user_id": "student-456",
  "subscription_batch_id": "batch-uuid",
  "subscription_plan": { /* plan details */ },
  "status": "active",
  "current_period_start": "2025-01-01T00:00:00Z",
  "current_period_end": "2025-02-01T00:00:00Z"
}
```

**Errors**:
- `403 Forbidden` - Not the purchaser
- `400 Bad Request` - No available licenses or invalid user ID
- `404 Not Found` - Batch not found

**Use Cases**:
- Teacher assigns license to student
- Admin assigns license to new employee
- Bulk assignment UI

---

### 6. **Revoke License Assignment**

Remove a license assignment and return it to the pool.

**Endpoint**: `DELETE /api/v1/subscription-batches/:batch_id/licenses/:license_id/revoke`

**Response** (200 OK):
```json
{
  "message": "License revoked successfully"
}
```

**Errors**:
- `403 Forbidden` - Not the purchaser
- `404 Not Found` - License not found

**Use Cases**:
- Student leaves class
- Employee leaves organization
- Reassignment needed

---

### 7. **Update Batch Quantity**

Scale up or down the number of licenses (updates Stripe subscription).

**Endpoint**: `PATCH /api/v1/subscription-batches/:batch_id/quantity`

**Request Body**:
```json
{
  "new_quantity": 40  // Scale from 30 to 40
}
```

**Response** (200 OK):
```json
{
  "message": "Batch quantity updated to 40"
}
```

**Errors**:
- `400 Bad Request` - Cannot reduce below assigned quantity
- `403 Forbidden` - Not the purchaser

**Use Cases**:
- Class grows, need more licenses
- Reduce licenses to save costs
- Mid-subscription adjustments

---

## 📦 Data Structures

### SubscriptionPlanOutput
```typescript
interface SubscriptionPlanOutput {
  id: string;
  name: string;
  description: string;
  price_amount: number;           // Base price in cents (first tier if tiered)
  currency: string;               // "eur", "usd"
  billing_interval: string;       // "month", "year"
  features: string[];             // ["bulk_purchase", "group_management"]
  use_tiered_pricing: boolean;    // TRUE if plan has volume pricing
  pricing_tiers: PricingTier[];   // Empty array if not tiered
  is_active: boolean;

  // Additional fields
  stripe_product_id: string;
  stripe_price_id: string;
  trial_days: number;
  max_concurrent_users: number;
  max_courses: number;
  max_lab_sessions: number;
  required_role: string;
  created_at: string;             // ISO 8601
  updated_at: string;
}

interface PricingTier {
  min_quantity: number;           // Start of tier (e.g., 1, 6, 16)
  max_quantity: number;           // End of tier (0 = unlimited)
  unit_amount: number;            // Price per license in cents
  description?: string;           // Optional description
}
```

**Example Response** (XS Plan with Volume Pricing):
```json
{
  "id": "0199f869-83ef-734c-8387-4441da13f598",
  "name": "XS",
  "description": "Small volume plan with graduated pricing",
  "stripe_product_id": "prod_TGAJ4N07Uf1GKL",
  "stripe_price_id": "price_1SJdyX2VDBCbFKoanstbeLH9",
  "price_amount": 400,
  "currency": "eur",
  "billing_interval": "month",
  "use_tiered_pricing": true,
  "pricing_tiers": [
    {
      "min_quantity": 1,
      "max_quantity": 1,
      "unit_amount": 400
    },
    {
      "min_quantity": 2,
      "max_quantity": 5,
      "unit_amount": 350
    },
    {
      "min_quantity": 6,
      "max_quantity": 10,
      "unit_amount": 300
    },
    {
      "min_quantity": 11,
      "max_quantity": 0,
      "unit_amount": 250
    }
  ]
}
```

**Important**: The `pricing_tiers` array is **always present** in the response. Check `use_tiered_pricing` to determine if the plan uses volume pricing.

### SubscriptionBatchOutput
```typescript
interface SubscriptionBatchOutput {
  id: string;
  purchaser_user_id: string;
  subscription_plan_id: string;
  subscription_plan: SubscriptionPlanOutput;
  group_id?: string;
  stripe_subscription_id: string;
  total_quantity: number;
  assigned_quantity: number;
  available_quantity: number;     // Calculated field
  status: "active" | "cancelled" | "expired";
  current_period_start: string;   // ISO 8601
  current_period_end: string;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;
}
```

### PricingBreakdown
```typescript
interface PricingBreakdown {
  plan_name: string;
  total_quantity: number;
  tier_breakdown: TierCost[];
  total_monthly_cost: number;     // In cents
  average_per_license: number;    // In currency (e.g., 9.33)
  savings_vs_individual: number;  // In cents
  currency: string;
}

interface TierCost {
  range: string;                  // "1-10", "11-25", "26+"
  quantity: number;
  unit_price: number;             // In cents
  subtotal: number;               // In cents
}
```

---

## 👤 User Workflows

### Workflow 1: Teacher Purchases 30 Licenses for Class

1. **View Plans**
   - `GET /api/v1/subscription-plans`
   - Filter plans with `use_tiered_pricing: true`

2. **Calculate Pricing**
   - User adjusts quantity slider (1-50)
   - Call `GET /api/v1/subscription-plans/pricing-preview?subscription_plan_id=xxx&quantity=30`
   - Display breakdown in real-time

3. **Purchase**
   - `POST /api/v1/user-subscriptions/purchase-bulk`
   - Redirect to Stripe checkout (or handle payment)
   - On success, show batch details

4. **View Batch**
   - `GET /api/v1/subscription-batches`
   - Display batch with 30 total, 0 assigned, 30 available

5. **Assign to Students**
   - For each student:
     - `POST /api/v1/subscription-batches/:id/assign` with `{"user_id": "student-xxx"}`
   - Or bulk assign via CSV upload

6. **Monitor Usage**
   - `GET /api/v1/subscription-batches/:id/licenses`
   - Show table: Student Name | Email | Status | Assigned Date

### Workflow 2: Adding More Licenses Mid-Subscription

1. **View Current Batch**
   - `GET /api/v1/subscription-batches`
   - Identify batch needing more licenses

2. **Calculate New Cost**
   - `GET /api/v1/subscription-plans/pricing-preview?subscription_plan_id=xxx&quantity=40`
   - Show proration calculation (done by Stripe)

3. **Update Quantity**
   - `PATCH /api/v1/subscription-batches/:id/quantity` with `{"new_quantity": 40}`
   - Stripe prorates the difference automatically

4. **Assign New Licenses**
   - `POST /api/v1/subscription-batches/:id/assign` for new students

### Workflow 3: Revoking a License

1. **Find License**
   - `GET /api/v1/subscription-batches/:id/licenses`
   - Identify license assigned to departing student

2. **Revoke**
   - `DELETE /api/v1/subscription-batches/:id/licenses/:license_id/revoke`

3. **Reassign** (Optional)
   - License returns to pool
   - `POST /api/v1/subscription-batches/:id/assign` with new user

---

## 🎨 UI/UX Requirements

### 1. **Pricing Calculator Component**

**Location**: Plan selection page

**Features**:
- Quantity slider/input (1-100+)
- Real-time pricing preview
- Visual breakdown by tier
- Savings badge

**Example UI**:
```
┌────────────────────────────────────────┐
│ Trainer Plan                           │
│                                        │
│ How many licenses do you need?         │
│ [■■■■■■■■■■░░░░░░░░░░] 30 licenses    │
│                                        │
│ Pricing Breakdown:                     │
│ ┌────────────────────────────────────┐ │
│ │ 1-5 licenses:   5 × €12 = €60    │ │
│ │ 6-15 licenses: 10 × €10 = €100   │ │
│ │ 16-30 licenses: 15 × €8 = €120   │ │
│ └────────────────────────────────────┘ │
│                                        │
│ Total: €280/month                      │
│ Average: €9.33/license                 │
│ 💰 Save €80 vs individual pricing!    │
│                                        │
│ [Purchase 30 Licenses]                 │
└────────────────────────────────────────┘
```

### 2. **License Management Dashboard**

**Location**: User dashboard (for purchasers)

**Features**:
- List of all batches
- Quick stats: Total, Assigned, Available
- Actions: View details, Add licenses, Manage assignments

**Example UI**:
```
┌─────────────────────────────────────────────────────────────┐
│ My License Batches                              [+ Purchase]│
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Trainer Plan - Class 2024A                             │ │
│ │ 30 Total │ 25 Assigned │ 5 Available                   │ │
│ │ Renews: Feb 1, 2025                                     │ │
│ │ [View Details] [Add Licenses] [Manage]                 │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Basic Plan - Training Center                           │ │
│ │ 50 Total │ 50 Assigned │ 0 Available ⚠️                 │ │
│ │ Renews: Mar 15, 2025                                    │ │
│ │ [View Details] [Add Licenses] [Manage]                 │ │
│ └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

### 3. **License Assignment Interface**

**Location**: Batch details page

**Features**:
- Table of all licenses (assigned & available)
- Bulk actions: Import CSV, Assign all
- Individual actions: Assign, Revoke, View user
- Filters: Show assigned, Show available

**Example UI**:
```
┌─────────────────────────────────────────────────────────────┐
│ Trainer Plan - Class 2024A                                  │
│ 30 Total │ 25 Assigned │ 5 Available                        │
│                                                              │
│ [Assign License] [Import CSV] [Export]  Filters: [All ▼]   │
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐│
│ │ User          │ Email              │ Status    │ Actions ││
│ ├──────────────────────────────────────────────────────────┤│
│ │ John Doe      │ john@school.com    │ Active    │ Revoke  ││
│ │ Jane Smith    │ jane@school.com    │ Active    │ Revoke  ││
│ │ ...           │ ...                │ ...       │ ...     ││
│ │ (Unassigned)  │ -                  │ Available │ Assign  ││
│ │ (Unassigned)  │ -                  │ Available │ Assign  ││
│ └──────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

### 4. **Assign License Modal**

**Trigger**: Click "Assign" button

**Features**:
- User search/autocomplete
- Email input (if user doesn't exist, send invite)
- Confirmation message

**Example UI**:
```
┌────────────────────────────────────────┐
│ Assign License                         │
│                                        │
│ Search for user:                       │
│ [jane@school.com        ] 🔍          │
│                                        │
│ Results:                               │
│ ┌────────────────────────────────────┐ │
│ │ ● Jane Smith (jane@school.com)    │ │
│ │   Already registered              │ │
│ │                                    │ │
│ │   [Assign to this user]           │ │
│ └────────────────────────────────────┘ │
│                                        │
│ Or send email invite:                  │
│ [Send Invitation Email]                │
│                                        │
│ [Cancel]                               │
└────────────────────────────────────────┘
```

---

## 💡 Implementation Notes

### Displaying Tiered Pricing

When displaying plans to users, check the `use_tiered_pricing` field:

**Simple Pricing** (`use_tiered_pricing: false`):
- Display `price_amount` as the per-license cost
- Calculate total: `price_amount × quantity`

**Tiered Pricing** (`use_tiered_pricing: true`):
- Show the `pricing_tiers` array with ranges and prices
- Use the pricing preview API for accurate calculations
- Display savings compared to individual pricing

**Example Display Logic**:
```
if (plan.use_tiered_pricing) {
  // Show tier breakdown: "1-5 licenses: €4.00 each"
  // Call GET /pricing-preview for exact total
} else {
  // Show simple price: "€12.00 per license"
  // Calculate: price_amount × quantity
}
```

### Handling the `max_quantity` Field

In `pricing_tiers`, the `max_quantity` field uses a special convention:
- **0 = unlimited** (last tier, e.g., "31+ licenses")
- **Non-zero** = hard limit for that tier (e.g., "1-5 licenses")

**Display Examples**:
- Tier with `max_quantity: 5` → "1-5 licenses"
- Tier with `max_quantity: 0` → "31+ licenses" or "31-∞ licenses"

---

## ⚠️ Error Handling

### Common Errors

| Status | Error | Meaning | User Action |
|--------|-------|---------|-------------|
| 403 | Feature not available | User's plan doesn't include `bulk_purchase` | Upgrade plan |
| 400 | No available licenses | All licenses in batch are assigned | Add more licenses |
| 400 | Cannot reduce quantity | Trying to reduce below assigned count | Revoke licenses first |
| 404 | Batch not found | Invalid batch ID | Check batch list |
| 401 | Unauthorized | Invalid or expired token | Re-login |

### Error Response Format

```json
{
  "error_code": 403,
  "error_message": "Your current plan does not include bulk_purchase. Please upgrade your subscription."
}
```

### Recommended UI Messages

```typescript
const ERROR_MESSAGES = {
  403: "Your plan doesn't support bulk purchases. Please upgrade to continue.",
  400: "All licenses are assigned. Add more licenses or revoke existing ones.",
  404: "License batch not found. It may have been cancelled.",
  500: "Something went wrong. Please try again or contact support.",
};
```

---

## 🧪 Testing Guide

### Test Scenarios

#### 0. **Stripe Import** (Admin Only)
```bash
# Import/sync plans from Stripe
curl -X POST http://localhost:8080/api/v1/subscription-plans/import-stripe \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json"

# Expected response:
# {
#   "processed_plans": 4,
#   "created_plans": 1,
#   "updated_plans": 3,
#   "created_details": [
#     "Created plan: XS (Stripe price: price_1SJdyX..., pricing: tiered (4 tiers))"
#   ],
#   "updated_details": [...]
# }

# Verify tiered pricing was imported correctly
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/subscription-plans | \
  jq '.data[] | select(.name == "XS") | {name, use_tiered_pricing, tier_count: (.pricing_tiers | length)}'

# Expected output:
# {
#   "name": "XS",
#   "use_tiered_pricing": true,
#   "tier_count": 4
# }
```

#### 1. **Pricing Preview**
```bash
# Test various quantities
curl "http://localhost:8080/api/v1/subscription-plans/pricing-preview?subscription_plan_id=<PLAN_ID>&quantity=5"
curl "http://localhost:8080/api/v1/subscription-plans/pricing-preview?subscription_plan_id=<PLAN_ID>&quantity=30"
curl "http://localhost:8080/api/v1/subscription-plans/pricing-preview?subscription_plan_id=<PLAN_ID>&quantity=100"

# Verify tier calculations match expectations
```

#### 2. **Bulk Purchase**
```bash
curl -X POST http://localhost:8080/api/v1/user-subscriptions/purchase-bulk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "subscription_plan_id": "<PLAN_ID>",
    "quantity": 10
  }'

# Verify:
# - Batch created with correct quantity
# - 10 UserSubscription records created
# - All licenses are "unassigned"
```

#### 3. **License Assignment**
```bash
# List batches
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/subscription-batches

# Get licenses in batch
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/subscription-batches/<BATCH_ID>/licenses

# Assign a license
curl -X POST http://localhost:8080/api/v1/subscription-batches/<BATCH_ID>/assign \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "student-123"}'

# Verify:
# - License status changed to "active"
# - user_id set correctly
# - assigned_quantity incremented
```

#### 4. **License Revocation**
```bash
curl -X DELETE http://localhost:8080/api/v1/subscription-batches/<BATCH_ID>/licenses/<LICENSE_ID>/revoke \
  -H "Authorization: Bearer $TOKEN"

# Verify:
# - License status back to "unassigned"
# - user_id cleared
# - assigned_quantity decremented
```

#### 5. **Quantity Update**
```bash
# Scale up
curl -X PATCH http://localhost:8080/api/v1/subscription-batches/<BATCH_ID>/quantity \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"new_quantity": 20}'

# Verify:
# - 10 new licenses created
# - total_quantity updated to 20
```

### Edge Cases to Test

1. **Permissions**:
   - User without `bulk_purchase` feature tries to purchase → 403
   - User tries to assign license from another user's batch → 403

2. **Constraints**:
   - Try to reduce quantity below assigned count → 400
   - Try to assign when no licenses available → 400

3. **Pricing Tiers**:
   - Purchase exactly at tier boundary (5, 15, 30)
   - Purchase quantity spanning multiple tiers (e.g., 25)
   - Purchase in unlimited tier (40+)

---

## 📚 Additional Notes

### Recent Updates (January 2025)

#### ✅ Fixed: Tiered Pricing Display Issue
**Problem**: Plans with tiered pricing were being saved correctly to the database but API responses showed `use_tiered_pricing: false` with empty `pricing_tiers` array.

**Root Cause**: The entity registration converter (`subscriptionPlanRegistration.go`) was missing the tiered pricing fields when converting from model to DTO.

**Solution**: Updated `subscriptionPlanPtrModelToOutput()` to include:
- Convert `plan.PricingTiers` (model) → `dto.PricingTier` array
- Include `UseTieredPricing` field in output
- Include `PricingTiers` array in output

**Files Modified**:
- `/src/payment/entityRegistration/subscriptionPlanRegistration.go:63-110`

**Verification**:
```bash
# All plans now correctly display tiered pricing
GET /api/v1/subscription-plans
# XS plan now shows:
# "use_tiered_pricing": true
# "pricing_tiers": [4 tiers with proper min/max/amount]
```

#### ✅ Added: Stripe Import Functionality
**Feature**: New endpoint to import plans from Stripe Dashboard into database (reverse sync).

**Key Implementation Details**:
- Uses `priceParams.AddExpand("data.tiers")` to fetch tier data from Stripe API
- Automatically detects volume/graduated pricing schemes
- Converts Stripe's `UpTo` boundaries to `min_quantity`/`max_quantity` ranges
- Handles unlimited tiers (where `UpTo = 0`)

**Endpoint**: `POST /api/v1/subscription-plans/import-stripe`

**Files Created/Modified**:
- `/src/payment/services/stripeService.go:1987-2200` - Import logic
- `/src/payment/routes/userSubscriptionController.go` - Controller
- `/src/payment/routes/subscriptionPlanRoutes.go` - Route registration

**Use Cases**:
1. Initial setup: Import existing Stripe products
2. New plans: Create plan in Stripe Dashboard, then import
3. Updates: Modify pricing in Stripe Dashboard, then sync

### Feature Flags

To enable bulk purchase for a plan, ensure it has the correct feature:

```sql
UPDATE subscription_plans
SET features = features || '["bulk_purchase"]'::jsonb
WHERE name = 'Trainer Plan';
```

### Stripe Integration (TODO)

The current implementation uses placeholder Stripe IDs. To complete Stripe integration:

1. **Create Tiered Prices in Stripe Dashboard**:
   - Create Product: "Trainer Plan"
   - Add Price with `billing_scheme: tiered`
   - Define tiers matching your pricing model

2. **Update `bulkLicenseService.go`** (line 46):
   Replace placeholder with actual Stripe call:
   ```go
   stripeSub, err := s.stripeService.CreateSubscriptionWithQuantity(
       purchaserUserID,
       plan,
       input.Quantity,
   )
   ```

3. **Handle Stripe Webhooks**:
   - `invoice.payment_succeeded` - Mark subscription active
   - `invoice.payment_failed` - Mark subscription past_due
   - `customer.subscription.updated` - Update quantity/status

### Database Schema

```sql
-- New tables created automatically via AutoMigrate
SELECT * FROM subscription_batches;
SELECT * FROM user_subscriptions WHERE subscription_batch_id IS NOT NULL;
```

### Performance Considerations

- **Pagination**: List endpoints support pagination (add `?page=1&limit=20`)
- **Caching**: Cache pricing previews for 5 minutes
- **Bulk Operations**: Batch assign/revoke operations for better performance

---

## 🎉 Summary

You now have a complete bulk license management system with:

✅ **Tiered pricing** with volume discounts
✅ **Bulk purchase** API
✅ **License assignment/revocation**
✅ **Dynamic quantity updates**
✅ **Feature-based access control**
✅ **Real-time pricing preview**

### Quick Start Checklist for Frontend

- [ ] Implement pricing calculator component
- [ ] Create bulk purchase flow
- [ ] Build license management dashboard
- [ ] Add license assignment interface
- [ ] Handle all error states
- [ ] Test with real Stripe integration
- [ ] Add loading states and optimistic updates

---
  🚀 Deployment Checklist

  - Backend code implemented
  - Database migrations ready
  - API endpoints tested
  - Feature gates in place
  - Documentation complete
  - Stripe integration (API keys needed)
  - Frontend UI implementation
  - End-to-end testing

  ---
  🎯 Key Features Summary

  | Feature             | Status     | Notes                                   |
  |---------------------|------------|-----------------------------------------|
  | Tiered Pricing      | ✅ Complete | 4 tiers defined in sample plans         |
  | Pricing Preview API | ✅ Complete | Public endpoint, no auth                |
  | Bulk Purchase       | ✅ Complete | Feature-gated, creates batch + licenses |
  | License Assignment  | ✅ Complete | Assign/revoke/reassign                  |
  | Quantity Scaling    | ✅ Complete | Add/remove licenses dynamically         |
  | Feature Middleware  | ✅ Complete | Checks bulk_purchase in plan features   |
  | Sample Data         | ✅ Complete | 2 plans created on first startup        |
  | Stripe Placeholders | ✅ Complete | Ready for API key integration           |
  | Frontend Docs       | ✅ Complete | Full guide with code examples           |

  ---
  💡 Example: Teacher Workflow

  1. Teacher visits Plan Selection page
  2. Selects "Trainer Plan" with tiered pricing
  3. Adjusts slider to 30 students
  4. Sees breakdown:
    - 5 licenses × €12 = €60
    - 10 licenses × €10 = €100
    - 15 licenses × €8 = €120
    - Total: €280/month (saves €80!)
  5. Clicks "Purchase 30 Licenses"
  6. Goes to License Dashboard
  7. Clicks "Assign License" for each student
  8. Mid-year: 5 more students join
  9. Clicks "Add 5 Licenses" → Stripe prorates automatically


# Frontend Testing Required - Recent API Changes

**Date**: 2025-10-17
**Backend Changes**: Group management, terminal filtering improvements, bulk terminal creation, and admin panel features

## Summary

Four features have been implemented or planned that require frontend testing and integration:

1. **New filter on terminal sessions endpoint** - Group-based terminal filtering
2. **Fixed group owner assignment** - `owner_user_id` now properly populated
3. **Bulk terminal creation for groups** - Create terminals for all group members in one API call
4. **Stripe invoice cleanup (Admin Panel)** - Backend API ready with selective cleanup support, frontend UI needed

---

## 1. Terminal Sessions - Group Filter (NEW FEATURE)

### API Change

**Endpoint**: `GET /api/v1/terminals/user-sessions`

**New Query Parameter**: `group_id` (optional)

### Behavior

- **Without `group_id`**: Returns user's own terminals (existing behavior - unchanged)
- **With `group_id`**: Returns terminals shared with the specified group

### Example Usage

```bash
# Get user's own terminals (existing behavior)
GET /api/v1/terminals/user-sessions?include_hidden=true

# Get terminals shared with a specific group (NEW)
GET /api/v1/terminals/user-sessions?group_id=0199f416-2ec9-7087-8939-57937480b13c&include_hidden=false
```

### Response Format

Response structure is **unchanged** - same terminal session array format.

### Frontend Testing Checklist

- [ ] Verify existing terminal list functionality still works (without `group_id` parameter)
- [ ] Test group filter - pass a valid group ID and verify only group-shared terminals appear
- [ ] Test with invalid group ID format - should return 400 error with helpful message
- [ ] Test combination of `group_id` + `include_hidden` parameters
- [ ] Verify UI correctly displays group-shared terminals vs. user-owned terminals
- [ ] Check that terminal actions (connect, delete, hide) work on group-shared terminals

### Potential UI Considerations

- May want to add a group selector/filter in the terminals view
- Consider showing visual indicator for group-shared vs. personally owned terminals
- Check permission levels (read/write/admin) when accessing group-shared terminals

---

## 2. Group Owner Assignment (BUG FIX)

### Issue Fixed

Previously, when creating a group, the `owner_user_id` field was empty in the response. This has been fixed.

### API Affected

**Endpoint**: `POST /api/v1/class-groups`

### What Changed

- `owner_user_id` is now automatically set to the authenticated user's ID during group creation
- Owner is automatically added as a group member with "owner" role
- Owner receives full permissions on the group

### Before (Broken)

```json
{
    "id": "...",
    "owner_user_id": "",  // ❌ Empty
    "name": "my-group",
    "display_name": "My Group"
}
```

### After (Fixed)

```json
{
    "id": "0199f416-2ec9-7087-8939-57937480b13c",
    "owner_user_id": "1d660660-7637-4a5d-9d1e-8d05bbf7363f",  // ✅ Populated
    "name": "my-group",
    "display_name": "My Group"
}
```

### Frontend Testing Checklist

- [ ] Create a new group and verify `owner_user_id` is populated in the response
- [ ] Verify the authenticated user appears in the group members list as "owner"
- [ ] Check that group owner has full permissions (edit, delete, manage members)
- [ ] Test `GET /api/v1/class-groups/{id}` - verify `owner_user_id` is present
- [ ] Test `GET /api/v1/class-groups` list - verify all groups show their owners

### Potential UI Considerations

- If you were working around the empty `owner_user_id`, remove any workarounds
- Display the group owner in the UI (e.g., "Created by: [owner name]")
- Use `owner_user_id` to determine if current user can edit/delete the group
- Show "owner" badge on the user who created the group in member lists

---

## Authentication

Both endpoints require authentication. Use the standard JWT bearer token:

```bash
curl -X GET "http://localhost:8080/api/v1/terminals/user-sessions?group_id=xxx" \
  -H "Authorization: Bearer $TOKEN"
```

Test credentials:
- Email: `1.supervisor@test.com`
- Password: `test`

---

## 3. Bulk Terminal Creation for Groups (NEW FEATURE)

### API Change

**Endpoint**: `POST /api/v1/class-groups/{groupId}/bulk-create-terminals`

**Purpose**: Create terminal sessions for all active members of a group in a single API call, replacing the need to loop through members on the frontend.

### Request Body

```json
{
  "terms": "I accept the terms of service...",
  "expiry": 3600,
  "instance_type": "debian",
  "name_template": "{group_name} - {user_email}"
}
```

**Name Template Variables:**
- `{group_name}` - Group's display name
- `{user_email}` - Member's email address
- `{user_id}` - Member's user ID

**Example**: Template `"{group_name} - {user_email}"` → `"DevOps Class - student1@example.com"`

### Response Format

```json
{
  "success": true,
  "created_count": 15,
  "failed_count": 0,
  "total_members": 15,
  "terminals": [
    {
      "user_id": "user123",
      "user_email": "student1@example.com",
      "terminal_id": "term-uuid",
      "session_id": "session-id",
      "name": "DevOps Class - student1@example.com",
      "success": true,
      "error": null
    }
    // ... one entry per group member
  ],
  "errors": []
}
```

### Frontend Testing Checklist

- [ ] Verify only group owner/admin can call this endpoint (403 for regular members)
- [ ] Test with valid group ID - should create terminals for all active members
- [ ] Test with invalid group ID - should return 404
- [ ] Verify name template works correctly with all placeholders
- [ ] Check partial success scenario (some terminals succeed, some fail)
- [ ] Verify `created_count` and `failed_count` match actual results
- [ ] Test with empty name_template (should use default: "{group_name} - {user_email}")
- [ ] Confirm subscription plan limits are enforced (instance_type, expiry)
- [ ] Check UI handles loading state during bulk creation
- [ ] Verify error messages are displayed for failed terminal creations

### Benefits vs. Frontend Loop

**Old Approach:**
```javascript
// Less efficient: N separate API calls
for (const member of members) {
  await POST /terminals/start-session {
    terms, expiry, instance_type, name
  }
}
```

**New Approach:**
```javascript
// Single API call, atomic transaction
await POST /class-groups/{groupId}/bulk-create-terminals {
  terms, expiry, instance_type, name_template
}
```

**Advantages:**
1. **Performance**: Single network request instead of N requests
2. **Atomicity**: Backend handles partial failures gracefully
3. **Quota Checking**: Backend validates subscription limits before creating any terminals
4. **Audit Trail**: Single operation log instead of N separate logs
5. **Error Handling**: Backend provides detailed per-user error reporting

### Potential UI Considerations

- Add "Create Terminals for All Members" button in group management UI
- Show progress indicator during bulk creation
- Display summary of results: "15/15 terminals created successfully"
- Show expandable list of per-user results (success/failure with errors)
- Consider retry mechanism for failed terminal creations
- Add confirmation dialog with estimated resource usage before bulk creation

---

## 4. Stripe Invoice Cleanup - Admin Panel (PLANNED FEATURE)

### Overview

A Stripe invoice cleanup system has been implemented in the backend service layer (`stripeService.CleanupIncompleteInvoices`) but **requires an API endpoint and admin panel UI** to be usable.

**Purpose**: Allow administrators to clean up old, incomplete invoices in Stripe to maintain a clean billing system and reduce clutter.

### Backend Implementation Status

✅ **Service Layer**: Fully implemented in `src/payment/services/stripeService.go:1803-1957`

✅ **API Endpoint**: `POST /api/v1/invoices/admin/cleanup` (implemented in `invoiceController.go:179-219`)

✅ **Swagger Documentation**: Updated and available at `http://localhost:8080/swagger/`

✅ **Selective Cleanup**: NEW! Support for `invoice_ids` parameter to cleanup specific invoices

❌ **Frontend UI**: Not yet implemented (admin panel required)

### What the Cleanup Functionality Does

The backend service can perform two cleanup actions on incomplete Stripe invoices:

1. **Void Action**:
   - For **draft** invoices → Deletes them (Stripe API: `invoice.Delete`)
   - For **open** invoices → Voids them permanently (Stripe API: `invoice.Void`)
   - Result: Invoice is canceled and cannot be reopened

2. **Mark Uncollectible**: Keeps the invoice record but stops collection attempts (works for both draft and open invoices)

**Features:**
- Filter by invoice status: `draft`, `open`, or `uncollectible`
- Filter by age: Clean invoices older than N days
- **Dry Run Mode**: Preview what will be cleaned without making changes
- Detailed reporting: Shows exactly which invoices were processed, cleaned, skipped, or failed

### API Endpoint That Needs to Be Created

**Proposed Endpoint**: `POST /api/v1/invoices/admin/cleanup`

**Permissions**: Administrator role only

**Request Body (Full Cleanup):**
```json
{
  "action": "void",           // "void" or "uncollectible"
  "older_than_days": 30,      // Cleanup invoices older than N days (0 = all invoices, no age filter)
  "dry_run": true,            // If true, preview only (don't make changes)
  "status": "open"            // Optional: "draft", "open", or "uncollectible"
}
```

**Request Body (Selective Cleanup - NEW!):**
```json
{
  "action": "void",
  "older_than_days": 0,       // Still required, but ignored when invoice_ids is provided
  "dry_run": false,           // Execute the cleanup
  "invoice_ids": [            // Optional: specific invoice IDs to clean
    "in_1234567890",
    "in_9876543210",
    "in_abcdefghij"
  ]
}
```

**Field Details:**
- `older_than_days`: **Integer ≥ 0** (REQUIRED - `0` is now supported!)
  - `0` = Cleanup ALL incomplete invoices (no age restriction)
  - `1` = Cleanup invoices older than 1 day
  - `30` = Cleanup invoices older than 30 days (recommended default)
  - Common values: `0`, `7`, `30`, `60`, `90`
  - **Note**: Backend uses pointer type to properly handle `0` value in validation
  - **When `invoice_ids` is provided**: Age filter is ignored (selective mode)

- `invoice_ids`: **Array of strings** (OPTIONAL - enables selective cleanup)
  - If empty/omitted: Cleanup ALL invoices matching the filters
  - If provided: Cleanup ONLY the specified invoice IDs
  - **Use case**: Two-step workflow (preview → select → cleanup)

**Response:**
```json
{
  "dry_run": true,
  "action": "void",
  "processed_invoices": 45,
  "cleaned_invoices": 12,
  "skipped_invoices": 30,
  "failed_invoices": 3,
  "total_amount_cleaned": 245000,  // In cents ($2,450.00)
  "currency": "usd",
  "cleaned_details": [
    {
      "invoice_id": "in_1234567890",
      "invoice_number": "INV-2024-001",
      "customer_id": "cus_abc123",
      "amount": 2500,              // In cents ($25.00)
      "currency": "usd",
      "original_status": "open",
      "action_taken": "voided",    // Can be: "deleted", "voided", or "marked_uncollectible"
      "created_at": "2024-01-15 14:30:00"
    }
    // ... more details
  ],
  "skipped_details": [
    "Invoice in_xxx too recent (created 2024-12-01)",
    "Invoice in_yyy already uncollectible"
  ],
  "failed_details": [
    {
      "invoice_id": "in_failed123",
      "customer_id": "cus_xyz",
      "error": "Invoice already paid"
    }
  ]
}
```

### Admin Panel UI Requirements

**Recommended Features:**

1. **Cleanup Configuration Form:**
   - Action selector: Radio buttons for "Void" vs "Mark Uncollectible"
   - Age filter: Number input for "Older than X days" (min: 0, default: 30)
     - Hint text: "Use 0 to cleanup all invoices regardless of age"
   - Status filter: Dropdown for "Draft", "Open", "All incomplete"
   - Dry Run toggle: Checkbox for "Preview only (don't make changes)" (default: checked)

2. **Preview Results (Dry Run):**
   - Display summary statistics before executing
   - Show list of invoices that will be affected
   - Display total amount that will be cleaned
   - Require confirmation before actual cleanup

3. **Results Display:**
   - Success/failure counts
   - Total amount cleaned
   - Expandable table with per-invoice details
   - Show skipped invoices with reasons
   - Highlight failed invoices with error messages

4. **Safety Features:**
   - Default to dry_run = true
   - Require explicit confirmation before actual cleanup
   - Show warning message: "This action cannot be undone"
   - Display age threshold clearly (e.g., "Will clean invoices older than 30 days")

### Implementation Steps for Frontend Team

1. **Frontend Implementation:**
   - Create admin panel page/section for "Invoice Management"
   - Add cleanup configuration form with validation
   - Implement dry-run preview before actual cleanup
   - Add results table with filtering/sorting
   - Show loading indicator during cleanup operation
   - Add export functionality for cleanup reports

2. **Testing Checklist:**
   - [ ] Verify only administrators can access the cleanup endpoint
   - [ ] Test dry-run mode shows accurate preview
   - [ ] Verify actual cleanup performs as previewed
   - [ ] Test with different age thresholds (7, 30, 60, 90 days)
   - [ ] Test with different statuses (draft, open, all)
   - [ ] Verify "void" action works correctly
   - [ ] Verify "mark uncollectible" action works correctly
   - [ ] Check error handling for failed cleanups
   - [ ] Verify skipped invoices are correctly identified
   - [ ] Test with large number of invoices (pagination)

### Use Cases

**Scenario 1: Regular Maintenance**
- Run monthly cleanup of draft invoices older than 30 days
- Mark them as uncollectible to keep records

**Scenario 2: Payment System Migration**
- Void all old open invoices before switching payment providers
- Clean up historical data

**Scenario 3: Billing Error Cleanup**
- Remove failed invoice drafts from testing
- Void incorrectly generated invoices

### Security Considerations

- ✅ Administrator role required (enforced by Casbin)
- ✅ Dry-run mode prevents accidental data loss
- ✅ Detailed audit trail in response
- ✅ Age threshold prevents cleaning recent invoices
- ⚠️ **Action is irreversible** - UI must make this clear

### Documentation References

- **Backend Code**: `src/payment/services/stripeService.go:1802-1932`
- **DTOs**: `src/payment/dto/subscriptionDto.go:327-363`
- **Stripe API Docs**:
  - [Void Invoice](https://stripe.com/docs/api/invoices/void)
  - [Mark Uncollectible](https://stripe.com/docs/api/invoices/mark_uncollectible)

### Example UI Workflow

#### Option 1: Full Cleanup (Simple)
```
1. Admin navigates to "Invoice Management" in admin panel
   ↓
2. Fills out cleanup form:
   - Action: Void
   - Older than: 30 days
   - Status: Open
   - Dry Run: ✓ Enabled
   ↓
3. Clicks "Preview Cleanup"
   ↓
4. System shows: "12 invoices will be voided ($2,450.00 total)"
   - Table shows invoice details
   ↓
5. Admin reviews and clicks "Confirm Cleanup"
   ↓
6. System disables dry-run and re-submits
   ↓
7. Results page shows:
   ✓ 12 invoices voided
   ✓ 30 skipped (too recent)
   ✗ 3 failed (with error messages)
```

#### Option 2: Selective Cleanup (Recommended UX)
```
1. Admin navigates to "Invoice Management" in admin panel
   ↓
2. Fills out cleanup form:
   - Action: Void
   - Older than: 30 days
   - Status: Open
   - Dry Run: ✓ Enabled
   ↓
3. Clicks "Preview Cleanup" (dry_run=true, no invoice_ids)
   ↓
4. System shows preview table with 45 invoices:
   ┌────────────────┬──────────┬──────────┬────────────┬──────────┐
   │ ☑ Invoice ID   │ Customer │ Amount   │ Status     │ Created  │
   ├────────────────┼──────────┼──────────┼────────────┼──────────┤
   │ ☑ in_12345     │ cus_abc  │ $25.00   │ draft      │ 2024-01  │
   │ ☑ in_67890     │ cus_def  │ $50.00   │ open       │ 2024-02  │
   │ ☐ in_11111     │ cus_ghi  │ $100.00  │ open       │ 2024-03  │
   └────────────────┴──────────┴──────────┴────────────┴──────────┘

   [ Select All ] [ Deselect All ]

   Selected: 2 invoices, $75.00 total
   ↓
5. Admin reviews and UNCHECKS invoices they want to keep
   - Can filter/sort table
   - Can search by invoice ID, customer, etc.
   ↓
6. Clicks "Clean Selected Invoices"
   ↓
7. System sends request with:
   {
     "action": "void",
     "older_than_days": 0,
     "dry_run": false,
     "invoice_ids": ["in_12345", "in_67890"]  // Only selected IDs
   }
   ↓
8. Results page shows:
   ✓ 2 invoices voided ($75.00)
   ⊘ 43 invoices skipped (not selected)
```

### 🎯 Frontend Implementation Note: Selective Cleanup

**NEW FEATURE**: The cleanup endpoint now supports selective invoice cleanup through the `invoice_ids` parameter.

**Recommended Two-Step User Flow:**

**Step 1: Preview (Discovery)**
```javascript
// User fills out cleanup criteria
const previewRequest = {
  action: 'void',
  older_than_days: 30,
  status: 'open',
  dry_run: true          // Preview mode
  // NO invoice_ids - get all matching invoices
};

const preview = await POST('/api/v1/invoices/admin/cleanup', previewRequest);

// Display results with checkboxes
preview.cleaned_details.forEach(invoice => {
  // Render selectable table row
  // All invoices pre-selected by default
});
```

**Step 2: Selective Cleanup (Execution)**
```javascript
// User deselects invoices they want to keep
const selectedInvoiceIds = getSelectedInvoices(); // ["in_123", "in_456"]

const cleanupRequest = {
  action: 'void',
  older_than_days: 0,    // Ignored in selective mode
  dry_run: false,        // Execute cleanup
  invoice_ids: selectedInvoiceIds  // ONLY clean these
};

const result = await POST('/api/v1/invoices/admin/cleanup', cleanupRequest);
```

**Key Implementation Points:**

1. **Preview Table:**
   - Show checkboxes for each invoice
   - Default: All invoices selected
   - Allow select all / deselect all
   - Show running total of selected invoices + amount
   - Display invoice details: ID, customer, amount, status, date

2. **Confirmation:**
   - Before executing, show summary: "You are about to void 5 invoices ($125.00)"
   - Require explicit confirmation

3. **Safety:**
   - Disable "Clean All" button if > 50 invoices without selection review
   - Show warning if cleaning all invoices

4. **Validation:**
   - `older_than_days` is still required (use `0` in selective mode)
   - `invoice_ids` is optional (empty = clean all matching)
   - Backend ignores age filter when `invoice_ids` is provided

**Benefits:**
- ✅ User can review BEFORE cleanup
- ✅ User can exclude specific invoices
- ✅ Safer than "clean all"
- ✅ Better audit trail (see exactly what was cleaned)
- ✅ No risk of accidentally cleaning important invoices

**API Behavior:**
- `invoice_ids` empty → Clean all invoices matching filters (age, status)
- `invoice_ids` provided → Clean ONLY those invoices (age filter ignored)
- Invalid invoice IDs → Skipped with message "Invoice xxx not in selection"

---

## Questions or Issues?

If you encounter any problems during testing:
1. Check the Swagger docs at `http://localhost:8080/swagger/`
2. Verify the API response format matches expectations
3. Check browser console for any errors
4. Report issues with:
   - Expected behavior
   - Actual behavior
   - Request/response details
   - Browser console errors

---

## Backend Status

✅ Both features are implemented, tested, and deployed
✅ Swagger documentation updated
✅ Server logs show successful operation

**Backend Team**: Ready for frontend integration and testing
# Groups Management - Frontend Integration Guide

This document provides frontend developers with all necessary information to integrate the new groups management system.

## Overview

The groups management system allows users to create classes/teams and share terminals with entire groups instead of individual users. The system uses a hybrid approach with OCF database entities and Casbin for permissions.

### Feature Flag

The groups module is controlled by the `class_groups` feature flag:
- **Key**: `class_groups`
- **Module**: `groups`
- **Default**: Enabled (`enabled: true`)

You can check if the feature is enabled:
```http
GET /api/v1/features?filter[key]=class_groups
```

The feature can be toggled via API (admin only):
```http
PATCH /api/v1/features/{feature_id}
Content-Type: application/json

{
  "enabled": false
}
```

**Note**: Feature flags are global database toggles. This is different from subscription-based feature access (which controls whether a user's plan includes group functionality).

## Authentication Requirements

All endpoints require JWT authentication via Bearer token in the `Authorization` header:

```http
Authorization: Bearer {access_token}
```

Get the token via the login endpoint (see `CLAUDE.md` for test credentials).

## API Endpoints

Base URL: `http://localhost:8080/api/v1`

### Class Groups Endpoints

#### 1. List All Groups

```http
GET /api/v1/class-groups
```

**Query Parameters:**
- `page` (int, optional): Page number for pagination (default: 1)
- `pageSize` (int, optional): Items per page (default: 20)
- `includes` (string, optional): Comma-separated list of relations to preload (e.g., `Members`)

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "name": "group-slug",
      "displayName": "My Training Class",
      "description": "Description of the group",
      "ownerUserID": "user-id",
      "subscriptionPlanID": "uuid-or-null",
      "maxMembers": 50,
      "memberCount": 12,
      "expiresAt": "2025-12-31T23:59:59Z",
      "casdoorGroupName": "optional-casdoor-sync-name",
      "isActive": true,
      "isExpired": false,
      "isFull": false,
      "metadata": {},
      "createdAt": "2025-01-15T10:00:00Z",
      "updatedAt": "2025-01-15T10:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "pageSize": 20,
    "total": 5
  }
}
```

#### 2. Get Groups for Current User

```http
GET /api/v1/class-groups?filter[ownerUserID]={userId}
```

Or use the repository method to get all groups where user is a member (includes owned groups):

```http
GET /api/v1/class-groups?includes=Members&filter[Members.UserID]={userId}
```

#### 3. Create a Group

```http
POST /api/v1/class-groups
Content-Type: application/json
```

**Request Body:**
```json
{
  "name": "python-bootcamp-2025",
  "displayName": "Python Bootcamp 2025",
  "description": "6-week intensive Python training program",
  "organizationID": "uuid-or-null",
  "parentGroupID": "uuid-or-null",
  "subscriptionPlanID": "uuid-or-null",
  "maxMembers": 30,
  "expiresAt": "2025-12-31T23:59:59Z",
  "metadata": {
    "startDate": "2025-02-01",
    "instructor": "John Doe",
    "level": "intermediate"
  }
}
```

**Notes:**
- `ownerUserID` is automatically set to the authenticated user (via hook)
- The creator is automatically added as a member with `owner` role
- Casbin permissions are automatically granted to the owner
- `organizationID` (optional): Link group to an organization for cascading permissions
- `parentGroupID` (optional): Create a nested subgroup under an existing parent group
- For nested groups, see the "Nested Groups (Hierarchical)" section below

**Response:**
```json
{
  "id": "uuid",
  "name": "python-bootcamp-2025",
  "displayName": "Python Bootcamp 2025",
  "ownerUserID": "authenticated-user-id",
  "memberCount": 1,
  "isActive": true,
  ...
}
```

#### 4. Update a Group

```http
PATCH /api/v1/class-groups/{groupId}
Content-Type: application/json
```

**Request Body** (all fields optional):
```json
{
  "displayName": "Updated Display Name",
  "description": "Updated description",
  "organizationID": "uuid-or-null",
  "parentGroupID": "uuid-or-null",
  "maxMembers": 40,
  "isActive": false
}
```

**Permissions Required:**
- User must be the group owner or have admin role in the group

**Notes:**
- `parentGroupID` can be updated to move a group to a different parent or make it a top-level group (set to null)
- Changing `parentGroupID` does not affect permissions; only organizational hierarchy

#### 5. Delete a Group

```http
DELETE /api/v1/class-groups/{groupId}
```

**Notes:**
- Soft delete (sets `deletedAt` timestamp)
- Automatically removes all group members (cascade)
- Revokes all Casbin permissions for group members
- Only group owner can delete

### Group Members Endpoints

#### 1. List Group Members

```http
GET /api/v1/class-group-members?filter[groupID]={groupId}
```

**Query Parameters:**
- `includes` (string, optional): Preload relations (currently none defined)
- `filter[groupID]` (uuid): Filter by group ID
- `filter[role]` (string): Filter by role (owner, admin, assistant, member)
- `filter[isActive]` (bool): Filter by active status

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "groupID": "uuid",
      "userID": "user-id",
      "role": "admin",
      "invitedBy": "inviter-user-id",
      "joinedAt": "2025-01-15T10:00:00Z",
      "isActive": true,
      "metadata": {
        "invitationEmail": "user@example.com",
        "notes": "Teaching assistant"
      },
      "createdAt": "2025-01-15T10:00:00Z",
      "updatedAt": "2025-01-15T10:00:00Z"
    }
  ]
}
```

#### 2. Add Members to Group

**Note:** Use the service method for proper validation and permission handling.

```http
POST /api/v1/class-group-members
Content-Type: application/json
```

**Request Body:**
```json
{
  "groupID": "uuid",
  "userID": "user-to-add-id",
  "role": "member",
  "metadata": {
    "invitationEmail": "user@example.com"
  }
}
```

**Roles Available:**
- `owner` - Full control, can delete group
- `admin` - Can manage members and settings
- `assistant` - Can moderate but not manage
- `member` - Standard access

**Validations:**
- Group must not be full (`memberCount < maxMembers`)
- Group must not be expired (`expiresAt > now`)
- Group must be active (`isActive = true`)
- User must have admin or owner role to add members
- Cannot add duplicate members

**Auto-granted Permissions:**
- User automatically gets Casbin group permissions
- User can access group-shared terminals

#### 3. Update Member Role

```http
PATCH /api/v1/class-group-members/{memberId}
Content-Type: application/json
```

**Request Body:**
```json
{
  "role": "admin",
  "isActive": true
}
```

**Permissions Required:**
- User must be group owner or admin
- Cannot demote the group owner

#### 4. Remove Member from Group

```http
DELETE /api/v1/class-group-members/{memberId}
```

**Notes:**
- Soft delete by default
- Revokes Casbin group permissions
- User loses access to group-shared terminals
- Owner cannot be removed (must delete group instead)

### Nested Groups (Hierarchical)

Groups support parent-child hierarchical relationships, allowing you to create organizational structures like departments with classes or programs with cohorts.

#### Creating a Nested Group

To create a subgroup under a parent group, include `parent_group_id` in the create request:

```http
POST /api/v1/class-groups
Content-Type: application/json
```

**Example: Create a class under a program**
```json
{
  "name": "m1_devops_class_a",
  "displayName": "M1 DevOps - Class A",
  "description": "Morning class for M1 DevOps program",
  "parentGroupID": "uuid-of-parent-program",
  "organizationID": "uuid-of-organization",
  "maxMembers": 30,
  "expiresAt": "2025-06-30T23:59:59Z"
}
```

#### Moving Groups in the Hierarchy

You can change a group's parent by updating `parent_group_id`:

```http
PATCH /api/v1/class-groups/{groupId}
Content-Type: application/json
```

```json
{
  "parentGroupID": "new-parent-uuid-or-null"
}
```

**Notes:**
- Set `parentGroupID` to `null` to make a group top-level (remove from parent)
- The backend prevents circular references (e.g., making a group its own ancestor)
- Moving a group does NOT change permissions, only organizational structure

#### Fetching Nested Groups

**Get a group with its subgroups:**
```http
GET /api/v1/class-groups/{groupId}?include=SubGroups
```

**Response:**
```json
{
  "id": "parent-group-uuid",
  "name": "m1_devops_program",
  "displayName": "M1 DevOps Program",
  "parentGroupID": null,
  "subGroups": [
    {
      "id": "child-1-uuid",
      "name": "m1_devops_class_a",
      "displayName": "M1 DevOps - Class A",
      "parentGroupID": "parent-group-uuid",
      "memberCount": 25
    },
    {
      "id": "child-2-uuid",
      "name": "m1_devops_class_b",
      "displayName": "M1 DevOps - Class B",
      "parentGroupID": "parent-group-uuid",
      "memberCount": 28
    }
  ]
}
```

**Get a group with its parent:**
```http
GET /api/v1/class-groups/{groupId}?include=ParentGroup
```

**Response:**
```json
{
  "id": "child-group-uuid",
  "name": "m1_devops_class_a",
  "displayName": "M1 DevOps - Class A",
  "parentGroupID": "parent-group-uuid",
  "parentGroup": {
    "id": "parent-group-uuid",
    "name": "m1_devops_program",
    "displayName": "M1 DevOps Program",
    "memberCount": 150
  }
}
```

#### Common Hierarchy Patterns

**Pattern 1: School Organization**
```
Organization: "School of Paris"
├── Group: "M1 DevOps Program" (parentGroupID: null)
│   ├── Subgroup: "M1 DevOps - Class A" (parentGroupID: program-uuid)
│   ├── Subgroup: "M1 DevOps - Class B" (parentGroupID: program-uuid)
│   └── Subgroup: "M1 DevOps - Class C" (parentGroupID: program-uuid)
└── Group: "M2 Cloud Program" (parentGroupID: null)
    ├── Subgroup: "M2 Cloud - Class A" (parentGroupID: program-uuid)
    └── Subgroup: "M2 Cloud - Class B" (parentGroupID: program-uuid)
```

**Pattern 2: Company Departments**
```
Organization: "ACME Corp"
├── Group: "Engineering Department" (parentGroupID: null)
│   ├── Subgroup: "Backend Team" (parentGroupID: dept-uuid)
│   ├── Subgroup: "Frontend Team" (parentGroupID: dept-uuid)
│   └── Subgroup: "DevOps Team" (parentGroupID: dept-uuid)
└── Group: "Sales Department" (parentGroupID: null)
    ├── Subgroup: "EMEA Sales" (parentGroupID: dept-uuid)
    └── Subgroup: "Americas Sales" (parentGroupID: dept-uuid)
```

#### Important Notes

**Permissions:**
- Nested groups do NOT inherit permissions from parent groups
- Each group has independent members and permissions
- To access a subgroup, users must be:
  1. Direct members of the subgroup, OR
  2. Organization managers (if subgroup belongs to an organization)

**Use Cases:**
- Organizing classes within programs (education)
- Structuring teams within departments (corporate)
- Creating cohorts within courses (training)
- Grouping projects within portfolios (consulting)

**Bulk Import:**
- Nested groups can be created via CSV import (see bulk import documentation)
- Use the `parent_group` column to specify parent group name
- Parent groups must exist before creating children in the CSV

### Terminal Sharing with Groups

The existing terminal sharing system has been extended to support group sharing.

#### Share Terminal with Group

```http
POST /api/v1/terminal-shares
Content-Type: application/json
```

**Request Body:**
```json
{
  "terminalID": "uuid",
  "sharedWithGroupID": "group-uuid",
  "accessLevel": "read",
  "expiresAt": "2025-12-31T23:59:59Z",
  "metadata": {
    "purpose": "Lab exercise for class"
  }
}
```

**Access Levels:**
- `read` - View only
- `write` - Can execute commands
- `admin` - Full control including sharing

**Notes:**
- Either `sharedWithUserID` OR `sharedWithGroupID` must be provided (not both)
- All group members automatically get access to the terminal
- Share creator must have admin access to the terminal

#### List Terminal Shares for Group

```http
GET /api/v1/terminal-shares?filter[sharedWithGroupID]={groupId}
```

**Response:**
```json
{
  "data": [
    {
      "id": "uuid",
      "terminalID": "uuid",
      "sharedWithUserID": null,
      "sharedWithGroupID": "group-uuid",
      "sharedByUserID": "owner-user-id",
      "shareType": "group",
      "accessLevel": "read",
      "expiresAt": "2025-12-31T23:59:59Z",
      "isActive": true,
      "createdAt": "2025-01-15T10:00:00Z"
    }
  ]
}
```

**Share Types:**
- `shareType: "user"` - Shared with individual user
- `shareType: "group"` - Shared with entire group

## Frontend Implementation Examples

### React/TypeScript Example

```typescript
// types.ts
export interface ClassGroup {
  id: string;
  name: string;
  displayName: string;
  description: string;
  ownerUserID: string;
  subscriptionPlanID: string | null;
  maxMembers: number;
  memberCount: number;
  expiresAt: string | null;
  isActive: boolean;
  isExpired: boolean;
  isFull: boolean;
  metadata: Record<string, any>;
  createdAt: string;
  updatedAt: string;
}

export interface GroupMember {
  id: string;
  groupID: string;
  userID: string;
  role: 'owner' | 'admin' | 'assistant' | 'member';
  invitedBy: string;
  joinedAt: string;
  isActive: boolean;
  metadata: Record<string, any>;
}

export interface CreateGroupInput {
  name: string;
  displayName: string;
  description: string;
  subscriptionPlanID?: string;
  maxMembers?: number;
  expiresAt?: string;
  metadata?: Record<string, any>;
}

// api.ts
import axios from 'axios';

const API_BASE_URL = 'http://localhost:8080/api/v1';

export const groupsApi = {
  // List all groups
  listGroups: async (page = 1, pageSize = 20, includes?: string) => {
    const params = new URLSearchParams({
      page: page.toString(),
      pageSize: pageSize.toString(),
    });
    if (includes) params.append('includes', includes);

    const response = await axios.get<{ data: ClassGroup[] }>(
      `${API_BASE_URL}/class-groups?${params}`,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
        },
      }
    );
    return response.data;
  },

  // Create a group
  createGroup: async (input: CreateGroupInput) => {
    const response = await axios.post<ClassGroup>(
      `${API_BASE_URL}/class-groups`,
      input,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
          'Content-Type': 'application/json',
        },
      }
    );
    return response.data;
  },

  // Get group by ID
  getGroup: async (groupId: string, includes?: string) => {
    const params = includes ? `?includes=${includes}` : '';
    const response = await axios.get<ClassGroup>(
      `${API_BASE_URL}/class-groups/${groupId}${params}`,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
        },
      }
    );
    return response.data;
  },

  // Update group
  updateGroup: async (groupId: string, updates: Partial<CreateGroupInput>) => {
    const response = await axios.patch<ClassGroup>(
      `${API_BASE_URL}/class-groups/${groupId}`,
      updates,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
          'Content-Type': 'application/json',
        },
      }
    );
    return response.data;
  },

  // Delete group
  deleteGroup: async (groupId: string) => {
    await axios.delete(`${API_BASE_URL}/class-groups/${groupId}`, {
      headers: {
        Authorization: `Bearer ${localStorage.getItem('access_token')}`,
      },
    });
  },

  // List group members
  listMembers: async (groupId: string) => {
    const response = await axios.get<{ data: GroupMember[] }>(
      `${API_BASE_URL}/class-group-members?filter[groupID]=${groupId}`,
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
        },
      }
    );
    return response.data;
  },

  // Add member to group
  addMember: async (groupId: string, userID: string, role: GroupMember['role']) => {
    const response = await axios.post<GroupMember>(
      `${API_BASE_URL}/class-group-members`,
      { groupID: groupId, userID, role },
      {
        headers: {
          Authorization: `Bearer ${localStorage.getItem('access_token')}`,
          'Content-Type': 'application/json',
        },
      }
    );
    return response.data;
  },

  // Remove member
  removeMember: async (memberId: string) => {
    await axios.delete(`${API_BASE_URL}/class-group-members/${memberId}`, {
      headers: {
        Authorization: `Bearer ${localStorage.getItem('access_token')}`,
      },
    });
  },
};

// React Component Example
import React, { useEffect, useState } from 'react';

export const GroupsList: React.FC = () => {
  const [groups, setGroups] = useState<ClassGroup[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadGroups = async () => {
      try {
        const data = await groupsApi.listGroups(1, 20, 'Members');
        setGroups(data.data);
      } catch (error) {
        console.error('Failed to load groups:', error);
      } finally {
        setLoading(false);
      }
    };
    loadGroups();
  }, []);

  const handleCreateGroup = async () => {
    const newGroup = await groupsApi.createGroup({
      name: 'new-group',
      displayName: 'New Group',
      description: 'A new training group',
      maxMembers: 30,
    });
    setGroups([...groups, newGroup]);
  };

  if (loading) return <div>Loading...</div>;

  return (
    <div>
      <button onClick={handleCreateGroup}>Create Group</button>
      <ul>
        {groups.map(group => (
          <li key={group.id}>
            {group.displayName} ({group.memberCount}/{group.maxMembers} members)
            {group.isFull && <span> - FULL</span>}
            {group.isExpired && <span> - EXPIRED</span>}
          </li>
        ))}
      </ul>
    </div>
  );
};
```

### Vue.js Example

```typescript
// composables/useGroups.ts
import { ref, computed } from 'vue';
import axios from 'axios';

export function useGroups() {
  const groups = ref<ClassGroup[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const API_BASE_URL = 'http://localhost:8080/api/v1';

  const authHeaders = computed(() => ({
    Authorization: `Bearer ${localStorage.getItem('access_token')}`,
  }));

  const fetchGroups = async () => {
    loading.value = true;
    error.value = null;
    try {
      const response = await axios.get(
        `${API_BASE_URL}/class-groups?includes=Members`,
        { headers: authHeaders.value }
      );
      groups.value = response.data.data;
    } catch (err) {
      error.value = 'Failed to fetch groups';
      console.error(err);
    } finally {
      loading.value = false;
    }
  };

  const createGroup = async (input: CreateGroupInput) => {
    try {
      const response = await axios.post(
        `${API_BASE_URL}/class-groups`,
        input,
        { headers: { ...authHeaders.value, 'Content-Type': 'application/json' } }
      );
      groups.value.push(response.data);
      return response.data;
    } catch (err) {
      error.value = 'Failed to create group';
      throw err;
    }
  };

  return {
    groups,
    loading,
    error,
    fetchGroups,
    createGroup,
  };
}
```

## UI/UX Recommendations

### Group Creation Form
- **Required Fields**: `displayName`, `name` (auto-generate from displayName if not provided)
- **Optional Fields**: `description`, `maxMembers`, `expiresAt`
- **Validation**:
  - Name must be URL-friendly (lowercase, hyphens only)
  - Max members must be > 0
  - Expiration date must be in the future

### Group Card/List Item
Display:
- Group display name (prominent)
- Member count with progress bar (`12/30 members`)
- Status badges: `FULL`, `EXPIRED`, `INACTIVE`
- Owner name or "You" if current user
- Action buttons based on role (Edit/Delete for owner, Leave for members)

### Member Management
- **Role Badge Colors**:
  - Owner: Gold/Yellow
  - Admin: Blue
  - Assistant: Green
  - Member: Gray
- **Actions by Role**:
  - Owner: Can manage all members, delete group
  - Admin: Can add/remove members (except owner)
  - Assistant/Member: Read-only view

### Terminal Sharing
- Add "Share with Group" option alongside "Share with User"
- Show group icon/badge for group-shared terminals
- Display "Shared with X groups" count in terminal list
- Allow filtering terminals by "Shared with my groups"

## Permissions & Access Control

### Who Can Do What

**Group Creation:**
- Any authenticated user can create a group
- Creator becomes owner automatically

**Group Management:**
- **Owner**: Full control (update, delete, manage members, change all settings)
- **Admin**: Manage members, update group settings (except ownership)
- **Assistant**: View members, cannot modify
- **Member**: View members, cannot modify

**Terminal Sharing:**
- Group owner/admin can share terminals with the group
- Individual terminal owner can share their terminal with any group
- All group members inherit terminal access based on share's access level

**Automatic Permission Handling:**
- Backend automatically grants Casbin permissions when adding members
- Backend automatically revokes permissions when removing members
- No manual permission management needed in frontend

## Error Handling

### Common Error Responses

```json
{
  "error": "Error message",
  "code": "ERROR_CODE",
  "details": {}
}
```

**Common Error Codes:**
- `401 Unauthorized` - Invalid or missing JWT token
- `403 Forbidden` - User lacks permission for this action
- `404 Not Found` - Group/member not found
- `400 Bad Request` - Validation error (e.g., group full, expired, duplicate member)
- `409 Conflict` - Member already exists in group

**Frontend Error Handling Example:**
```typescript
try {
  await groupsApi.addMember(groupId, userId, 'member');
} catch (error) {
  if (axios.isAxiosError(error)) {
    switch (error.response?.status) {
      case 400:
        alert('Cannot add member: Group may be full or expired');
        break;
      case 403:
        alert('You do not have permission to add members');
        break;
      case 409:
        alert('User is already a member of this group');
        break;
      default:
        alert('Failed to add member');
    }
  }
}
```

## Testing

### Test Data Available

Use the test credentials from `CLAUDE.md`:
- Email: `1.supervisor@test.com`
- Password: `test`

### Manual Testing Steps

1. **Login and get token:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"email":"1.supervisor@test.com","password":"test"}'
   ```

2. **Create a test group:**
   ```bash
   curl -X POST http://localhost:8080/api/v1/class-groups \
     -H "Authorization: Bearer {token}" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "test-group",
       "displayName": "Test Group",
       "description": "Testing groups feature",
       "maxMembers": 10
     }'
   ```

3. **List groups:**
   ```bash
   curl -X GET http://localhost:8080/api/v1/class-groups \
     -H "Authorization: Bearer {token}"
   ```

## Swagger Documentation

Full interactive API documentation is available at:
```
http://localhost:8080/swagger/
```

Search for tags:
- `class-groups` - Group management endpoints
- `class-group-members` - Member management endpoints
- `terminal-shares` - Terminal sharing (includes group sharing)

## Migration Notes

### Backward Compatibility

The terminal sharing system is fully backward compatible:
- Existing user-only shares continue to work
- `sharedWithUserID` field changed from `string` to `*string` (nullable pointer)
- Frontend should check `shareType` field to determine share type:
  - `shareType: "user"` → Check `sharedWithUserID`
  - `shareType: "group"` → Check `sharedWithGroupID`

### Database Changes

New tables created:
- `groups` - Main group entity
- `group_members` - Join table for group membership

Modified tables:
- `terminal_shares` - Added `shared_with_group_id` column (nullable UUID)

## Support & Troubleshooting

### Common Issues

**Issue: "Route conflict" or 404 on `/api/v1/groups`**
- Solution: Use `/api/v1/class-groups` instead (legacy routes disabled)

**Issue: "User not found when adding member"**
- Solution: Ensure the user ID is a valid Casdoor user ID
- Check that user exists in the system first

**Issue: "Cannot add member - group full"**
- Solution: Increase `maxMembers` or remove inactive members

**Issue: Terminal not visible to group members**
- Solution: Verify terminal share was created with correct `sharedWithGroupID`
- Check that share is active and not expired

### Debugging

Enable debug logging by setting in `.env`:
```
ENVIRONMENT=development
```

Check logs for:
- Group creation events: `"Creating group for user: ..."`
- Permission grants: `"Granting group permissions to user: ..."`
- Member additions: `"Adding X members to group ..."`

## Future Enhancements

Planned features (not yet implemented):
- Email invitations with accept/reject workflow
- Group analytics and activity tracking
- Group-level subscriptions and billing
- Casdoor synchronization for external auth providers
- Bulk member import from CSV
- Group templates for quick setup

## Questions or Issues?

For backend issues or questions:
- Check the main codebase documentation in `CLAUDE.md`
- Review Swagger docs at `/swagger/`
- Examine entity registration files in `src/groups/entityRegistration/`

For permission issues:
- Verify JWT token is valid and not expired
- Check user roles in Casdoor dashboard (port 8000)
- Review Casbin policies in database `casbin_rule` table
# Terminal Hiding Feature - Frontend Implementation Guide

## Overview

The OCF Core backend provides a comprehensive terminal hiding system that allows users to hide inactive terminal sessions from their interface. This guide documents all available API endpoints and implementation patterns for the frontend team.

## Key Concepts

### Hiding vs. Deleting
- **Hiding**: Soft removal from the user's view - terminal remains in database and can be unhidden
- **Deleting**: Permanent removal (different operation, not covered here)

### Who Can Hide Terminals?
1. **Terminal Owner**: Can hide their own terminals (uses `is_hidden_by_owner` flag)
2. **Share Recipients**: Can hide terminals shared with them (uses `is_hidden_by_recipient` flag in TerminalShare)

### Hiding Rules
- **Only inactive terminals can be hidden** (status != "active")
- Active terminals must be stopped/expired before hiding
- Both owner and recipients have independent hiding states

---

## API Endpoints

### 1. Get User Terminal Sessions (with hiding support)

**Endpoint**: `GET /api/v1/terminals/user-sessions`

**Query Parameters**:
- `include_hidden` (boolean, optional): Include hidden terminals in results
  - `false` or omitted: Returns only visible (non-hidden) terminals (default)
  - `true`: Returns all terminals including hidden ones
- `user_id` (string, optional, admin-only): Get sessions for a specific user

**Authentication**: Required (Bearer token)

**Response**: Array of `TerminalOutput` objects

```typescript
interface TerminalOutput {
  id: string;                      // UUID
  session_id: string;              // Terminal Trainer session ID
  user_id: string;                 // Owner's user ID
  name: string;                    // User-friendly name
  status: string;                  // "active", "stopped", "expired", etc.
  expires_at: string;              // ISO 8601 timestamp
  instance_type: string;           // Instance type prefix
  machine_size: string;            // "XS", "S", "M", "L", "XL"
  is_hidden_by_owner: boolean;    // Hidden status (true = hidden)
  hidden_by_owner_at: string | null; // When it was hidden (ISO 8601)
  created_at: string;              // ISO 8601 timestamp
}
```

**Example Requests**:

```javascript
// Get only visible terminals (default behavior)
const response = await fetch('/api/v1/terminals/user-sessions', {
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  }
});
const visibleTerminals = await response.json();

// Get all terminals including hidden ones
const response = await fetch('/api/v1/terminals/user-sessions?include_hidden=true', {
  headers: {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json'
  }
});
const allTerminals = await response.json();
```

**Frontend Implementation Pattern**:

```javascript
// Component state
const [showHidden, setShowHidden] = useState(false);
const [terminals, setTerminals] = useState([]);

// Fetch terminals based on toggle
const fetchTerminals = async () => {
  const url = showHidden
    ? '/api/v1/terminals/user-sessions?include_hidden=true'
    : '/api/v1/terminals/user-sessions';

  const response = await fetch(url, {
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (response.ok) {
    const data = await response.json();
    setTerminals(data);
  }
};

// UI Toggle
<label>
  <input
    type="checkbox"
    checked={showHidden}
    onChange={(e) => setShowHidden(e.target.checked)}
  />
  Show hidden terminals
</label>
```

---

### 2. Hide a Terminal

**Endpoint**: `POST /api/v1/terminals/{id}/hide`

**Path Parameters**:
- `id` (string, required): Terminal UUID

**Authentication**: Required (Bearer token)

**Business Logic**:
1. **Ownership Check**:
   - If user is the owner → Sets `is_hidden_by_owner = true` on Terminal
   - If user is a share recipient → Sets `is_hidden_by_recipient = true` on TerminalShare
2. **Status Check**: Terminal must NOT be active (status != "active")
3. **Access Check**: User must be owner OR have at least "read" access via share

**Success Response** (200 OK):
```json
{
  "message": "Terminal hidden successfully"
}
```

**Error Responses**:

| Status | Error Message | Reason |
|--------|--------------|--------|
| 400 | "cannot hide active terminals" | Terminal is currently active |
| 403 | "access denied" | User doesn't own or have access to terminal |
| 404 | "terminal not found" | Invalid terminal ID |
| 500 | (various) | Internal server error |

**Example Request**:

```javascript
const hideTerminal = async (terminalId) => {
  const response = await fetch(`/api/v1/terminals/${terminalId}/hide`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (!response.ok) {
    const error = await response.json();

    if (response.status === 400) {
      alert('Cannot hide active terminals. Please stop the terminal first.');
    } else if (response.status === 403) {
      alert('You do not have permission to hide this terminal.');
    } else if (response.status === 404) {
      alert('Terminal not found.');
    } else {
      alert(`Error: ${error.error_message}`);
    }
    return false;
  }

  return true;
};
```

**UI Pattern - Hide Button**:

```javascript
const HideButton = ({ terminal, onHide }) => {
  const [isHiding, setIsHiding] = useState(false);

  const handleHide = async () => {
    // Check if terminal is active
    if (terminal.status === 'active') {
      alert('Cannot hide active terminals. Please stop the terminal first.');
      return;
    }

    setIsHiding(true);
    const success = await hideTerminal(terminal.id);
    setIsHiding(false);

    if (success) {
      onHide(); // Refresh terminal list
    }
  };

  return (
    <button
      onClick={handleHide}
      disabled={isHiding || terminal.status === 'active'}
      title={terminal.status === 'active' ? 'Stop the terminal before hiding' : 'Hide terminal'}
    >
      {isHiding ? 'Hiding...' : 'Hide'}
    </button>
  );
};
```

---

### 3. Unhide a Terminal

**Endpoint**: `DELETE /api/v1/terminals/{id}/hide`

**Path Parameters**:
- `id` (string, required): Terminal UUID

**Authentication**: Required (Bearer token)

**Business Logic**:
1. **Ownership Check**:
   - If user is the owner → Sets `is_hidden_by_owner = false` on Terminal
   - If user is a share recipient → Sets `is_hidden_by_recipient = false` on TerminalShare
2. **Access Check**: User must be owner OR have at least "read" access via share

**Success Response** (200 OK):
```json
{
  "message": "Terminal unhidden successfully"
}
```

**Error Responses**:

| Status | Error Message | Reason |
|--------|--------------|--------|
| 403 | "access denied" | User doesn't own or have access to terminal |
| 404 | "terminal not found" | Invalid terminal ID |
| 500 | (various) | Internal server error |

**Example Request**:

```javascript
const unhideTerminal = async (terminalId) => {
  const response = await fetch(`/api/v1/terminals/${terminalId}/hide`, {
    method: 'DELETE',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (!response.ok) {
    const error = await response.json();
    alert(`Error: ${error.error_message}`);
    return false;
  }

  return true;
};
```

**UI Pattern - Unhide Button**:

```javascript
const UnhideButton = ({ terminal, onUnhide }) => {
  const [isUnhiding, setIsUnhiding] = useState(false);

  const handleUnhide = async () => {
    setIsUnhiding(true);
    const success = await unhideTerminal(terminal.id);
    setIsUnhiding(false);

    if (success) {
      onUnhide(); // Refresh terminal list
    }
  };

  return (
    <button
      onClick={handleUnhide}
      disabled={isUnhiding}
    >
      {isUnhiding ? 'Unhiding...' : 'Unhide'}
    </button>
  );
};
```

---

### 4. Fix Terminal Hide Permissions (Admin/User Utility)

**Endpoint**: `POST /api/v1/terminals/fix-hide-permissions`

**Query Parameters**:
- `user_id` (string, optional, admin-only): Fix permissions for a specific user

**Authentication**: Required (Bearer token)

**Purpose**: Automatically adds Casbin permissions for hide/unhide operations to user's owned terminals and shared terminals. Useful for:
- New users who don't have permissions set up yet
- Migration after permission system changes
- Troubleshooting permission issues

**Success Response** (200 OK):
```typescript
interface FixPermissionsResponse {
  user_id: string;
  success: boolean;
  message: string;
  processed_terminals: number;  // Number of owned terminals processed
  processed_shares: number;     // Number of shared terminals processed
  errors: string[];             // Any errors encountered (optional)
}
```

**Example Response**:
```json
{
  "user_id": "user-123",
  "success": true,
  "message": "Permissions fixed successfully",
  "processed_terminals": 5,
  "processed_shares": 3,
  "errors": []
}
```

**Example Request**:

```javascript
// Fix permissions for current user
const fixPermissions = async () => {
  const response = await fetch('/api/v1/terminals/fix-hide-permissions', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (response.ok) {
    const result = await response.json();
    console.log(`Fixed permissions for ${result.processed_terminals} terminals and ${result.processed_shares} shares`);
  }
};

// Admin: Fix permissions for specific user
const fixUserPermissions = async (userId) => {
  const response = await fetch(`/api/v1/terminals/fix-hide-permissions?user_id=${userId}`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  if (response.ok) {
    const result = await response.json();
    console.log(`Fixed permissions for user ${result.user_id}`);
  }
};
```

**When to Use**:
- On first login (if user has permission issues)
- In admin panel to troubleshoot user issues
- After sharing terminals (permissions are auto-added, but this can fix if something went wrong)

---

## Complete UI Implementation Example

### Terminal List Component with Hiding Support

```typescript
import React, { useState, useEffect } from 'react';

interface Terminal {
  id: string;
  session_id: string;
  name: string;
  status: string;
  is_hidden_by_owner: boolean;
  hidden_by_owner_at: string | null;
  created_at: string;
  expires_at: string;
  machine_size: string;
}

const TerminalListComponent = () => {
  const [terminals, setTerminals] = useState<Terminal[]>([]);
  const [showHidden, setShowHidden] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Fetch terminals based on showHidden state
  const fetchTerminals = async () => {
    setLoading(true);
    setError(null);

    try {
      const url = showHidden
        ? '/api/v1/terminals/user-sessions?include_hidden=true'
        : '/api/v1/terminals/user-sessions';

      const response = await fetch(url, {
        headers: {
          'Authorization': `Bearer ${getToken()}`,
          'Content-Type': 'application/json'
        }
      });

      if (!response.ok) {
        throw new Error('Failed to fetch terminals');
      }

      const data = await response.json();
      setTerminals(data);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  // Fetch on mount and when showHidden changes
  useEffect(() => {
    fetchTerminals();
  }, [showHidden]);

  // Hide terminal
  const handleHide = async (terminal: Terminal) => {
    // Prevent hiding active terminals
    if (terminal.status === 'active') {
      alert('Cannot hide active terminals. Please stop the terminal first.');
      return;
    }

    try {
      const response = await fetch(`/api/v1/terminals/${terminal.id}/hide`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${getToken()}`,
          'Content-Type': 'application/json'
        }
      });

      if (!response.ok) {
        const error = await response.json();

        if (response.status === 400) {
          alert('Cannot hide active terminals.');
        } else if (response.status === 403) {
          alert('You do not have permission to hide this terminal.');
        } else {
          alert(`Error: ${error.error_message}`);
        }
        return;
      }

      // Refresh the list
      fetchTerminals();
    } catch (err) {
      alert(`Error hiding terminal: ${err.message}`);
    }
  };

  // Unhide terminal
  const handleUnhide = async (terminal: Terminal) => {
    try {
      const response = await fetch(`/api/v1/terminals/${terminal.id}/hide`, {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${getToken()}`,
          'Content-Type': 'application/json'
        }
      });

      if (!response.ok) {
        const error = await response.json();
        alert(`Error: ${error.error_message}`);
        return;
      }

      // Refresh the list
      fetchTerminals();
    } catch (err) {
      alert(`Error unhiding terminal: ${err.message}`);
    }
  };

  // Render loading state
  if (loading) {
    return <div>Loading terminals...</div>;
  }

  // Render error state
  if (error) {
    return <div>Error: {error}</div>;
  }

  return (
    <div className="terminal-list">
      <div className="controls">
        <label>
          <input
            type="checkbox"
            checked={showHidden}
            onChange={(e) => setShowHidden(e.target.checked)}
          />
          Show hidden terminals
        </label>
      </div>

      <div className="terminals">
        {terminals.length === 0 ? (
          <p>No terminals found</p>
        ) : (
          terminals.map(terminal => (
            <div
              key={terminal.id}
              className={`terminal-card ${terminal.is_hidden_by_owner ? 'hidden' : ''}`}
            >
              <h3>
                {terminal.name}
                {terminal.is_hidden_by_owner && <span className="badge">Hidden</span>}
              </h3>

              <div className="terminal-info">
                <p>Status: <span className={`status-${terminal.status}`}>{terminal.status}</span></p>
                <p>Size: {terminal.machine_size}</p>
                <p>Created: {new Date(terminal.created_at).toLocaleString()}</p>
                {terminal.hidden_by_owner_at && (
                  <p>Hidden: {new Date(terminal.hidden_by_owner_at).toLocaleString()}</p>
                )}
              </div>

              <div className="terminal-actions">
                {terminal.is_hidden_by_owner ? (
                  <button
                    onClick={() => handleUnhide(terminal)}
                    className="btn-unhide"
                  >
                    Unhide
                  </button>
                ) : (
                  <button
                    onClick={() => handleHide(terminal)}
                    disabled={terminal.status === 'active'}
                    className="btn-hide"
                    title={terminal.status === 'active' ? 'Stop terminal before hiding' : 'Hide terminal'}
                  >
                    Hide
                  </button>
                )}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
};

export default TerminalListComponent;
```

---

## UI/UX Best Practices

### 1. Visual Indicators for Hidden Terminals

When `include_hidden=true`, use visual cues to distinguish hidden terminals:

```css
.terminal-card.hidden {
  opacity: 0.6;
  background-color: #f5f5f5;
  border: 1px dashed #ccc;
}

.terminal-card.hidden .badge {
  background-color: #999;
  color: white;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.8em;
}
```

### 2. Disable Hide Button for Active Terminals

```javascript
<button
  onClick={() => handleHide(terminal)}
  disabled={terminal.status === 'active'}
  title={terminal.status === 'active' ? 'Stop terminal before hiding' : 'Hide terminal'}
  className={terminal.status === 'active' ? 'btn-disabled' : 'btn-hide'}
>
  Hide
</button>
```

### 3. Confirmation Before Hiding

```javascript
const handleHideWithConfirmation = async (terminal) => {
  const confirmed = window.confirm(
    `Are you sure you want to hide "${terminal.name}"? ` +
    `You can unhide it later by toggling "Show hidden terminals".`
  );

  if (confirmed) {
    await handleHide(terminal);
  }
};
```

### 4. Bulk Hide/Unhide

```javascript
const handleBulkHide = async (terminalIds) => {
  const promises = terminalIds.map(id =>
    fetch(`/api/v1/terminals/${id}/hide`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json'
      }
    })
  );

  await Promise.all(promises);
  fetchTerminals(); // Refresh list
};
```

### 5. Filter Buttons

Instead of just a checkbox, provide filter options:

```javascript
const [filter, setFilter] = useState('visible'); // 'visible', 'hidden', 'all'

// In URL construction
const getFilterParam = () => {
  if (filter === 'all') return '?include_hidden=true';
  if (filter === 'hidden') return '?include_hidden=true'; // filter client-side
  return '';
};

// UI
<div className="filter-buttons">
  <button
    className={filter === 'visible' ? 'active' : ''}
    onClick={() => setFilter('visible')}
  >
    Visible Only
  </button>
  <button
    className={filter === 'all' ? 'active' : ''}
    onClick={() => setFilter('all')}
  >
    All Terminals
  </button>
  <button
    className={filter === 'hidden' ? 'active' : ''}
    onClick={() => setFilter('hidden')}
  >
    Hidden Only
  </button>
</div>

// Filter client-side for 'hidden' filter
const displayTerminals = filter === 'hidden'
  ? terminals.filter(t => t.is_hidden_by_owner)
  : terminals;
```

---

## Testing Checklist

### Functional Tests

- [ ] Can hide inactive terminal (status != "active")
- [ ] Cannot hide active terminal (status == "active")
- [ ] Can unhide previously hidden terminal
- [ ] Hidden terminals disappear from default list
- [ ] Hidden terminals appear when `include_hidden=true`
- [ ] Hide/Unhide persists across page reloads
- [ ] Owner can hide their own terminals
- [ ] Share recipient can hide shared terminals
- [ ] User cannot hide terminals they don't own/have access to
- [ ] `is_hidden_by_owner` flag updates correctly in response
- [ ] `hidden_by_owner_at` timestamp is set when hiding
- [ ] `hidden_by_owner_at` is null when unhiding

### Edge Cases

- [ ] Hiding already-hidden terminal (should succeed, idempotent)
- [ ] Unhiding already-visible terminal (should succeed, idempotent)
- [ ] Hiding terminal immediately after stopping it
- [ ] Network errors during hide/unhide operations
- [ ] Multiple rapid hide/unhide clicks (debouncing)
- [ ] Permission errors (403) handled gracefully
- [ ] Invalid terminal ID (404) handled gracefully

### UI/UX Tests

- [ ] Loading states during API calls
- [ ] Success feedback after hide/unhide
- [ ] Error messages display correctly
- [ ] Toggle/filter updates list immediately
- [ ] Visual distinction between hidden and visible terminals
- [ ] Button states (enabled/disabled) update correctly
- [ ] Keyboard navigation works
- [ ] Screen reader accessibility

---

## Error Handling Reference

### HTTP Status Codes

| Code | Meaning | Action |
|------|---------|--------|
| 200 | Success | Update UI, show success message |
| 400 | Bad Request | Show error message (e.g., "Cannot hide active terminals") |
| 403 | Forbidden | Show permission error, check user access |
| 404 | Not Found | Show "Terminal not found", refresh list |
| 500 | Server Error | Show generic error, log for debugging |

### Error Response Format

All error responses follow this format:

```json
{
  "error_code": 400,
  "error_message": "cannot hide active terminals"
}
```

---

## Performance Considerations

### 1. Polling vs. Real-time Updates

If terminals update frequently, consider WebSocket updates instead of polling:

```javascript
// Polling (simple, but less efficient)
useEffect(() => {
  const interval = setInterval(() => {
    fetchTerminals();
  }, 30000); // Every 30 seconds

  return () => clearInterval(interval);
}, [showHidden]);

// WebSocket (more efficient, if available)
useEffect(() => {
  const ws = new WebSocket('ws://api/terminals/updates');

  ws.onmessage = (event) => {
    const update = JSON.parse(event.data);
    // Update specific terminal in state
    setTerminals(prev =>
      prev.map(t => t.id === update.id ? { ...t, ...update } : t)
    );
  };

  return () => ws.close();
}, []);
```

### 2. Caching

Cache terminal list to reduce API calls:

```javascript
import { useQuery, useMutation, useQueryClient } from 'react-query';

const useTerminals = (includeHidden: boolean) => {
  return useQuery(
    ['terminals', includeHidden],
    () => fetchTerminals(includeHidden),
    {
      staleTime: 30000, // 30 seconds
      cacheTime: 300000, // 5 minutes
    }
  );
};

const useHideTerminal = () => {
  const queryClient = useQueryClient();

  return useMutation(
    (terminalId: string) => hideTerminal(terminalId),
    {
      onSuccess: () => {
        // Invalidate cache to refetch
        queryClient.invalidateQueries('terminals');
      },
    }
  );
};
```

### 3. Optimistic Updates

Update UI immediately before API response:

```javascript
const handleHide = async (terminal) => {
  // Optimistically update UI
  setTerminals(prev =>
    prev.map(t =>
      t.id === terminal.id
        ? { ...t, is_hidden_by_owner: true, hidden_by_owner_at: new Date().toISOString() }
        : t
    )
  );

  try {
    await hideTerminal(terminal.id);
    // Success - optimistic update was correct
  } catch (error) {
    // Rollback optimistic update
    setTerminals(prev =>
      prev.map(t =>
        t.id === terminal.id
          ? { ...t, is_hidden_by_owner: false, hidden_by_owner_at: null }
          : t
      )
    );
    alert('Failed to hide terminal');
  }
};
```

---

## Security Notes

1. **Authentication Required**: All endpoints require valid Bearer token
2. **Authorization Checks**:
   - Users can only hide terminals they own or have access to
   - Casbin enforces permissions at the API level
3. **Input Validation**: Terminal IDs must be valid UUIDs
4. **HTTPS Only**: Always use HTTPS in production for token security

---

## Additional Resources

- **Swagger Documentation**: `http://localhost:8080/swagger/` (development)
- **Backend Code Reference**:
  - Controller: `src/terminalTrainer/routes/terminalController.go`
  - Service: `src/terminalTrainer/services/terminalTrainerService.go`
  - Repository: `src/terminalTrainer/repositories/terminalRepository.go`
  - Models: `src/terminalTrainer/models/terminal.go`, `terminalShare.go`
  - DTOs: `src/terminalTrainer/dto/terminalDto.go`

---

## Support

For questions or issues:
1. Check the Swagger documentation first
2. Review this guide for implementation patterns
3. Test endpoints using Postman/Insomnia with sample requests
4. Contact backend team with specific error messages and request details
# Terminal Sharing and Access Rights System

## Overview

The OCF Core terminal sharing system allows terminal owners to grant controlled access to other users with granular permission levels. The system supports three access levels (read, write, admin), time-based expiration, activation/deactivation, and independent hiding functionality for both owners and recipients.

## Architecture Components

### 1. Data Model

**Location**: `src/terminalTrainer/models/terminalShare.go`

```go
type TerminalShare struct {
    BaseModel
    TerminalID           uuid.UUID  // UUID of the shared terminal
    SharedWithUserID     string     // User receiving access (Casdoor user ID)
    SharedByUserID       string     // Terminal owner (creator of share)
    AccessLevel          string     // read | write | admin
    ExpiresAt            *time.Time // Optional expiration timestamp
    IsActive             bool       // Manual activation toggle
    IsHiddenByRecipient  bool       // Recipient can hide from their list
    HiddenAt             *time.Time // Timestamp when hidden
    Terminal             Terminal   `gorm:"foreignKey:TerminalID"`
}
```

**Key Fields:**
- **TerminalID**: Links to the terminal being shared
- **SharedWithUserID**: Casdoor user ID of the recipient
- **SharedByUserID**: Casdoor user ID of the terminal owner
- **AccessLevel**: Permission level (see Access Levels section)
- **ExpiresAt**: Optional auto-expiration (NULL = never expires)
- **IsActive**: Manual toggle to enable/disable share without deletion
- **IsHiddenByRecipient**: Allows recipient to hide shared terminal from their interface
- **HiddenAt**: Tracks when terminal was hidden

### 2. Access Levels

The system implements a hierarchical access control model with three levels:

#### Level 1: READ (`"read"`)
**Permissions:**
- View terminal console output
- Read terminal metadata (name, status, configuration)
- Access terminal info endpoint

**Casbin Permissions Added:**
- `GET /api/v1/terminals/:id`
- `POST /api/v1/terminals/:id/hide` (hide functionality)
- `DELETE /api/v1/terminals/:id/hide` (unhide functionality)

**Use Case**: Observers, students viewing instructor demonstrations

#### Level 2: WRITE (`"write"`)
**Permissions:**
- All READ permissions
- Modify terminal settings (name, configuration)
- Send commands to terminal (via WebSocket)
- Edit terminal metadata

**Casbin Permissions Added:**
- All READ permissions
- `PATCH /api/v1/terminals/:id`

**Use Case**: Collaborators, teaching assistants who need to interact with terminals

#### Level 3: ADMIN (`"admin"`)
**Permissions:**
- All WRITE permissions
- Stop/delete terminal sessions
- Share terminal with other users
- Revoke access from other users
- View all shares for the terminal

**Casbin Permissions Added:**
- All WRITE permissions
- `DELETE /api/v1/terminals/:id`
- Full control equivalent to terminal owner (except ownership transfer)

**Use Case**: Co-administrators, trusted collaborators who need full management capabilities

### Access Level Hierarchy Implementation

**Location**: `src/terminalTrainer/models/terminalShare.go:44-67`

```go
func (ts *TerminalShare) HasAccess(requiredLevel string) bool {
    if !ts.IsActive || ts.IsExpired() {
        return false
    }

    accessLevels := map[string]int{
        "read":  1,
        "write": 2,
        "admin": 3,
    }

    currentLevel := accessLevels[ts.AccessLevel]
    requiredLevelInt := accessLevels[requiredLevel]

    return currentLevel >= requiredLevelInt
}
```

**Logic**: Higher access levels inherit all permissions from lower levels (admin includes write and read, write includes read).

## 3. API Endpoints

### Sharing Management Routes

**Location**: `src/terminalTrainer/routes/terminalRoutes.go:27-36`

#### Share Terminal
```
POST /api/v1/terminals/:id/share
Authorization: Bearer {JWT_TOKEN}
Content-Type: application/json

Request Body:
{
  "shared_with_user_id": "casdoor-user-id",
  "access_level": "read|write|admin",
  "expires_at": "2025-12-31T23:59:59Z" // Optional
}

Response (200 OK):
{
  "message": "Terminal shared successfully"
}

Errors:
- 404: Terminal not found
- 403: User is not terminal owner
- 400: Invalid access level or user ID
```

#### Revoke Terminal Access
```
DELETE /api/v1/terminals/:id/share/:user_id
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
{
  "message": "Access revoked successfully"
}

Errors:
- 404: Terminal or share not found
- 403: User is not terminal owner
```

#### Get Terminal Shares
```
GET /api/v1/terminals/:id/shares
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
[
  {
    "id": "uuid",
    "terminal_id": "uuid",
    "shared_with_user_id": "user-id",
    "shared_by_user_id": "owner-id",
    "access_level": "write",
    "expires_at": "2025-12-31T23:59:59Z",
    "is_active": true,
    "is_hidden_by_recipient": false,
    "hidden_at": null,
    "created_at": "2025-01-15T10:30:00Z"
  }
]

Errors:
- 403: User is not terminal owner
- 404: Terminal not found
```

#### Get Shared Terminals (Recipient View)
```
GET /api/v1/terminals/shared-with-me?include_hidden=false
Authorization: Bearer {JWT_TOKEN}

Query Parameters:
- include_hidden (optional): boolean, default false

Response (200 OK):
[
  {
    "terminal": {
      "id": "uuid",
      "name": "Production Server",
      "session_id": "term-123",
      "status": "running",
      ...
    },
    "shared_by": "owner-user-id",
    "shared_by_display_name": "John Doe",
    "access_level": "write",
    "expires_at": "2025-12-31T23:59:59Z",
    "shared_at": "2025-01-15T10:30:00Z",
    "shares": [] // Empty for recipients, populated for owners
  }
]
```

#### Get Shared Terminal Info
```
GET /api/v1/terminals/:id/info
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
{
  "terminal": { /* terminal details */ },
  "shared_by": "owner-user-id",
  "shared_by_display_name": "John Doe",
  "access_level": "write", // Current user's access level
  "expires_at": "2025-12-31T23:59:59Z",
  "shared_at": "2025-01-15T10:30:00Z",
  "shares": [ /* all shares - owner only */ ]
}

Errors:
- 403: User has no access to terminal
- 404: Terminal not found
```

### Hiding Management Routes

**Location**: `src/terminalTrainer/routes/terminalRoutes.go:38-43`

#### Hide Terminal
```
POST /api/v1/terminals/:id/hide
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
{
  "message": "Terminal hidden successfully"
}

Errors:
- 403: User is not owner or recipient
- 400: Terminal is still active (must be stopped first)
- 404: Terminal not found
```

**Constraints**: Can only hide inactive (stopped/expired) terminals.

#### Unhide Terminal
```
DELETE /api/v1/terminals/:id/hide
Authorization: Bearer {JWT_TOKEN}

Response (200 OK):
{
  "message": "Terminal unhidden successfully"
}

Errors:
- 403: User is not owner or recipient
- 404: Terminal not found or not hidden
```

## 4. Service Layer Logic

**Location**: `src/terminalTrainer/services/terminalTrainerService.go`

### ShareTerminal Method (Lines 860-956)

**Workflow:**
1. **Validate Terminal Ownership**: Check if `sharedByUserID` is the terminal owner
2. **Prevent Self-Sharing**: Ensure `sharedByUserID != sharedWithUserID`
3. **Validate Access Level**: Must be `"read"`, `"write"`, or `"admin"`
4. **Check Existing Share**: Query database for existing share
5. **Update or Create**:
   - If share exists: Update access level, expiration, reactivate if needed
   - If new: Create new `TerminalShare` record
6. **Add Casbin Permissions**:
   - Call `addTerminalHidePermissions()` to allow hiding
   - Call `addTerminalSharePermissions()` with specific access level
7. **Persist**: Save to database

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:860-956`

### RevokeTerminalAccess Method (Lines 971-1017)

**Workflow:**
1. **Validate Ownership**: Check if `requestingUserID` owns the terminal
2. **Find Share**: Query for active share with `sharedWithUserID`
3. **Remove Permissions**: Call `removeTerminalSharePermissions()`
4. **Delete Share**: Hard delete from database (not soft delete)

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:971-1017`

### HasTerminalAccess Method (Lines 1019-1037)

**Workflow:**
1. **Owner Check**: If `userID` matches terminal owner, return `true` immediately
2. **Query Share**: Find active share for user
3. **Validate Expiration**: Check if share is expired
4. **Check Access Level**: Use `HasAccess()` method to verify hierarchy
5. **Return Result**: `true` if access granted, `false` otherwise

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1019-1037`

### GetSharedTerminals Method (Lines 1039-1074)

**Workflow:**
1. **Query Shares**: Get all active shares for `userID` from repository
2. **Filter by Hidden Status**: Apply `include_hidden` filter if specified
3. **Load Terminals**: Preload terminal data and owner information
4. **Fetch Owner Display Names**: Query Casdoor API for owner details
5. **Build Response**: Create `SharedTerminalInfo` DTOs with sharing context

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1039-1074`

### HideTerminal Method (Lines 1133-1162)

**Workflow:**
1. **Validate Access**: Check if user is owner OR recipient with valid share
2. **Check Terminal Status**: Ensure terminal is inactive (stopped/expired)
3. **Update Hidden Status**:
   - If owner: Set `is_hidden` on `Terminal` model
   - If recipient: Set `is_hidden_by_recipient` on `TerminalShare` model
4. **Set Timestamp**: Record `hidden_at` timestamp

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1133-1162`

## 5. Permission System Integration (Casbin)

### Permission Structure

**Format**: `userID, route, methods`

**Example**:
```
("user-123", "/api/v1/terminals/abc-uuid", "GET")
("user-123", "/api/v1/terminals/abc-uuid", "PATCH")
("user-123", "/api/v1/terminals/abc-uuid/hide", "POST|DELETE")
```

### Permission Assignment Methods

**Location**: `src/terminalTrainer/services/terminalTrainerService.go:1190-1298`

#### addTerminalHidePermissions (Lines 1190-1211)

**Purpose**: Grant hide/unhide capabilities to shared users

**Permissions Added**:
- `POST /api/v1/terminals/:id/hide`
- `DELETE /api/v1/terminals/:id/hide`

**Called**: During terminal sharing (all access levels)

```go
func (tts *terminalTrainerService) addTerminalHidePermissions(
    userID string,
    sessionID string,
) error {
    route := "/api/v1/terminals/" + sessionID + "/hide"
    methods := "POST|DELETE"

    _, err := casdoor.Enforcer.AddPolicy(userID, route, methods)
    return err
}
```

#### addTerminalSharePermissions (Lines 1213-1258)

**Purpose**: Grant access-level-specific permissions to shared users

**Permissions by Access Level**:

**READ Level**:
- `GET /api/v1/terminals/:id`

**WRITE Level**:
- `GET /api/v1/terminals/:id`
- `PATCH /api/v1/terminals/:id`

**ADMIN Level**:
- `GET /api/v1/terminals/:id`
- `PATCH /api/v1/terminals/:id`
- `DELETE /api/v1/terminals/:id`

**Called**: During terminal sharing after validating access level

```go
func (tts *terminalTrainerService) addTerminalSharePermissions(
    userID string,
    sessionID string,
    accessLevel string,
) error {
    route := "/api/v1/terminals/" + sessionID

    var methods string
    switch accessLevel {
    case "read":
        methods = "GET"
    case "write":
        methods = "GET|PATCH"
    case "admin":
        methods = "GET|PATCH|DELETE"
    }

    _, err := casdoor.Enforcer.AddPolicy(userID, route, methods)
    return err
}
```

#### removeTerminalSharePermissions (Lines 1260-1298)

**Purpose**: Revoke all sharing permissions when access is revoked

**Permissions Removed**:
- Terminal access route: `/api/v1/terminals/:id`
- Hide route: `/api/v1/terminals/:id/hide`

**Called**: During access revocation

```go
func (tts *terminalTrainerService) removeTerminalSharePermissions(
    userID string,
    sessionID string,
) error {
    terminalRoute := "/api/v1/terminals/" + sessionID
    hideRoute := "/api/v1/terminals/" + sessionID + "/hide"

    casdoor.Enforcer.RemoveFilteredPolicy(0, userID, terminalRoute)
    casdoor.Enforcer.RemoveFilteredPolicy(0, userID, hideRoute)

    return casdoor.Enforcer.SavePolicy()
}
```

### Automatic Permission Setup

**During Terminal Creation** (`src/terminalTrainer/services/terminalTrainerService.go:1300-1330`):
- Automatically calls `addTerminalHidePermissions()` for the owner
- Grants owner hide/unhide capabilities for their own terminals

**During Terminal Sharing**:
1. Hide permissions added (all access levels)
2. Access-level-specific permissions added (based on `accessLevel` parameter)

**During Access Revocation**:
- All permissions removed via `removeTerminalSharePermissions()`

## 6. Database Queries and Repository

**Location**: `src/terminalTrainer/repositories/terminalRepository.go`

### Key Repository Methods

#### GetSharedTerminalsForUser (Lines 362-379)
```go
func (tr *terminalRepository) GetSharedTerminalsForUser(
    userID string,
) (*[]models.Terminal, error) {
    var terminals []models.Terminal

    err := tr.db.
        Joins("JOIN terminal_shares ON terminals.id = terminal_shares.terminal_id").
        Where("terminal_shares.shared_with_user_id = ?", userID).
        Where("terminal_shares.is_active = ?", true).
        Where("terminal_shares.expires_at IS NULL OR terminal_shares.expires_at > ?", time.Now()).
        Preload("TerminalShares").
        Find(&terminals).Error

    return &terminals, err
}
```

**Features**:
- Joins `terminals` with `terminal_shares` table
- Filters by recipient user ID
- Only returns active, non-expired shares
- Preloads sharing metadata

#### GetSharedTerminalsForUserWithHidden (Lines 398-414)
```go
func (tr *terminalRepository) GetSharedTerminalsForUserWithHidden(
    userID string,
    includeHidden bool,
) (*[]models.Terminal, error) {
    query := tr.db.
        Joins("JOIN terminal_shares ON terminals.id = terminal_shares.terminal_id").
        Where("terminal_shares.shared_with_user_id = ?", userID).
        Where("terminal_shares.is_active = ?", true).
        Where("terminal_shares.expires_at IS NULL OR terminal_shares.expires_at > ?", time.Now())

    if !includeHidden {
        query = query.Where("terminal_shares.is_hidden_by_recipient = ?", false)
    }

    return query.Preload("TerminalShares").Find(&terminals).Error
}
```

**Features**:
- Same as `GetSharedTerminalsForUser` but with hidden filter
- Conditional `WHERE` clause based on `includeHidden` parameter

#### HideTerminalForUser (Lines 416-429)
```go
func (tr *terminalRepository) HideTerminalForUser(
    terminalID uuid.UUID,
    userID string,
) error {
    return tr.db.
        Model(&models.TerminalShare{}).
        Where("terminal_id = ?", terminalID).
        Where("shared_with_user_id = ?", userID).
        Updates(map[string]interface{}{
            "is_hidden_by_recipient": true,
            "hidden_at": time.Now(),
        }).Error
}
```

#### UnhideTerminalForUser (Lines 431-447)
```go
func (tr *terminalRepository) UnhideTerminalForUser(
    terminalID uuid.UUID,
    userID string,
) error {
    return tr.db.
        Model(&models.TerminalShare{}).
        Where("terminal_id = ?", terminalID).
        Where("shared_with_user_id = ?", userID).
        Updates(map[string]interface{}{
            "is_hidden_by_recipient": false,
            "hidden_at": nil,
        }).Error
}
```

#### GetTerminalShareByUserAndTerminal (Lines 381-396)
```go
func (tr *terminalRepository) GetTerminalShareByUserAndTerminal(
    userID string,
    terminalID uuid.UUID,
) (*models.TerminalShare, error) {
    var share models.TerminalShare

    err := tr.db.
        Where("shared_with_user_id = ?", userID).
        Where("terminal_id = ?", terminalID).
        Where("is_active = ?", true).
        Where("expires_at IS NULL OR expires_at > ?", time.Now()).
        First(&share).Error

    return &share, err
}
```

**Features**:
- Validates active, non-expired shares
- Used for access level checking

## 7. Entity Registration and Generic CRUD

**Location**: `src/terminalTrainer/entityRegistration/terminalShareRegistration.go`

### Generic Entity Permissions (Lines 277-286)

```go
func (tsr TerminalShareRegistration) GetEntityRoles() EntityRoles {
    roleMap := make(map[string]string)

    // Members can view and create shares
    roleMap[string(authModels.Member)] = "(GET|POST)"

    // Group managers can view, create, and delete shares
    roleMap[string(authModels.GroupManager)] = "(GET|POST|DELETE)"

    // Admins have full control (including PATCH for updates)
    roleMap[string(authModels.Admin)] = "(GET|POST|DELETE|PATCH)"

    return roleMap
}
```

**Note**: These are generic entity permissions for the `/api/v1/terminal-shares` route. **Specific terminal sharing routes** (e.g., `/terminals/:id/share`) use **custom Casbin policies** added in service methods.

### Queryable Fields (Lines 147-153)

**Filterable Fields**:
- `terminal_id` - Filter shares by terminal
- `shared_with_user_id` - Filter by recipient
- `shared_by_user_id` - Filter by owner
- `access_level` - Filter by permission level
- `is_active` - Filter by active status

**Example Query**:
```
GET /api/v1/terminal-shares?terminal_id=abc-uuid&is_active=true
```

### Validation Rules (Lines 134-135)

```go
type CreateTerminalShareInput struct {
    TerminalID       uuid.UUID  `json:"terminal_id" binding:"required"`
    SharedWithUserID string     `json:"shared_with_user_id" binding:"required"`
    AccessLevel      string     `json:"access_level" binding:"required,oneof=read write admin"`
    ExpiresAt        *time.Time `json:"expires_at"`
}
```

**Validation**:
- `terminal_id`: Required, must be valid UUID
- `shared_with_user_id`: Required, non-empty string
- `access_level`: Required, must be exactly "read", "write", or "admin"
- `expires_at`: Optional, must be valid timestamp if provided

## 8. Terminal Hiding System

### Dual Hiding Mechanism

The system supports **two independent hiding mechanisms**:

#### 1. Owner Hiding (Terminal Model)
**Location**: `src/terminalTrainer/models/terminal.go`

```go
type Terminal struct {
    // ... other fields
    IsHidden bool       `gorm:"default:false"`
    HiddenAt *time.Time `gorm:"default:null"`
}
```

**Behavior**:
- Owner can hide their own terminals
- Hides terminal from owner's terminal list
- Does NOT affect recipient visibility
- Persisted on `terminals.is_hidden` and `terminals.hidden_at`

#### 2. Recipient Hiding (TerminalShare Model)
**Location**: `src/terminalTrainer/models/terminalShare.go`

```go
type TerminalShare struct {
    // ... other fields
    IsHiddenByRecipient bool       `gorm:"default:false"`
    HiddenAt            *time.Time `gorm:"default:null"`
}
```

**Behavior**:
- Recipient can hide shared terminals from their "shared with me" list
- Does NOT affect owner's visibility
- Does NOT affect other recipients
- Each recipient has independent hiding status
- Persisted on `terminal_shares.is_hidden_by_recipient` and `terminal_shares.hidden_at`

### Hiding Constraints

**Constraint**: Can only hide **inactive terminals**

**Enforcement**: `src/terminalTrainer/services/terminalTrainerService.go:1147-1150`

```go
if terminal.Status != "stopped" && !terminal.IsExpired() {
    return errors.New("can only hide inactive terminals")
}
```

**Rationale**: Prevents hiding active terminals that may have ongoing operations.

### Hiding API Endpoints

#### Hide Terminal
```
POST /api/v1/terminals/:id/hide
```

**Authorization**: Owner OR recipient with valid share

**Logic**:
- If user is owner: Sets `terminals.is_hidden = true`
- If user is recipient: Sets `terminal_shares.is_hidden_by_recipient = true` for that user's share

#### Unhide Terminal
```
DELETE /api/v1/terminals/:id/hide
```

**Authorization**: Owner OR recipient with valid share

**Logic**:
- If user is owner: Sets `terminals.is_hidden = false`
- If user is recipient: Sets `terminal_shares.is_hidden_by_recipient = false` for that user's share

### Hiding Permissions

**Added During**:
- Terminal creation (for owner)
- Terminal sharing (for recipients)

**Permission Format**:
```
userID, /api/v1/terminals/:id/hide, POST|DELETE
```

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1190-1211`

## 9. Use Case Examples

### Example 1: Instructor Sharing Terminal with Students

**Scenario**: Instructor wants to share a demo terminal with students for read-only observation.

**Steps**:
1. Instructor creates terminal (becomes owner)
2. Instructor shares terminal with student:
   ```bash
   curl -X POST http://localhost:8080/api/v1/terminals/{terminal-id}/share \
     -H "Authorization: Bearer {instructor-token}" \
     -H "Content-Type: application/json" \
     -d '{
       "shared_with_user_id": "student-123",
       "access_level": "read"
     }'
   ```
3. System creates `TerminalShare` record with `access_level = "read"`
4. Casbin permissions added:
   - `GET /api/v1/terminals/{terminal-id}` (read terminal)
   - `POST|DELETE /api/v1/terminals/{terminal-id}/hide` (hide/unhide)
5. Student can now:
   - View terminal console output (WebSocket connection)
   - Read terminal metadata
   - Hide terminal from their list when done
6. Student CANNOT:
   - Send commands to terminal
   - Modify terminal settings
   - Delete terminal
   - Share with others

### Example 2: Time-Limited Collaboration

**Scenario**: Developer grants temporary write access to a colleague for pair programming.

**Steps**:
1. Developer shares terminal with 24-hour expiration:
   ```bash
   curl -X POST http://localhost:8080/api/v1/terminals/{terminal-id}/share \
     -H "Authorization: Bearer {developer-token}" \
     -H "Content-Type: application/json" \
     -d '{
       "shared_with_user_id": "colleague-456",
       "access_level": "write",
       "expires_at": "2025-01-16T10:30:00Z"
     }'
   ```
2. Colleague can edit and send commands for 24 hours
3. After expiration, `HasAccess()` returns `false` automatically
4. Casbin permissions remain but are ineffective (service layer blocks access)
5. No manual revocation needed

### Example 3: Admin Delegation

**Scenario**: Team lead delegates terminal management to trusted team member.

**Steps**:
1. Team lead shares terminal with admin access:
   ```bash
   curl -X POST http://localhost:8080/api/v1/terminals/{terminal-id}/share \
     -H "Authorization: Bearer {lead-token}" \
     -H "Content-Type: application/json" \
     -d '{
       "shared_with_user_id": "team-member-789",
       "access_level": "admin"
     }'
   ```
2. Team member can:
   - Stop/delete terminal
   - Share with additional users
   - Revoke access from others (except owner)
   - Full control equivalent to owner
3. Team member CANNOT:
   - Transfer ownership
   - Delete the share granted by owner to themselves (owner must revoke)

### Example 4: Hiding Shared Terminals

**Scenario**: Student receives 20 shared terminals from various instructors over a semester. After completing courses, student wants to hide old terminals.

**Steps**:
1. Student views shared terminals:
   ```bash
   curl -X GET http://localhost:8080/api/v1/terminals/shared-with-me \
     -H "Authorization: Bearer {student-token}"
   ```
2. Student hides completed terminals:
   ```bash
   curl -X POST http://localhost:8080/api/v1/terminals/{terminal-id}/hide \
     -H "Authorization: Bearer {student-token}"
   ```
3. Hidden terminals no longer appear in default list
4. Student can unhide anytime:
   ```bash
   curl -X DELETE http://localhost:8080/api/v1/terminals/{terminal-id}/hide \
     -H "Authorization: Bearer {student-token}"
   ```
5. Instructor's view is unaffected (still sees terminal as shared)

## 10. Security Considerations

### Access Control Validation

**Multi-Layer Security**:
1. **Authentication Middleware**: Validates JWT token
2. **Casbin Authorization**: Checks route-level permissions
3. **Service Layer Validation**: Verifies ownership and access levels
4. **Repository Layer**: Filters expired/inactive shares

### Permission Isolation

**User-Specific Permissions**:
- Each permission includes `userID` as first parameter
- Permissions are scoped to specific terminal UUIDs
- No wildcard permissions (e.g., cannot grant access to all terminals)

**Example**:
```
Allowed: ("user-123", "/api/v1/terminals/abc-uuid", "GET")
NOT Allowed: ("user-123", "/api/v1/terminals/*", "GET")
```

### Expiration Handling

**Automatic Expiration**:
- `IsExpired()` method checks `expires_at` timestamp
- `HasAccess()` returns `false` for expired shares
- Repository queries filter out expired shares automatically
- No cron job needed (validation on-demand)

**Code Reference**: `src/terminalTrainer/models/terminalShare.go:33-42`

```go
func (ts *TerminalShare) IsExpired() bool {
    if ts.ExpiresAt == nil {
        return false // NULL = never expires
    }
    return time.Now().After(*ts.ExpiresAt)
}
```

### Owner Protection

**Cannot Be Locked Out**:
- Owner ALWAYS has full access (service layer override)
- Owner cannot share terminal with themselves (validation blocks)
- Owner cannot revoke their own access
- Shares cannot modify ownership

**Code Reference**: `src/terminalTrainer/services/terminalTrainerService.go:1025-1028`

```go
// Owner always has full access
if terminal.UserID == userID {
    return true, nil
}
```

### Activation Toggle

**Manual Disable Without Deletion**:
- `is_active` field allows temporary deactivation
- Preserves share configuration for reactivation
- Safer than deletion (no data loss)

**Use Case**: Temporarily revoke access during sensitive operations, then restore.

## 11. Database Schema

### TerminalShares Table

```sql
CREATE TABLE terminal_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL,

    terminal_id UUID NOT NULL REFERENCES terminals(id) ON DELETE CASCADE,
    shared_with_user_id VARCHAR(255) NOT NULL,
    shared_by_user_id VARCHAR(255) NOT NULL,
    access_level VARCHAR(50) NOT NULL CHECK (access_level IN ('read', 'write', 'admin')),
    expires_at TIMESTAMP DEFAULT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    is_hidden_by_recipient BOOLEAN DEFAULT FALSE,
    hidden_at TIMESTAMP DEFAULT NULL,

    UNIQUE (terminal_id, shared_with_user_id)
);

CREATE INDEX idx_terminal_shares_terminal_id ON terminal_shares(terminal_id);
CREATE INDEX idx_terminal_shares_shared_with_user_id ON terminal_shares(shared_with_user_id);
CREATE INDEX idx_terminal_shares_shared_by_user_id ON terminal_shares(shared_by_user_id);
CREATE INDEX idx_terminal_shares_expires_at ON terminal_shares(expires_at) WHERE expires_at IS NOT NULL;
```

**Key Constraints**:
- `UNIQUE (terminal_id, shared_with_user_id)`: One share per terminal-user pair
- `CHECK (access_level IN ('read', 'write', 'admin'))`: Valid access levels only
- `ON DELETE CASCADE`: Deleting terminal removes all shares

### Terminals Table (Hiding Fields)

```sql
CREATE TABLE terminals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- ... other fields
    is_hidden BOOLEAN DEFAULT FALSE,
    hidden_at TIMESTAMP DEFAULT NULL
);

CREATE INDEX idx_terminals_is_hidden ON terminals(is_hidden);
```

## 12. Testing Guidelines

### Unit Testing Access Levels

**Test Cases**:
1. READ access cannot modify terminals
2. WRITE access can modify but not delete
3. ADMIN access has full control
4. Owner always has access regardless of shares
5. Expired shares return `false` for `HasAccess()`
6. Inactive shares block access

**Code Reference**: `tests/terminalTrainer/terminalShare_test.go` (if exists)

### Integration Testing Sharing Flow

**Test Scenarios**:
1. Share terminal → verify Casbin permissions added
2. Revoke access → verify permissions removed
3. Update access level → verify permissions updated
4. Expired share → verify access blocked
5. Deactivate share → verify access blocked
6. Hide terminal (owner) → verify visibility change
7. Hide terminal (recipient) → verify independent visibility

### Testing Casbin Permission Enforcement

**Verification Steps**:
1. Share terminal with READ access
2. Attempt PATCH request as recipient → should return 403
3. Share same terminal with WRITE access
4. Attempt PATCH request as recipient → should return 200
5. Revoke access
6. Attempt GET request as recipient → should return 403

## 13. Future Enhancements

### Potential Improvements

1. **Group Sharing**: Share terminal with entire groups instead of individual users
2. **Share Templates**: Predefined access configurations for common use cases
3. **Audit Logging**: Track all sharing actions for compliance
4. **Notification System**: Notify users when terminals are shared with them
5. **Access Request System**: Allow users to request access to terminals
6. **Cascade Sharing**: Allow admin-level users to re-share terminals
7. **Custom Access Levels**: Define organization-specific permission sets
8. **Usage Analytics**: Track shared terminal usage metrics

### Known Limitations

1. **No Transfer Ownership**: Original owner cannot transfer ownership to another user
2. **No Revoke by Recipient**: Recipients cannot decline/remove shared access themselves
3. **No Share Limits**: No maximum number of shares per terminal
4. **No Bulk Operations**: Cannot share one terminal with multiple users in single request
5. **No Share History**: Deleted shares are not tracked historically

## 14. Troubleshooting

### Common Issues

#### Issue: User cannot access shared terminal (403 Forbidden)

**Possible Causes**:
1. Share is expired (`expires_at` passed)
2. Share is inactive (`is_active = false`)
3. Casbin permissions not properly added
4. JWT token doesn't match `shared_with_user_id`

**Debugging Steps**:
1. Check database: `SELECT * FROM terminal_shares WHERE shared_with_user_id = 'user-id'`
2. Verify expiration: `SELECT expires_at, is_active FROM terminal_shares WHERE id = 'share-id'`
3. Check Casbin policies: Query enforcer for user's permissions
4. Verify JWT token claims

#### Issue: Permissions not updated after access level change

**Possible Causes**:
1. Casbin cache not refreshed
2. Old permissions not removed before adding new ones

**Solution**:
1. Restart application to reload Casbin enforcer
2. Manually remove old permissions via `removeTerminalSharePermissions()`
3. Re-add permissions via `addTerminalSharePermissions()`

#### Issue: Cannot hide active terminal

**Cause**: Hiding restricted to inactive terminals only

**Solution**:
1. Stop terminal first
2. Then hide terminal

#### Issue: Hidden terminals still appear in shared list

**Possible Causes**:
1. `include_hidden=true` query parameter used
2. Hiding applied to wrong user (owner vs recipient)

**Solution**:
1. Verify query parameter: `GET /api/v1/terminals/shared-with-me?include_hidden=false`
2. Check `is_hidden_by_recipient` field for recipient, `is_hidden` for owner

## 15. Code References Summary

### Models
- `src/terminalTrainer/models/terminalShare.go` - TerminalShare model and access logic

### Services
- `src/terminalTrainer/services/terminalTrainerService.go:860-1330` - Sharing service methods
  - Lines 860-956: ShareTerminal
  - Lines 971-1017: RevokeTerminalAccess
  - Lines 1019-1037: HasTerminalAccess
  - Lines 1039-1074: GetSharedTerminals
  - Lines 1133-1162: HideTerminal
  - Lines 1164-1188: UnhideTerminal
  - Lines 1190-1298: Casbin permission management

### Repositories
- `src/terminalTrainer/repositories/terminalRepository.go:362-447` - Database queries
  - Lines 362-379: GetSharedTerminalsForUser
  - Lines 381-396: GetTerminalShareByUserAndTerminal
  - Lines 398-414: GetSharedTerminalsForUserWithHidden
  - Lines 416-429: HideTerminalForUser
  - Lines 431-447: UnhideTerminalForUser

### Routes
- `src/terminalTrainer/routes/terminalRoutes.go:27-43` - API endpoints

### Controllers
- `src/terminalTrainer/routes/terminalController.go:874-1201` - HTTP handlers

### Entity Registration
- `src/terminalTrainer/entityRegistration/terminalShareRegistration.go` - Generic CRUD config

## 16. Conclusion

The OCF Core terminal sharing system provides a robust, hierarchical access control mechanism with:

- **Three access levels** (read, write, admin) with inheritance
- **Time-based expiration** for temporary access
- **Manual activation toggle** for flexible control
- **Independent hiding** for both owners and recipients
- **Automatic Casbin integration** for route-level enforcement
- **Owner protection** ensuring owners cannot be locked out
- **Granular permissions** scoped to specific terminals and users

The system is production-ready and extensible for future enhancements like group sharing, audit logging, and custom access levels.
# User Settings Frontend Integration Guide

## Overview

The OCF Core API now provides a complete user preferences system. This guide shows how to integrate it into your frontend application.

## API Endpoints

### Base URL
```
http://localhost:8080/api/v1
```

### Authentication
All endpoints require a Bearer token in the Authorization header:
```
Authorization: Bearer YOUR_JWT_TOKEN
```

---

## 📋 Available Endpoints

### 1. Get Current User Settings
**GET** `/users/me/settings`

Retrieves the current user's settings. If settings don't exist, they will be automatically created with defaults.

**Response 200:**
```json
{
  "id": 1,
  "user_id": "1d660660-7637-4a5d-9d1e-8d05bbf7363f",
  "default_landing_page": "/dashboard",
  "preferred_language": "en",
  "timezone": "UTC",
  "theme": "light",
  "compact_mode": false,
  "email_notifications": true,
  "desktop_notifications": false,
  "password_last_changed": "2025-10-09T10:30:00Z",
  "two_factor_enabled": false,
  "created_at": "2025-10-09T09:00:00Z",
  "updated_at": "2025-10-09T09:00:00Z"
}
```

---

### 2. Update User Settings
**PATCH** `/users/me/settings`

Updates specific user preferences. All fields are optional - only send what you want to update.

**Request Body:**
```json
{
  "default_landing_page": "/courses",
  "preferred_language": "fr",
  "theme": "dark"
}
```

**Response 200:** (Returns updated settings)
```json
{
  "id": 1,
  "user_id": "1d660660-7637-4a5d-9d1e-8d05bbf7363f",
  "default_landing_page": "/courses",
  "preferred_language": "fr",
  "timezone": "UTC",
  "theme": "dark",
  "compact_mode": false,
  "email_notifications": true,
  "desktop_notifications": false,
  "password_last_changed": null,
  "two_factor_enabled": false,
  "created_at": "2025-10-09T09:00:00Z",
  "updated_at": "2025-10-09T10:45:00Z"
}
```

---

### 3. Change Password
**POST** `/users/me/change-password`

Securely changes the user's password. Requires current password verification.

**Request Body:**
```json
{
  "current_password": "oldPassword123",
  "new_password": "newSecurePassword456",
  "confirm_password": "newSecurePassword456"
}
```

**Response 200:**
```json
{
  "message": "Password changed successfully"
}
```

**Response 401:** (Invalid current password)
```json
{
  "error": "current password is incorrect"
}
```

**Response 400:** (Validation error)
```json
{
  "error": "new password and confirmation do not match"
}
```

---

## 🎨 Frontend Implementation Examples

### React / TypeScript

#### Types Definition
```typescript
// types/userSettings.ts
export interface UserSettings {
  id: number;
  user_id: string;
  default_landing_page: string;
  preferred_language: string;
  timezone: string;
  theme: 'light' | 'dark' | 'auto';
  compact_mode: boolean;
  email_notifications: boolean;
  desktop_notifications: boolean;
  password_last_changed: string | null;
  two_factor_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface UpdateSettingsRequest {
  default_landing_page?: string;
  preferred_language?: string;
  timezone?: string;
  theme?: 'light' | 'dark' | 'auto';
  compact_mode?: boolean;
  email_notifications?: boolean;
  desktop_notifications?: boolean;
}

export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
  confirm_password: string;
}
```

#### API Service
```typescript
// services/userSettingsService.ts
import axios from 'axios';
import { UserSettings, UpdateSettingsRequest, ChangePasswordRequest } from '../types/userSettings';

const API_BASE_URL = 'http://localhost:8080/api/v1';

// Get JWT token from your auth store/context
const getAuthToken = () => localStorage.getItem('access_token');

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add auth token to all requests
api.interceptors.request.use((config) => {
  const token = getAuthToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

export const userSettingsService = {
  // Get current user settings
  async getSettings(): Promise<UserSettings> {
    const response = await api.get<UserSettings>('/users/me/settings');
    return response.data;
  },

  // Update specific settings
  async updateSettings(updates: UpdateSettingsRequest): Promise<UserSettings> {
    const response = await api.patch<UserSettings>('/users/me/settings', updates);
    return response.data;
  },

  // Change password
  async changePassword(data: ChangePasswordRequest): Promise<void> {
    await api.post('/users/me/change-password', data);
  },
};
```

#### React Component Example - Settings Page
```typescript
// components/SettingsPage.tsx
import React, { useState, useEffect } from 'react';
import { userSettingsService } from '../services/userSettingsService';
import type { UserSettings } from '../types/userSettings';

export const SettingsPage: React.FC = () => {
  const [settings, setSettings] = useState<UserSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Load settings on mount
  useEffect(() => {
    loadSettings();
  }, []);

  const loadSettings = async () => {
    try {
      setLoading(true);
      const data = await userSettingsService.getSettings();
      setSettings(data);
    } catch (err) {
      setError('Failed to load settings');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const updateSetting = async (key: keyof UserSettings, value: any) => {
    try {
      const updated = await userSettingsService.updateSettings({ [key]: value });
      setSettings(updated);
    } catch (err) {
      alert('Failed to update setting');
      console.error(err);
    }
  };

  if (loading) return <div>Loading settings...</div>;
  if (error) return <div>Error: {error}</div>;
  if (!settings) return null;

  return (
    <div className="settings-page">
      <h1>User Settings</h1>

      {/* Theme Selection */}
      <section>
        <h2>Appearance</h2>
        <label>
          Theme:
          <select
            value={settings.theme}
            onChange={(e) => updateSetting('theme', e.target.value)}
          >
            <option value="light">Light</option>
            <option value="dark">Dark</option>
            <option value="auto">Auto</option>
          </select>
        </label>

        <label>
          <input
            type="checkbox"
            checked={settings.compact_mode}
            onChange={(e) => updateSetting('compact_mode', e.target.checked)}
          />
          Compact Mode
        </label>
      </section>

      {/* Language Selection */}
      <section>
        <h2>Localization</h2>
        <label>
          Language:
          <select
            value={settings.preferred_language}
            onChange={(e) => updateSetting('preferred_language', e.target.value)}
          >
            <option value="en">English</option>
            <option value="fr">Français</option>
            <option value="es">Español</option>
            <option value="de">Deutsch</option>
          </select>
        </label>

        <label>
          Timezone:
          <select
            value={settings.timezone}
            onChange={(e) => updateSetting('timezone', e.target.value)}
          >
            <option value="UTC">UTC</option>
            <option value="Europe/Paris">Europe/Paris</option>
            <option value="America/New_York">America/New York</option>
            <option value="Asia/Tokyo">Asia/Tokyo</option>
          </select>
        </label>
      </section>

      {/* Navigation */}
      <section>
        <h2>Navigation</h2>
        <label>
          Default Landing Page:
          <select
            value={settings.default_landing_page}
            onChange={(e) => updateSetting('default_landing_page', e.target.value)}
          >
            <option value="/dashboard">Dashboard</option>
            <option value="/courses">Courses</option>
            <option value="/terminals">Terminals</option>
            <option value="/labs">Labs</option>
          </select>
        </label>
      </section>

      {/* Notifications */}
      <section>
        <h2>Notifications</h2>
        <label>
          <input
            type="checkbox"
            checked={settings.email_notifications}
            onChange={(e) => updateSetting('email_notifications', e.target.checked)}
          />
          Email Notifications
        </label>

        <label>
          <input
            type="checkbox"
            checked={settings.desktop_notifications}
            onChange={(e) => updateSetting('desktop_notifications', e.target.checked)}
          />
          Desktop Notifications
        </label>
      </section>
    </div>
  );
};
```

#### Password Change Component
```typescript
// components/ChangePasswordForm.tsx
import React, { useState } from 'react';
import { userSettingsService } from '../services/userSettingsService';

export const ChangePasswordForm: React.FC = () => {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(false);

    // Client-side validation
    if (newPassword !== confirmPassword) {
      setError('New passwords do not match');
      return;
    }

    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters long');
      return;
    }

    try {
      await userSettingsService.changePassword({
        current_password: currentPassword,
        new_password: newPassword,
        confirm_password: confirmPassword,
      });

      setSuccess(true);
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (err: any) {
      const errorMessage = err.response?.data?.error || 'Failed to change password';
      setError(errorMessage);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="change-password-form">
      <h2>Change Password</h2>

      {error && <div className="error">{error}</div>}
      {success && <div className="success">Password changed successfully!</div>}

      <label>
        Current Password:
        <input
          type="password"
          value={currentPassword}
          onChange={(e) => setCurrentPassword(e.target.value)}
          required
        />
      </label>

      <label>
        New Password:
        <input
          type="password"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          required
          minLength={8}
        />
      </label>

      <label>
        Confirm New Password:
        <input
          type="password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          required
          minLength={8}
        />
      </label>

      <button type="submit">Change Password</button>
    </form>
  );
};
```

---

### Vue 3 / TypeScript

#### Composable
```typescript
// composables/useUserSettings.ts
import { ref, computed } from 'vue';
import axios from 'axios';
import type { UserSettings, UpdateSettingsRequest } from '@/types/userSettings';

const API_BASE_URL = 'http://localhost:8080/api/v1';

export function useUserSettings() {
  const settings = ref<UserSettings | null>(null);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const api = axios.create({
    baseURL: API_BASE_URL,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${localStorage.getItem('access_token')}`,
    },
  });

  const loadSettings = async () => {
    try {
      loading.value = true;
      error.value = null;
      const response = await api.get<UserSettings>('/users/me/settings');
      settings.value = response.data;
    } catch (err: any) {
      error.value = err.response?.data?.error || 'Failed to load settings';
    } finally {
      loading.value = false;
    }
  };

  const updateSettings = async (updates: UpdateSettingsRequest) => {
    try {
      const response = await api.patch<UserSettings>('/users/me/settings', updates);
      settings.value = response.data;
    } catch (err: any) {
      throw new Error(err.response?.data?.error || 'Failed to update settings');
    }
  };

  const changePassword = async (currentPassword: string, newPassword: string, confirmPassword: string) => {
    try {
      await api.post('/users/me/change-password', {
        current_password: currentPassword,
        new_password: newPassword,
        confirm_password: confirmPassword,
      });
    } catch (err: any) {
      throw new Error(err.response?.data?.error || 'Failed to change password');
    }
  };

  return {
    settings: computed(() => settings.value),
    loading: computed(() => loading.value),
    error: computed(() => error.value),
    loadSettings,
    updateSettings,
    changePassword,
  };
}
```

---

## 🎯 Quick Start Checklist

1. **Authentication**: Ensure you have a valid JWT token from login
2. **Fetch Settings**: Call `GET /users/me/settings` on app load or settings page mount
3. **Update on Change**: Call `PATCH /users/me/settings` with only the changed fields
4. **Apply Locally**: Update your app's theme/language/etc based on the settings
5. **Persist**: Settings are automatically stored in the database

---

## 🔒 Security Notes

- All endpoints require authentication via Bearer token
- Password changes require the current password for verification
- New passwords must be at least 8 characters long
- Users can only access and modify their own settings

---

## 🌐 Available Options

### Default Landing Pages
- `/dashboard` - Main dashboard
- `/courses` - Course list
- `/terminals` - Terminal sessions
- `/labs` - Lab environments

### Languages
- `en` - English
- `fr` - Français
- `es` - Español
- `de` - Deutsch
- `it` - Italiano
- (Add more as needed in your frontend)

### Themes
- `light` - Light mode
- `dark` - Dark mode
- `auto` - System preference

### Timezones
Use standard IANA timezone identifiers:
- `UTC`
- `Europe/Paris`
- `America/New_York`
- `Asia/Tokyo`
- etc.

---

## 🐛 Error Handling

### Common Error Responses

**401 Unauthorized**
```json
{
  "error": "User not authenticated"
}
```

**404 Not Found** (shouldn't happen anymore - auto-creates)
```json
{
  "error": "Settings not found"
}
```

**400 Bad Request**
```json
{
  "error": "new password and confirmation do not match"
}
```

---

## 📝 Testing with cURL

```bash
# 1. Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"1.supervisor@test.com","password":"test"}' \
  | jq -r '.access_token')

# 2. Get settings
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/users/me/settings | jq

# 3. Update theme
curl -X PATCH \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"theme":"dark"}' \
  http://localhost:8080/api/v1/users/me/settings | jq

# 4. Change password
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "current_password":"test",
    "new_password":"newPassword123",
    "confirm_password":"newPassword123"
  }' \
  http://localhost:8080/api/v1/users/me/change-password
```

---

## 🔄 Automatic Settings Creation

Settings are now automatically created with defaults when:
1. A new user registers (via the user creation hook)
2. An existing user accesses `GET /users/me/settings` for the first time

**Default Values:**
- Default Landing Page: `/dashboard`
- Language: `en`
- Timezone: `UTC`
- Theme: `light`
- Compact Mode: `false`
- Email Notifications: `true`
- Desktop Notifications: `false`
- Two-Factor Enabled: `false`

---

## 🚀 Production Recommendations

1. **Cache Settings**: Store settings in your state management (Redux, Vuex, Pinia, etc.)
2. **Debounce Updates**: Don't send a PATCH request on every keystroke - debounce or save on blur
3. **Optimistic Updates**: Update UI immediately, rollback on error
4. **Error Handling**: Show user-friendly error messages
5. **Loading States**: Show spinners/skeletons while loading
6. **Validation**: Validate on frontend before sending to API

---

## 📚 Full API Documentation

Visit the Swagger UI for complete API documentation:
```
http://localhost:8080/swagger/
```

Look for the `user-settings` tag in the Swagger UI.
