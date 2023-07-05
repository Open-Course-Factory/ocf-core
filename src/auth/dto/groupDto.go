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
	GroupName    string `binding:"required"`
	ParentGroup  uuid.UUID
	Organisation uuid.UUID
}

type GroupEditInput struct {
	GroupName    string    `json:"groupName"`
	ParentGroup  uuid.UUID `json:"parentGroupId"`
	Organisation uuid.UUID `json:"organisation"`
}

type GroupEditOutput struct {
	GroupName    string    `json:"groupName"`
	ParentGroup  uuid.UUID `json:"parentGroupId"`
	Organisation uuid.UUID `json:"organisation"`
}

func GroupModelToGroupOutput(groupModel models.Group) *GroupOutput {

	groupOutput := GroupOutput{
		ID:        groupModel.ID,
		GroupName: groupModel.GroupName,
	}

	if groupModel.ParentGroupID != nil {
		groupOutput.ParentGroup = *groupModel.ParentGroupID
	}

	if groupModel.OrganisationID != nil {
		groupOutput.Organisation = *groupModel.OrganisationID
	}

	return &groupOutput
}
