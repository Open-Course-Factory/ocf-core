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

**Endpoint**: `POST /api/v1/groups`

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
- [ ] Test `GET /api/v1/groups/{id}` - verify `owner_user_id` is present
- [ ] Test `GET /api/v1/groups` list - verify all groups show their owners

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
