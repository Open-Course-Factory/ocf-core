package dto

type UsernameInput struct {
	Username string `binding:"required"`
}

type UsernameOutput struct {
	Username string
	ID       string
}
