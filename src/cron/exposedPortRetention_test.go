package cron

import (
	"testing"
	"time"

	configModels "soli/formations/src/configuration/models"
	"soli/formations/src/terminalTrainer/models"
	terminalServices "soli/formations/src/terminalTrainer/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func retentionTestDB(t *testing.T, retentionValue string) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ExposedPort{}, &configModels.Feature{}))
	if retentionValue != "<no row>" {
		require.NoError(t, db.Create(&configModels.Feature{Key: terminalServices.ExposedPortRetentionDaysKey, Name: "r", Enabled: true, Value: retentionValue}).Error)
	}
	return db
}

func exposureExpired(daysAgo int) *models.ExposedPort {
	return &models.ExposedPort{TerminalID: uuid.New(), SessionID: "s", UserID: "u", ContainerPort: 8000,
		Slug: uuid.NewString()[:10], ContainerIP: "10.0.0.1", ExpiresAt: time.Now().Add(-time.Duration(daysAgo) * 24 * time.Hour)}
}

func remaining(t *testing.T, db *gorm.DB) int64 {
	var n int64
	require.NoError(t, db.Unscoped().Model(&models.ExposedPort{}).Count(&n).Error)
	return n
}

func TestExposedPortRetention_UsesThePlatformSetting(t *testing.T) {
	db := retentionTestDB(t, "10")
	old, recent := exposureExpired(11), exposureExpired(9)
	require.NoError(t, db.Create(old).Error)
	require.NoError(t, db.Create(recent).Error)
	require.NoError(t, db.Delete(old).Error, "a withdrawn (soft-deleted) row must go too")

	StartExposedPortRetentionJob(db)
	require.EqualValues(t, 1, remaining(t, db), "the 11-day-old row goes, the 9-day-old one stays under a 10-day setting")
}

func TestExposedPortRetention_DefaultsToThirtyDays(t *testing.T) {
	for _, value := range []string{"", "soon", "0", "-5", "<no row>"} {
		db := retentionTestDB(t, value)
		require.NoError(t, db.Create(exposureExpired(31)).Error)
		require.NoError(t, db.Create(exposureExpired(29)).Error)
		StartExposedPortRetentionJob(db)
		require.EqualValues(t, 1, remaining(t, db), "value %q must mean 30 days", value)
	}
}
