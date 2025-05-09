package dto

import "soli/formations/src/courses/models"

type PackageInput struct {
	OwnerID                  string
	Format                   *int
	ThemeGitRepository       string
	ThemeGitRepositoryBranch string
	ThemeId                  string
	ScheduleId               string
	CourseId                 string
}

type PackageOutput struct {
	ID                       string   `json:"id"`
	OwnerIDs                 []string `gorm:"serializer:json"`
	Format                   *int
	ThemeGitRepository       string
	ThemeGitRepositoryBranch string
	ThemeId                  string
	ScheduleId               string
	CourseId                 string
}

func PackageModelToPackageOutput(packageModel models.Package) *PackageOutput {

	return &PackageOutput{
		ID:                       packageModel.ID.String(),
		OwnerIDs:                 packageModel.OwnerIDs,
		ThemeGitRepository:       packageModel.ThemeGitRepository,
		ThemeGitRepositoryBranch: packageModel.ThemeGitRepositoryBranch,
		ThemeId:                  packageModel.ThemeId,
		ScheduleId:               packageModel.ScheduleID.String(),
		CourseId:                 packageModel.CourseID.String(),
	}
}

func PackageModelToPackageInput(packageModel models.Package) *PackageInput {

	return &PackageInput{
		OwnerID:                  packageModel.OwnerIDs[0],
		ThemeGitRepository:       packageModel.ThemeGitRepository,
		ThemeGitRepositoryBranch: packageModel.ThemeGitRepositoryBranch,
		ThemeId:                  packageModel.ThemeId,
		ScheduleId:               packageModel.ScheduleID.String(),
		CourseId:                 packageModel.CourseID.String(),
	}
}
