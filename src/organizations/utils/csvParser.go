package utils

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"strings"
	"unicode/utf8"

	"soli/formations/src/organizations/dto"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// columnAliases maps alternative column names to their canonical names.
// Keys must be lowercase (headerMap is already lowercased).
var columnAliases = map[string]string{
	"e-mail":          "email",
	"mail":            "email",
	"nom":             "name",
	"prénom":          "first_name",
	"prenom":          "first_name",
	"nom de famille":  "last_name",
}

// resolveColumnAliases replaces alias column names with their canonical equivalents.
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

// openAndDecodeCSV opens a multipart file, strips UTF-8 BOM if present,
// and converts non-UTF-8 content (assumed Windows-1252) to UTF-8.
func openAndDecodeCSV(file *multipart.FileHeader) (io.Reader, error) {
	f, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	// Strip UTF-8 BOM (EF BB BF)
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	// Check if valid UTF-8; if not, convert from Windows-1252
	if !utf8.Valid(data) {
		decoder := charmap.Windows1252.NewDecoder()
		utf8Data, _, err := transform.Bytes(decoder, data)
		if err != nil {
			return nil, fmt.Errorf("failed to convert encoding: %w", err)
		}
		data = utf8Data
	}

	return bytes.NewReader(data), nil
}

// ParseUsersCSV parses the users.csv file
func ParseUsersCSV(file *multipart.FileHeader) ([]dto.UserImportRow, []dto.ImportError, []dto.ImportWarning) {
	r, err := openAndDecodeCSV(file)
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "users",
			Message: fmt.Sprintf("Could not open users file: %v", err),
			Code:    dto.ErrCodeValidation,
		}}, nil
	}

	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "users",
			Message: fmt.Sprintf("Could not read CSV header: %v", err),
			Code:    dto.ErrCodeValidation,
		}}, nil
	}

	// Build header map and resolve aliases
	headerMap := make(map[string]int)
	for i, col := range header {
		headerMap[strings.TrimSpace(strings.ToLower(col))] = i
	}
	resolveColumnAliases(headerMap)

	// Validate required columns: email + (first_name AND last_name) OR name
	if _, hasEmail := headerMap["email"]; !hasEmail {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "users",
			Field:   "email",
			Message: "Missing required column: email",
			Code:    dto.ErrCodeValidation,
		}}, nil
	}

	_, hasFirstName := headerMap["first_name"]
	_, hasLastName := headerMap["last_name"]
	_, hasName := headerMap["name"]

	if !(hasFirstName && hasLastName) && !hasName {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "users",
			Field:   "name",
			Message: "Missing required columns: need (first_name AND last_name) or name",
			Code:    dto.ErrCodeValidation,
		}}, nil
	}

	users := make([]dto.UserImportRow, 0, 100)
	errors := make([]dto.ImportError, 0, 10)
	warnings := make([]dto.ImportWarning, 0, 10)
	rowNum := 1 // Header is row 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		if err != nil {
			errors = append(errors, dto.ImportError{
				Row:     rowNum,
				File:    "users",
				Message: fmt.Sprintf("Error reading row: %v", err),
				Code:    dto.ErrCodeValidation,
			})
			continue
		}

		// Parse row
		user := dto.UserImportRow{
			Email:          getColumnValue(record, headerMap, "email"),
			FirstName:      getColumnValue(record, headerMap, "first_name"),
			LastName:       getColumnValue(record, headerMap, "last_name"),
			Password:       getColumnValue(record, headerMap, "password"),
			Role:           getColumnValue(record, headerMap, "role"),
			ExternalID:     getColumnValue(record, headerMap, "external_id"),
			ForceReset:     getColumnValue(record, headerMap, "force_reset"),
			UpdateIfExists: getColumnValue(record, headerMap, "update_existing"),
			Name:           getColumnValue(record, headerMap, "name"),
		}

		// Validate required fields (may mutate user for name splitting)
		rowErrors, rowWarnings := validateUserRow(&user, rowNum)
		if len(rowWarnings) > 0 {
			warnings = append(warnings, rowWarnings...)
		}
		if len(rowErrors) > 0 {
			errors = append(errors, rowErrors...)
			continue
		}

		users = append(users, user)
	}

	return users, errors, warnings
}

// ParseGroupsCSV parses the groups.csv file
func ParseGroupsCSV(file *multipart.FileHeader) ([]dto.GroupImportRow, []dto.ImportError) {
	r, err := openAndDecodeCSV(file)
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "groups",
			Message: fmt.Sprintf("Could not open groups file: %v", err),
			Code:    dto.ErrCodeValidation,
		}}
	}

	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "groups",
			Message: fmt.Sprintf("Could not read CSV header: %v", err),
			Code:    dto.ErrCodeValidation,
		}}
	}

	// Validate header
	requiredColumns := []string{"group_name", "display_name"}
	headerMap := make(map[string]int)
	for i, col := range header {
		headerMap[strings.TrimSpace(strings.ToLower(col))] = i
	}

	for _, required := range requiredColumns {
		if _, exists := headerMap[required]; !exists {
			return nil, []dto.ImportError{{
				Row:     0,
				File:    "groups",
				Field:   required,
				Message: fmt.Sprintf("Missing required column: %s", required),
				Code:    dto.ErrCodeValidation,
			}}
		}
	}

	groups := make([]dto.GroupImportRow, 0, 50)
	errors := make([]dto.ImportError, 0, 10)
	rowNum := 1

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		if err != nil {
			errors = append(errors, dto.ImportError{
				Row:     rowNum,
				File:    "groups",
				Message: fmt.Sprintf("Error reading row: %v", err),
				Code:    dto.ErrCodeValidation,
			})
			continue
		}

		group := dto.GroupImportRow{
			GroupName:   getColumnValue(record, headerMap, "group_name"),
			DisplayName: getColumnValue(record, headerMap, "display_name"),
			Description: getColumnValue(record, headerMap, "description"),
			ParentGroup: getColumnValue(record, headerMap, "parent_group"),
			MaxMembers:  getColumnValue(record, headerMap, "max_members"),
			ExpiresAt:   getColumnValue(record, headerMap, "expires_at"),
			ExternalID:  getColumnValue(record, headerMap, "external_id"),
		}

		// Validate required fields
		rowErrors := validateGroupRow(group, rowNum)
		if len(rowErrors) > 0 {
			errors = append(errors, rowErrors...)
			continue
		}

		groups = append(groups, group)
	}

	return groups, errors
}

// ParseMembershipsCSV parses the memberships.csv file
func ParseMembershipsCSV(file *multipart.FileHeader) ([]dto.MembershipImportRow, []dto.ImportError) {
	r, err := openAndDecodeCSV(file)
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "memberships",
			Message: fmt.Sprintf("Could not open memberships file: %v", err),
			Code:    dto.ErrCodeValidation,
		}}
	}

	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "memberships",
			Message: fmt.Sprintf("Could not read CSV header: %v", err),
			Code:    dto.ErrCodeValidation,
		}}
	}

	// Validate header
	requiredColumns := []string{"user_email", "group_name", "role"}
	headerMap := make(map[string]int)
	for i, col := range header {
		headerMap[strings.TrimSpace(strings.ToLower(col))] = i
	}

	for _, required := range requiredColumns {
		if _, exists := headerMap[required]; !exists {
			return nil, []dto.ImportError{{
				Row:     0,
				File:    "memberships",
				Field:   required,
				Message: fmt.Sprintf("Missing required column: %s", required),
				Code:    dto.ErrCodeValidation,
			}}
		}
	}

	memberships := make([]dto.MembershipImportRow, 0, 200)
	errors := make([]dto.ImportError, 0, 20)
	rowNum := 1

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowNum++
		if err != nil {
			errors = append(errors, dto.ImportError{
				Row:     rowNum,
				File:    "memberships",
				Message: fmt.Sprintf("Error reading row: %v", err),
				Code:    dto.ErrCodeValidation,
			})
			continue
		}

		membership := dto.MembershipImportRow{
			UserEmail: getColumnValue(record, headerMap, "user_email"),
			GroupName: getColumnValue(record, headerMap, "group_name"),
			Role:      getColumnValue(record, headerMap, "role"),
		}

		// Validate required fields
		rowErrors := validateMembershipRow(membership, rowNum)
		if len(rowErrors) > 0 {
			errors = append(errors, rowErrors...)
			continue
		}

		memberships = append(memberships, membership)
	}

	return memberships, errors
}

// Helper function to get column value by name
func getColumnValue(record []string, headerMap map[string]int, columnName string) string {
	if idx, exists := headerMap[columnName]; exists && idx < len(record) {
		return strings.TrimSpace(record[idx])
	}
	return ""
}

// validateUserRow validates a user row and performs name splitting if needed.
// It accepts a pointer so it can set FirstName/LastName from the Name field.
// Returns (errors, warnings).
func validateUserRow(user *dto.UserImportRow, rowNum int) ([]dto.ImportError, []dto.ImportWarning) {
	var errors []dto.ImportError
	var warnings []dto.ImportWarning

	if user.Email == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "email",
			Message: "Email is required",
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
			Message: "Name is required: provide first_name and last_name, or name",
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

	// Validate role
	validRoles := map[string]bool{"member": true, "admin": true, "assistant": true, "owner": true}
	if membership.Role == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "memberships",
			Field:   "role",
			Message: "Role is required",
			Code:    dto.ErrCodeValidation,
		})
	} else if !validRoles[strings.ToLower(membership.Role)] {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "memberships",
			Field:   "role",
			Message: fmt.Sprintf("Invalid role '%s'. Must be one of: member, admin, assistant, owner", membership.Role),
			Code:    dto.ErrCodeInvalidRole,
		})
	}

	return errors
}
