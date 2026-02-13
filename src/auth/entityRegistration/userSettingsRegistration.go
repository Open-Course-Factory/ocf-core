package registration

import (
	"net/http"

	"soli/formations/src/auth/dto"
	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func RegisterUserSettings(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[authModels.UserSettings, dto.UserSettingsInput, dto.EditUserSettingsInput, dto.UserSettingsOutput](
		service,
		"UserSettings",
		entityManagementInterfaces.TypedEntityRegistration[authModels.UserSettings, dto.UserSettingsInput, dto.EditUserSettingsInput, dto.UserSettingsOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[authModels.UserSettings, dto.UserSettingsInput, dto.EditUserSettingsInput, dto.UserSettingsOutput]{
				ModelToDto: func(model *authModels.UserSettings) (dto.UserSettingsOutput, error) {
					return dto.UserSettingsOutput{
						ID:                   model.ID,
						UserID:               model.UserID,
						DefaultLandingPage:   model.DefaultLandingPage,
						PreferredLanguage:    model.PreferredLanguage,
						Timezone:             model.Timezone,
						Theme:                model.Theme,
						CompactMode:          model.CompactMode,
						EmailNotifications:   model.EmailNotifications,
						DesktopNotifications: model.DesktopNotifications,
						PasswordLastChanged:  model.PasswordLastChanged,
						TwoFactorEnabled:     model.TwoFactorEnabled,
						CreatedAt:            model.CreatedAt,
						UpdatedAt:            model.UpdatedAt,
					}, nil
				},
				DtoToModel: func(input dto.UserSettingsInput) *authModels.UserSettings {
					settings := &authModels.UserSettings{
						UserID: input.UserID,
					}
					if input.DefaultLandingPage != nil {
						settings.DefaultLandingPage = *input.DefaultLandingPage
					}
					if input.PreferredLanguage != nil {
						settings.PreferredLanguage = *input.PreferredLanguage
					}
					if input.Timezone != nil {
						settings.Timezone = *input.Timezone
					}
					if input.Theme != nil {
						settings.Theme = *input.Theme
					}
					if input.CompactMode != nil {
						settings.CompactMode = *input.CompactMode
					}
					if input.EmailNotifications != nil {
						settings.EmailNotifications = *input.EmailNotifications
					}
					if input.DesktopNotifications != nil {
						settings.DesktopNotifications = *input.DesktopNotifications
					}
					return settings
				},
				DtoToMap: func(input dto.EditUserSettingsInput) map[string]any {
					updateMap := make(map[string]any)
					if input.DefaultLandingPage != nil {
						updateMap["default_landing_page"] = *input.DefaultLandingPage
					}
					if input.PreferredLanguage != nil {
						updateMap["preferred_language"] = *input.PreferredLanguage
					}
					if input.Timezone != nil {
						updateMap["timezone"] = *input.Timezone
					}
					if input.Theme != nil {
						updateMap["theme"] = *input.Theme
					}
					if input.CompactMode != nil {
						updateMap["compact_mode"] = *input.CompactMode
					}
					if input.EmailNotifications != nil {
						updateMap["email_notifications"] = *input.EmailNotifications
					}
					if input.DesktopNotifications != nil {
						updateMap["desktop_notifications"] = *input.DesktopNotifications
					}
					return updateMap
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPatch + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodPatch + "|" + http.MethodDelete + ")",
				},
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
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
			},
		},
	)
}
