package dto

type PageInput struct {
	Number             int      `json:"number"`
	ParentSectionTitle string   `json:"parentSectionTitle"`
	Toc                []string `json:"toc"`
	Content            []string `json:"content"`
	Hide               bool     `json:"hide"`
}

type PageOutput struct {
	ID                 uint     `json:"id"`
	Number             int      `json:"number"`
	ParentSectionTitle string   `json:"parentSectionTitle"`
	Toc                []string `json:"toc"`
	Content            []string `json:"content"`
	Hide               bool     `json:"hide"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
}
