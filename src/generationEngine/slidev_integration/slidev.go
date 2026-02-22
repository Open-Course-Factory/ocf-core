package slidev

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/models"
	"soli/formations/src/utils"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/util"
)

type Option string

const (
	HTML Option = "html"
	PDF  Option = "pdf"
)

const DOCKER_IMAGE = "registry.gitlab.com/open-course-factory/ocf-core/ocf_slidev:latest"
const PUBLIC_DIR = "public"

func (o Option) GetTypeOpts() []string {
	var res []string
	return res
}

var (
	capabilitiesMap = map[string]Option{
		"html": HTML,
		"pdf":  PDF,
	}
)

type SlidevCourseGenerator struct {
}

func (scg SlidevCourseGenerator) ParseDocType(str string) Option {
	c := capabilitiesMap[strings.ToLower(str)]
	return c
}

func (scg SlidevCourseGenerator) GetThemesSetOpts(course *models.Course) []string {
	options := make([]string, 0)
	options = append(options, "--theme-set")
	for _, t := range course.GetThemes() {
		options = append(options, config.THEMES_ROOT+t+"/"+t+".scss")
	}
	return options
}

func (scg SlidevCourseGenerator) GetCmd(course *models.Course) (*exec.Cmd, error) {

	pwd, err := os.Getwd()
	if err != nil {
		utils.Error("failed to get working directory: %v", err)
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	outputDir := config.COURSES_OUTPUT_DIR + course.Theme.Name
	srcFile := outputDir + "/" + course.GetFilename("md")
	//destFile := course.GetFilename(*docType)

	baseCmd := []string{"run", "--rm", "-e", `NPM_MIRROR="https://registry.npmmirror.com"`, "-v", pwd + "/dist:/slidev/dist", DOCKER_IMAGE, srcFile, "--download", "true"}

	cmd := exec.Command("/usr/bin/docker", baseCmd...)

	return cmd, nil
}

func (scg SlidevCourseGenerator) Run(course *models.Course) error {
	cmd, err := scg.GetCmd(course)
	if err != nil {
		return err
	}

	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	utils.Info("Command ready to be executed: %s", cmd.String())

	if *config.DRY_RUN {
		return nil
	}

	err = cmd.Run()

	if err != nil {
		utils.Error("%s", err.Error())
		errByte := errb.String()
		if len(errb.Bytes()) > 0 {
			utils.Error("%s", errByte)
		}
		return err
	}

	utils.Info("%s", outb.String())

	return nil
}

func (scg SlidevCourseGenerator) CompileResources(c *models.Course) error {
	outputDir := config.COURSES_OUTPUT_DIR + c.Theme.Name
	outputFolders := [2]string{"/" + PUBLIC_DIR, "/theme"}

	for _, f := range outputFolders {
		err := os.MkdirAll(outputDir+f, os.ModePerm)
		if err != nil {
			utils.Error("failed to create output directory %s: %v", outputDir+f, err)
			return fmt.Errorf("failed to create output directory %s: %w", outputDir+f, err)
		}
	}

	// Copy Themes
	// Determine source type - default to git if not specified for backwards compatibility
	sourceType := c.Theme.SourceType
	source := ""
	branch := c.Theme.RepositoryBranch

	if sourceType == "" {
		// Legacy behavior: if Repository is set, assume git
		if c.Theme.Repository != "" {
			sourceType = "git"
			source = c.Theme.Repository
		} else if c.Theme.SourcePath != "" {
			sourceType = "local"
			source = c.Theme.SourcePath
		} else {
			utils.Error("No theme source specified (neither Repository nor SourcePath)")
			return fmt.Errorf("no theme source specified (neither Repository nor SourcePath)")
		}
	} else {
		// New behavior: use SourceType to determine which field to read
		if sourceType == "git" {
			source = c.Theme.Repository
		} else if sourceType == "local" {
			source = c.Theme.SourcePath
		}
	}

	fs, errClone := models.LoadTheme(c.OwnerIDs[0], sourceType, source, branch)
	if errClone != nil {
		utils.Error("failed to load theme: %v", errClone)
		return fmt.Errorf("failed to load theme: %w", errClone)
	}

	markFn := func(path string, entry os.FileInfo, err error) error {

		if !entry.IsDir() {
			// Create your file
			//create file locally
			err := scg.writeFileFromFsToDisk(fs, path, outputDir, entry)
			if err != nil {
				return err
			}

		} else {
			if _, err := os.Stat(outputDir + path); os.IsNotExist(err) {
				os.MkdirAll(outputDir+path, 0700) // Create your directory
			}
		}

		return nil
	}

	util.Walk(fs, "/", markFn)

	// Copy global images
	if _, err := os.Stat(config.IMAGES_ROOT); !os.IsNotExist(err) {
		cpiErr := models.CopyDir(config.IMAGES_ROOT, outputDir+"/"+PUBLIC_DIR)
		if cpiErr != nil {
			utils.Error("failed to copy global images: %v", cpiErr)
			return fmt.Errorf("failed to copy global images: %w", cpiErr)
		}
	}

	// Copy course-specific images
	// First, try to copy from the course source (local or git)
	if c.SourceType != "" {
		// Course was loaded from a source (local or git), load the filesystem and copy images
		courseSourceType := c.SourceType
		courseSource := ""
		courseBranch := ""

		if courseSourceType == "git" {
			courseSource = c.GitRepository
			courseBranch = c.GitRepositoryBranch
		} else if courseSourceType == "local" {
			courseSource = c.SourcePath
		}

		if courseSource != "" {
			courseFS, errLoadCourse := models.LoadTheme(c.OwnerIDs[0], courseSourceType, courseSource, courseBranch)
			if errLoadCourse == nil {
				// Copy images directory if it exists
				courseImagesFn := func(path string, entry os.FileInfo, err error) error {
					// Only copy files from /images or /public directories
					if strings.HasPrefix(path, "/images/") || strings.HasPrefix(path, "/public/") {
						if !entry.IsDir() {
							file, errFileOpen := courseFS.Open(path)
							if errFileOpen != nil {
								return errFileOpen
							}

							fileContent, errRead := io.ReadAll(file)
							if errRead != nil {
								return errRead
							}

							// Determine target paths
							// For Slidev to work properly, images need to be in multiple locations:
							// 1. outputDir/images/ for markdown build-time resolution (./images/foo.svg)
							// 2. outputDir/public/images/ for Slidev runtime/PDF (/public/images/foo.svg)
							// 3. outputDir/public/ for Slidev runtime/PDF (./images/ gets rewritten to /public/)
							var targetPaths []string

							if strings.HasPrefix(path, "/public/") {
								// Files in /public/ go directly to outputDir/public/
								targetPaths = append(targetPaths, outputDir+path)
							} else if strings.HasPrefix(path, "/images/") {
								// Files in /images/ go to THREE locations for maximum compatibility
								targetPaths = append(targetPaths, outputDir+path)                          // dist/mds/images/
								targetPaths = append(targetPaths, outputDir+"/"+PUBLIC_DIR+path)          // dist/mds/public/images/
								// Also copy to public root with just the filename for ./images/ references
								filename := filepath.Base(path)
								targetPaths = append(targetPaths, outputDir+"/"+PUBLIC_DIR+"/"+filename)  // dist/mds/public/filename
							}

							// Copy to all target paths
							for _, targetPath := range targetPaths {
								// Create directory if needed
								targetDir := filepath.Dir(targetPath)
								os.MkdirAll(targetDir, 0755)

								// Write file
								err := os.WriteFile(targetPath, fileContent, 0644)
								if err != nil {
									return err
								}
								utils.Debug("Copied course image: %s -> %s", path, targetPath)
							}
						}
					}
					return nil
				}

				util.Walk(courseFS, "/", courseImagesFn)
			} else {
				utils.Warn("Could not load course filesystem for images: %v", errLoadCourse)
			}
		}
	}

	// Fallback: Copy course-specific images from COURSES_ROOT (for backwards compatibility)
	courseImages := config.COURSES_ROOT + c.FolderName + "/" + PUBLIC_DIR
	if _, ciiErr := os.Stat(courseImages); !os.IsNotExist(ciiErr) {
		cpic_err := models.CopyDir(courseImages, outputDir+"/"+PUBLIC_DIR)
		if cpic_err != nil {
			utils.Error("failed to copy course images: %v", cpic_err)
			return fmt.Errorf("failed to copy course images: %w", cpic_err)
		}
	}

	return nil
}

func (scg SlidevCourseGenerator) writeFileFromFsToDisk(fs billy.Filesystem, path string, outputDir string, entry fs.FileInfo) error {
	file, errFileOpen := fs.Open(path)
	if errFileOpen != nil {
		utils.Error("opening file")
		return errFileOpen
	}

	fileContent, errRead := io.ReadAll(file)
	if errRead != nil {
		utils.Error("reading file")
		return errRead
	}

	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		os.MkdirAll(outputDir, 0700)
	}

	err := os.WriteFile(outputDir+path, fileContent, 0600)

	if err != nil {
		utils.Error("writing file")
		return err
	}

	if strings.Contains(path, "/theme/public/") {
		os.WriteFile(outputDir+"/"+scg.GetPublicDir()+"/"+entry.Name(), fileContent, 0600)
	}
	return nil
}

func (scg SlidevCourseGenerator) GetPublicDir() string {
	return PUBLIC_DIR
}

func (scg SlidevCourseGenerator) ExportPDF(course *models.Course) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	//outputDir := config.COURSES_OUTPUT_DIR + course.Theme.Name
	srcFile := course.GetFilename("md") // Just the filename, not the full path

	// Docker command for PDF export using Slidev
	// The Docker image might have "export" as default entrypoint command
	// Use -w to set working directory inside container
	// Add timing parameters to ensure CSS and images are fully loaded before export
	baseCmd := []string{
		"run", "--rm", "-i",
		"-e", `NPM_MIRROR="https://registry.npmmirror.com"`,
		"-v", pwd + "/dist:/slidev/dist",
		"-w", "/slidev/dist/" + course.Theme.Name,
		DOCKER_IMAGE,
		srcFile,
		"--format", "pdf",
		"--output", "slides-exported.pdf",
		"--wait-until", "networkidle", // Wait for all network requests (CSS, images) to complete
		"--wait", "2000",              // Additional 2 second delay after networkidle
		"--timeout", "60000",          // 60 second timeout for rendering
	}

	cmd := exec.Command("/usr/bin/docker", baseCmd...)

	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	utils.Info("PDF export command ready: %s", cmd.String())

	if *config.DRY_RUN {
		return nil
	}

	err = cmd.Run()
	if err != nil {
		utils.Error("PDF export stdout: %s", outb.String())
		utils.Error("PDF export stderr: %s", errb.String())
		return err
	}

	utils.Info("PDF export completed successfully")
	utils.Debug("PDF export stdout: %s", outb.String())
	return nil
}
