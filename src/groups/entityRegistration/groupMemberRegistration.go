package groupRegistration

import (
	"net/http"
	"sync"
	"time"

	authModels "soli/formations/src/auth/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/groups/dto"
	"soli/formations/src/groups/models"
	"soli/formations/src/utils"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// casdoorUserCacheTTL is the duration cached user entries remain valid.
const casdoorUserCacheTTL = 30 * time.Second

// userCache caches Casdoor users to prevent redundant HTTP calls during
// group member DTO conversion. The entity management framework calls
// ModelToDto per item, so without a cache every list request triggers
// N sequential HTTP calls (one per member). On the first cache miss the
// cache bulk-loads ALL users with a single GetUsers() call, turning the
// N+1 problem into a single HTTP round-trip.
var userCache = &casdoorUserCache{
	fetchAllUsers: casdoorsdk.GetUsers,
	fetchUserByID: casdoorsdk.GetUserByUserId,
}

type casdoorUserCache struct {
	mu        sync.Mutex
	users     map[string]*casdoorsdk.User // keyed by Casdoor user ID (e.g. "abc123/username")
	fetchedAt time.Time
	// Injected fetchers for testability — default to Casdoor SDK functions
	fetchAllUsers func() ([]*casdoorsdk.User, error)
	fetchUserByID func(string) (*casdoorsdk.User, error)
}

// get returns a Casdoor user from cache, bulk-loading all users on the
// first miss or when the cache has expired.
func (c *casdoorUserCache) get(userID string) (*casdoorsdk.User, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return from cache if still fresh
	if c.users != nil && time.Since(c.fetchedAt) < casdoorUserCacheTTL {
		return c.users[userID], nil
	}

	// Bulk-load all users with a single HTTP call
	allUsers, err := c.fetchAllUsers()
	if err != nil {
		// Fallback: fetch just this one user
		user, err := c.fetchUserByID(userID)
		return user, err
	}

	c.users = make(map[string]*casdoorsdk.User, len(allUsers))
	for _, u := range allUsers {
		c.users[u.Id] = u
	}
	c.fetchedAt = time.Now()

	return c.users[userID], nil
}

func enrichGroupMemberWithUser(output *dto.GroupMemberOutput) *dto.GroupMemberOutput {
	if output.UserID == "" {
		return output
	}

	user, err := userCache.get(output.UserID)
	if err != nil {
		utils.Debug("Failed to fetch user %s from Casdoor: %v", output.UserID, err)
		return output
	}

	if user == nil {
		utils.Debug("User %s not found in Casdoor", output.UserID)
		return output
	}

	output.User = &dto.UserSummary{
		ID:          user.Id,
		Name:        user.Name,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Username:    user.Name,
	}

	if output.User.DisplayName == "" {
		output.User.DisplayName = user.Name
	}

	return output
}

func RegisterGroupMember(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.GroupMember, dto.CreateGroupMemberInput, dto.CreateGroupMemberInput, dto.GroupMemberOutput](
		service,
		"GroupMember",
		entityManagementInterfaces.TypedEntityRegistration[models.GroupMember, dto.CreateGroupMemberInput, dto.CreateGroupMemberInput, dto.GroupMemberOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.GroupMember, dto.CreateGroupMemberInput, dto.CreateGroupMemberInput, dto.GroupMemberOutput]{
				ModelToDto: func(model *models.GroupMember) (dto.GroupMemberOutput, error) {
					output := dto.GroupMemberModelToGroupMemberOutput(model)
					output = enrichGroupMemberWithUser(output)
					return *output, nil
				},
				DtoToModel: func(input dto.CreateGroupMemberInput) *models.GroupMember {
					role := input.Role
					if role == "" {
						role = models.GroupMemberRoleMember
					}
					return &models.GroupMember{
						GroupID:   input.GroupID,
						UserID:    input.UserID,
						Role:      role,
						InvitedBy: input.InvitedBy,
						JoinedAt:  time.Now(),
						IsActive:  true,
					}
				},
				DtoToMap: func(input dto.CreateGroupMemberInput) map[string]any {
					return make(map[string]any)
				},
			},
			Roles: entityManagementInterfaces.EntityRoles{
				Roles: map[string]string{
					string(authModels.Member): "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")",
					string(authModels.Admin):  "(" + http.MethodGet + "|" + http.MethodPost + "|" + http.MethodDelete + ")",
				},
			},
			MembershipConfig: &entityManagementInterfaces.MembershipConfig{
				MemberTable:      "group_members",
				EntityIDColumn:   "group_id",
				UserIDColumn:     "user_id",
				RoleColumn:       "role",
				IsActiveColumn:   "is_active",
				OrgAccessEnabled: false,
			},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "group-members",
				EntityName: "GroupMember",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Récupérer tous les membres de groupe",
					Description: "Retourne la liste de tous les membres de groupes",
					Tags:        []string{"group-members"},
					Security:    true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Récupérer un membre de groupe",
					Description: "Retourne les détails d'un membre de groupe spécifique",
					Tags:        []string{"group-members"},
					Security:    true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Ajouter un membre à un groupe",
					Description: "Ajoute un utilisateur à un groupe avec un rôle spécifique",
					Tags:        []string{"group-members"},
					Security:    true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary:     "Retirer un membre d'un groupe",
					Description: "Retire un membre d'un groupe",
					Tags:        []string{"group-members"},
					Security:    true,
				},
			},
		},
	)
}
