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
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

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
	// Missing email column entirely — should fail
	content := `first_name,last_name
John,Doe`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.NotEmpty(t, errors, "Should have errors for missing email column")
	assert.Nil(t, users, "Should not return users when header is invalid")
	assert.Contains(t, errors[0].Message, "Missing required column: email")
	assert.Equal(t, dto.ErrCodeValidation, errors[0].Code)
}

func TestParseUsersCSV_InvalidEmail(t *testing.T) {
	content := `email,first_name,last_name
not-an-email,John,Doe`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.NotEmpty(t, errors, "Should have validation errors")
	assert.Empty(t, users, "Should not return invalid users")
	assert.Contains(t, errors[0].Message, "Invalid email format")
	assert.Equal(t, dto.ErrCodeInvalidEmail, errors[0].Code)
	assert.Equal(t, 2, errors[0].Row, "Error should be on row 2")
}

func TestParseUsersCSV_InvalidRole(t *testing.T) {
	content := `email,first_name,last_name,role
john@test.com,John,Doe,invalid_role`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.NotEmpty(t, errors, "Should have validation errors")
	assert.Empty(t, users, "Should not return invalid users")
	assert.Contains(t, errors[0].Message, "Invalid role")
	assert.Equal(t, dto.ErrCodeInvalidRole, errors[0].Code)
}

func TestParseUsersCSV_MissingRequiredFields(t *testing.T) {
	// Only email missing value should error (password and role are now optional)
	content := `email,first_name,last_name,password,role
,John,Doe,Pass123!,member
john@test.com,,Doe,Pass123!,member
john2@test.com,John,,Pass123!,member`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	// Row 1: empty email → error
	// Row 2: empty first_name but last_name present → valid
	// Row 3: empty last_name but first_name present → valid
	assert.Len(t, errors, 1, "Should have 1 validation error (missing email)")
	assert.Len(t, users, 2, "Should return 2 valid users")
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
	content := `email,first_name,last_name`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

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

// --- New tests for simplified CSV import ---

func TestParseUsersCSV_NameSplitting_TwoWords(t *testing.T) {
	content := `email,name
marie@test.com,DUPONT Marie`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no errors")
	require.Len(t, users, 1)
	assert.Equal(t, "DUPONT", users[0].LastName, "Everything before last space should be last name")
	assert.Equal(t, "Marie", users[0].FirstName, "Last word should be first name")
}

func TestParseUsersCSV_NameSplitting_ThreeWords(t *testing.T) {
	content := `email,name
jean@test.com,DE LA FONTAINE Jean`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no errors")
	require.Len(t, users, 1)
	assert.Equal(t, "DE LA FONTAINE", users[0].LastName)
	assert.Equal(t, "Jean", users[0].FirstName)
}

func TestParseUsersCSV_NameSplitting_SingleWord(t *testing.T) {
	content := `email,name
mono@test.com,DUPONT`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, warnings := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no errors (warning only)")
	require.Len(t, users, 1)
	assert.Equal(t, "DUPONT", users[0].LastName, "Single word should be last name")
	assert.Equal(t, "", users[0].FirstName, "First name should be empty for single word")

	// Verify warning is returned for single-word name
	require.Len(t, warnings, 1, "Should have 1 warning for single-word name")
	assert.Contains(t, warnings[0].Message, "no space")
}

func TestParseUsersCSV_OptionalPassword(t *testing.T) {
	content := `email,first_name,last_name
john@test.com,John,Doe`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Password should be optional")
	require.Len(t, users, 1)
	assert.Equal(t, "", users[0].Password)
}

func TestParseUsersCSV_OptionalRole(t *testing.T) {
	content := `email,first_name,last_name
john@test.com,John,Doe`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Role should be optional")
	require.Len(t, users, 1)
	assert.Equal(t, "", users[0].Role)
}

func TestParseUsersCSV_FrenchSchoolCSV(t *testing.T) {
	content := "Nom,E-mail,Sexe,Né(e) le\nDUPONT Marie,marie.dupont@ecole.fr,F,2000-01-15"

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should parse French school CSV without errors")
	require.Len(t, users, 1)
	assert.Equal(t, "marie.dupont@ecole.fr", users[0].Email)
	assert.Equal(t, "DUPONT", users[0].LastName, "Name should be split: last name = DUPONT")
	assert.Equal(t, "Marie", users[0].FirstName, "Name should be split: first name = Marie")
}

func TestParseUsersCSV_ColumnAliases(t *testing.T) {
	content := `E-mail,Nom
jean@test.com,MARTIN Jean`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "E-mail and Nom should resolve as aliases")
	require.Len(t, users, 1)
	assert.Equal(t, "jean@test.com", users[0].Email)
	assert.Equal(t, "MARTIN", users[0].LastName)
	assert.Equal(t, "Jean", users[0].FirstName)
}

func TestParseUsersCSV_ColumnAliases_PreservesCanonical(t *testing.T) {
	// When both alias and canonical column exist, canonical takes priority
	content := `email,mail,first_name,last_name
john@test.com,other@test.com,John,Doe`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors)
	require.Len(t, users, 1)
	assert.Equal(t, "john@test.com", users[0].Email, "Canonical column should take priority over alias")
}

func TestParseUsersCSV_MissingNameColumns(t *testing.T) {
	// CSV with only email — no name columns at all
	content := `email
john@test.com`

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.NotEmpty(t, errors, "Should error when no name columns present")
	assert.Nil(t, users)
	assert.Contains(t, errors[0].Message, "Missing required columns")
}

// --- Encoding detection tests ---

// Helper to create a multipart.FileHeader from raw bytes (for non-UTF-8 encoded content)
func createMultipartFileHeaderFromBytes(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)

	_, err = part.Write(content)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	reader := multipart.NewReader(body, writer.Boundary())
	form, err := reader.ReadForm(32 << 20)
	require.NoError(t, err)

	files := form.File["file"]
	require.NotEmpty(t, files, "Expected at least one file in form")

	return files[0]
}

func TestParseUsersCSV_Latin1Encoding(t *testing.T) {
	// Latin-1 encoded CSV: "Jérôme" is encoded as 4A E9 72 F4 6D 65 in Latin-1
	// \xe9 = é, \xf4 = ô in Latin-1/ISO-8859-1
	latin1CSV := []byte("email,first_name,last_name\njerome@test.com,J\xe9r\xf4me,Dupont\n")

	fileHeader := createMultipartFileHeaderFromBytes(t, "users.csv", latin1CSV)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors for Latin-1 CSV")
	require.Len(t, users, 1)
	assert.Equal(t, "jerome@test.com", users[0].Email)
	assert.Equal(t, "Jérôme", users[0].FirstName, "Latin-1 encoded é (0xe9) and ô (0xf4) should be converted to UTF-8")
	assert.Equal(t, "Dupont", users[0].LastName)
}

func TestParseUsersCSV_UTF8BOM(t *testing.T) {
	// UTF-8 BOM (EF BB BF) prepended to a valid UTF-8 CSV
	// The BOM should be stripped so that the first column header "email" is recognized correctly
	bom := []byte{0xEF, 0xBB, 0xBF}
	csvContent := []byte("email,first_name,last_name\npierre@test.com,Pierre,Martin\n")
	bomCSV := append(bom, csvContent...)

	fileHeader := createMultipartFileHeaderFromBytes(t, "users.csv", bomCSV)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors for UTF-8 BOM CSV")
	require.Len(t, users, 1, "Should parse 1 user from BOM-prefixed CSV")
	assert.Equal(t, "pierre@test.com", users[0].Email, "Email should be correctly parsed despite BOM prefix on header")
	assert.Equal(t, "Pierre", users[0].FirstName)
	assert.Equal(t, "Martin", users[0].LastName)
}

func TestParseUsersCSV_Windows1252Encoding(t *testing.T) {
	// Windows-1252 specific characters:
	// \x93 = left double quotation mark (U+201C)
	// \x94 = right double quotation mark (U+201D)
	// \x85 = horizontal ellipsis (U+2026)
	// These bytes are NOT valid in Latin-1 or UTF-8, only in Windows-1252
	// Use them in a last_name field to verify conversion
	win1252CSV := []byte("email,first_name,last_name\ntest@test.com,Marie,L\x93Arch\x85\x94\n")

	fileHeader := createMultipartFileHeaderFromBytes(t, "users.csv", win1252CSV)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors for Windows-1252 CSV")
	require.Len(t, users, 1)
	assert.Equal(t, "test@test.com", users[0].Email)
	assert.Equal(t, "Marie", users[0].FirstName)
	// Windows-1252: \x93 → U+201C ("), \x85 → U+2026 (…), \x94 → U+201D (")
	assert.Equal(t, "L\u201cArch\u2026\u201d", users[0].LastName, "Windows-1252 smart quotes and ellipsis should be converted to UTF-8")
}

func TestParseUsersCSV_Latin1FrenchAliases(t *testing.T) {
	// French headers with Latin-1 encoding:
	// "prénom" in Latin-1: 70 72 E9 6E 6F 6D (the é is 0xE9)
	// "e-mail" is pure ASCII so no encoding issue
	// The alias "prénom" in the code is UTF-8 (70 72 C3 A9 6E 6F 6D) which won't match
	// the Latin-1 bytes (70 72 E9 6E 6F 6D) unless encoding conversion happens first
	header := []byte("e-mail,pr\xe9nom,nom de famille\n")
	row := []byte("marie@test.com,Marie,Dupont\n")
	latin1CSV := append(header, row...)

	fileHeader := createMultipartFileHeaderFromBytes(t, "users.csv", latin1CSV)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors for Latin-1 French headers")
	require.Len(t, users, 1, "Should parse 1 user when French aliases are Latin-1 encoded")
	assert.Equal(t, "marie@test.com", users[0].Email, "e-mail alias should resolve to email")
	assert.Equal(t, "Marie", users[0].FirstName, "Latin-1 prénom alias should resolve to first_name")
	assert.Equal(t, "Dupont", users[0].LastName, "nom de famille alias should resolve to last_name")
}

func TestParseUsersCSV_UTF8AccentsPreserved(t *testing.T) {
	// Valid UTF-8 accented characters should be preserved as-is
	// This is a regression guard — should pass already
	content := "email,first_name,last_name\n" +
		"jerome@test.com,Jérôme,Müller\n" +
		"jose@test.com,José,García\n" +
		"francois@test.com,François,Lefèvre\n"

	fileHeader := createMultipartFileHeader(t, "users.csv", content)
	users, errors, _ := orgUtils.ParseUsersCSV(fileHeader)

	assert.Empty(t, errors, "Should have no parsing errors for valid UTF-8")
	require.Len(t, users, 3, "Should parse 3 users")

	assert.Equal(t, "Jérôme", users[0].FirstName, "UTF-8 é and ô should be preserved")
	assert.Equal(t, "Müller", users[0].LastName, "UTF-8 ü should be preserved")

	assert.Equal(t, "José", users[1].FirstName, "UTF-8 é should be preserved")
	assert.Equal(t, "García", users[1].LastName, "UTF-8 í should be preserved")

	assert.Equal(t, "François", users[2].FirstName, "UTF-8 ç should be preserved")
	assert.Equal(t, "Lefèvre", users[2].LastName, "UTF-8 è should be preserved")
}
