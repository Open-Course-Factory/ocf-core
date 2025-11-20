package services

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"time"

	"gorm.io/gorm"
)

type EmailService interface {
	SendEmail(to, subject, body string) error
	SendPasswordResetEmail(to, resetToken, resetURL string) error
	SendTemplatedEmail(to, templateName string, variables map[string]interface{}) error
}

type emailService struct {
	smtpHost        string
	smtpPort        string
	smtpUsername    string
	smtpPassword    string
	fromEmail       string
	fromName        string
	db              *gorm.DB
	templateService TemplateService
}

func NewEmailService() EmailService {
	return &emailService{
		smtpHost:     getEnv("SMTP_HOST", "smtp.gmail.com"),
		smtpPort:     getEnv("SMTP_PORT", "587"),
		smtpUsername: getEnv("SMTP_USERNAME", ""),
		smtpPassword: getEnv("SMTP_PASSWORD", ""),
		fromEmail:    getEnv("SMTP_FROM_EMAIL", "noreply@yourdomain.com"),
		fromName:     getEnv("SMTP_FROM_NAME", "OCF Platform"),
		db:           nil, // Will be set when used with templates
	}
}

func NewEmailServiceWithDB(db *gorm.DB) EmailService {
	return &emailService{
		smtpHost:        getEnv("SMTP_HOST", "smtp.gmail.com"),
		smtpPort:        getEnv("SMTP_PORT", "587"),
		smtpUsername:    getEnv("SMTP_USERNAME", ""),
		smtpPassword:    getEnv("SMTP_PASSWORD", ""),
		fromEmail:       getEnv("SMTP_FROM_EMAIL", "noreply@yourdomain.com"),
		fromName:        getEnv("SMTP_FROM_NAME", "OCF Platform"),
		db:              db,
		templateService: NewTemplateService(db),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (s *emailService) SendEmail(to, subject, body string) error {
	// Validate configuration
	if s.smtpUsername == "" || s.smtpPassword == "" {
		return fmt.Errorf("SMTP credentials not configured. Please set SMTP_USERNAME and SMTP_PASSWORD environment variables")
	}

	// Build email message
	from := fmt.Sprintf("%s <%s>", s.fromName, s.fromEmail)

	msg := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/html; charset=UTF-8\r\n"+
			"\r\n"+
			"%s\r\n",
		from, to, subject, body,
	))

	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)

	// Port 465 requires implicit TLS, port 587 uses STARTTLS
	if s.smtpPort == "465" {
		return s.sendMailTLS(addr, from, to, msg)
	} else {
		return s.sendMailSTARTTLS(addr, from, to, msg)
	}
}

// sendMailSTARTTLS sends email using port 587 (STARTTLS)
func (s *emailService) sendMailSTARTTLS(addr, from, to string, msg []byte) error {
	auth := smtp.PlainAuth("", s.smtpUsername, s.smtpPassword, s.smtpHost)
	err := smtp.SendMail(addr, auth, s.fromEmail, []string{to}, msg)
	if err != nil {
		return fmt.Errorf("failed to send email via STARTTLS: %w", err)
	}
	return nil
}

// sendMailTLS sends email using port 465 (implicit TLS)
func (s *emailService) sendMailTLS(addr, from, to string, msg []byte) error {
	// TLS config
	tlsConfig := &tls.Config{
		ServerName: s.smtpHost,
		MinVersion: tls.VersionTLS12,
	}

	// Connect with timeout
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	// Establish TLS connection
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server with TLS: %w", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, s.smtpHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Quit()

	// Authenticate
	auth := smtp.PlainAuth("", s.smtpUsername, s.smtpPassword, s.smtpHost)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Set sender
	if err = client.Mail(s.fromEmail); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipient
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("failed to set recipient: %w", err)
	}

	// Send message
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	_, err = writer.Write(msg)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	fmt.Printf("✅ Email sent successfully via TLS (port 465) to: %s\n", to)
	return nil
}

func (s *emailService) SendPasswordResetEmail(to, resetToken, resetURL string) error {
	// Try to use template if DB is available
	if s.db != nil && s.templateService != nil {
		resetLink := fmt.Sprintf("%s?token=%s", resetURL, resetToken)
		variables := map[string]interface{}{
			"ResetLink": resetLink,
			"ResetURL":  resetURL,
			"Token":     resetToken,
		}
		return s.SendTemplatedEmail(to, "password_reset", variables)
	}

	// Fallback to hardcoded email (backwards compatibility)
	resetLink := fmt.Sprintf("%s?token=%s", resetURL, resetToken)
	subject := "Password Reset Request"

	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #4CAF50; color: white; padding: 20px; text-align: center; }
        .content { background-color: #f9f9f9; padding: 30px; }
        .button {
            display: inline-block;
            padding: 12px 24px;
            background-color: #4CAF50;
            color: white;
            text-decoration: none;
            border-radius: 4px;
            margin: 20px 0;
        }
        .footer { text-align: center; padding: 20px; font-size: 12px; color: #666; }
        .warning { color: #d32f2f; font-size: 14px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Password Reset Request</h1>
        </div>
        <div class="content">
            <p>Hello,</p>
            <p>We received a request to reset your password. Click the button below to create a new password:</p>
            <p style="text-align: center;">
                <a href="%s" class="button">Reset Password</a>
            </p>
            <p>Or copy and paste this link into your browser:</p>
            <p style="word-break: break-all; background-color: #fff; padding: 10px; border: 1px solid #ddd;">%s</p>
            <p class="warning">⚠️ This link will expire in 1 hour.</p>
            <p>If you didn't request a password reset, please ignore this email. Your password will remain unchanged.</p>
        </div>
        <div class="footer">
            <p>© 2025 OCF Platform. All rights reserved.</p>
        </div>
    </div>
</body>
</html>
`, resetLink, resetLink)

	return s.SendEmail(to, subject, body)
}

// SendTemplatedEmail sends an email using a template from the database
func (s *emailService) SendTemplatedEmail(to, templateName string, variables map[string]interface{}) error {
	if s.templateService == nil {
		return fmt.Errorf("template service not initialized - use NewEmailServiceWithDB")
	}

	subject, body, err := s.templateService.RenderTemplate(templateName, variables)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	return s.SendEmail(to, subject, body)
}
