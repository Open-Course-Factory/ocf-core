package models

// Branding is the singleton, public visual identity of an OCF instance.
// It is deliberately independent of organizations and personal theme settings.
type Branding struct {
	ID   uint   `gorm:"primaryKey" json:"-"`
	Name string `json:"name"`
	// Logo is served by its own cacheable endpoint, never inlined in JSON.
	Logo string `gorm:"type:text" json:"-"`
	// LogoVersion fingerprints Logo: it versions the logo URL and is its ETag.
	// Width and height let clients reserve the logo box before it loads.
	LogoVersion    string `json:"logo_version"`
	LogoWidth      int    `json:"logo_width"`
	LogoHeight     int    `json:"logo_height"`
	PrimaryColor   string `json:"primary_color"`
	SecondaryColor string `json:"secondary_color"`
}

func DefaultBranding() Branding {
	return Branding{ID: 1, Name: "OCF", PrimaryColor: "#007bff", SecondaryColor: "#6c757d"}
}
