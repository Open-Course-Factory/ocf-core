package models

type ptyRequestMsg struct {
	Term     string
	Columns  uint32
	Rows     uint32
	Width    uint32
	Height   uint32
	ModeList string
}

type Terminal struct {
	Columns uint32 `json:"cols"`
	Rows    uint32 `json:"rows"`
}
