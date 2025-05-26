package slidev

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/models"
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

func (scg SlidevCourseGenerator) GetCmd(course *models.Course) *exec.Cmd {

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	outputDir := config.COURSES_OUTPUT_DIR + course.Theme.Name
	srcFile := outputDir + "/" + course.GetFilename("md")
	//destFile := course.GetFilename(*docType)

	baseCmd := []string{"run", "--rm", "-e", `NPM_MIRROR="https://registry.npmmirror.com"`, "-v", pwd + "/dist:/slidev/dist", DOCKER_IMAGE, srcFile, "--download", "true"}

	cmd := exec.Command("/usr/bin/docker", baseCmd...)

	return cmd
}

func (scg SlidevCourseGenerator) Run(course *models.Course) error {
	cmd := scg.GetCmd(course)

	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	fmt.Println("Command ready to be executed: " + cmd.String())

	if *config.DRY_RUN {
		return nil
	}

	err := cmd.Run()

	if err != nil {
		fmt.Println(err.Error())
		errByte := errb.String()
		if len(errb.Bytes()) > 0 {
			fmt.Println(errByte)
		}
		return err
	}

	fmt.Println(outb.String())

	return nil
}

func (scg SlidevCourseGenerator) CompileResources(c *models.Course) error {
	outputDir := config.COURSES_OUTPUT_DIR + c.Theme.Name
	outputFolders := [2]string{"/" + PUBLIC_DIR, "/theme"}

	for _, f := range outputFolders {
		err := os.MkdirAll(outputDir+f, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Copy Themes
	fs, errClone := models.GitClone(c.OwnerIDs[0], c.Theme.Repository, c.Theme.RepositoryBranch)
	if errClone != nil {
		log.Fatal(errClone)
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
			log.Fatal(cpiErr)
		}
	}

	// Copy course specifique images
	courseImages := config.COURSES_ROOT + c.FolderName + "/" + PUBLIC_DIR
	if _, ciiErr := os.Stat(courseImages); !os.IsNotExist(ciiErr) {
		cpic_err := models.CopyDir(courseImages, outputDir+"/"+PUBLIC_DIR)
		if cpic_err != nil {
			log.Fatal(cpic_err)
		}
	}

	return nil
}

func (scg SlidevCourseGenerator) writeFileFromFsToDisk(fs billy.Filesystem, path string, outputDir string, entry fs.FileInfo) error {
	file, errFileOpen := fs.Open(path)
	if errFileOpen != nil {
		log.Printf("opening file")
		return errFileOpen
	}

	fileContent, errRead := io.ReadAll(file)
	if errRead != nil {
		log.Printf("reading file")
		return errRead
	}

	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		os.MkdirAll(outputDir, 0700)
	}

	err := os.WriteFile(outputDir+path, fileContent, 0600)

	if err != nil {
		log.Printf("writing file")
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
