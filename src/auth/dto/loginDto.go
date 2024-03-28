package dto

type LoginOutput struct {
	Email            string
	AccessToken      string
	RenewAccessToken string
}

type LoginInput struct {
	Email    string `binding:"required"`
	Password string `binding:"required"`
}
