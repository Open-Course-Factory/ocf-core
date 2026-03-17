package feedback_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configModels "soli/formations/src/configuration/models"
	"soli/formations/src/feedback/dto"
	"soli/formations/src/feedback/services"
)

// ==========================================
// Mock implementations
// ==========================================

// mockEmailSender implements services.EmailSender for testing
type mockEmailSender struct {
	sendEmailFunc func(to, subject, body string) error
	calls         []emailCall
}

type emailCall struct {
	To      string
	Subject string
	Body    string
}

func (m *mockEmailSender) SendEmail(to, subject, body string) error {
	m.calls = append(m.calls, emailCall{To: to, Subject: subject, Body: body})
	if m.sendEmailFunc != nil {
		return m.sendEmailFunc(to, subject, body)
	}
	return nil
}

func (m *mockEmailSender) SendEmailWithAttachment(to, subject, body, attachmentName, attachmentBase64 string) error {
	m.calls = append(m.calls, emailCall{To: to, Subject: subject, Body: body + attachmentBase64})
	if m.sendEmailFunc != nil {
		return m.sendEmailFunc(to, subject, body)
	}
	return nil
}

// mockFeatureGetter implements services.FeatureGetter for testing
type mockFeatureGetter struct {
	getFeatureByKeyFunc func(key string) (*configModels.Feature, error)
}

func (m *mockFeatureGetter) GetFeatureByKey(key string) (*configModels.Feature, error) {
	if m.getFeatureByKeyFunc != nil {
		return m.getFeatureByKeyFunc(key)
	}
	return nil, errors.New("feature not found")
}

// ==========================================
// Helper functions
// ==========================================

// newValidFeedbackInput returns a valid feedback input for tests
func newValidFeedbackInput() dto.SendFeedbackInput {
	return dto.SendFeedbackInput{
		Type:      "bug",
		Message:   "The page crashes when I click the submit button on the course editor",
		PageURL:   "https://ocf.solution-libre.fr/courses/edit/123",
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
	}
}

// featureWithRecipients creates a Feature with the given email recipients as JSON array
func featureWithRecipients(emails []string) *configModels.Feature {
	recipientsJSON, _ := json.Marshal(emails)
	return &configModels.Feature{
		Key:     "feedback_recipients",
		Name:    "Feedback Recipients",
		Enabled: true,
		Value:   string(recipientsJSON),
	}
}

// ==========================================
// Service Tests
// ==========================================

func TestFeedbackService_SendFeedback_Success(t *testing.T) {
	// Arrange: configure mocks with valid recipients
	emailSender := &mockEmailSender{}
	featureGetter := &mockFeatureGetter{
		getFeatureByKeyFunc: func(key string) (*configModels.Feature, error) {
			assert.Equal(t, "feedback_recipients", key)
			return featureWithRecipients([]string{"admin@example.com", "support@example.com"}), nil
		},
	}

	svc := services.NewFeedbackService(emailSender, featureGetter)
	input := newValidFeedbackInput()

	// Act
	err := svc.SendFeedback(input, "user-123", "testuser@example.com")

	// Assert
	require.NoError(t, err)
	require.Len(t, emailSender.calls, 2, "should send one email per recipient")

	// Verify emails were sent to the correct recipients
	assert.Equal(t, "admin@example.com", emailSender.calls[0].To)
	assert.Equal(t, "support@example.com", emailSender.calls[1].To)

	// Verify email content includes feedback details
	for _, call := range emailSender.calls {
		assert.Contains(t, call.Subject, "bug", "subject should contain the feedback type")
		assert.Contains(t, call.Body, input.Message, "body should contain the user message")
		assert.Contains(t, call.Body, input.PageURL, "body should contain the page URL")
	}
}

func TestFeedbackService_SendFeedback_NoRecipients(t *testing.T) {
	t.Run("feature key not found", func(t *testing.T) {
		emailSender := &mockEmailSender{}
		featureGetter := &mockFeatureGetter{
			getFeatureByKeyFunc: func(key string) (*configModels.Feature, error) {
				return nil, errors.New("record not found")
			},
		}

		svc := services.NewFeedbackService(emailSender, featureGetter)
		input := newValidFeedbackInput()

		err := svc.SendFeedback(input, "user-123", "testuser@example.com")

		require.Error(t, err, "should return error when feature key not found")
		assert.Contains(t, err.Error(), "recipient")
		assert.Empty(t, emailSender.calls, "no emails should be sent")
	})

	t.Run("empty recipients array", func(t *testing.T) {
		emailSender := &mockEmailSender{}
		featureGetter := &mockFeatureGetter{
			getFeatureByKeyFunc: func(key string) (*configModels.Feature, error) {
				return featureWithRecipients([]string{}), nil
			},
		}

		svc := services.NewFeedbackService(emailSender, featureGetter)
		input := newValidFeedbackInput()

		err := svc.SendFeedback(input, "user-123", "testuser@example.com")

		require.Error(t, err, "should return error when recipients array is empty")
		assert.Contains(t, err.Error(), "recipient")
		assert.Empty(t, emailSender.calls, "no emails should be sent")
	})
}

func TestFeedbackService_SendFeedback_InvalidRecipientsJSON(t *testing.T) {
	emailSender := &mockEmailSender{}
	featureGetter := &mockFeatureGetter{
		getFeatureByKeyFunc: func(key string) (*configModels.Feature, error) {
			return &configModels.Feature{
				Key:     "feedback_recipients",
				Name:    "Feedback Recipients",
				Enabled: true,
				Value:   "not-valid-json{[",
			}, nil
		},
	}

	svc := services.NewFeedbackService(emailSender, featureGetter)
	input := newValidFeedbackInput()

	err := svc.SendFeedback(input, "user-123", "testuser@example.com")

	require.Error(t, err, "should return error when recipients JSON is invalid")
	assert.Contains(t, err.Error(), "JSON", "error should mention JSON parsing failure")
	assert.Empty(t, emailSender.calls, "no emails should be sent")
}

func TestFeedbackService_SendFeedback_EmailFailure(t *testing.T) {
	emailSender := &mockEmailSender{
		sendEmailFunc: func(to, subject, body string) error {
			return errors.New("SMTP connection refused")
		},
	}
	featureGetter := &mockFeatureGetter{
		getFeatureByKeyFunc: func(key string) (*configModels.Feature, error) {
			return featureWithRecipients([]string{"admin@example.com"}), nil
		},
	}

	svc := services.NewFeedbackService(emailSender, featureGetter)
	input := newValidFeedbackInput()

	err := svc.SendFeedback(input, "user-123", "testuser@example.com")

	require.Error(t, err, "should return error when email sending fails")
	assert.Contains(t, err.Error(), "email", "error should mention email sending failure")
}

func TestFeedbackService_SendFeedback_WithScreenshot(t *testing.T) {
	emailSender := &mockEmailSender{}
	featureGetter := &mockFeatureGetter{
		getFeatureByKeyFunc: func(key string) (*configModels.Feature, error) {
			return featureWithRecipients([]string{"admin@example.com"}), nil
		},
	}

	svc := services.NewFeedbackService(emailSender, featureGetter)
	input := newValidFeedbackInput()
	input.Screenshot = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	err := svc.SendFeedback(input, "user-123", "testuser@example.com")

	require.NoError(t, err)
	require.Len(t, emailSender.calls, 1, "should send one email to the recipient")

	// The email body should include the screenshot as an embedded image
	assert.Contains(t, emailSender.calls[0].Body, input.Screenshot, "email body should contain the base64 screenshot")
}

// ==========================================
// Input Validation Tests
// ==========================================

func TestFeedbackInput_Validation_ValidInput(t *testing.T) {
	validate := validator.New()
	input := newValidFeedbackInput()

	err := validate.Struct(input)

	assert.NoError(t, err, "valid input should pass validation")
}

func TestFeedbackInput_Validation_InvalidType(t *testing.T) {
	validate := validator.New()
	input := newValidFeedbackInput()
	input.Type = "complaint" // not in [bug, suggestion, question]

	err := validate.Struct(input)

	assert.Error(t, err, "invalid type should fail validation")

	var validationErrors validator.ValidationErrors
	require.True(t, errors.As(err, &validationErrors))

	found := false
	for _, fieldErr := range validationErrors {
		if fieldErr.Field() == "Type" {
			found = true
			assert.Equal(t, "oneof", fieldErr.Tag(), "should fail on oneof validation")
		}
	}
	assert.True(t, found, "Type field should have a validation error")
}

func TestFeedbackInput_Validation_MessageTooShort(t *testing.T) {
	validate := validator.New()
	input := newValidFeedbackInput()
	input.Message = "short" // less than 10 characters

	err := validate.Struct(input)

	assert.Error(t, err, "message shorter than 10 chars should fail validation")

	var validationErrors validator.ValidationErrors
	require.True(t, errors.As(err, &validationErrors))

	found := false
	for _, fieldErr := range validationErrors {
		if fieldErr.Field() == "Message" {
			found = true
			assert.Equal(t, "min", fieldErr.Tag(), "should fail on min length validation")
		}
	}
	assert.True(t, found, "Message field should have a validation error")
}

func TestFeedbackInput_Validation_MissingType(t *testing.T) {
	validate := validator.New()
	input := newValidFeedbackInput()
	input.Type = "" // empty type

	err := validate.Struct(input)

	assert.Error(t, err, "empty type should fail validation")

	var validationErrors validator.ValidationErrors
	require.True(t, errors.As(err, &validationErrors))

	found := false
	for _, fieldErr := range validationErrors {
		if fieldErr.Field() == "Type" {
			found = true
			assert.Equal(t, "required", fieldErr.Tag(), "should fail on required validation")
		}
	}
	assert.True(t, found, "Type field should have a validation error")
}

func TestFeedbackInput_Validation_MissingMessage(t *testing.T) {
	validate := validator.New()
	input := newValidFeedbackInput()
	input.Message = "" // empty message

	err := validate.Struct(input)

	assert.Error(t, err, "empty message should fail validation")

	var validationErrors validator.ValidationErrors
	require.True(t, errors.As(err, &validationErrors))

	found := false
	for _, fieldErr := range validationErrors {
		if fieldErr.Field() == "Message" {
			found = true
			assert.Equal(t, "required", fieldErr.Tag(), "should fail on required validation")
		}
	}
	assert.True(t, found, "Message field should have a validation error")
}

// ==========================================
// Additional validation edge cases
// ==========================================

func TestFeedbackInput_Validation_AllValidTypes(t *testing.T) {
	validate := validator.New()

	validTypes := []string{"bug", "suggestion", "question"}
	for _, feedbackType := range validTypes {
		t.Run(feedbackType, func(t *testing.T) {
			input := newValidFeedbackInput()
			input.Type = feedbackType

			err := validate.Struct(input)
			assert.NoError(t, err, "type '%s' should be valid", feedbackType)
		})
	}
}

func TestFeedbackInput_Validation_OptionalFieldsCanBeEmpty(t *testing.T) {
	validate := validator.New()
	input := dto.SendFeedbackInput{
		Type:    "bug",
		Message: "This is a valid feedback message with enough length",
		// PageURL, UserAgent, Screenshot are all omitted
	}

	err := validate.Struct(input)
	assert.NoError(t, err, "optional fields should not cause validation errors when empty")
}
