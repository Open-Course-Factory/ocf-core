package dto

// SendFeedbackInput is the input DTO for the POST /api/v1/feedback/send endpoint
type SendFeedbackInput struct {
	Type       string `json:"type" binding:"required,oneof=bug suggestion question" validate:"required,oneof=bug suggestion question"`
	Message    string `json:"message" binding:"required,min=10" validate:"required,min=10"`
	PageURL    string `json:"page_url,omitempty" binding:"omitempty,url" validate:"omitempty,url"`
	UserAgent  string `json:"user_agent,omitempty" binding:"omitempty"`
	Screenshot string `json:"screenshot,omitempty" binding:"omitempty"`
}

// SendFeedbackResponse is the response DTO for the feedback endpoint
type SendFeedbackResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
