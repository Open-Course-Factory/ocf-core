package registration

import (
	"net/http"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type UserSettingsRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s UserSettingsRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "user-settings",
		EntityName: "UserSettings",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Get all user settings",
			Description: "Returns all user settings (admin only)",
			Tags:        []string{"user-settings"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Get user settings",
			Description: "Returns settings for a specific user",
			Tags:        []string{"user-settings"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Create user settings",
			Description: "Creates new user settings (typically done automatically on user creation)",
			Tags:        []string{"user-settings"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Update user settings",
			Description: "Updates user preferences and settings",
			Tags:        []string{"user-settings"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Delete user settings",
			Description: "Deletes user settings (admin only)",
			Tags:        []string{"user-settings"},
			Security:    true,
		},
	}
}

func (s UserSettingsRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		settings := ptr.(*models.UserSettings)
		return &dto.UserSettingsOutput{
			ID:                   settings.ID,
			UserID:               settings.UserID,
			DefaultLandingPage:   settings.DefaultLandingPage,
			PreferredLanguage:    settings.PreferredLanguage,
			Timezone:             settings.Timezone,
			Theme:                settings.Theme,
			CompactMode:          settings.CompactMode,
			EmailNotifications:   settings.EmailNotifications,
			DesktopNotifications: settings.DesktopNotifications,
			PasswordLastChanged:  settings.PasswordLastChanged,
			TwoFactorEnabled:     settings.TwoFactorEnabled,
			CreatedAt:            settings.CreatedAt,
			UpdatedAt:            settings.UpdatedAt,
		}, nil
	})
}

func (s UserSettingsRegistration) EntityInputDtoToEntityModel(input any) any {
	settingsInput := input.(dto.UserSettingsInput)

	settings := &models.UserSettings{
		UserID: settingsInput.UserID,
	}

	// Apply optional fields if provided
	if settingsInput.DefaultLandingPage != nil {
		settings.DefaultLandingPage = *settingsInput.DefaultLandingPage
	}
	if settingsInput.PreferredLanguage != nil {
		settings.PreferredLanguage = *settingsInput.PreferredLanguage
	}
	if settingsInput.Timezone != nil {
		settings.Timezone = *settingsInput.Timezone
	}
	if settingsInput.Theme != nil {
		settings.Theme = *settingsInput.Theme
	}
	if settingsInput.CompactMode != nil {
		settings.CompactMode = *settingsInput.CompactMode
	}
	if settingsInput.EmailNotifications != nil {
		settings.EmailNotifications = *settingsInput.EmailNotifications
	}
	if settingsInput.DesktopNotifications != nil {
		settings.DesktopNotifications = *settingsInput.DesktopNotifications
	}

	return settings
}

func (s UserSettingsRegistration) EntityDtoToMap(input any) map[string]any {
	editInput := input.(dto.EditUserSettingsInput)
	updateMap := make(map[string]any)

	if editInput.DefaultLandingPage != nil {
		updateMap["default_landing_page"] = *editInput.DefaultLandingPage
	}
	if editInput.PreferredLanguage != nil {
		updateMap["preferred_language"] = *editInput.PreferredLanguage
	}
	if editInput.Timezone != nil {
		updateMap["timezone"] = *editInput.Timezone
	}
	if editInput.Theme != nil {
		updateMap["theme"] = *editInput.Theme
	}
	if editInput.CompactMode != nil {
		updateMap["compact_mode"] = *editInput.CompactMode
	}
	if editInput.EmailNotifications != nil {
		updateMap["email_notifications"] = *editInput.EmailNotifications
	}
	if editInput.DesktopNotifications != nil {
		updateMap["desktop_notifications"] = *editInput.DesktopNotifications
	}

	return updateMap
}

func (s UserSettingsRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.UserSettings{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.UserSettingsInput{},
			OutputDto:      dto.UserSettingsOutput{},
			InputEditDto:   dto.EditUserSettingsInput{},
		},
	}
}

func (s UserSettingsRegistration) GetEntityRoles() entityManagementInterfaces.EntityRoles {
	roleMap := make(map[string]string)

	// Members can only GET and PATCH their own settings (enforced by middleware)
	roleMap[string(models.Member)] = "(" + http.MethodGet + "|" + http.MethodPatch + ")"

	// Admins have full access
	roleMap[string(models.Admin)] = "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")"

	return entityManagementInterfaces.EntityRoles{
		Roles: roleMap,
	}
}
