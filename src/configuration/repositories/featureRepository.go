package repositories

import (
	"soli/formations/src/configuration/models"

	"gorm.io/gorm"
)

type FeatureRepository interface {
	GetFeatureByKey(key string) (*models.Feature, error)
	IsFeatureEnabled(key string) bool
	UpdateFeatureValue(key string, value string) error
}

type featureRepository struct {
	db *gorm.DB
}

func NewFeatureRepository(db *gorm.DB) FeatureRepository {
	return &featureRepository{db: db}
}

func (r *featureRepository) GetFeatureByKey(key string) (*models.Feature, error) {
	var feature models.Feature
	err := r.db.Where("key = ?", key).First(&feature).Error
	return &feature, err
}

// IsFeatureEnabled checks if a feature is enabled (defaults to true if not found)
func (r *featureRepository) IsFeatureEnabled(key string) bool {
	feature, err := r.GetFeatureByKey(key)
	if err != nil {
		// If feature doesn't exist in DB, default to enabled
		return true
	}
	return feature.Enabled
}

// UpdateFeatureValue upserts a feature's value by key
func (r *featureRepository) UpdateFeatureValue(key string, value string) error {
	var feature models.Feature
	err := r.db.Where("key = ?", key).First(&feature).Error
	if err == gorm.ErrRecordNotFound {
		feature = models.Feature{
			Key:     key,
			Name:    key,
			Value:   value,
			Enabled: true,
		}
		return r.db.Create(&feature).Error
	}
	if err != nil {
		return err
	}
	feature.Value = value
	return r.db.Save(&feature).Error
}
