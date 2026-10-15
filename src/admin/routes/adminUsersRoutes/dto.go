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
	EmailVerified bool              `json:"email_verified"`
	Organizations []OrgMembership   `json:"organizations"`
	Groups        []GroupMembership `json:"groups"`
}

// SetForbiddenInput is the body of PUT /admin/users/:id/forbidden.
type SetForbiddenInput struct {
	Forbidden bool `json:"forbidden"`
}

// SetPasswordInput is the body of PUT /admin/users/:id/password. The admin
// sets the password directly and hands it to the user themselves — unlike
// POST .../send-password-reset-email, no email is ever sent.
type SetPasswordInput struct {
	Password string `json:"password" binding:"required"`
}

// ActionResult is the JSON body every write action in this module returns
// on success — the actions are fire-and-forget from the frontend's point of
// view (no resource body to echo back), so a flat ack is enough.
type ActionResult struct {
	Success bool `json:"success"`
}

// AdminCreateUserInput is the body of POST /admin/users. Unlike the public
// self-signup endpoint (dto.CreateUserInput), it carries no ToS acceptance
// fields — an admin creating an account on someone's behalf isn't the one
// accepting terms, so the acceptance timestamp/version are synthesized at
// creation time instead (see NewCreateUserHandler), the same way the
// organization bulk-import path already does for its own admin-created
// accounts (src/organizations/services/importService.go).
type AdminCreateUserInput struct {
	Username    string `json:"username" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	Email       string `json:"email" binding:"required"`
	Password    string `json:"password" binding:"required"`
	FirstName   string `json:"first_name" binding:"required"`
	LastName    string `json:"last_name" binding:"required"`
	// EmailVerified marks the account verified immediately, at the admin's
	// discretion (e.g. the admin already confirmed the address out of band).
	// Default false: the account otherwise goes through the normal
	// self-service verification flow like any other account.
	EmailVerified bool `json:"email_verified"`
	// DefaultRole is an extra Casbin role to grant alongside "member" (e.g. a
	// custom role registered via access.RegisterRole), mirroring
	// dto.CreateUserInput.DefaultRole on the public signup endpoint. Optional.
	DefaultRole string `json:"default_role,omitempty"`
}

// AdminCreateUserOutput is the body returned by a successful POST /admin/users.
type AdminCreateUserOutput struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}
