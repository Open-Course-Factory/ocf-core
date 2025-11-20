-- Migration script to convert email_templates from uint ID to UUID
-- WARNING: This will delete all existing email templates!
-- For production, you would need to write a proper data migration.

-- Drop the old table
DROP TABLE IF EXISTS email_templates;

-- The table will be automatically recreated by GORM AutoMigrate
-- with the new UUID-based schema when the application starts.

-- Notes:
-- - EmailTemplate now uses entityManagementModels.BaseModel
-- - ID field is now uuid.UUID instead of uint
-- - OwnerIDs field is now available for ownership tracking
-- - Default templates will be recreated by emailServices.InitDefaultTemplates()
