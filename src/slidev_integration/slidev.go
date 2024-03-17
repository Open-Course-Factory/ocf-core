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

const DOCKER_IMAGE = "marpteam/marp-cli"
const ENGINE_CONFIGURATION = "./src/marp_integration/engine/engine.js"

func (o Option) GetTypeOpts() []string {
	var res []string
	switch o {
	case HTML:
		res = []string{"--bespoke.progress", "true", "--html"}
	case PDF:
		res = []string{"--pdf", "--allow-local-files", "--html", "--pdf-notes"}
	}

	return res
}

var (
	capabilitiesMap = map[string]Option{
		"html": HTML,
		"pdf":  PDF,
	}
)

func ParseDocType(str string) Option {
	c := capabilitiesMap[strings.ToLower(str)]
	return c
}

func GetThemesSetOpts(course *models.Course) []string {
	options := make([]string, 0)
	options = append(options, "--theme-set")
	for _, t := range course.GetThemes() {
		options = append(options, config.THEMES_ROOT+t+"/"+t+".scss")
	}
	return options
}

func GetCmd(course *models.Course, docType *string) *exec.Cmd {
	// cUser, errc := user.Current()
	// if errc != nil {
	// 	log.Fatal(errc)
	// }

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	outputDir := config.COURSES_OUTPUT_DIR + course.Theme
	srcFile := outputDir + "/" + course.GetFilename("md")
	destFile := course.GetFilename(*docType)

	// baseCmd := []string{"run", "--rm", "--init", "-e", "MARP_USER=" + cUser.Uid + ":" + cUser.Gid, "-v", pwd + ":/home/marp/app", DOCKER_IMAGE}
	// cmdOptions := []string{srcFile, "-o", destFile, "--no-config", "--theme", course.Theme}
	// cmdOptions = append(cmdOptions, GetThemesSetOpts(course)...)
	// cmdOptions = append(cmdOptions, []string{"--engine", ENGINE_CONFIGURATION}...)
	// cmdOptions = append(cmdOptions, ParseDocType(*docType).GetTypeOpts()...)

	//cmdFull := append(baseCmd, cmdOptions...)
	baseCmd := []string{"run", "--rm", "-e", `NPM_MIRROR="https://registry.npmmirror.com"`, "--entrypoint", "/entrypoint.sh", "-v", pwd + ":/slidev", "-v", "./src/slidev_integration/entrypoint.sh:/entrypoint.sh", "ocf_slidev", srcFile, "--output", destFile}
	//cmdFull := append(cmdFull, "docker run --name slidev_export --entrypoint /entrypoint.sh  -dit -v ./entrypoint.sh:/entrypoint.sh -v ${PWD}:/slidev -e NPM_MIRROR=\"https://registry.npmmirror.com\" tangramor/slidev:playwright")

	cmd := exec.Command("/usr/bin/docker", baseCmd...)

	return cmd
}

func Run(configuration *config.Configuration, course *models.Course, docType *string) error {
	cmd := GetCmd(course, docType)

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

func CompileResources(c *models.Course, configuration *config.Configuration) error {
	outputDir := config.COURSES_OUTPUT_DIR + c.Theme
	outputFolders := [2]string{"images", "theme"}

	for _, f := range outputFolders {
		err := os.MkdirAll(outputDir+"/"+f, os.ModePerm)
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
		cpiErr := models.CopyDir(config.IMAGES_ROOT, outputDir+"/images")
		if cpiErr != nil {
			log.Fatal(cpiErr)
		}
	}

	// Copy course specifique images
	courseImages := config.COURSES_ROOT + c.Category + "/images"
	if _, ciiErr := os.Stat(courseImages); !os.IsNotExist(ciiErr) {
		cpic_err := models.CopyDir(courseImages, outputDir+"/images")
		if cpic_err != nil {
			log.Fatal(cpic_err)
		}
	}

	return nil
}
