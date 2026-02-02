package dto

type VerifyEmailInput struct {
	Token string `json:"token" binding:"required,len=64"`
}

type ResendVerificationInput struct {
	Email string `json:"email" binding:"required,email"`
}

type VerificationStatusOutput struct {
	Verified   bool   `json:"verified"`
	VerifiedAt string `json:"verified_at,omitempty"`
	Email      string `json:"email"`
}
