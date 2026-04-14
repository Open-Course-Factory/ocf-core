package models

import (
	"strings"

	"gorm.io/gorm"

	"soli/formations/src/auth/crypto"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type SshKey struct {
	entityManagementModels.BaseModel
	KeyName    string `gorm:"type:varchar(255)"`
	PrivateKey string `gorm:"type:text"`
}

// BeforeSave encrypts PrivateKey before writing to the database.
// It is idempotent: already-encrypted values (prefixed with "enc::v1:") are
// left unchanged to avoid double-encryption on updates.
func (k *SshKey) BeforeSave(tx *gorm.DB) error {
	if k.PrivateKey != "" && !strings.HasPrefix(k.PrivateKey, "enc::v1:") {
		encrypted, err := crypto.Encrypt(k.PrivateKey)
		if err != nil {
			return err
		}
		k.PrivateKey = encrypted
	}
	return nil
}

// AfterFind decrypts PrivateKey after loading from the database so that
// callers always work with plaintext.
func (k *SshKey) AfterFind(tx *gorm.DB) error {
	if strings.HasPrefix(k.PrivateKey, "enc::v1:") {
		decrypted, err := crypto.Decrypt(k.PrivateKey)
		if err != nil {
			return err
		}
		k.PrivateKey = decrypted
	}
	return nil
}
