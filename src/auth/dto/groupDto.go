package dto

import (
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
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

type AddUserInGroupInput struct {
	UserId    string `binding:"required"`
	GroupName string `json:"name"`
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
