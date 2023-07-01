package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type GroupOutput struct {
	ID           uuid.UUID `json:"id"`
	GroupName    string    `json:"groupName"`
	ParentGroup  uuid.UUID `json:"parentGroupId"`
	Organisation uuid.UUID `json:"organisation"`
}

type CreateGroupInput struct {
	GroupName   string `binding:"required"`
	ParentGroup uuid.UUID
}

type GroupEditInput struct {
	GroupName   string    `json:"groupName"`
	ParentGroup uuid.UUID `json:"parentGroupId"`
}

type GroupEditOutput struct {
	GroupName   string    `json:"groupName"`
	ParentGroup uuid.UUID `json:"parentGroupId"`
}

func GroupModelToGroupOutput(groupModel models.Group) *GroupOutput {
	return &GroupOutput{
		ID:           groupModel.ID,
		GroupName:    groupModel.GroupName,
		ParentGroup:  groupModel.ParentGroup.ID,
		Organisation: groupModel.Organisation.ID,
	}
}
