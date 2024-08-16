package dto

type MachineInput struct {
	Name     string `binding:"required"`
	OwnerIDs []string
}

type MachineOutput struct {
	Name string
}
