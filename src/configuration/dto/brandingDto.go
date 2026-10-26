package dto

// UpdateBrandingInput replaces the instance identity. Logo is a data URI;
// nil keeps the current logo, "" removes it.
type UpdateBrandingInput struct {
	Name           string  `json:"name"`
	PrimaryColor   string  `json:"primary_color"`
	SecondaryColor string  `json:"secondary_color"`
	Logo           *string `json:"logo"`
}
