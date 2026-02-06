package dto

// CurrentUserOutput represents basic information about the current authenticated user
type CurrentUserOutput struct {
	UserID          string   `json:"user_id"`
	UserName        string   `json:"username"`
	DisplayName     string   `json:"display_name"`
	Email           string   `json:"email"`
	FirstName       string   `json:"first_name,omitempty"`
	LastName        string   `json:"last_name,omitempty"`
	Avatar          string   `json:"avatar,omitempty"`
	Roles           []string `json:"roles"`
	IsAdmin         bool     `json:"is_admin"`
	EmailVerified   bool     `json:"email_verified"`
	EmailVerifiedAt string   `json:"email_verified_at,omitempty"`
}
