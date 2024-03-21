package slidev

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/models"
	"strings"
)

type Option string

const (
	HTML Option = "html"
	PDF  Option = "pdf"
)

const DOCKER_IMAGE = "TO BE DEFINED"
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

func (scg SlidevCourseGenerator) GetCmd(course *models.Course, docType *string) *exec.Cmd {

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	outputDir := config.COURSES_OUTPUT_DIR + course.Theme
	srcFile := outputDir + "/" + course.GetFilename("md")
	destFile := course.GetFilename(*docType)

	baseCmd := []string{"run", "--rm", "-e", `NPM_MIRROR="https://registry.npmmirror.com"`, "-v", pwd + "/dist:/slidev/dist", "ocf_slidev", srcFile, "--output", destFile}

	cmd := exec.Command("/usr/bin/docker", baseCmd...)

	return cmd
}

func (scg SlidevCourseGenerator) Run(configuration *config.Configuration, course *models.Course, docType *string) error {
	cmd := scg.GetCmd(course, docType)

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
		//log.Fatal(err)
	}

	errByte := errb.String()
	if len(errb.Bytes()) > 0 {
		fmt.Println(errByte)
	}

	fmt.Println(outb.String())

	return nil
}

func (scg SlidevCourseGenerator) CompileResources(c *models.Course, configuration *config.Configuration) error {
	outputDir := config.COURSES_OUTPUT_DIR + c.Theme
	outputFolders := [2]string{"/" + PUBLIC_DIR, "/theme"}

	for _, f := range outputFolders {
		err := os.MkdirAll(outputDir+f, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Copy Themes
	for _, t := range c.GetThemes() {
		themeSrc := config.THEMES_ROOT + t
		cptErr := models.CopyDir(themeSrc, outputDir)
		if cptErr != nil {
			log.Fatal(cptErr)
		}
	}

	// Copy global images
	if _, err := os.Stat(config.IMAGES_ROOT); !os.IsNotExist(err) {
		cpiErr := models.CopyDir(config.IMAGES_ROOT, outputDir+"/"+PUBLIC_DIR)
		if cpiErr != nil {
			log.Fatal(cpiErr)
		}
	}

	// Copy course specifique images
	courseImages := config.COURSES_ROOT + PUBLIC_DIR
	if _, ciiErr := os.Stat(courseImages); !os.IsNotExist(ciiErr) {
		cpic_err := models.CopyDir(courseImages, outputDir+"/"+PUBLIC_DIR)
		if cpic_err != nil {
			log.Fatal(cpic_err)
		}
	}

	return nil
}

func (scg SlidevCourseGenerator) GetPublicDir() string {
	return PUBLIC_DIR
}
