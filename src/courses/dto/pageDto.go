package dto

import "soli/formations/src/courses/models"

type PageInput struct {
	OwnerID string
	Order   int      `json:"order"`
	Content []string `json:"content" gorm:"serializer:json"`
}

// ParentSectionOutput contains minimal section information for page's parent section
type ParentSectionOutput struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Number int    `json:"number"`
	Intro  string `json:"intro"`
}

type PageOutput struct {
	ID                 string                `json:"id"`
	Order              int                   `json:"order"`
	ParentSectionTitle string                `json:"parentSectionTitle"`
	Sections           []ParentSectionOutput `json:"sections,omitempty"` // Parent section information
	Toc                []string              `json:"toc"`
	Content            []string              `json:"content"`
	Hide               bool                  `json:"hide"`
	CreatedAt          string                `json:"createdAt"`
	UpdatedAt          string                `json:"updatedAt"`
}

type EditPageInput struct {
	Order              *int     `json:"order,omitempty" mapstructure:"order"`
	ParentSectionTitle *string  `json:"parentSectionTitle,omitempty" mapstructure:"parentSectionTitle"`
	Toc                []string `json:"toc,omitempty" mapstructure:"toc"`
	Content            []string `json:"content,omitempty" mapstructure:"content"`
	Hide               *bool    `json:"hide,omitempty" mapstructure:"hide"`
}

func PageModelToPageInput(pageModel models.Page) *PageInput {
	return &PageInput{
		OwnerID: pageModel.OwnerIDs[0],
		Order:   pageModel.Order,
		Content: pageModel.Content,
	}
}
