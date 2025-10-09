# User Settings Frontend Integration Guide

## Overview

The OCF Core API now provides a complete user preferences system. This guide shows how to integrate it into your frontend application.

## API Endpoints

### Base URL
```
http://localhost:8080/api/v1
```

### Authentication
All endpoints require a Bearer token in the Authorization header:
```
Authorization: Bearer YOUR_JWT_TOKEN
```

---

## 📋 Available Endpoints

### 1. Get Current User Settings
**GET** `/users/me/settings`

Retrieves the current user's settings. If settings don't exist, they will be automatically created with defaults.

**Response 200:**
```json
{
  "id": 1,
  "user_id": "1d660660-7637-4a5d-9d1e-8d05bbf7363f",
  "default_landing_page": "/dashboard",
  "preferred_language": "en",
  "timezone": "UTC",
  "theme": "light",
  "compact_mode": false,
  "email_notifications": true,
  "desktop_notifications": false,
  "password_last_changed": "2025-10-09T10:30:00Z",
  "two_factor_enabled": false,
  "created_at": "2025-10-09T09:00:00Z",
  "updated_at": "2025-10-09T09:00:00Z"
}
```

---

### 2. Update User Settings
**PATCH** `/users/me/settings`

Updates specific user preferences. All fields are optional - only send what you want to update.

**Request Body:**
```json
{
  "default_landing_page": "/courses",
  "preferred_language": "fr",
  "theme": "dark"
}
```

**Response 200:** (Returns updated settings)
```json
{
  "id": 1,
  "user_id": "1d660660-7637-4a5d-9d1e-8d05bbf7363f",
  "default_landing_page": "/courses",
  "preferred_language": "fr",
  "timezone": "UTC",
  "theme": "dark",
  "compact_mode": false,
  "email_notifications": true,
  "desktop_notifications": false,
  "password_last_changed": null,
  "two_factor_enabled": false,
  "created_at": "2025-10-09T09:00:00Z",
  "updated_at": "2025-10-09T10:45:00Z"
}
```

---

### 3. Change Password
**POST** `/users/me/change-password`

Securely changes the user's password. Requires current password verification.

**Request Body:**
```json
{
  "current_password": "oldPassword123",
  "new_password": "newSecurePassword456",
  "confirm_password": "newSecurePassword456"
}
```

**Response 200:**
```json
{
  "message": "Password changed successfully"
}
```

**Response 401:** (Invalid current password)
```json
{
  "error": "current password is incorrect"
}
```

**Response 400:** (Validation error)
```json
{
  "error": "new password and confirmation do not match"
}
```

---

## 🎨 Frontend Implementation Examples

### React / TypeScript

#### Types Definition
```typescript
// types/userSettings.ts
export interface UserSettings {
  id: number;
  user_id: string;
  default_landing_page: string;
  preferred_language: string;
  timezone: string;
  theme: 'light' | 'dark' | 'auto';
  compact_mode: boolean;
  email_notifications: boolean;
  desktop_notifications: boolean;
  password_last_changed: string | null;
  two_factor_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface UpdateSettingsRequest {
  default_landing_page?: string;
  preferred_language?: string;
  timezone?: string;
  theme?: 'light' | 'dark' | 'auto';
  compact_mode?: boolean;
  email_notifications?: boolean;
  desktop_notifications?: boolean;
}

export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
  confirm_password: string;
}
```

#### API Service
```typescript
// services/userSettingsService.ts
import axios from 'axios';
import { UserSettings, UpdateSettingsRequest, ChangePasswordRequest } from '../types/userSettings';

const API_BASE_URL = 'http://localhost:8080/api/v1';

// Get JWT token from your auth store/context
const getAuthToken = () => localStorage.getItem('access_token');

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add auth token to all requests
api.interceptors.request.use((config) => {
  const token = getAuthToken();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

export const userSettingsService = {
  // Get current user settings
  async getSettings(): Promise<UserSettings> {
    const response = await api.get<UserSettings>('/users/me/settings');
    return response.data;
  },

  // Update specific settings
  async updateSettings(updates: UpdateSettingsRequest): Promise<UserSettings> {
    const response = await api.patch<UserSettings>('/users/me/settings', updates);
    return response.data;
  },

  // Change password
  async changePassword(data: ChangePasswordRequest): Promise<void> {
    await api.post('/users/me/change-password', data);
  },
};
```

#### React Component Example - Settings Page
```typescript
// components/SettingsPage.tsx
import React, { useState, useEffect } from 'react';
import { userSettingsService } from '../services/userSettingsService';
import type { UserSettings } from '../types/userSettings';

export const SettingsPage: React.FC = () => {
  const [settings, setSettings] = useState<UserSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Load settings on mount
  useEffect(() => {
    loadSettings();
  }, []);

  const loadSettings = async () => {
    try {
      setLoading(true);
      const data = await userSettingsService.getSettings();
      setSettings(data);
    } catch (err) {
      setError('Failed to load settings');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const updateSetting = async (key: keyof UserSettings, value: any) => {
    try {
      const updated = await userSettingsService.updateSettings({ [key]: value });
      setSettings(updated);
    } catch (err) {
      alert('Failed to update setting');
      console.error(err);
    }
  };

  if (loading) return <div>Loading settings...</div>;
  if (error) return <div>Error: {error}</div>;
  if (!settings) return null;

  return (
    <div className="settings-page">
      <h1>User Settings</h1>

      {/* Theme Selection */}
      <section>
        <h2>Appearance</h2>
        <label>
          Theme:
          <select
            value={settings.theme}
            onChange={(e) => updateSetting('theme', e.target.value)}
          >
            <option value="light">Light</option>
            <option value="dark">Dark</option>
            <option value="auto">Auto</option>
          </select>
        </label>

        <label>
          <input
            type="checkbox"
            checked={settings.compact_mode}
            onChange={(e) => updateSetting('compact_mode', e.target.checked)}
          />
          Compact Mode
        </label>
      </section>

      {/* Language Selection */}
      <section>
        <h2>Localization</h2>
        <label>
          Language:
          <select
            value={settings.preferred_language}
            onChange={(e) => updateSetting('preferred_language', e.target.value)}
          >
            <option value="en">English</option>
            <option value="fr">Français</option>
            <option value="es">Español</option>
            <option value="de">Deutsch</option>
          </select>
        </label>

        <label>
          Timezone:
          <select
            value={settings.timezone}
            onChange={(e) => updateSetting('timezone', e.target.value)}
          >
            <option value="UTC">UTC</option>
            <option value="Europe/Paris">Europe/Paris</option>
            <option value="America/New_York">America/New York</option>
            <option value="Asia/Tokyo">Asia/Tokyo</option>
          </select>
        </label>
      </section>

      {/* Navigation */}
      <section>
        <h2>Navigation</h2>
        <label>
          Default Landing Page:
          <select
            value={settings.default_landing_page}
            onChange={(e) => updateSetting('default_landing_page', e.target.value)}
          >
            <option value="/dashboard">Dashboard</option>
            <option value="/courses">Courses</option>
            <option value="/terminals">Terminals</option>
            <option value="/labs">Labs</option>
          </select>
        </label>
      </section>

      {/* Notifications */}
      <section>
        <h2>Notifications</h2>
        <label>
          <input
            type="checkbox"
            checked={settings.email_notifications}
            onChange={(e) => updateSetting('email_notifications', e.target.checked)}
          />
          Email Notifications
        </label>

        <label>
          <input
            type="checkbox"
            checked={settings.desktop_notifications}
            onChange={(e) => updateSetting('desktop_notifications', e.target.checked)}
          />
          Desktop Notifications
        </label>
      </section>
    </div>
  );
};
```

#### Password Change Component
```typescript
// components/ChangePasswordForm.tsx
import React, { useState } from 'react';
import { userSettingsService } from '../services/userSettingsService';

export const ChangePasswordForm: React.FC = () => {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(false);

    // Client-side validation
    if (newPassword !== confirmPassword) {
      setError('New passwords do not match');
      return;
    }

    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters long');
      return;
    }

    try {
      await userSettingsService.changePassword({
        current_password: currentPassword,
        new_password: newPassword,
        confirm_password: confirmPassword,
      });

      setSuccess(true);
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (err: any) {
      const errorMessage = err.response?.data?.error || 'Failed to change password';
      setError(errorMessage);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="change-password-form">
      <h2>Change Password</h2>

      {error && <div className="error">{error}</div>}
      {success && <div className="success">Password changed successfully!</div>}

      <label>
        Current Password:
        <input
          type="password"
          value={currentPassword}
          onChange={(e) => setCurrentPassword(e.target.value)}
          required
        />
      </label>

      <label>
        New Password:
        <input
          type="password"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
          required
          minLength={8}
        />
      </label>

      <label>
        Confirm New Password:
        <input
          type="password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          required
          minLength={8}
        />
      </label>

      <button type="submit">Change Password</button>
    </form>
  );
};
```

---

### Vue 3 / TypeScript

#### Composable
```typescript
// composables/useUserSettings.ts
import { ref, computed } from 'vue';
import axios from 'axios';
import type { UserSettings, UpdateSettingsRequest } from '@/types/userSettings';

const API_BASE_URL = 'http://localhost:8080/api/v1';

export function useUserSettings() {
  const settings = ref<UserSettings | null>(null);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const api = axios.create({
    baseURL: API_BASE_URL,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${localStorage.getItem('access_token')}`,
    },
  });

  const loadSettings = async () => {
    try {
      loading.value = true;
      error.value = null;
      const response = await api.get<UserSettings>('/users/me/settings');
      settings.value = response.data;
    } catch (err: any) {
      error.value = err.response?.data?.error || 'Failed to load settings';
    } finally {
      loading.value = false;
    }
  };

  const updateSettings = async (updates: UpdateSettingsRequest) => {
    try {
      const response = await api.patch<UserSettings>('/users/me/settings', updates);
      settings.value = response.data;
    } catch (err: any) {
      throw new Error(err.response?.data?.error || 'Failed to update settings');
    }
  };

  const changePassword = async (currentPassword: string, newPassword: string, confirmPassword: string) => {
    try {
      await api.post('/users/me/change-password', {
        current_password: currentPassword,
        new_password: newPassword,
        confirm_password: confirmPassword,
      });
    } catch (err: any) {
      throw new Error(err.response?.data?.error || 'Failed to change password');
    }
  };

  return {
    settings: computed(() => settings.value),
    loading: computed(() => loading.value),
    error: computed(() => error.value),
    loadSettings,
    updateSettings,
    changePassword,
  };
}
```

---

## 🎯 Quick Start Checklist

1. **Authentication**: Ensure you have a valid JWT token from login
2. **Fetch Settings**: Call `GET /users/me/settings` on app load or settings page mount
3. **Update on Change**: Call `PATCH /users/me/settings` with only the changed fields
4. **Apply Locally**: Update your app's theme/language/etc based on the settings
5. **Persist**: Settings are automatically stored in the database

---

## 🔒 Security Notes

- All endpoints require authentication via Bearer token
- Password changes require the current password for verification
- New passwords must be at least 8 characters long
- Users can only access and modify their own settings

---

## 🌐 Available Options

### Default Landing Pages
- `/dashboard` - Main dashboard
- `/courses` - Course list
- `/terminals` - Terminal sessions
- `/labs` - Lab environments

### Languages
- `en` - English
- `fr` - Français
- `es` - Español
- `de` - Deutsch
- `it` - Italiano
- (Add more as needed in your frontend)

### Themes
- `light` - Light mode
- `dark` - Dark mode
- `auto` - System preference

### Timezones
Use standard IANA timezone identifiers:
- `UTC`
- `Europe/Paris`
- `America/New_York`
- `Asia/Tokyo`
- etc.

---

## 🐛 Error Handling

### Common Error Responses

**401 Unauthorized**
```json
{
  "error": "User not authenticated"
}
```

**404 Not Found** (shouldn't happen anymore - auto-creates)
```json
{
  "error": "Settings not found"
}
```

**400 Bad Request**
```json
{
  "error": "new password and confirmation do not match"
}
```

---

## 📝 Testing with cURL

```bash
# 1. Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"1.supervisor@test.com","password":"test"}' \
  | jq -r '.access_token')

# 2. Get settings
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/users/me/settings | jq

# 3. Update theme
curl -X PATCH \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"theme":"dark"}' \
  http://localhost:8080/api/v1/users/me/settings | jq

# 4. Change password
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "current_password":"test",
    "new_password":"newPassword123",
    "confirm_password":"newPassword123"
  }' \
  http://localhost:8080/api/v1/users/me/change-password
```

---

## 🔄 Automatic Settings Creation

Settings are now automatically created with defaults when:
1. A new user registers (via the user creation hook)
2. An existing user accesses `GET /users/me/settings` for the first time

**Default Values:**
- Default Landing Page: `/dashboard`
- Language: `en`
- Timezone: `UTC`
- Theme: `light`
- Compact Mode: `false`
- Email Notifications: `true`
- Desktop Notifications: `false`
- Two-Factor Enabled: `false`

---

## 🚀 Production Recommendations

1. **Cache Settings**: Store settings in your state management (Redux, Vuex, Pinia, etc.)
2. **Debounce Updates**: Don't send a PATCH request on every keystroke - debounce or save on blur
3. **Optimistic Updates**: Update UI immediately, rollback on error
4. **Error Handling**: Show user-friendly error messages
5. **Loading States**: Show spinners/skeletons while loading
6. **Validation**: Validate on frontend before sending to API

---

## 📚 Full API Documentation

Visit the Swagger UI for complete API documentation:
```
http://localhost:8080/swagger/
```

Look for the `user-settings` tag in the Swagger UI.
