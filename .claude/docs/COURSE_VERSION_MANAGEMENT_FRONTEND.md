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
