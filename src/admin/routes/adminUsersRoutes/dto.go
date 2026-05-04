package adminUsersRoutes

// OrgMembership describes a user's link to an organization in the admin
// users-with-memberships listing.
type OrgMembership struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// GroupMembership describes a user's link to a class group in the admin
// users-with-memberships listing.
type GroupMembership struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// UserListing is the row returned by the admin users-with-memberships
// endpoint. Organizations and Groups MUST be non-nil empty slices when
// the user has no memberships so that the JSON encodes to [] not null.
type UserListing struct {
	ID            string            `json:"id"`
	Username      string            `json:"username"`
	DisplayName   string            `json:"display_name"`
	Email         string            `json:"email"`
	Avatar        string            `json:"avatar,omitempty"`
	IsActive      bool              `json:"is_active"`
	IsAdmin       bool              `json:"is_admin"`
	Organizations []OrgMembership   `json:"organizations"`
	Groups        []GroupMembership `json:"groups"`
}
