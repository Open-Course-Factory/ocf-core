package models

type Organisation struct {
	BaseModel
	OrganisationName string  `json:"organisationName"`
	Groups           []Group `json:"groups"`
}
