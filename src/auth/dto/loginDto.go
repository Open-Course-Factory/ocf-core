package dto

type LoginOutput struct {
	UserName         string   `json:"user_name"`
	DisplayName      string   `json:"display_name"`
	UserId           string   `json:"user_id"`
	AccessToken      string   `json:"access_token"`
	RenewAccessToken string   `json:"renew_access_token"`
	UserRoles        []string `json:"user_roles"`
}

type LoginInput struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}
