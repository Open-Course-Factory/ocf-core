package courseController

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"soli/formations/src/courses/dto"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// Create Course from Git godoc
//
//	@Summary		Création cours à partir d'un dépôt git
//	@Description	Ajoute un nouveau cours dans la base de données
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			gitRepository	body	dto.CreateCourseFromGitInput	true	"cours"
//
//	@Security		Bearer
//
//	@Success		201	{object}	dto.CreateCourseFromGitOutput
//
//	@Failure		400	{object}	errors.APIError	"Impossible de parser le json"
//	@Failure		400	{object}	errors.APIError	"Impossible de créer un cours"
//	@Failure		400	{object}	errors.APIError	"Repository URL must be https://host/path or git@host:path"
//	@Failure		409	{object}	errors.APIError	"Cours déjà présent pour cet utilisateur"
//	@Router			/courses/git [post]
func (c courseController) CreateCourseFromGit(ctx *gin.Context) {
	createCourseFromGitDTO := dto.CreateCourseFromGitInput{}

	bindError := ctx.BindJSON(&createCourseFromGitDTO)
	if bindError != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Impossible de parser le json"+bindError.Error())
		return
	}

	if !isAllowedRepoURL(createCourseFromGitDTO.Url) {
		errors.Respond(ctx, http.StatusBadRequest, repoURLRefused)
		return
	}

	userId := ctx.GetString("userId")

	_, errGetCourse := c.service.GetGitCourse(userId, createCourseFromGitDTO.Name, createCourseFromGitDTO.Url, createCourseFromGitDTO.BranchName, "course.json")

	if errGetCourse != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Impossible de récupérer le cours : "+errGetCourse.Error())
		return
	}

	//ToDo: generate outputDto

	ctx.JSON(http.StatusCreated, nil)
}

const repoURLRefused = "Repository URL must be https://host/path or git@host:path"

// git@<host>:<path>, host without '/', ':' or '@' and not starting with '-' (no ssh option injection).
var sshRepoURL = regexp.MustCompile(`^git@([^-/:@\s][^/:@\s]*):\S+$`)

// isAllowedRepoURL accepts only the remote forms the HTTP import offers; the CLI calls the
// loader directly and keeps its local and other forms.
// shortcut: literal internal addresses are refused, but a public name resolving to an internal address is not (DNS-level SSRF out of scope).
func isAllowedRepoURL(raw string) bool {
	if m := sshRepoURL.FindStringSubmatch(raw); m != nil {
		return isPublicHost(m[1])
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" && isPublicHost(u.Hostname())
}

// isPublicHost refuses an empty host, localhost and loopback, private, link-local or unspecified IP literals.
func isPublicHost(host string) bool {
	if host == "" || strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified())
}
