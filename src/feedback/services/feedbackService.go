package services

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	configModels "soli/formations/src/configuration/models"
	"soli/formations/src/feedback/dto"
)

// EmailSender is the interface for sending emails
type EmailSender interface {
	SendEmail(to, subject, body string) error
	SendEmailWithAttachment(to, subject, body, attachmentName, attachmentBase64 string) error
}

// FeatureGetter is the interface for retrieving feature configuration
type FeatureGetter interface {
	GetFeatureByKey(key string) (*configModels.Feature, error)
}

// FeedbackService handles feedback submission logic
type FeedbackService interface {
	SendFeedback(input dto.SendFeedbackInput, userID, userEmail string) error
}

// NewFeedbackService creates a new FeedbackService
func NewFeedbackService(emailSender EmailSender, featureGetter FeatureGetter) FeedbackService {
	return &feedbackService{
		emailSender:   emailSender,
		featureGetter: featureGetter,
	}
}

type feedbackService struct {
	emailSender   EmailSender
	featureGetter FeatureGetter
}

// SendFeedback retrieves feedback recipients from configuration, builds an HTML
// email with the feedback details, and sends it to each recipient.
func (s *feedbackService) SendFeedback(input dto.SendFeedbackInput, userID, userEmail string) error {
	// 1. Read feedback_recipients from feature table
	feature, err := s.featureGetter.GetFeatureByKey("feedback_recipients")
	if err != nil {
		return fmt.Errorf("no feedback recipient configured: %w", err)
	}

	// 2. Parse JSON array of recipient emails
	var recipients []string
	if err := json.Unmarshal([]byte(feature.Value), &recipients); err != nil {
		return fmt.Errorf("failed to parse feedback recipients JSON: %w", err)
	}

	if len(recipients) == 0 {
		return fmt.Errorf("no feedback recipient configured")
	}

	// 3. Build email subject (first 50 chars of message)
	truncatedMessage := input.Message
	if len(truncatedMessage) > 50 {
		truncatedMessage = truncatedMessage[:50]
	}
	subject := fmt.Sprintf("[OCF Feedback] %s: %s", input.Type, truncatedMessage)

	// 4. HTML-escape all user-provided values to prevent XSS
	safeType := html.EscapeString(input.Type)
	safeMessage := html.EscapeString(input.Message)
	safePageURL := html.EscapeString(input.PageURL)
	safeUserAgent := html.EscapeString(input.UserAgent)
	safeUserEmail := html.EscapeString(userEmail)
	safeUserID := html.EscapeString(userID)

	// 5. Build HTML email body with escaped values
	body := fmt.Sprintf(`<h2>OCF Feedback — %s</h2>
<p><strong>Type:</strong> %s</p>
<p><strong>Message:</strong> %s</p>
<p><strong>Page URL:</strong> %s</p>
<p><strong>User Agent:</strong> %s</p>
<p><strong>User Email:</strong> %s</p>
<p><strong>User ID:</strong> %s</p>
<p><strong>Timestamp:</strong> %s</p>`,
		safeType,
		safeType,
		safeMessage,
		safePageURL,
		safeUserAgent,
		safeUserEmail,
		safeUserID,
		time.Now().UTC().Format(time.RFC3339),
	)

	// If screenshot is provided, add a note and send as attachment
	hasScreenshot := input.Screenshot != ""
	if hasScreenshot {
		body += `<p><strong>Screenshot:</strong> see attached image</p>`
	}

	// 6. Send email to each recipient, collecting errors
	var sendErrors []string
	for _, recipient := range recipients {
		var err error
		if hasScreenshot {
			err = s.emailSender.SendEmailWithAttachment(recipient, subject, body, "screenshot.png", input.Screenshot)
		} else {
			err = s.emailSender.SendEmail(recipient, subject, body)
		}
		if err != nil {
			sendErrors = append(sendErrors, fmt.Sprintf("%s: %v", recipient, err))
		}
	}
	if len(sendErrors) > 0 {
		return fmt.Errorf("failed to send feedback email to: %s", strings.Join(sendErrors, "; "))
	}

	return nil
}
