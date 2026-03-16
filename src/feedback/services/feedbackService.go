package services

import (
	"encoding/json"
	"fmt"
	"time"

	configModels "soli/formations/src/configuration/models"
	"soli/formations/src/feedback/dto"
)

// EmailSender is the interface for sending emails
type EmailSender interface {
	SendEmail(to, subject, body string) error
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

	// 4. Build HTML email body
	body := fmt.Sprintf(`<h2>OCF Feedback — %s</h2>
<p><strong>Type:</strong> %s</p>
<p><strong>Message:</strong> %s</p>
<p><strong>Page URL:</strong> %s</p>
<p><strong>User Agent:</strong> %s</p>
<p><strong>User Email:</strong> %s</p>
<p><strong>User ID:</strong> %s</p>
<p><strong>Timestamp:</strong> %s</p>`,
		input.Type,
		input.Type,
		input.Message,
		input.PageURL,
		input.UserAgent,
		userEmail,
		userID,
		time.Now().UTC().Format(time.RFC3339),
	)

	// If screenshot is provided, embed it as an inline image
	if input.Screenshot != "" {
		body += fmt.Sprintf(`<p><strong>Screenshot:</strong></p><img src="data:image/png;base64,%s" />`, input.Screenshot)
	}

	// 5. Send email to each recipient
	for _, recipient := range recipients {
		if err := s.emailSender.SendEmail(recipient, subject, body); err != nil {
			return fmt.Errorf("failed to send feedback email to %s: %w", recipient, err)
		}
	}

	return nil
}
