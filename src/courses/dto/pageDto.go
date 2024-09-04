package dto

type PageInput struct {
	Content []string `json:"content" gorm:"serializer:json"`
}

type PageOutput struct {
	ID                 string   `json:"id"`
	Number             int      `json:"number"`
	ParentSectionTitle string   `json:"parentSectionTitle"`
	Toc                []string `json:"toc"`
	Content            []string `json:"content"`
	Hide               bool     `json:"hide"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
}

type EditPageInput struct {
	Number             int      `json:"number"`
	ParentSectionTitle string   `json:"parentSectionTitle"`
	Toc                []string `json:"toc" gorm:"serializer:json"`
	Content            []string `json:"content" gorm:"serializer:json"`
	Hide               bool     `json:"hide"`
}
