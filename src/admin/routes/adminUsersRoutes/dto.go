package adminUsersRoutes

// OrgMembership describes a user's link to an organization in the admin
// users-with-memberships listing. MemberID is the organization_members row
// id (distinct from ID, the organization's own id) — PATCH
// /organizations/:id/members/:userId doesn't need it, but it is carried for
// symmetry with GroupMembership, which does.
type OrgMembership struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	MemberID string `json:"member_id"`
}

// GroupMembership describes a user's link to a class group in the admin
// users-with-memberships listing. MemberID is the group_members row id —
// the frontend's role-change control targets it directly via the generic
// PATCH /group-members/:id entity route, since there is no dedicated
// /groups/:id/members/:userId endpoint (see OrganizationMembersManager.vue
// for the organization equivalent, which doesn't need this field).
type GroupMembership struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	MemberID string `json:"member_id"`
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

// SetForbiddenInput is the body of PUT /admin/users/:id/forbidden.
type SetForbiddenInput struct {
	Forbidden bool `json:"forbidden"`
}

// ActionResult is the JSON body every write action in this module returns
// on success — the actions are fire-and-forget from the frontend's point of
// view (no resource body to echo back), so a flat ack is enough.
type ActionResult struct {
	Success bool `json:"success"`
}
