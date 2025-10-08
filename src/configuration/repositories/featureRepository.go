package repositories

import (
	"soli/formations/src/configuration/models"

	"gorm.io/gorm"
)

type FeatureRepository interface {
	GetFeatureByKey(key string) (*models.Feature, error)
	GetAllFeatures() ([]models.Feature, error)
	IsFeatureEnabled(key string) bool
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

func (r *featureRepository) GetAllFeatures() ([]models.Feature, error) {
	var features []models.Feature
	err := r.db.Find(&features).Error
	return features, err
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
