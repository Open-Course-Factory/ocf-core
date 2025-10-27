package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	"soli/formations/src/entityManagement/converters"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

type CourseRegistration struct {
	entityManagementInterfaces.AbstractRegistrableInterface
}

func (s CourseRegistration) GetSwaggerConfig() entityManagementInterfaces.EntitySwaggerConfig {
	return entityManagementInterfaces.EntitySwaggerConfig{
		Tag:        "courses",
		EntityName: "Course",
		GetAll: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer tous les cours",
			Description: "Retourne la liste de tous les cours disponibles",
			Tags:        []string{"courses"},
			Security:    true,
		},
		GetOne: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Récupérer un cours",
			Description: "Retourne les détails complets d'un cours spécifique",
			Tags:        []string{"courses"},
			Security:    true,
		},
		Create: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Créer un cours",
			Description: "Crée un nouveau cours",
			Tags:        []string{"courses"},
			Security:    true,
		},
		Update: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Mettre à jour un cours",
			Description: "Modifie un cours existant",
			Tags:        []string{"courses"},
			Security:    true,
		},
		Delete: &entityManagementInterfaces.SwaggerOperation{
			Summary:     "Supprimer un cours",
			Description: "Supprime un cours",
			Tags:        []string{"courses"},
			Security:    true,
		},
	}
}

func (s CourseRegistration) EntityModelToEntityOutput(input any) (any, error) {
	return converters.GenericModelToOutput(input, func(ptr any) (any, error) {
		return dto.CourseModelToCourseOutputDto(*ptr.(*models.Course)), nil
	})
}

func (s CourseRegistration) EntityInputDtoToEntityModel(input any) any {

	var chapters []*models.Chapter

	courseInputDto, ok := input.(dto.CourseInput)
	if !ok {
		ptrCourseInputDto := input.(*dto.CourseInput)
		courseInputDto = *ptrCourseInputDto
	}

	for _, chapterInput := range courseInputDto.ChaptersInput {
		chapterModel := ChapterRegistration{}.EntityInputDtoToEntityModel(chapterInput)
		chapter := chapterModel.(*models.Chapter)
		chapters = append(chapters, chapter)
	}

	courseToReturn := &models.Course{
		Name:                courseInputDto.Name,
		Category:            courseInputDto.Category,
		Version:             courseInputDto.Version,
		Title:               courseInputDto.Title,
		Subtitle:            courseInputDto.Subtitle,
		Header:              courseInputDto.Header,
		Footer:              courseInputDto.Footer,
		Logo:                courseInputDto.Logo,
		Description:         courseInputDto.Description,
		Prelude:             courseInputDto.Prelude,
		LearningObjectives:  courseInputDto.LearningObjectives,
		Chapters:            chapters,
		GitRepository:       courseInputDto.GitRepository,
		GitRepositoryBranch: courseInputDto.GitRepositoryBranch,
	}

	courseToReturn.OwnerIDs = append(courseToReturn.OwnerIDs, courseInputDto.OwnerID)

	return courseToReturn
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
		EntitySubEntities: []interface{}{
			models.Chapter{},
		},
	}
}
