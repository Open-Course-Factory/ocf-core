package dto

type UsernameInput struct {
	Username string `binding:"required"`
}

type UsernameOutput struct {
	Username string
	Id       string `json:"id"`
}
