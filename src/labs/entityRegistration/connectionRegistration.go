package registration

import (
	"reflect"
	sqldb "soli/formations/src/db"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementRepository "soli/formations/src/entityManagement/repositories"
	"soli/formations/src/labs/dto"
	"soli/formations/src/labs/models"

	"github.com/google/uuid"
)

type ConnectionRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s ConnectionRegistration) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return connectionPtrModelToConnectionOutputDto(input.(*models.Connection))
	} else {
		return connectionValueModelToConnectionOutputDto(input.(models.Connection))
	}
}

func connectionPtrModelToConnectionOutputDto(connectionModel *models.Connection) *dto.ConnectionOutput {
	repo := entityManagementRepository.NewGenericRepository(sqldb.DB)
	machine, _ := repo.GetEntity(connectionModel.MachineID, models.Machine{})
	username, _ := repo.GetEntity(connectionModel.UsernameID, models.Username{})

	return &dto.ConnectionOutput{
		Machine:  machine.(*models.Machine),
		Username: username.(*models.Username),
	}
}

func connectionValueModelToConnectionOutputDto(connectionModel models.Connection) *dto.ConnectionOutput {

	repo := entityManagementRepository.NewGenericRepository(sqldb.DB)
	machine, _ := repo.GetEntity(connectionModel.MachineID, models.Machine{})
	username, _ := repo.GetEntity(connectionModel.UsernameID, models.Username{})
	return &dto.ConnectionOutput{
		Machine:  machine.(*models.Machine),
		Username: username.(*models.Username),
	}
}

func (s ConnectionRegistration) EntityInputDtoToEntityModel(input any) any {

	connectionInputDto := input.(dto.ConnectionInput)
	usernameUuid := uuid.MustParse(connectionInputDto.UsernameID)
	machineUuid := uuid.MustParse(connectionInputDto.MachineID)
	return &models.Connection{
		UsernameID: usernameUuid,
		MachineID:  machineUuid,
	}
}

func (s ConnectionRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Connection{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputDto:  dto.ConnectionInput{},
			OutputDto: dto.ConnectionOutput{},
		},
	}
}
