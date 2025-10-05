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

func (s ConnectionRegistration) EntityModelToEntityOutput(input any) (any, error) {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return connectionPtrModelToConnectionOutputDto(input.(*models.Connection))
	} else {
		return connectionValueModelToConnectionOutputDto(input.(models.Connection))
	}
}

func connectionPtrModelToConnectionOutputDto(connectionModel *models.Connection) (*dto.ConnectionOutput, error) {
	repo := entityManagementRepository.NewGenericRepository(sqldb.DB)
	machine, errorGettingMachine := repo.GetEntity(connectionModel.MachineID, models.Machine{}, "Machine", nil)
	if errorGettingMachine != nil {
		return nil, errorGettingMachine
	}
	username, errorGettingUsername := repo.GetEntity(connectionModel.UsernameID, models.Username{}, "Username", nil)
	if errorGettingUsername != nil {
		return nil, errorGettingUsername
	}
	machineOutputDto, errMachineConvertingToDto := machinePtrModelToMachineOutputDto(machine.(*models.Machine))
	if errMachineConvertingToDto != nil {
		return nil, errMachineConvertingToDto
	}

	usernameOutputDto, errUsernameConvertingToDto := usernamePtrModelToUsernameOutputDto(username.(*models.Username))
	if errUsernameConvertingToDto != nil {
		return nil, errUsernameConvertingToDto
	}

	return &dto.ConnectionOutput{
		MachineDtoOutput:  machineOutputDto,
		UsernameDtoOutput: usernameOutputDto,
		ID:                connectionModel.ID.String(),
	}, nil
}

func connectionValueModelToConnectionOutputDto(connectionModel models.Connection) (*dto.ConnectionOutput, error) {

	repo := entityManagementRepository.NewGenericRepository(sqldb.DB)
	machine, errorGettingMachine := repo.GetEntity(connectionModel.MachineID, models.Machine{}, "Machine", nil)
	if errorGettingMachine != nil {
		return nil, errorGettingMachine
	}
	username, errorGettingUsername := repo.GetEntity(connectionModel.UsernameID, models.Username{}, "Username", nil)
	if errorGettingUsername != nil {
		return nil, errorGettingUsername
	}

	machineOutputDto, errMachineConvertingToDto := machinePtrModelToMachineOutputDto(machine.(*models.Machine))
	if errMachineConvertingToDto != nil {
		return nil, errMachineConvertingToDto
	}

	usernameOutputDto, errUsernameConvertingToDto := usernamePtrModelToUsernameOutputDto(username.(*models.Username))
	if errUsernameConvertingToDto != nil {
		return nil, errUsernameConvertingToDto
	}

	return &dto.ConnectionOutput{
		MachineDtoOutput:  machineOutputDto,
		UsernameDtoOutput: usernameOutputDto,
		ID:                connectionModel.ID.String(),
	}, nil
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
			DtoToMap:   s.EntityDtoToMap,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.ConnectionInput{},
			OutputDto:      dto.ConnectionOutput{},
		},
	}
}
