package registration

import (
	"reflect"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type CourseRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s CourseRegistration) EntityModelToEntityOutput(input any) any {
	if reflect.ValueOf(input).Kind() == reflect.Ptr {
		return coursePtrModelToCourseOutputDto(input.(*models.Course))
	} else {
		return courseValueModelToCourseOutputDto(input.(models.Course))
	}
}

func coursePtrModelToCourseOutputDto(courseModel *models.Course) *dto.CourseOutput {

	return &dto.CourseOutput{
		Name: courseModel.Name,
	}
}

func courseValueModelToCourseOutputDto(courseModel models.Course) *dto.CourseOutput {

	return &dto.CourseOutput{
		Name: courseModel.Name,
	}
}

func (s CourseRegistration) EntityInputDtoToEntityModel(input any) any {

	courseInputDto := input.(dto.CourseInput)
	return &models.Course{
		Name:               courseInputDto.Name,
		Theme:              courseInputDto.Theme,
		Format:             config.Format(*courseInputDto.Format),
		Category:           courseInputDto.Category,
		Version:            courseInputDto.Version,
		Title:              courseInputDto.Title,
		Subtitle:           courseInputDto.Subtitle,
		Header:             courseInputDto.Header,
		Footer:             courseInputDto.Footer,
		Logo:               courseInputDto.Logo,
		Description:        courseInputDto.Description,
		Schedule:           courseInputDto.Schedule,
		Prelude:            courseInputDto.Prelude,
		LearningObjectives: courseInputDto.LearningObjectives,
		Chapters:           courseInputDto.Chapters,
	}
}

func (s CourseRegistration) GetEntityRegistrationInput() entityManagementInterfaces.EntityRegistrationInput {
	return entityManagementInterfaces.EntityRegistrationInput{
		EntityInterface: models.Course{},
		EntityConverters: entityManagementInterfaces.EntityConverters{
			ModelToDto: s.EntityModelToEntityOutput,
			DtoToModel: s.EntityInputDtoToEntityModel,
		},
		EntityDtos: entityManagementInterfaces.EntityDtos{
			InputCreateDto: dto.CourseInput{},
			OutputDto:      dto.CourseOutput{},
			InputEditDto:   dto.EditCourseInput{},
		},
	}
}
