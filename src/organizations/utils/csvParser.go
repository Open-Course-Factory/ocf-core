package utils

import (
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"soli/formations/src/organizations/dto"
)

// ParseUsersCSV parses the users.csv file
func ParseUsersCSV(file *multipart.FileHeader) ([]dto.UserImportRow, []dto.ImportError) {
	f, err := file.Open()
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "users",
			Message: fmt.Sprintf("Could not open users file: %v", err),
			Code:    dto.ErrCodeValidation,
		}}
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "users",
			Message: fmt.Sprintf("Could not read CSV header: %v", err),
			Code:    dto.ErrCodeValidation,
		}}
	}

	// Validate header
	requiredColumns := []string{"email", "first_name", "last_name", "password", "role"}
	headerMap := make(map[string]int)
	for i, col := range header {
		headerMap[strings.TrimSpace(strings.ToLower(col))] = i
	}

	for _, required := range requiredColumns {
		if _, exists := headerMap[required]; !exists {
			return nil, []dto.ImportError{{
				Row:     0,
				File:    "users",
				Field:   required,
				Message: fmt.Sprintf("Missing required column: %s", required),
				Code:    dto.ErrCodeValidation,
			}}
		}
	}

	users := make([]dto.UserImportRow, 0, 100)
	errors := make([]dto.ImportError, 0, 10)
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
		}

		// Validate required fields
		rowErrors := validateUserRow(user, rowNum)
		if len(rowErrors) > 0 {
			errors = append(errors, rowErrors...)
			continue
		}

		users = append(users, user)
	}

	return users, errors
}

// ParseGroupsCSV parses the groups.csv file
func ParseGroupsCSV(file *multipart.FileHeader) ([]dto.GroupImportRow, []dto.ImportError) {
	f, err := file.Open()
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "groups",
			Message: fmt.Sprintf("Could not open groups file: %v", err),
			Code:    dto.ErrCodeValidation,
		}}
	}
	defer f.Close()

	reader := csv.NewReader(f)
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
	f, err := file.Open()
	if err != nil {
		return nil, []dto.ImportError{{
			Row:     0,
			File:    "memberships",
			Message: fmt.Sprintf("Could not open memberships file: %v", err),
			Code:    dto.ErrCodeValidation,
		}}
	}
	defer f.Close()

	reader := csv.NewReader(f)
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

// validateUserRow validates a user row
func validateUserRow(user dto.UserImportRow, rowNum int) []dto.ImportError {
	var errors []dto.ImportError

	if user.Email == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "email",
			Message: "Email is required",
			Code:    dto.ErrCodeValidation,
		})
	}

	if user.FirstName == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "first_name",
			Message: "First name is required",
			Code:    dto.ErrCodeValidation,
		})
	}

	if user.LastName == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "last_name",
			Message: "Last name is required",
			Code:    dto.ErrCodeValidation,
		})
	}

	if user.Password == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "password",
			Message: "Password is required",
			Code:    dto.ErrCodeValidation,
		})
	}

	// Validate role
	validRoles := map[string]bool{"member": true, "supervisor": true, "admin": true, "trainer": true}
	if user.Role == "" {
		errors = append(errors, dto.ImportError{
			Row:     rowNum,
			File:    "users",
			Field:   "role",
			Message: "Role is required",
			Code:    dto.ErrCodeValidation,
		})
	} else if !validRoles[strings.ToLower(user.Role)] {
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

	return errors
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
