package registration

import (
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

func chapterInputToModel(input *dto.ChapterInput) *models.Chapter {
	var sectionModels []*models.Section
	for _, sectionInput := range input.Sections {
		section := sectionInputToModel(sectionInput)
		sectionModels = append(sectionModels, section)
	}
	chapter := &models.Chapter{
		Footer:       input.Footer,
		Introduction: input.Introduction,
		Title:        input.Title,
		Number:       input.Number,
		Sections:     sectionModels,
	}
	chapter.OwnerIDs = append(chapter.OwnerIDs, input.OwnerID)
	return chapter
}

func sectionInputToModel(input *dto.SectionInput) *models.Section {
	var pageModels []*models.Page
	for _, pageInput := range input.Pages {
		page := pageInputToModel(pageInput)
		pageModels = append(pageModels, page)
	}
	section := &models.Section{
		FileName:    input.FileName,
		Title:       input.Title,
		Intro:       input.Intro,
		Conclusion:  input.Conclusion,
		Number:      input.Number,
		Pages:       pageModels,
		HiddenPages: input.HiddenPages,
	}
	section.OwnerIDs = append(section.OwnerIDs, input.OwnerID)
	return section
}

func pageInputToModel(input *dto.PageInput) *models.Page {
	page := &models.Page{
		Order:   input.Order,
		Content: input.Content,
	}
	page.OwnerIDs = append(page.OwnerIDs, input.OwnerID)
	return page
}

func RegisterCourse(service *ems.EntityRegistrationService) {
	ems.RegisterTypedEntity[models.Course, dto.CourseInput, dto.EditCourseInput, dto.CourseOutput](
		service,
		"Course",
		entityManagementInterfaces.TypedEntityRegistration[models.Course, dto.CourseInput, dto.EditCourseInput, dto.CourseOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[models.Course, dto.CourseInput, dto.EditCourseInput, dto.CourseOutput]{
				ModelToDto: func(model *models.Course) (dto.CourseOutput, error) {
					return *dto.CourseModelToCourseOutputDto(*model), nil
				},
				DtoToModel: func(input dto.CourseInput) *models.Course {
					var chapters []*models.Chapter
					for _, chapterInput := range input.ChaptersInput {
						chapters = append(chapters, chapterInputToModel(chapterInput))
					}
					course := &models.Course{
						Name:                input.Name,
						Category:            input.Category,
						Version:             input.Version,
						Title:               input.Title,
						Subtitle:            input.Subtitle,
						Header:              input.Header,
						Footer:              input.Footer,
						Logo:                input.Logo,
						Description:         input.Description,
						Prelude:             input.Prelude,
						LearningObjectives:  input.LearningObjectives,
						Chapters:            chapters,
						GitRepository:       input.GitRepository,
						GitRepositoryBranch: input.GitRepositoryBranch,
					}
					course.OwnerIDs = append(course.OwnerIDs, input.OwnerID)
					return course
				},
			},
			SubEntities: []any{models.Chapter{}},
			SwaggerConfig: &entityManagementInterfaces.EntitySwaggerConfig{
				Tag:        "courses",
				EntityName: "Course",
				GetAll: &entityManagementInterfaces.SwaggerOperation{
					Summary: "Récupérer tous les cours", Description: "Retourne la liste de tous les cours disponibles",
					Tags: []string{"courses"}, Security: true,
				},
				GetOne: &entityManagementInterfaces.SwaggerOperation{
					Summary: "Récupérer un cours", Description: "Retourne les détails complets d'un cours spécifique",
					Tags: []string{"courses"}, Security: true,
				},
				Create: &entityManagementInterfaces.SwaggerOperation{
					Summary: "Créer un cours", Description: "Crée un nouveau cours",
					Tags: []string{"courses"}, Security: true,
				},
				Update: &entityManagementInterfaces.SwaggerOperation{
					Summary: "Mettre à jour un cours", Description: "Modifie un cours existant",
					Tags: []string{"courses"}, Security: true,
				},
				Delete: &entityManagementInterfaces.SwaggerOperation{
					Summary: "Supprimer un cours", Description: "Supprime un cours",
					Tags: []string{"courses"}, Security: true,
				},
			},
		},
	)
}
