package dto

type ThemeInput struct {
	Name             string `json:"name"`
	Repository       string
	RepositoryBranch string
	Size             string
}

type ThemeOutput struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Repository       string `json:"repository"`
	RepositoryBranch string `json:"repositoryBranch"`
	Size             string `json:"size"`
	CreatedAt        string `json:"createdAt"`
	UpdatedAt        string `json:"updatedAt"`
}

type EditThemeInput struct {
	Name             string `json:"name"`
	Repository       string `json:"repository"`
	RepositoryBranch string `json:"repositoryBranch"`
	Size             string `json:"size"`
}
