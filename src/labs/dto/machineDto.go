package dto

import (
	"reflect"
	emi "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/labs/models"
)

type MachineEntity struct {
}

type MachineInput struct {
	Name string `binding:"required"`
}

type MachineOutput struct {
	Name string
}

func (s MachineEntity) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return machinePtrModelToMachineOutputDto(input.(*models.Machine))
	} else {
		return machineValueModelToMachineOutputDto(input.(models.Machine))
	}
}

func machinePtrModelToMachineOutputDto(machineModel *models.Machine) *MachineOutput {

	return &MachineOutput{
		Name: machineModel.Name,
	}
}

func machineValueModelToMachineOutputDto(machineModel models.Machine) *MachineOutput {

	return &MachineOutput{
		Name: machineModel.Name,
	}
}

func (s MachineEntity) EntityInputDtoToEntityModel(input any) any {

	machineInputDto := input.(MachineInput)
	return &models.Machine{
		Name: machineInputDto.Name,
	}
}

func (s MachineEntity) GetEntityRegistrationInput() emi.EntityRegistrationInput {
	return emi.EntityRegistrationInput{
		EntityInterface: models.Machine{},
		EntityConverters: emi.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: emi.EntityDtos{
			InputDto:  MachineInput{},
			OutputDto: MachineOutput{},
		},
	}
}
