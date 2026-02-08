package services

import (
	"log"
	"soli/formations/src/configuration/models"

	"gorm.io/gorm"
)

// FeatureRegistryService manages module feature registration
type FeatureRegistryService struct {
	db                 *gorm.DB
	registeredFeatures []models.FeatureDefinition
}

var GlobalFeatureRegistry *FeatureRegistryService

// InitFeatureRegistry initializes the global feature registry
func InitFeatureRegistry(db *gorm.DB) {
	GlobalFeatureRegistry = &FeatureRegistryService{
		db:                 db,
		registeredFeatures: []models.FeatureDefinition{},
	}
}

// RegisterFeature registers a feature from a module
// This should be called by each module during initialization
func (frs *FeatureRegistryService) RegisterFeature(feature models.FeatureDefinition) {
	frs.registeredFeatures = append(frs.registeredFeatures, feature)
	log.Printf("📋 Module '%s' registered feature: %s (%s)", feature.Module, feature.Key, feature.Name)
}

// RegisterFeatures registers multiple features at once
func (frs *FeatureRegistryService) RegisterFeatures(features []models.FeatureDefinition) {
	for _, feature := range features {
		frs.RegisterFeature(feature)
	}
}

// SeedRegisteredFeatures seeds all registered features into the database
// Only creates features that don't already exist
func (frs *FeatureRegistryService) SeedRegisteredFeatures() {
	if len(frs.registeredFeatures) == 0 {
		log.Println("⚠️ No features registered to seed")
		return
	}

	log.Printf("🌱 Seeding %d registered features...", len(frs.registeredFeatures))

	for _, featureDef := range frs.registeredFeatures {
		var existing models.Feature
		err := frs.db.Where("key = ?", featureDef.Key).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// Feature doesn't exist, create it
			feature := models.Feature{
				Key:         featureDef.Key,
				Name:        featureDef.Name,
				Description: featureDef.Description,
				Enabled:     featureDef.Enabled,
				Category:    featureDef.Category,
				Module:      featureDef.Module,
				Value:       featureDef.Value,
			}

			if err := frs.db.Create(&feature).Error; err != nil {
				log.Printf("❌ Failed to create feature %s: %v", featureDef.Key, err)
			} else {
				log.Printf("✅ Created feature: %s (module: %s, enabled: %v)",
					featureDef.Key, featureDef.Module, featureDef.Enabled)
			}
		} else if err != nil {
			log.Printf("❌ Error checking feature %s: %v", featureDef.Key, err)
		} else {
			// Feature exists, optionally update module field if missing
			if existing.Module == "" && featureDef.Module != "" {
				existing.Module = featureDef.Module
				if err := frs.db.Save(&existing).Error; err != nil {
					log.Printf("⚠️ Failed to update module for feature %s: %v", featureDef.Key, err)
				} else {
					log.Printf("🔄 Updated module for existing feature: %s → %s", featureDef.Key, featureDef.Module)
				}
			}
		}
	}

	log.Println("✅ Feature seeding complete")
}

// GetRegisteredFeatures returns all registered features (for debugging)
func (frs *FeatureRegistryService) GetRegisteredFeatures() []models.FeatureDefinition {
	return frs.registeredFeatures
}
