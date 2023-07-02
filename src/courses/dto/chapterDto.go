package dto

import "soli/formations/src/courses/models"

type ChapterInput struct {
	Title        string           `json:"title"`
	Number       int              `json:"number"`
	Footer       string           `json:"footer"`
	Introduction string           `json:"introduction"`
	Sections     []models.Section `json:"sections"`
}

type ChapterOutput struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Number       int              `json:"number"`
	Footer       string           `json:"footer"`
	Introduction string           `json:"introduction"`
	Sections     []models.Section `json:"sections"`
}
