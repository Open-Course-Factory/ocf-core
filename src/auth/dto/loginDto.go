package dto

type LoginOutput struct {
	UserName         string `json:"user_name"`
	AccessToken      string `json:"access_token"`
	RenewAccessToken string `json:"renew_access_token"`
}

type LoginInput struct {
	Email    string `binding:"required"`
	Password string `binding:"required"`
}
