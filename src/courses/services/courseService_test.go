package services

import (
	"reflect"
	authController "soli/formations/src/auth"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	repositories "soli/formations/src/courses/repositories"
	sqldb "soli/formations/src/db"
	testtools "soli/formations/src/testTools"
	"testing"
)

func Test_courseService_CreateCourse(t *testing.T) {
	type fields struct {
		repository repositories.CourseRepository
	}
	type args struct {
		courseCreateDTO dto.CreateCourseInput
	}

	authController.InitCasdoorConnection()
	sqldb.InitDBConnection()
	sqldb.AutoMigrate()
	testtools.DeleteAllObjects()
	testtools.SetupUsers()

	courseInput := dto.CreateCourseInput{
		Name:               "cours pour le test",
		Theme:              "sdv",
		Format:             int(models.HTML),
		AuthorEmail:        "1.supervisor@test.com",
		Category:           "prog",
		Version:            "1.0",
		Title:              "Cours de Test",
		Subtitle:           "Sous titre du cours de test",
		Header:             "Entête du super cours de test",
		Footer:             "Pied de page du super cours de test",
		Logo:               "",
		Description:        "Description du cours",
		Schedule:           "",
		Prelude:            "",
		LearningObjectives: "",
		Chapters:           []models.Chapter{},
	}

	courseOutput := dto.CreateCourseOutput{}

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *dto.CreateCourseOutput
		wantErr bool
	}{

		struct {
			name    string
			fields  fields
			args    args
			want    *dto.CreateCourseOutput
			wantErr bool
		}{
			name: "test 1",
			fields: fields{
				repositories.NewCourseRepository(sqldb.DB),
			},
			args: args{
				courseCreateDTO: courseInput,
			},
			want:    &courseOutput,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := courseService{
				repository: tt.fields.repository,
			}
			got, err := c.CreateCourse(tt.args.courseCreateDTO)
			if (err != nil) != tt.wantErr {
				t.Errorf("courseService.CreateCourse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("courseService.CreateCourse() = %v, want %v", got, tt.want)
			}
		})
	}
}
