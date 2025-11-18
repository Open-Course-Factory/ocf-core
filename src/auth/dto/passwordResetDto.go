package dto

// RequestPasswordResetInput is the request body for requesting a password reset
type RequestPasswordResetInput struct {
	Email string `json:"email" binding:"required,email"`
}

// ResetPasswordInput is the request body for resetting a password with a token
type ResetPasswordInput struct {
	Token       string `json:"token" binding:"required,min=10"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// PasswordResetResponse is the response for password reset operations
type PasswordResetResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
