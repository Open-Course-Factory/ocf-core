package dto

import "soli/formations/src/labs/models"

type ConnectionInput struct {
	MachineID  string
	UsernameID string
}

type ConnectionOutput struct {
	Machine  *models.Machine
	Username *models.Username
}
