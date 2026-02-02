package services

import (
	"encoding/json"
	"log"

	"soli/formations/src/email/models"

	"gorm.io/gorm"
)

// InitDefaultTemplates creates default email templates if they don't exist
func InitDefaultTemplates(db *gorm.DB) {
	log.Println("📧 Initializing default email templates...")

	templates := []models.EmailTemplate{
		{
			Name:        "password_reset",
			DisplayName: "Password Reset",
			Description: "Email sent when a user requests a password reset",
			Subject:     "Reset Your Password - {{.PlatformName}}",
			HTMLBody: getPasswordResetTemplate(),
			Variables: func() string {
				vars := []models.EmailTemplateVariable{
					{Name: "ResetLink", Description: "Full URL for password reset", Example: "https://example.com/reset-password?token=abc123"},
					{Name: "ResetURL", Description: "Base reset URL", Example: "https://example.com/reset-password"},
					{Name: "Token", Description: "Reset token", Example: "abc123def456"},
					{Name: "PlatformName", Description: "Name of the platform", Example: "OCF Platform"},
				}
				data, _ := json.Marshal(vars)
				return string(data)
			}(),
			IsActive: true,
			IsSystem: true,
		},
		{
			Name:        "welcome",
			DisplayName: "Welcome Email",
			Description: "Email sent to new users when they register",
			Subject:     "Welcome to {{.PlatformName}}!",
			HTMLBody: getWelcomeTemplate(),
			Variables: func() string {
				vars := []models.EmailTemplateVariable{
					{Name: "UserName", Description: "User's display name", Example: "John Doe"},
					{Name: "Email", Description: "User's email address", Example: "john@example.com"},
					{Name: "PlatformName", Description: "Name of the platform", Example: "OCF Platform"},
					{Name: "LoginURL", Description: "URL to login page", Example: "https://example.com/login"},
				}
				data, _ := json.Marshal(vars)
				return string(data)
			}(),
			IsActive: true,
			IsSystem: false,
		},
		{
			Name:        "email_verification",
			DisplayName: "Email Verification",
			Description: "Email sent to verify user's email address",
			Subject:     "Verify Your Email - {{.PlatformName}}",
			HTMLBody:    getEmailVerificationTemplate(),
			Variables: func() string {
				vars := []models.EmailTemplateVariable{
					{Name: "VerificationLink", Description: "Full verification URL", Example: "https://example.com/verify-email?token=abc123"},
					{Name: "Token", Description: "64-character verification token", Example: "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6a7b8c9d0e1f2"},
					{Name: "UserName", Description: "User's display name", Example: "John Doe"},
					{Name: "PlatformName", Description: "Platform name", Example: "OCF Platform"},
					{Name: "ExpiryHours", Description: "Hours until expiry", Example: "48"},
				}
				data, _ := json.Marshal(vars)
				return string(data)
			}(),
			IsActive: true,
			IsSystem: true,
		},
	}

	for _, template := range templates {
		var existing models.EmailTemplate
		result := db.Where("name = ?", template.Name).First(&existing)

		if result.Error == gorm.ErrRecordNotFound {
			if err := db.Create(&template).Error; err != nil {
				log.Printf("❌ Failed to create template '%s': %v", template.Name, err)
			} else {
				log.Printf("✅ Created template: %s", template.DisplayName)
			}
		} else {
			log.Printf("ℹ️  Template already exists: %s", template.DisplayName)
		}
	}

	log.Println("✅ Email templates initialization complete")
}

// getPasswordResetTemplate returns the HTML template for password reset emails
func getPasswordResetTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Password Reset</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 600px;
            margin: 40px auto;
            background-color: #ffffff;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 40px 30px;
            text-align: center;
        }
        .header h1 {
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 8px;
        }
        .content {
            padding: 40px 30px;
        }
        .content p {
            margin-bottom: 20px;
            color: #555;
            font-size: 16px;
        }
        .button-container {
            text-align: center;
            margin: 35px 0;
        }
        .button {
            display: inline-block;
            padding: 14px 32px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            text-decoration: none;
            border-radius: 6px;
            font-weight: 600;
            font-size: 16px;
            transition: transform 0.2s;
        }
        .button:hover {
            transform: translateY(-2px);
        }
        .link-box {
            background-color: #f8f9fa;
            padding: 15px;
            border-radius: 6px;
            border: 1px solid #e9ecef;
            word-break: break-all;
            font-size: 14px;
            color: #6c757d;
            margin: 20px 0;
        }
        .warning {
            background-color: #fff3cd;
            border-left: 4px solid #ffc107;
            padding: 15px;
            margin: 20px 0;
            border-radius: 4px;
        }
        .warning-icon {
            font-size: 20px;
            margin-right: 8px;
        }
        .warning-text {
            color: #856404;
            font-size: 14px;
            font-weight: 500;
        }
        .footer {
            background-color: #f8f9fa;
            padding: 25px 30px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer p {
            color: #6c757d;
            font-size: 13px;
            margin-bottom: 8px;
        }
        @media only screen and (max-width: 600px) {
            .container {
                margin: 0;
                border-radius: 0;
            }
            .header, .content, .footer {
                padding: 25px 20px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔐 Password Reset Request</h1>
        </div>
        <div class="content">
            <p>Hello,</p>
            <p>We received a request to reset your password for your account. Click the button below to create a new password:</p>

            <div class="button-container">
                <a href="{{.ResetLink}}" class="button">Reset My Password</a>
            </div>

            <p>Or copy and paste this link into your browser:</p>
            <div class="link-box">{{.ResetLink}}</div>

            <div class="warning">
                <span class="warning-icon">⚠️</span>
                <span class="warning-text">This link will expire in 1 hour for security reasons.</span>
            </div>

            <p style="margin-top: 30px; color: #6c757d; font-size: 14px;">
                If you didn't request a password reset, you can safely ignore this email. Your password will remain unchanged.
            </p>
        </div>
        <div class="footer">
            <p>© 2025 {{.PlatformName}}. All rights reserved.</p>
            <p>This is an automated message, please do not reply to this email.</p>
        </div>
    </div>
</body>
</html>`
}

// getWelcomeTemplate returns the HTML template for welcome emails
func getWelcomeTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Welcome</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 600px;
            margin: 40px auto;
            background-color: #ffffff;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 40px 30px;
            text-align: center;
        }
        .header h1 {
            font-size: 32px;
            font-weight: 600;
            margin-bottom: 8px;
        }
        .content {
            padding: 40px 30px;
        }
        .content p {
            margin-bottom: 20px;
            color: #555;
            font-size: 16px;
        }
        .button-container {
            text-align: center;
            margin: 35px 0;
        }
        .button {
            display: inline-block;
            padding: 14px 32px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            text-decoration: none;
            border-radius: 6px;
            font-weight: 600;
            font-size: 16px;
        }
        .footer {
            background-color: #f8f9fa;
            padding: 25px 30px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer p {
            color: #6c757d;
            font-size: 13px;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🎉 Welcome!</h1>
        </div>
        <div class="content">
            <p>Hi {{.UserName}},</p>
            <p>Welcome to {{.PlatformName}}! We're excited to have you on board.</p>
            <p>Your account has been successfully created with the email: <strong>{{.Email}}</strong></p>

            <div class="button-container">
                <a href="{{.LoginURL}}" class="button">Get Started</a>
            </div>

            <p>If you have any questions, feel free to reach out to our support team.</p>
        </div>
        <div class="footer">
            <p>© 2025 {{.PlatformName}}. All rights reserved.</p>
        </div>
    </div>
</body>
</html>`
}

// getEmailVerificationTemplate returns the HTML template for email verification emails
func getEmailVerificationTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Email Verification</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 600px;
            margin: 40px auto;
            background-color: #ffffff;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 40px 30px;
            text-align: center;
        }
        .header h1 {
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 8px;
        }
        .content {
            padding: 40px 30px;
        }
        .content p {
            margin-bottom: 20px;
            color: #555;
            font-size: 16px;
        }
        .button-container {
            text-align: center;
            margin: 35px 0;
        }
        .button {
            display: inline-block;
            padding: 14px 32px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            text-decoration: none;
            border-radius: 6px;
            font-weight: 600;
            font-size: 16px;
            transition: transform 0.2s;
        }
        .button:hover {
            transform: translateY(-2px);
        }
        .link-box {
            background-color: #f8f9fa;
            padding: 15px;
            border-radius: 6px;
            border: 1px solid #e9ecef;
            word-break: break-all;
            font-size: 14px;
            color: #6c757d;
            margin: 20px 0;
        }
        .info-box {
            background-color: #e7f3ff;
            border-left: 4px solid #2196f3;
            padding: 15px;
            margin: 20px 0;
            border-radius: 4px;
        }
        .info-icon {
            font-size: 20px;
            margin-right: 8px;
        }
        .info-text {
            color: #0d47a1;
            font-size: 14px;
            font-weight: 500;
        }
        .token-box {
            background-color: #f8f9fa;
            padding: 15px;
            border-radius: 6px;
            border: 2px dashed #d1d5db;
            word-break: break-all;
            font-family: monospace;
            font-size: 13px;
            color: #374151;
            margin: 15px 0;
            user-select: all;
        }
        .token-label {
            font-size: 14px;
            font-weight: 600;
            color: #374151;
            margin-bottom: 8px;
        }
        .footer {
            background-color: #f8f9fa;
            padding: 25px 30px;
            text-align: center;
            border-top: 1px solid #e9ecef;
        }
        .footer p {
            color: #6c757d;
            font-size: 13px;
            margin-bottom: 8px;
        }
        @media only screen and (max-width: 600px) {
            .container {
                margin: 0;
                border-radius: 0;
            }
            .header, .content, .footer {
                padding: 25px 20px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>✉️ Verify Your Email</h1>
        </div>
        <div class="content">
            <p>Hi {{.UserName}},</p>
            <p>Thank you for signing up! Please verify your email address to complete your registration and access all features.</p>

            <div class="button-container">
                <a href="{{.VerificationLink}}" class="button">Verify My Email</a>
            </div>

            <p>Or copy and paste this link into your browser:</p>
            <div class="link-box">{{.VerificationLink}}</div>

            <p style="margin-top: 20px; font-size: 14px; color: #6b7280;">
                If the link doesn't work, you can copy this verification code:
            </p>
            <div class="token-label">Verification Code:</div>
            <div class="token-box">{{.Token}}</div>

            <div class="info-box">
                <span class="info-icon">ℹ️</span>
                <span class="info-text">This link will expire in {{.ExpiryHours}} hours for security reasons.</span>
            </div>

            <p style="margin-top: 30px; color: #6c757d; font-size: 14px;">
                If you didn't create an account with {{.PlatformName}}, you can safely ignore this email.
            </p>
        </div>
        <div class="footer">
            <p>© 2025 {{.PlatformName}}. All rights reserved.</p>
            <p>This is an automated message, please do not reply to this email.</p>
        </div>
    </div>
</body>
</html>`
}
