// src/worker/services/generationPackageService.go
package services

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	authInterfaces "soli/formations/src/auth/interfaces"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/models"
)

// GenerationPackageService prépare les packages pour la génération
type GenerationPackageService interface {
	PrepareGenerationPackage(course *models.Course, authorEmail string) (*GenerationPackage, error)
	GenerateMDContent(course *models.Course, authorEmail string) (string, error)
	CollectAssets(course *models.Course) (map[string][]byte, error)
	CollectThemeFiles(themeName string) (map[string][]byte, error)
}

type generationPackageService struct {
	casdoorService authInterfaces.CasdoorService
}

func NewGenerationPackageService() GenerationPackageService {
	return &generationPackageService{
		casdoorService: authInterfaces.NewCasdoorService(),
	}
}

// NewGenerationPackageServiceWithDependencies permet d'injecter les dépendances (utile pour les tests)
func NewGenerationPackageServiceWithDependencies(casdoorService authInterfaces.CasdoorService) GenerationPackageService {
	return &generationPackageService{
		casdoorService: casdoorService,
	}
}

// PrepareGenerationPackage prépare un package complet pour la génération
func (gps *generationPackageService) PrepareGenerationPackage(course *models.Course, authorEmail string) (*GenerationPackage, error) {
	// Générer le contenu MD
	mdContent, err := gps.GenerateMDContent(course, authorEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to generate MD content: %w", err)
	}

	// Collecter les assets
	assets, err := gps.CollectAssets(course)
	if err != nil {
		return nil, fmt.Errorf("failed to collect assets: %w", err)
	}

	// Collecter les fichiers de thème si nécessaire
	var themeFiles map[string][]byte
	if course.Theme != nil && gps.isCustomTheme(course.Theme.Name) {
		themeFiles, err = gps.CollectThemeFiles(course.Theme.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to collect theme files: %w", err)
		}
	} else {
		themeFiles = make(map[string][]byte)
	}

	// Récupérer les informations de l'utilisateur
	user, err := gps.casdoorService.GetUserByEmail(authorEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Créer les métadonnées
	metadata := GenerationMetadata{
		CourseID:   course.ID.String(),
		CourseName: course.Name,
		Format:     1, // Slidev par défaut
		Theme:      gps.getThemeName(course),
		Author:     user.DisplayName,
		Version:    course.Version,
	}

	return &GenerationPackage{
		MDContent:  mdContent,
		Assets:     assets,
		ThemeFiles: themeFiles,
		Metadata:   metadata,
	}, nil
}

// GenerateMDContent génère le contenu Markdown en mémoire
func (gps *generationPackageService) GenerateMDContent(course *models.Course, authorEmail string) (string, error) {
	// S'assurer que les TOCs sont initialisés
	course.InitTocs()

	// Récupérer les informations de l'utilisateur
	user, err := gps.casdoorService.GetUserByEmail(authorEmail)
	if err != nil {
		return "", fmt.Errorf("failed to get user info: %w", err)
	}

	// Générer le contenu Slidev
	courseContent := course.String()

	// Remplacer les placeholders
	courseContent = strings.ReplaceAll(courseContent, "@@author@@", user.Name)
	courseContent = strings.ReplaceAll(courseContent, "@@author_fullname@@", user.DisplayName)
	courseContent = strings.ReplaceAll(courseContent, "@@author_email@@", authorEmail)
	courseContent = strings.ReplaceAll(courseContent, "@@version@@", course.Version)

	return courseContent, nil
}

// CollectAssets collecte tous les assets nécessaires (images, etc.)
func (gps *generationPackageService) CollectAssets(course *models.Course) (map[string][]byte, error) {
	assets := make(map[string][]byte)

	// Collecter les assets à partir du dossier du cours si disponible
	if course.FolderName != "" {
		assetsPath := filepath.Join(config.COURSES_ROOT, course.FolderName, "assets")
		if err := gps.collectFilesFromDirectory(assetsPath, "assets/", assets); err != nil {
			log.Printf("Warning: failed to collect assets from %s: %v", assetsPath, err)
		}
	}

	// Collecter les images référencées dans le contenu
	if err := gps.collectReferencedImages(course, assets); err != nil {
		log.Printf("Warning: failed to collect referenced images: %v", err)
	}

	return assets, nil
}

// CollectThemeFiles collecte les fichiers d'un thème personnalisé
func (gps *generationPackageService) CollectThemeFiles(themeName string) (map[string][]byte, error) {
	themeFiles := make(map[string][]byte)

	themePath := filepath.Join(config.THEMES_ROOT, themeName)
	if _, err := os.Stat(themePath); os.IsNotExist(err) {
		return themeFiles, nil // Thème non local, sera géré par le worker
	}

	// Collecter tous les fichiers du thème
	err := gps.collectFilesFromDirectory(themePath, "theme/", themeFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to collect theme files: %w", err)
	}

	return themeFiles, nil
}

// collectFilesFromDirectory collecte récursivement les fichiers d'un répertoire
func (gps *generationPackageService) collectFilesFromDirectory(dirPath, prefix string, files map[string][]byte) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return nil // Répertoire n'existe pas, pas d'erreur
	}

	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Calculer le chemin relatif
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		// Normaliser le chemin pour le web (utiliser / au lieu de \)
		webPath := filepath.ToSlash(filepath.Join(prefix, relPath))

		// Lire le fichier
		content, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: failed to read file %s: %v", path, err)
			return nil // Continue avec les autres fichiers
		}

		files[webPath] = content
		return nil
	})
}

// collectReferencedImages collecte les images référencées dans le contenu du cours
func (gps *generationPackageService) collectReferencedImages(course *models.Course, assets map[string][]byte) error {
	// Cette fonction pourrait être étendue pour parser le contenu Markdown
	// et identifier les images référencées, puis les collecter
	// Pour le moment, on se contente de collecter les assets du répertoire
	return nil
}

// isCustomTheme vérifie si un thème est personnalisé (local) ou standard
func (gps *generationPackageService) isCustomTheme(themeName string) bool {
	// Les thèmes standards Slidev n'ont pas besoin d'être uploadés
	standardThemes := []string{"default", "apple-basic", "bricks", "carbon", "dracula", "geist", "light", "materia", "minimal", "news", "nordic", "penguin", "purplin", "seriph", "shibainu", "themes"}

	for _, standard := range standardThemes {
		if themeName == standard {
			return false
		}
	}

	// Vérifier si le thème existe localement
	themePath := filepath.Join(config.THEMES_ROOT, themeName)
	if _, err := os.Stat(themePath); !os.IsNotExist(err) {
		return true
	}

	return false
}

// getThemeName retourne le nom du thème à utiliser
func (gps *generationPackageService) getThemeName(course *models.Course) string {
	if course.Theme != nil {
		return course.Theme.Name
	}
	return "default"
}
