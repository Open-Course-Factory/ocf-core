package utils

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"sort"
	"strings"
	"unicode/utf8"

	access "soli/formations/src/auth/access"
	"soli/formations/src/organizations/dto"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

var columnAliases = map[string]string{
	"e-mail":         "email",
	"mail":           "email",
	"nom":            "name",
	"prénom":         "first_name",
	"prenom":         "first_name",
	"nom de famille": "last_name",
}

// candidateDelimiters are tried on the header line; the one that splits it
// into the most fields wins, comma first on a tie so a plain one-column file
// keeps the RFC 4180 default.
var candidateDelimiters = []rune{',', ';', '\t'}

func resolveColumnAliases(headerMap map[string]int) {
	for alias, canonical := range columnAliases {
		if idx, exists := headerMap[alias]; exists {
			if _, alreadyHasCanonical := headerMap[canonical]; !alreadyHasCanonical {
				headerMap[canonical] = idx
			}
			delete(headerMap, alias)
		}
	}
}

// aliasesFor lists the accepted spellings of a canonical column, derived from
// columnAliases so the error message can never drift from what the parser accepts.
func aliasesFor(canonical string) []string {
	var aliases []string
	for alias, target := range columnAliases {
		if target == canonical {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	return aliases
}

func describeExpectedColumn(canonical string) string {
	aliases := aliasesFor(canonical)
	if len(aliases) == 0 {
		return canonical
	}
	return fmt.Sprintf("%s (or %s)", canonical, strings.Join(aliases, ", "))
}

func openAndDecodeCSV(file *multipart.FileHeader) ([]byte, error) {
	f, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	if !utf8.Valid(data) {
		decoder := charmap.Windows1252.NewDecoder()
		utf8Data, _, err := transform.Bytes(decoder, data)
		if err != nil {
			return nil, fmt.Errorf("failed to convert encoding: %w", err)
		}
		data = utf8Data
	}

	return data, nil
}

func detectDelimiter(data []byte) rune {
	headerLine, _, _ := bytes.Cut(data, []byte("\n"))
	best := candidateDelimiters[0]
	bestCount := 0
	for _, candidate := range candidateDelimiters {
		if count := bytes.Count(headerLine, []byte(string(candidate))); count > bestCount {
			best, bestCount = candidate, count
		}
	}
	return best
}

// csvTable is an opened CSV file positioned after its header row.
type csvTable struct {
	fileName  string
	reader    *csv.Reader
	delimiter rune
	header    []string
	columns   map[string]int
	rowNum    int
}

func headerError(fileName, field, message string) *dto.ImportError {
	return &dto.ImportError{
		Row:     0,
		File:    fileName,
		Field:   field,
		Message: message,
		Code:    dto.ErrCodeValidation,
	}
}

func openCSVTable(file *multipart.FileHeader, fileName string) (*csvTable, *dto.ImportError) {
	data, err := openAndDecodeCSV(file)
	if err != nil {
		return nil, headerError(fileName, "", fmt.Sprintf("Could not open %s file: %v", fileName, err))
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, headerError(fileName, "", fmt.Sprintf("The %s file is empty", fileName))
	}

	delimiter := detectDelimiter(data)
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delimiter
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, headerError(fileName, "", fmt.Sprintf("Could not read CSV header: %v", err))
	}

	columns := make(map[string]int, len(header))
	for i, col := range header {
		columns[strings.TrimSpace(strings.ToLower(col))] = i
	}

	return &csvTable{
		fileName:  fileName,
		reader:    reader,
		delimiter: delimiter,
		header:    header,
		columns:   columns,
		rowNum:    1, // header errors report row 0; data rows count from 2, matching spreadsheet line numbers
	}, nil
}

func (t *csvTable) has(column string) bool {
	_, exists := t.columns[column]
	return exists
}

func (t *csvTable) foundColumns() string {
	trimmed := make([]string, len(t.header))
	for i, col := range t.header {
		trimmed[i] = strings.TrimSpace(col)
	}
	return "[" + strings.Join(trimmed, "; ") + "]"
}

func (t *csvTable) missingColumnError(field, missing, expected string) *dto.ImportError {
	return headerError(t.fileName, field,
		fmt.Sprintf("%s. Found columns: %s. Expected: %s", missing, t.foundColumns(), expected))
}

func (t *csvTable) requireColumns(required ...string) *dto.ImportError {
	for _, column := range required {
		if !t.has(column) {
			return t.missingColumnError(column,
				fmt.Sprintf("Missing required column: %s", column),
				describeExpectedColumn(column))
		}
	}
	return nil
}

// nextRow returns the next data row; ok is false once the file is exhausted.
// A malformed row yields an ImportError naming the row and its content so the
// caller can report it instead of silently dropping a line.
func (t *csvTable) nextRow() (record []string, rowErr *dto.ImportError, ok bool) {
	record, err := t.reader.Read()
	if err == io.EOF {
		return nil, nil, false
	}
	t.rowNum++
	if err == nil {
		return record, nil, true
	}

	message := fmt.Sprintf("Row %d could not be read: %v", t.rowNum, err)
	if errors.Is(err, csv.ErrFieldCount) {
		message = fmt.Sprintf("Row %d has %d field(s) but the header has %d (separator %q): %q",
			t.rowNum, len(record), len(t.header), t.delimiter, t.joinRow(record))
	}
	return nil, &dto.ImportError{
		Row:     t.rowNum,
		File:    t.fileName,
		Message: message,
		Code:    dto.ErrCodeValidation,
	}, true
}

func (t *csvTable) joinRow(record []string) string {
	return strings.Join(record, string(t.delimiter))
}

func (t *csvTable) value(record []string, column string) string {
	return getColumnValue(record, t.columns, column)
}

func ParseUsersCSV(file *multipart.FileHeader) ([]dto.UserImportRow, []dto.ImportError, []dto.ImportWarning) {
	table, headerErr := openCSVTable(file, "users")
	if headerErr != nil {
		return nil, []dto.ImportError{*headerErr}, nil
	}
	resolveColumnAliases(table.columns)

	if !table.has("email") {
		return nil, []dto.ImportError{*table.missingColumnError("email",
			"Missing required column: email",
			describeExpectedColumn("email"))}, nil
	}

	hasSplitName := table.has("first_name") && table.has("last_name")
	if !hasSplitName && !table.has("name") {
		return nil, []dto.ImportError{*table.missingColumnError("name",
			"Missing required columns: need (first_name AND last_name) or name",
			fmt.Sprintf("%s, or %s + %s",
				describeExpectedColumn("name"),
				describeExpectedColumn("first_name"),
				describeExpectedColumn("last_name")))}, nil
	}

	users := make([]dto.UserImportRow, 0, 100)
	importErrors := make([]dto.ImportError, 0, 10)
	warnings := make([]dto.ImportWarning, 0, 10)

	for {
		record, rowErr, ok := table.nextRow()
		if !ok {
			break
		}
		if rowErr != nil {
			importErrors = append(importErrors, *rowErr)
			continue
		}

		user := dto.UserImportRow{
			Email:          table.value(record, "email"),
			FirstName:      table.value(record, "first_name"),
			LastName:       table.value(record, "last_name"),
			Password:       table.value(record, "password"),
			Role:           table.value(record, "role"),
			ExternalID:     table.value(record, "external_id"),
			ForceReset:     table.value(record, "force_reset"),
			UpdateIfExists: table.value(record, "update_existing"),
			Name:           table.value(record, "name"),
		}

		rowErrors, rowWarnings := validateUserRow(&user, table.rowNum, table.joinRow(record))
		if len(rowWarnings) > 0 {
			warnings = append(warnings, rowWarnings...)
		}
		if len(rowErrors) > 0 {
			importErrors = append(importErrors, rowErrors...)
			continue
		}

		users = append(users, user)
	}

	return users, importErrors, warnings
}

func ParseGroupsCSV(file *multipart.FileHeader) ([]dto.GroupImportRow, []dto.ImportError) {
	table, headerErr := openCSVTable(file, "groups")
	if headerErr != nil {
		return nil, []dto.ImportError{*headerErr}
	}
	if missing := table.requireColumns("group_name", "display_name"); missing != nil {
		return nil, []dto.ImportError{*missing}
	}

	groups := make([]dto.GroupImportRow, 0, 50)
	importErrors := make([]dto.ImportError, 0, 10)

	for {
		record, rowErr, ok := table.nextRow()
		if !ok {
			break
		}
		if rowErr != nil {
			importErrors = append(importErrors, *rowErr)
			continue
		}

		group := dto.GroupImportRow{
			GroupName:   table.value(record, "group_name"),
			DisplayName: table.value(record, "display_name"),
			Description: table.value(record, "description"),
			ParentGroup: table.value(record, "parent_group"),
			MaxMembers:  table.value(record, "max_members"),
			ExpiresAt:   table.value(record, "expires_at"),
			ExternalID:  table.value(record, "external_id"),
		}

		rowErrors := validateGroupRow(group, table.rowNum)
		if len(rowErrors) > 0 {
			importErrors = append(importErrors, rowErrors...)
			continue
		}

		groups = append(groups, group)
	}

	return groups, importErrors
}

func ParseMembershipsCSV(file *multipart.FileHeader) ([]dto.MembershipImportRow, []dto.ImportError) {
	table, headerErr := openCSVTable(file, "memberships")
	if headerErr != nil {
		return nil, []dto.ImportError{*headerErr}
	}
	if missing := table.requireColumns("user_email", "group_name", "role"); missing != nil {
		return nil, []dto.ImportError{*missing}
	}

	memberships := make([]dto.MembershipImportRow, 0, 200)
	importErrors := make([]dto.ImportError, 0, 20)

	for {
		record, rowErr, ok := table.nextRow()
		if !ok {
			break
		}
		if rowErr != nil {
			importErrors = append(importErrors, *rowErr)
			continue
		}

		membership := dto.MembershipImportRow{
			UserEmail: table.value(record, "user_email"),
			GroupName: table.value(record, "group_name"),
			Role:      table.value(record, "role"),
		}

		rowErrors := validateMembershipRow(membership, table.rowNum)
		if len(rowErrors) > 0 {
			importErrors = append(importErrors, rowErrors...)
			continue
		}

		memberships = append(memberships, membership)
	}

	return memberships, importErrors
}

func getColumnValue(record []string, headerMap map[string]int, columnName string) string {
	if idx, exists := headerMap[columnName]; exists && idx < len(record) {
		return strings.TrimSpace(record[idx])
	}
	return ""
}

// validateUserRow validates a user row and performs name splitting if needed.
// It accepts a pointer so it can set FirstName/LastName from the Name field.
// Returns (errors, warnings).
func validateUserRow(user *dto.UserImportRow, rowNum int, rowContent string) ([]dto.ImportError, []dto.ImportWarning) {
	var errors []dto.ImportError
	var warnings []dto.ImportWarning
	rowRef := fmt.Sprintf(" (row %d: %q)", rowNum, rowContent)

	if user.Email == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "email",
			Message: "Email is required" + rowRef,
			Code:    dto.ErrCodeValidation,
		})
	}

	// Name splitting: if Name is set but FirstName/LastName are empty
	if user.Name != "" && user.FirstName == "" && user.LastName == "" {
		lastSpace := strings.LastIndex(user.Name, " ")
		if lastSpace == -1 {
			// Single word: last name only
			user.LastName = user.Name
			warnings = append(warnings, dto.ImportWarning{
				Row:     rowNum,
				File:    "users",
				Message: fmt.Sprintf("Name '%s' has no space; used as last name only (empty first name)", user.Name),
			})
		} else {
			user.LastName = user.Name[:lastSpace]
			user.FirstName = user.Name[lastSpace+1:]
		}
	}

	if user.FirstName == "" && user.LastName == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "name",
			Message: "Name is required: provide first_name and last_name, or name" + rowRef,
			Code:    dto.ErrCodeValidation,
		})
	}

	// Validate role only when provided (non-empty)
	validRoles := map[string]bool{"member": true, "supervisor": true, "admin": true, "trainer": true}
	if user.Role != "" && !validRoles[strings.ToLower(user.Role)] {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "role",
			Message: fmt.Sprintf("Invalid role '%s'. Must be one of: member, supervisor, admin, trainer", user.Role),
			Code:    dto.ErrCodeInvalidRole,
		})
	}

	// Validate email format (basic)
	if user.Email != "" && !strings.Contains(user.Email, "@") {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "email",
			Message: "Invalid email format",
			Code:    dto.ErrCodeInvalidEmail,
		})
	}

	return errors, warnings
}

// validateGroupRow validates a group row
func validateGroupRow(group dto.GroupImportRow, rowNum int) []dto.ImportError {
	var errors []dto.ImportError

	if group.GroupName == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "groups",
			Field:   "group_name",
			Message: "Group name is required",
			Code:    dto.ErrCodeValidation,
		})
	}

	if group.DisplayName == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "groups",
			Field:   "display_name",
			Message: "Display name is required",
			Code:    dto.ErrCodeValidation,
		})
	}

	return errors
}

// validateMembershipRow validates a membership row
func validateMembershipRow(membership dto.MembershipImportRow, rowNum int) []dto.ImportError {
	var errors []dto.ImportError

	if membership.UserEmail == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "memberships",
			Field:   "user_email",
			Message: "User email is required",
			Code:    dto.ErrCodeValidation,
		})
	}

	if membership.GroupName == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "memberships",
			Field:   "group_name",
			Message: "Group name is required",
			Code:    dto.ErrCodeValidation,
		})
	}

	// Validate role against the single hierarchy rather than a literal list, so a
	// role registered in auth/access is importable the day it exists instead of
	// being rejected by a copy nobody remembered to update (#460).
	if membership.Role == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "memberships",
			Field:   "role",
			Message: "Role is required",
			Code:    dto.ErrCodeValidation,
		})
	} else if !access.IsKnownRole(strings.ToLower(membership.Role)) {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "memberships",
			Field:   "role",
			Message: fmt.Sprintf("Invalid role '%s'. Must be one of: %s", membership.Role, access.KnownRolesForMessage()),
			Code:    dto.ErrCodeInvalidRole,
		})
	}

	return errors
}
