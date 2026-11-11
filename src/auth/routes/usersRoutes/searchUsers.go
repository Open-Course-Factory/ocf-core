package userController

import (
	"net/http"
	"strings"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Search Users godoc
//
//	@Summary		Rechercher des utilisateurs
//	@Description	Administrateurs : recherche par nom ou email. Membres : email exact uniquement.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			q	query		string	true	"Search query (minimum 2 characters)"
//	@Security		Bearer
//	@Success		200	{array}		dto.UserOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/users/search [get]
func (u UserController) SearchUsers(ctx *gin.Context) {
	query := ctx.Query("q")
	if query == "" {
		errors.Respond(ctx, http.StatusBadRequest, "Search query is required")
		return
	}

	if len(query) < 2 {
		errors.Respond(ctx, http.StatusBadRequest, "Search query must be at least 2 characters")
		return
	}

	// A substring search over every account is a user directory: members only
	// find someone whose full email they already know (managers adding a member,
	// terminal sharing). Administrators keep the substring search.
	if !access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		u.respondExactEmailMatch(ctx, strings.TrimSpace(query))
		return
	}

	users, userError := u.service.SearchUsers(query)
	if userError != nil {
		errors.Respond(ctx, http.StatusInternalServerError, userError.Error())
		return
	}

	ctx.JSON(http.StatusOK, users)
}

func (u UserController) respondExactEmailMatch(ctx *gin.Context, email string) {
	// Casdoor's get-user falls back to other lookups on an empty email, and
	// accounts without an email would match "": never ask it for one.
	if !strings.Contains(email, "@") {
		ctx.JSON(http.StatusOK, []dto.UserOutput{})
		return
	}
	user, err := u.service.FindUserByEmail(email)
	if err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if user == nil {
		ctx.JSON(http.StatusOK, []dto.UserOutput{})
		return
	}
	ctx.JSON(http.StatusOK, []dto.UserOutput{*user})
}
