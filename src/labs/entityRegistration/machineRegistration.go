package registration

import (
	"reflect"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/labs/dto"
	"soli/formations/src/labs/models"
)

type MachineRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s MachineRegistration) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return machinePtrModelToMachineOutputDto(input.(*models.Machine))
	} else {
		return machineValueModelToMachineOutputDto(input.(models.Machine))
	}
}

func machinePtrModelToMachineOutputDto(machineModel *models.Machine) *dto.MachineOutput {

	return &dto.MachineOutput{
		Name: machineModel.Name,
		ID:   machineModel.ID.String(),
		IP:   machineModel.IP,
		Port: machineModel.Port,
	}
}

func machineValueModelToMachineOutputDto(machineModel models.Machine) *dto.MachineOutput {

	return &dto.MachineOutput{
		Name: machineModel.Name,
		ID:   machineModel.ID.String(),
		IP:   machineModel.IP,
		Port: machineModel.Port,
	}
}

func (s MachineRegistration) EntityInputDtoToEntityModel(input any) any {

	machineInputDto := input.(dto.MachineInput)
	return &models.Machine{
		Name: machineInputDto.Name,
		IP:   machineInputDto.IP,
		Port: machineInputDto.Port,
	}
}

func (s MachineRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Machine{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputDto:  dto.MachineInput{},
			OutputDto: dto.MachineOutput{},
		},
	}
}
