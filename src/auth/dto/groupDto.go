package dto

import (
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
)

type Action int

const (
	ADD Action = iota
	REMOVE
)

type CreateGroupInput struct {
	Organization string `binding:"required"`
	Name         string `binding:"required"`
	DisplayName  string `binding:"required"`
	ParentGroup  string
}

type CreateGroupOutput struct {
	GroupName string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// ptr mandatory https://github.com/go-openapi/validate/issues/105
type ModifyUsersInGroupInput struct {
	UserIds []string `binding:"required"`
	Action  *Action  `binding:"required,gte=0,lte=1"`
}

type DeleteGroupInput struct {
	Id uuid.UUID `binding:"required"`
}

func GroupModelToGroupOutput(groupModel *casdoorsdk.Group) *CreateGroupOutput {
	return &CreateGroupOutput{
		GroupName: groupModel.Name,
		CreatedAt: groupModel.CreatedTime,
	}
}
