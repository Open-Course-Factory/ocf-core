// src/courses/services/courseService.go
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"reflect"
	"time"

	authInterfaces "soli/formations/src/auth/interfaces"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	repositories "soli/formations/src/courses/repositories"
	sqldb "soli/formations/src/db"
	genericService "soli/formations/src/entityManagement/services"
	workerServices "soli/formations/src/worker/services"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CourseService interface {
	// Méthodes existantes
	GetGitCourse(ownerId string, courseName string, courseURL string, courseBranch string) (*models.Course, error)
	GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error)
	GetCourseFromProgramInputs(courseName *string, courseGitRepository *string, courseGitRepositoryBranchName *string) models.Course

	// Nouvelles méthodes pour le worker
	GenerateCourseAsync(generateCourseInputDto dto.GenerateCourseInput) (*dto.AsyncGenerationOutput, error)
	CheckGenerationStatus(generationID string) (*dto.GenerationStatusOutput, error)
	DownloadGenerationResults(generationID string) ([]byte, error)
	RetryGeneration(generationID string) (*dto.AsyncGenerationOutput, error)

	// Méthode de compatibilité (deprecated)
	GenerateCourse(generateCourseInputDto dto.GenerateCourseInput) (*dto.GenerateCourseOutput, error)
}

type courseService struct {
	repository     repositories.CourseRepository
	workerService  workerServices.WorkerService
	packageService workerServices.GenerationPackageService
	workerConfig   *config.WorkerConfig
	casdoorService authInterfaces.CasdoorService
	genericService genericService.GenericService
}

func NewCourseService(db *gorm.DB) CourseService {
	workerConfig := config.LoadWorkerConfig()
	return &courseService{
		repository:     repositories.NewCourseRepository(db),
		workerService:  workerServices.NewWorkerService(workerConfig),
		packageService: workerServices.NewGenerationPackageService(),
		workerConfig:   workerConfig,
		casdoorService: authInterfaces.NewCasdoorService(),
		genericService: genericService.NewGenericService(db),
	}
}

// NewCourseServiceWithDependencies permet d'injecter les dépendances (utile pour les tests)
func NewCourseServiceWithDependencies(
	db *gorm.DB,
	workerService workerServices.WorkerService,
	packageService workerServices.GenerationPackageService,
	casdoorService authInterfaces.CasdoorService,
	genericService genericService.GenericService,
) CourseService {
	workerConfig := config.LoadWorkerConfig()
	return &courseService{
		repository:     repositories.NewCourseRepository(db),
		workerService:  workerService,
		packageService: packageService,
		workerConfig:   workerConfig,
		casdoorService: casdoorService,
		genericService: genericService,
	}
}

// GenerateCourseAsync génère un cours de manière asynchrone via le worker
func (c courseService) GenerateCourseAsync(generateCourseInputDto dto.GenerateCourseInput) (*dto.AsyncGenerationOutput, error) {
	ctx := context.Background()

	// 1. Récupérer la génération
	generationEntity, err := c.genericService.GetEntity(uuid.MustParse(generateCourseInputDto.GenerationId), models.Generation{}, "Generation")
	if err != nil {
		return nil, fmt.Errorf("failed to get generation: %w", err)
	}
	generation := generationEntity.(*models.Generation)

	// 2. Récupérer le cours
	courseEntity, err := c.genericService.GetEntity(uuid.MustParse(generation.CourseID.String()), models.Course{}, "Course")
	if err != nil {
		return nil, fmt.Errorf("failed to get course: %w", err)
	}
	course := courseEntity.(*models.Course)

	// 5. Préparer le package de génération
	pkg, err := c.packageService.PrepareGenerationPackage(course, generateCourseInputDto.AuthorEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare generation package: %w", err)
	}

	// 6. Soumettre au worker avec retry
	var workerStatus *workerServices.WorkerJobStatus
	var submitErr error

	for attempt := 0; attempt < c.workerConfig.RetryCount; attempt++ {
		workerStatus, submitErr = c.workerService.SubmitGeneration(ctx, generation, pkg)
		if submitErr == nil {
			break
		}

		log.Printf("Worker submission attempt %d failed: %v", attempt+1, submitErr)
		if attempt < c.workerConfig.RetryCount-1 {
			time.Sleep(time.Duration(attempt+1) * time.Second) // Backoff progressif
		}
	}

	if submitErr != nil {
		// Marquer la génération comme échouée
		generation.SetFailed(fmt.Sprintf("Failed to submit to worker after %d attempts: %v", c.workerConfig.RetryCount, submitErr))
		c.genericService.SaveEntity(generation)
		return nil, fmt.Errorf("failed to submit generation to worker: %w", submitErr)
	}

	// 7. Mettre à jour la génération avec l'ID du job worker
	generation.SetWorkerJobID(workerStatus.ID)
	c.genericService.SaveEntity(generation)

	return &dto.AsyncGenerationOutput{
		GenerationID: generation.ID.String(),
		Status:       generation.Status,
		Message:      "Generation submitted successfully",
	}, nil
}

// CheckGenerationStatus vérifie le statut d'une génération
func (c courseService) CheckGenerationStatus(generationID string) (*dto.GenerationStatusOutput, error) {
	ctx := context.Background()

	// 1. Récupérer la génération
	generationEntity, err := c.genericService.GetEntity(uuid.MustParse(generationID), models.Generation{}, "Generation")
	if err != nil {
		return nil, fmt.Errorf("failed to get generation: %w", err)
	}
	generation := generationEntity.(*models.Generation)

	if generation.ID != uuid.MustParse(generationID) {
		return nil, fmt.Errorf("failed to get generation: id not found")
	}

	// 2. Si la génération n'a pas de job worker, retourner le statut local
	if generation.WorkerJobID == nil {
		return dto.GenerationModelToStatusOutput(*generation), nil
	}

	// 3. Vérifier le statut auprès du worker
	workerStatus, err := c.workerService.CheckStatus(ctx, *generation.WorkerJobID)
	if err != nil {
		log.Printf("Failed to check worker status: %v", err)
		// Retourner le statut local en cas d'erreur de communication
		return dto.GenerationModelToStatusOutput(*generation), nil
	}

	// 4. Mettre à jour le statut local si nécessaire
	updated := false
	if workerStatus.Progress != nil && generation.Progress != workerStatus.Progress {
		generation.UpdateProgress(*workerStatus.Progress)
		updated = true
	}

	if workerStatus.Status == "completed" && generation.Status != models.StatusCompleted {
		// Récupérer les URLs des résultats
		resultURLs, err := c.workerService.GetResultFiles(ctx, generation.CourseID.String())
		if err != nil {
			log.Printf("Failed to get result files: %v", err)
			resultURLs = []string{} // Continuer avec une liste vide
		}
		generation.SetCompleted(resultURLs)
		updated = true
	} else if workerStatus.Status == "failed" && generation.Status != models.StatusFailed {
		errorMsg := "Generation failed"
		if workerStatus.Error != nil {
			errorMsg = *workerStatus.Error
		}
		generation.SetFailed(errorMsg)
		updated = true
	}

	// 5. Sauvegarder les changements si nécessaire
	if updated {
		c.genericService.SaveEntity(generation)
	}

	return dto.GenerationModelToStatusOutput(*generation), nil
}

// DownloadGenerationResults télécharge les résultats d'une génération
func (c courseService) DownloadGenerationResults(generationID string) ([]byte, error) {
	ctx := context.Background()

	// 1. Récupérer la génération
	generationEntity, err := c.genericService.GetEntity(uuid.MustParse(generationID), models.Generation{}, "Generation")
	if err != nil {
		return nil, fmt.Errorf("failed to get generation: %w", err)
	}
	generation := generationEntity.(*models.Generation)

	// 2. Vérifier que la génération est terminée avec succès
	if !generation.IsSuccessful() {
		return nil, fmt.Errorf("generation is not completed successfully (status: %s)", generation.Status)
	}

	// 3. Télécharger les résultats depuis le worker
	return c.workerService.DownloadResults(ctx, generation.CourseID.String())
}

// RetryGeneration relance une génération échouée
func (c courseService) RetryGeneration(generationID string) (*dto.AsyncGenerationOutput, error) {
	// 1. Récupérer la génération
	generationEntity, err := c.genericService.GetEntity(uuid.MustParse(generationID), models.Generation{}, "Generation")
	if err != nil {
		return nil, fmt.Errorf("failed to get generation: %w", err)
	}
	generation := generationEntity.(*models.Generation)

	// 2. Vérifier que la génération peut être relancée
	if generation.Status == models.StatusProcessing {
		return nil, fmt.Errorf("generation is already in progress")
	}

	// 3. Réinitialiser le statut
	generation.Status = models.StatusPending
	generation.ErrorMessage = nil
	generation.WorkerJobID = nil
	generation.Progress = nil
	generation.StartedAt = nil
	generation.CompletedAt = nil

	c.genericService.SaveEntity(generation)

	courseEntity, err := c.genericService.GetEntity(generation.CourseID, models.Course{}, "Course")
	if err != nil {
		return nil, fmt.Errorf("failed to get course: %w", err)
	}
	course := courseEntity.(*models.Course)

	user, errUser := casdoorsdk.GetUserByUserId(course.OwnerIDs[0])
	if errUser != nil {
		return nil, fmt.Errorf("failed to get user: %w", errUser)
	}

	// Simuler une requête de génération
	generateInput := dto.GenerateCourseInput{
		GenerationId: generation.ID.String(),
		Format:       generation.Format,
		AuthorEmail:  user.Email, // TODO: Récupérer l'email original
	}

	// 5. Relancer la génération
	return c.GenerateCourseAsync(generateInput)
}

// GenerateCourse - Méthode de compatibilité (deprecated)
// Cette méthode maintient la compatibilité avec l'ancien comportement
// mais utilise maintenant le worker en mode synchrone
func (c courseService) GenerateCourse(generateCourseInputDto dto.GenerateCourseInput) (*dto.GenerateCourseOutput, error) {
	// 1. Lancer la génération asynchrone
	asyncResult, err := c.GenerateCourseAsync(generateCourseInputDto)
	if err != nil {
		return nil, err
	}

	// 2. Attendre la completion (mode synchrone pour compatibilité)
	//ctx := context.Background()
	generationID := asyncResult.GenerationID

	// Poll jusqu'à la completion avec un timeout
	timeout := time.Duration(300) * time.Second // 5 minutes
	start := time.Now()

	for time.Since(start) < timeout {
		status, err := c.CheckGenerationStatus(generationID)
		if err != nil {
			return nil, fmt.Errorf("failed to check generation status: %w", err)
		}

		if status.Status == models.StatusCompleted {
			return &dto.GenerateCourseOutput{Result: true}, nil
		} else if status.Status == models.StatusFailed {
			errorMsg := "Generation failed"
			if status.ErrorMessage != nil {
				errorMsg = *status.ErrorMessage
			}
			return nil, fmt.Errorf("generation failed: %s", errorMsg)
		}

		// Attendre avant le prochain check
		time.Sleep(c.workerConfig.PollInterval)
	}

	return nil, fmt.Errorf("generation timeout after %v", timeout)
}

// Méthodes existantes conservées pour compatibilité...

func (c courseService) GetGitCourse(ownerId string, courseName string, courseURL string, courseBranch string) (*models.Course, error) {
	// Use cached clone for better performance
	log.Printf("Loading course from repository: %s (branch: %s)", courseURL, courseBranch)

	fs, cloneErr := models.GitCloneWithCache(ownerId, courseURL, courseBranch)
	if cloneErr != nil {
		return nil, cloneErr
	}

	jsonFile, err := fs.Open("course.json")
	if err != nil {
		log.Fatal("Error during ReadFile(): ", err)
	}

	fileByteArray, errReadingFile := fileToBytesWithoutSeeking(jsonFile)
	if errReadingFile != nil {
		return nil, errReadingFile
	}

	var course models.Course
	err = json.Unmarshal(fileByteArray, &course)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
	}

	course.OwnerIDs = append(course.OwnerIDs, ownerId)
	course.FolderName = courseName
	course.GitRepository = courseURL
	course.GitRepositoryBranch = courseBranch

	models.FillCourseModelFromFiles(&fs, &course)

	genericService := genericService.NewGenericService(sqldb.DB)
	courseInputDto := dto.CourseModelToCourseInputDto(course)
	_, errorSaving := genericService.CreateEntity(courseInputDto, reflect.TypeOf(models.Course{}).Name())

	if errorSaving != nil {
		fmt.Println(errorSaving.Error())
		return nil, err
	}

	return &course, nil
}

func (c courseService) GetSpecificCourseByUser(owner casdoorsdk.User, courseName string) (*models.Course, error) {
	course, err := c.repository.GetSpecificCourseByUser(owner, courseName)
	if err != nil {
		return nil, err
	}
	return course, nil
}

func (c courseService) GetCourseFromProgramInputs(courseName *string, courseGitRepository *string, courseGitRepositoryBranchName *string) models.Course {
	return models.Course{}
}

// Fonctions utilitaires

func fileToBytesWithoutSeeking(file billy.File) ([]byte, error) {
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		return nil, err
	}

	if _, err := file.Seek(currentPos, io.SeekStart); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
