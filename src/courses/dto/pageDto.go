package dto

import "soli/formations/src/courses/models"

type PageInput struct {
	OwnerID string
	Order   int      `json:"order"`
	Content []string `json:"content" gorm:"serializer:json"`
}

type PageOutput struct {
	ID                 string   `json:"id"`
	Order              int      `json:"order"`
	ParentSectionTitle string   `json:"parentSectionTitle"`
	Toc                []string `json:"toc"`
	Content            []string `json:"content"`
	Hide               bool     `json:"hide"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
}

type EditPageInput struct {
	Order              int      `json:"order"`
	ParentSectionTitle string   `json:"parentSectionTitle"`
	Toc                []string `json:"toc" gorm:"serializer:json"`
	Content            []string `json:"content" gorm:"serializer:json"`
	Hide               bool     `json:"hide"`
}

func PageModelToPageInput(pageModel models.Page) *PageInput {
	return &PageInput{
		OwnerID: pageModel.OwnerIDs[0],
		Order:   pageModel.Order,
		Content: pageModel.Content,
	}
}
