package entityManagementInterfaces

type EntityRegistrationInput struct {
	EntityInterface  interface{}
	EntityConverters EntityConverters
	EntityDtos       EntityDtos
}

type EntityConverters struct {
	ModelToDto interface{}
	DtoToModel interface{}
}

type EntityDtos struct {
	InputDto  interface{}
	OutputDto interface{}
}

type RegistrableInterface interface {
	GetEntityRegistrationInput() EntityRegistrationInput
	EntityModelToEntityOutput(input any) any
	EntityInputDtoToEntityModel(input any) any
}
