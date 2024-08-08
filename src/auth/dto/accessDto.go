package dto

type CreateEntityAccessInput struct {
	GroupName         string `binding:"required"`
	Route             string `binding:"required"`
	AuthorizedMethods string `binding:"required"`
}

type DeleteEntityAccessInput struct {
	GroupName string `binding:"required"`
	Route     string `binding:"required"`
}
