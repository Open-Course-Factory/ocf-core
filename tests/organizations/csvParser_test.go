package organizations_tests

import (
	"bytes"
	"io"
	"mime/multipart"
	"testing"

	"soli/formations/src/organizations/dto"
	orgUtils "soli/formations/src/organizations/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a multipart.FileHeader from CSV content
func createMultipartFileHeader(t *testing.T, filename string, content string) *multipart.FileHeader {
	// Create a buffer to write our multipart form to
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create a form file field
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)

	// Write the CSV content to the form file
	_, err = io.WriteString(part, content)
	require.NoError(t, err)

	// Close the writer to finalize the multipart form
	err = writer.Close()
	require.NoError(t, err)

	// Parse the multipart form we just created
	reader := multipart.NewReader(body, writer.Boundary())
	form, err := reader.ReadForm(32 << 20) // 32 MB max
	require.NoError(t, err)

	// Get the file header from the parsed form
	files := form.File["file"]
	require.NotEmpty(t, files, "Expected at least one file in form")

	return files[0]
}

func TestParseUsersCSV_ValidFile(t *testing.T) {
	content := `email,first_name,last_name,password,role,external_id,force_reset
john.doe@test.com,John,Doe,Pass123!,member,std_001,true
jane.smith@test.com,Jane,Smith,Pass456!,supervisor,tch_001,false`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors")
	assert.Len(t, users, 2, "Should parse 2 users")

	// Verify first user
	assert.Equal(t, "john.doe@test.com", users[0].Email)
	assert.Equal(t, "John", users[0].FirstName)
	assert.Equal(t, "Doe", users[0].LastName)
	assert.Equal(t, "Pass123!", users[0].Password)
	assert.Equal(t, "member", users[0].Role)
	assert.Equal(t, "std_001", users[0].ExternalID)
	assert.Equal(t, "true", users[0].ForceReset)

	// Verify second user
	assert.Equal(t, "jane.smith@test.com", users[1].Email)
	assert.Equal(t, "supervisor", users[1].Role)
}

func TestParseUsersCSV_MissingRequiredColumns(t *testing.T) {
	content := `email,first_name,last_name
john.doe@test.com,John,Doe`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors := orgUtils.ParseUsersCSV(fileHeader)

	assert.NotEmpty(t, errors, "Should have errors for missing columns")
	assert.Nil(t, users, "Should not return users when header is invalid")
	assert.Contains(t, errors[0].Message, "Missing required column: password")
	assert.Equal(t, dto.ErrCodeValidation, errors[0].Code)
}

func TestParseUsersCSV_InvalidEmail(t *testing.T) {
	content := `email,first_name,last_name,password,role
not-an-email,John,Doe,Pass123!,member`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors := orgUtils.ParseUsersCSV(fileHeader)

	assert.NotEmpty(t, errors, "Should have validation errors")
	assert.Empty(t, users, "Should not return invalid users")
	assert.Contains(t, errors[0].Message, "Invalid email format")
	assert.Equal(t, dto.ErrCodeInvalidEmail, errors[0].Code)
	assert.Equal(t, 2, errors[0].Row, "Error should be on row 2")
}

func TestParseUsersCSV_InvalidRole(t *testing.T) {
	content := `email,first_name,last_name,password,role
john@test.com,John,Doe,Pass123!,invalid_role`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors := orgUtils.ParseUsersCSV(fileHeader)

	assert.NotEmpty(t, errors, "Should have validation errors")
	assert.Empty(t, users, "Should not return invalid users")
	assert.Contains(t, errors[0].Message, "Invalid role")
	assert.Equal(t, dto.ErrCodeInvalidRole, errors[0].Code)
}

func TestParseUsersCSV_MissingRequiredFields(t *testing.T) {
	content := `email,first_name,last_name,password,role
,John,Doe,Pass123!,member
john@test.com,,Doe,Pass123!,member
john2@test.com,John,,Pass123!,member
john3@test.com,John,Doe,,member`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors := orgUtils.ParseUsersCSV(fileHeader)

	assert.Len(t, errors, 4, "Should have 4 validation errors")
	assert.Empty(t, users, "Should not return any users")
}

func TestParseGroupsCSV_ValidFile(t *testing.T) {
	content := `group_name,display_name,description,parent_group,max_members,expires_at
m1_devops,M1 DevOps,Master 1 DevOps,,150,
m1_devops_a,M1 DevOps A,Class A,m1_devops,50,2026-06-30T23:59:59Z`

	fileHeader := createMultipartFileHeader(t, "groups.csv", content)
	groups, errors := orgUtils.ParseGroupsCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors")
	assert.Len(t, groups, 2, "Should parse 2 groups")

	// Verify parent group
	assert.Equal(t, "m1_devops", groups[0].GroupName)
	assert.Equal(t, "M1 DevOps", groups[0].DisplayName)
	assert.Equal(t, "Master 1 DevOps", groups[0].Description)
	assert.Equal(t, "", groups[0].ParentGroup)
	assert.Equal(t, "150", groups[0].MaxMembers)

	// Verify child group
	assert.Equal(t, "m1_devops_a", groups[1].GroupName)
	assert.Equal(t, "m1_devops", groups[1].ParentGroup)
	assert.Equal(t, "50", groups[1].MaxMembers)
	assert.Equal(t, "2026-06-30T23:59:59Z", groups[1].ExpiresAt)
}

func TestParseGroupsCSV_MissingRequiredFields(t *testing.T) {
	content := `group_name,display_name,description
,M1 DevOps,Description
m1_devops,,Description`

	fileHeader := createMultipartFileHeader(t, "groups.csv", content)
	groups, errors := orgUtils.ParseGroupsCSV(fileHeader)

	assert.Len(t, errors, 2, "Should have 2 validation errors")
	assert.Empty(t, groups, "Should not return invalid groups")
}

func TestParseMembershipsCSV_ValidFile(t *testing.T) {
	content := `user_email,group_name,role
john@test.com,m1_devops_a,member
jane@test.com,m1_devops_a,admin`

	fileHeader := createMultipartFileHeader(t, "memberships.csv", content)
	memberships, errors := orgUtils.ParseMembershipsCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors")
	assert.Len(t, memberships, 2, "Should parse 2 memberships")

	assert.Equal(t, "john@test.com", memberships[0].UserEmail)
	assert.Equal(t, "m1_devops_a", memberships[0].GroupName)
	assert.Equal(t, "member", memberships[0].Role)

	assert.Equal(t, "jane@test.com", memberships[1].UserEmail)
	assert.Equal(t, "admin", memberships[1].Role)
}

func TestParseMembershipsCSV_InvalidRole(t *testing.T) {
	content := `user_email,group_name,role
john@test.com,m1_devops_a,invalid_role`

	fileHeader := createMultipartFileHeader(t, "memberships.csv", content)
	memberships, errors := orgUtils.ParseMembershipsCSV(fileHeader)

	assert.NotEmpty(t, errors, "Should have validation errors")
	assert.Empty(t, memberships, "Should not return invalid memberships")
	assert.Contains(t, errors[0].Message, "Invalid role")
	assert.Equal(t, dto.ErrCodeInvalidRole, errors[0].Code)
}

func TestParseMembershipsCSV_MissingRequiredFields(t *testing.T) {
	content := `user_email,group_name,role
,m1_devops_a,member
john@test.com,,member
john2@test.com,m1_devops_a,`

	fileHeader := createMultipartFileHeader(t, "memberships.csv", content)
	memberships, errors := orgUtils.ParseMembershipsCSV(fileHeader)

	assert.Len(t, errors, 3, "Should have 3 validation errors")
	assert.Empty(t, memberships, "Should not return invalid memberships")
}

func TestParseUsersCSV_EmptyFile(t *testing.T) {
	content := `email,first_name,last_name,password,role`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no errors for empty but valid file")
	assert.Empty(t, users, "Should return empty slice")
}

func TestParseGroupsCSV_NestedStructure(t *testing.T) {
	content := `group_name,display_name,description,parent_group
root,Root Group,Root,
child1,Child 1,First child,root
child2,Child 2,Second child,root
grandchild,Grand Child,Child of child1,child1`

	fileHeader := createMultipartFileHeader(t, "groups.csv", content)
	groups, errors := orgUtils.ParseGroupsCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors")
	assert.Len(t, groups, 4, "Should parse 4 groups")

	// Verify nested structure
	assert.Equal(t, "", groups[0].ParentGroup, "Root should have no parent")
	assert.Equal(t, "root", groups[1].ParentGroup, "Child1 parent should be root")
	assert.Equal(t, "root", groups[2].ParentGroup, "Child2 parent should be root")
	assert.Equal(t, "child1", groups[3].ParentGroup, "Grandchild parent should be child1")
}
