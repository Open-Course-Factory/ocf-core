package cli

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	"gorm.io/gorm"

	authInterfaces "soli/formations/src/auth/interfaces"
	config "soli/formations/src/configuration"
	courseDto "soli/formations/src/courses/dto"
	"soli/formations/src/courses/models"
	courseModels "soli/formations/src/courses/models"
	courseService "soli/formations/src/courses/services"
	genericService "soli/formations/src/entityManagement/services"
	generator "soli/formations/src/generationEngine"
	marp "soli/formations/src/generationEngine/marp_integration"
	slidev "soli/formations/src/generationEngine/slidev_integration"
)

// ParseFlags processes CLI flags for course generation
// Returns true if CLI mode was used (and app should exit after completion)
func ParseFlags(db *gorm.DB, enforcer authInterfaces.EnforcerInterface) bool {
	const COURSE_FLAG = "c"
	const COURSE_SOURCE_TYPE_FLAG = "source-type"
	const COURSE_SOURCE_FLAG = "source"
	const GIT_COURSE_REPO_FLAG = "course-repo"
	const GIT_COURSE_REPO_BRANCH_FLAG = "course-repo-branch"
	const THEME_FLAG = "t"
	const THEME_SOURCE_TYPE_FLAG = "theme-source-type"
	const THEME_SOURCE_FLAG = "theme-source"
	const GIT_THEME_REPO_FLAG = "theme-repo"
	const GIT_THEME_REPO_BRANCH_FLAG = "theme-repo-branch"
	const TYPE_FLAG = "e"
	const DRY_RUN_FLAG = "dry-run"
	const SLIDE_ENGINE_FLAG = "slide-engine"
	const USER_ID_FLAG = "user-id"
	const AUTHOR_FLAG = "author"
	const COURSE_JSON_FILENAME_FLAG = "course-json"

	courseName := flag.String(COURSE_FLAG, "git", "name of the course you need to generate")
	courseSourceType := flag.String(COURSE_SOURCE_TYPE_FLAG, "git", "source type: 'git' for git repository or 'local' for local filesystem path")
	courseSource := flag.String(COURSE_SOURCE_FLAG, "", "git repository URL or local filesystem path")
	courseGitRepository := flag.String(GIT_COURSE_REPO_FLAG, "", "git repository (deprecated, use --source)")
	courseBranchGitRepository := flag.String(GIT_COURSE_REPO_BRANCH_FLAG, "main", "git repository branch for course (only for git source type)")
	courseThemeName := flag.String(THEME_FLAG, "sdv", "name of the theme used to generate the website")
	themeSourceType := flag.String(THEME_SOURCE_TYPE_FLAG, "git", "theme source type: 'git' for git repository or 'local' for local filesystem path")
	themeSource := flag.String(THEME_SOURCE_FLAG, "", "theme git repository URL or local filesystem path")
	courseThemeGitRepository := flag.String(GIT_THEME_REPO_FLAG, "", "theme git repository (deprecated, use --theme-source)")
	courseThemeBranchGitRepository := flag.String(GIT_THEME_REPO_BRANCH_FLAG, "main", "git repository branch for theme (only for git source type)")
	courseType := flag.String(TYPE_FLAG, "html", "type generated : html (default) or pdf")
	config.DRY_RUN = flag.Bool(DRY_RUN_FLAG, false, "if set true, the cli stops before calling slide generator")
	slideEngine := flag.String(SLIDE_ENGINE_FLAG, "slidev", "slide generator used, marp or slidev (default)")
	userID := flag.String(USER_ID_FLAG, "00000000-0000-0000-0000-000000000000", "user ID (UUID) for authentication and git operations")
	author := flag.String(AUTHOR_FLAG, "cli", "author trigramme for loading author_XXX.md file")
	courseJsonFilename := flag.String(COURSE_JSON_FILENAME_FLAG, "course.json", "filename of the course JSON file in the repository")
	flag.Parse()

	fmt.Println(courseType)

	// check mandatory flags
	if !isFlagPassed(COURSE_FLAG) || !isFlagPassed(THEME_FLAG) || !isFlagPassed(TYPE_FLAG) {
		return false
	}

	switch *slideEngine {
	case "marp":
		generator.SLIDE_ENGINE = marp.MarpCourseGenerator{}
	case "slidev":
		generator.SLIDE_ENGINE = slidev.SlidevCourseGenerator{}
	default:
		generator.SLIDE_ENGINE = slidev.SlidevCourseGenerator{}
	}

	courseService := courseService.NewCourseService(db)

	var course courseModels.Course

	// Determine the source to use (new flags take precedence over old flags)
	source := *courseSource
	if source == "" && *courseGitRepository != "" {
		// Backwards compatibility: use old --course-repo flag if --source not provided
		source = *courseGitRepository
		*courseSourceType = "git"
	}

	// If we have a source, load the course from it
	if source != "" {
		fmt.Printf("Loading course from %s: %s\n", *courseSourceType, source)
		coursePtr, err := courseService.GetCourse(*userID, *courseName, *courseSourceType, source, *courseBranchGitRepository, *courseJsonFilename)
		if err != nil {
			fmt.Printf("Error loading course from %s: %v\n", *courseSourceType, err)
			return true
		}
		course = *coursePtr
		fmt.Printf("Course loaded and saved successfully: %s v%s (ID: %s)\n", course.Name, course.Version, course.ID.String())
	} else {
		// Fallback to empty course for CLI-only usage
		course = courseModels.Course{}
		// Set the owner ID for CLI usage
		course.OwnerIDs = append(course.OwnerIDs, *userID)
		// Set basic course info from CLI args
		course.Name = *courseName
		course.FolderName = *courseName

		// Save the course to database
		genericService := genericService.NewGenericService(db, enforcer)
		courseInputDto := courseDto.CourseModelToCourseInputDto(course)
		savedCourseEntity, errorSaving := genericService.CreateEntity(courseInputDto, reflect.TypeOf(models.Course{}).Name())

		if errorSaving != nil {
			fmt.Println(errorSaving.Error())
			return true
		}

		savedCourse := savedCourseEntity.(*models.Course)
		course.ID = savedCourse.ID
		fmt.Printf("Course created successfully with ID: %s\n", course.ID.String())
	}

	// Determine theme source (new flags take precedence over old flags)
	themeSourceToUse := *themeSource
	if themeSourceToUse == "" && *courseThemeGitRepository != "" {
		// Backwards compatibility: use old --theme-repo flag if --theme-source not provided
		themeSourceToUse = *courseThemeGitRepository
		*themeSourceType = "git"
	}

	setCourseThemeFromProgramInputs(&course, string(*courseThemeName), *themeSourceType, themeSourceToUse, string(*courseThemeBranchGitRepository))

	// Check DRY_RUN flag before proceeding with generation
	if *config.DRY_RUN {
		fmt.Println("DRY RUN mode: Stopping before slide generation")
		return true
	}

	// Generate the course using the selected slide engine
	fmt.Printf("Starting course generation using %T...\n", generator.SLIDE_ENGINE)

	// First, compile the course resources (create directories, etc.)
	fmt.Println("Compiling course resources...")
	errorCompiling := generator.SLIDE_ENGINE.CompileResources(&course)
	if errorCompiling != nil {
		fmt.Printf("Error compiling course resources: %v\n", errorCompiling)
		return true
	}

	// Create the course writer and generate markdown content
	fmt.Println("Creating course markdown file...")
	var courseWriter courseModels.CourseMdWriter
	switch generator.SLIDE_ENGINE.(type) {
	case slidev.SlidevCourseGenerator:
		courseWriter = &courseModels.SlidevCourseWriter{Course: course}
	case marp.MarpCourseGenerator:
		courseWriter = &courseModels.MarpCourseWriter{Course: course}
	default:
		courseWriter = &courseModels.SlidevCourseWriter{Course: course}
	}

	// Generate the course content
	courseContent := courseWriter.GetCourse()

	// Substitute template variables
	fmt.Println("Substituting template variables...")

	// Read author information from authors/author_XXX.md file
	authorInfo := readAuthorInfo(*author)

	courseContent = strings.ReplaceAll(courseContent, "@@author@@", *author)
	courseContent = strings.ReplaceAll(courseContent, "@@author_fullname@@", authorInfo.FullName)
	courseContent = strings.ReplaceAll(courseContent, "@@author_email@@", authorInfo.Email)
	courseContent = strings.ReplaceAll(courseContent, "@@author_page_content@@", authorInfo.PageContent)
	courseContent = strings.ReplaceAll(courseContent, "@@version@@", course.Version)

	// Write the course content to the expected file
	outputDir := "dist/mds/"
	os.MkdirAll(outputDir, 0755)
	courseFilePath := outputDir + course.GetFilename("md")

	fmt.Printf("Writing course content to: %s\n", courseFilePath)
	errorWriting := os.WriteFile(courseFilePath, []byte(courseContent), 0644)
	if errorWriting != nil {
		fmt.Printf("Error writing course file: %v\n", errorWriting)
		return true
	}

	// Then, run the slide engine
	fmt.Println("Running slide engine...")
	errorGenerating := generator.SLIDE_ENGINE.Run(&course)
	if errorGenerating != nil {
		fmt.Printf("Error generating course: %v\n", errorGenerating)
		return true
	}

	// Generate PDF export
	fmt.Println("Generating PDF export...")
	errorPDF := generator.SLIDE_ENGINE.ExportPDF(&course)
	if errorPDF != nil {
		fmt.Printf("Warning: PDF generation failed: %v\n", errorPDF)
		// Continue without failing, PDF is optional
	}

	fmt.Println("Course generated successfully!")
	return true
}

func setCourseThemeFromProgramInputs(course *courseModels.Course, themeName string, sourceType string, source string, branch string) {
	if course.Theme == nil {
		course.Theme = &courseModels.Theme{}
	}
	course.Theme.Name = themeName
	course.Theme.SourceType = sourceType
	course.Theme.RepositoryBranch = branch

	// Set the appropriate source field based on type
	if sourceType == "git" {
		course.Theme.Repository = source
	} else if sourceType == "local" {
		course.Theme.SourcePath = source
	}
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// AuthorInfo structure to hold author information
type AuthorInfo struct {
	FullName    string
	Email       string
	PageContent string
}

// readAuthorInfo reads author information from authors/author_XXX.md file
func readAuthorInfo(authorTrigramme string) AuthorInfo {
	// Default values in case file is not found or doesn't contain the info
	defaultAuthor := AuthorInfo{
		FullName: "CLI User",
		Email:    "cli@ocf.local",
	}

	// Try to read from the git cloned content first
	authorFilePath := fmt.Sprintf("dist/mds/authors/author_%s.md", authorTrigramme)

	// If not found in dist, try the current directory
	if _, err := os.Stat(authorFilePath); os.IsNotExist(err) {
		authorFilePath = fmt.Sprintf("authors/author_%s.md", authorTrigramme)
	}

	content, err := os.ReadFile(authorFilePath)
	if err != nil {
		fmt.Printf("Warning: Could not read author file %s, using default values: %v\n", authorFilePath, err)
		return defaultAuthor
	}

	// Store the full page content
	author := defaultAuthor
	author.PageContent = string(content)

	// Parse the markdown content to extract author info
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for the author name in bold format like "**Thomas Saquet**"
		if strings.HasPrefix(line, "**") && strings.Contains(line, "**") && !strings.Contains(line, ":") {
			// Find the first occurrence of ** and the next occurrence of **
			firstAsterisk := strings.Index(line, "**")
			if firstAsterisk != -1 {
				restOfLine := line[firstAsterisk+2:] // Skip the first **
				secondAsterisk := strings.Index(restOfLine, "**")
				if secondAsterisk != -1 {
					// Extract the name between the ** markers
					name := strings.TrimSpace(restOfLine[:secondAsterisk])
					// Skip empty names or generic headers
					if name != "" && name != "Qui suis-je ?" && !strings.Contains(strings.ToLower(name), "formateur") && !strings.Contains(strings.ToLower(name), "expert") {
						author.FullName = name
					}
				}
			}
		}

		// Look for email in the format "📧 email@domain.com"
		if strings.Contains(line, "📧") {
			// Extract email after the emoji
			parts := strings.Split(line, "📧")
			if len(parts) > 1 {
				emailPart := strings.TrimSpace(parts[1])
				// Remove any trailing markdown or whitespace characters
				emailPart = strings.Fields(emailPart)[0] // Get first word which should be the email
				// Basic email validation
				if strings.Contains(emailPart, "@") && strings.Contains(emailPart, ".") {
					author.Email = emailPart
				}
			}
		}
	}

	fmt.Printf("Author info loaded: %s <%s>\n", author.FullName, author.Email)
	return author
}
