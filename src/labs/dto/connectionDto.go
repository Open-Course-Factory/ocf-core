package dto

type ConnectionInput struct {
	MachineID  string
	UsernameID string
}

type ConnectionOutput struct {
	MachineDtoOutput  *MachineOutput  `json:"Machine"`
	UsernameDtoOutput *UsernameOutput `json:"Username"`
	ID                string          `json:"id"`
}
