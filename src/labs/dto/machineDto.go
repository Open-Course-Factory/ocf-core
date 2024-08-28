package dto

type MachineInput struct {
	Name string `binding:"required"`
	IP   string `binding:"required"`
	Port int    `binding:"required"`
}

type MachineOutput struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}
