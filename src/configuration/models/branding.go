package models

// Branding is the singleton, public visual identity of an OCF instance.
// It is deliberately independent of organizations and personal theme settings.
type Branding struct {
	ID             uint   `gorm:"primaryKey" json:"-"`
	Name           string `json:"name"`
	Logo           string `gorm:"type:text" json:"logo"`
	PrimaryColor   string `json:"primary_color"`
	SecondaryColor string `json:"secondary_color"`
}

func DefaultBranding() Branding {
	return Branding{ID: 1, Name: "OCF", PrimaryColor: "#007bff", SecondaryColor: "#6c757d"}
}
