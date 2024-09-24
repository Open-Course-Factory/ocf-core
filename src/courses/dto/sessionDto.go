package dto

import (
	"time"
)

type SessionEntity struct {
}

type CreateSessionOutput struct {
	ID        string    `json:"id"`
	CourseId  string    `json:"course"`
	GroupId   string    `json:"group"`
	Title     string    `json:"title"`
	StartTime time.Time `json:"start"`
	EndTime   time.Time `json:"end"`
}

type CreateSessionInput struct {
	CourseId  string    `binding:"required"`
	GroupId   string    `binding:"required"`
	Title     string    `binding:"required"`
	StartTime time.Time `binding:"required"`
	EndTime   time.Time `binding:"required"`
}
