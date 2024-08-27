package dto

type MachineInput struct {
	Name       string `binding:"required"`
	IP         string `binding:"required"`
	UsernameId string `binding:"required"`
	Port       int    `binding:"required"`
}

type MachineOutput struct {
	Name       string
	ID         string
	UsernameId string
	IP         string
	Port       int
}
