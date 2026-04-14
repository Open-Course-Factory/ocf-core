package auth_tests

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	authModels "soli/formations/src/auth/models"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// setupSshKeyTestDB creates an in-memory SQLite DB with the SshKey table.
func setupSshKeyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	err = db.AutoMigrate(&authModels.SshKey{})
	require.NoError(t, err)
	return db
}

// TestSshKey_GormHooks_EncryptOnSave_DecryptOnFind verifies that:
//   - BeforeSave: PrivateKey is encrypted and the stored value starts with "enc::v1:"
//   - AfterFind:  PrivateKey is transparently decrypted back to the original plaintext
func TestSshKey_GormHooks_EncryptOnSave_DecryptOnFind(t *testing.T) {
	t.Setenv("FIELD_ENCRYPTION_SECRET", "a-very-secret-key-for-testing-ok")

	db := setupSshKeyTestDB(t)

	original := "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAA=\n-----END OPENSSH PRIVATE KEY-----"

	key := &authModels.SshKey{
		BaseModel:  entityManagementModels.BaseModel{ID: uuid.New()},
		KeyName:    "test-key",
		PrivateKey: original,
	}

	err := db.Create(key).Error
	require.NoError(t, err, "db.Create should succeed")

	// Raw query to inspect the stored value (bypasses GORM hooks / AfterFind).
	var rawPrivateKey string
	err = db.Raw("SELECT private_key FROM ssh_keys WHERE id = ?", key.ID).Scan(&rawPrivateKey).Error
	require.NoError(t, err, "Raw SELECT should succeed")
	assert.True(t, strings.HasPrefix(rawPrivateKey, "enc::v1:"),
		"Stored private_key must be encrypted (must start with enc::v1:), got: %q", rawPrivateKey)

	// Now load via GORM (AfterFind hook should decrypt transparently).
	var found authModels.SshKey
	err = db.First(&found, "id = ?", key.ID).Error
	require.NoError(t, err, "db.First should succeed")
	assert.Equal(t, original, found.PrivateKey,
		"PrivateKey returned by GORM must equal the original plaintext after AfterFind decryption")
}
