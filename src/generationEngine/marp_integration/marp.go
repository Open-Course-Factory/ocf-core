package marp

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/models"
	"soli/formations/src/utils"
	"strings"
)

type Option string

const (
	HTML Option = "html"
	PDF  Option = "pdf"
)

const PUBLIC_DIR string = "images"

type MarpCourseGenerator struct {
}

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

func (mcg MarpCourseGenerator) ParseDocType(str string) Option {
	c := capabilitiesMap[strings.ToLower(str)]
	return c
}

func (mcg MarpCourseGenerator) GetThemesSetOpts(course *models.Course) []string {
	options := make([]string, 0)
	options = append(options, "--theme-set")
	for _, t := range course.GetThemes() {
		options = append(options, config.THEMES_ROOT+t+"/"+t+".scss")
	}
	return options
}

func (mcg MarpCourseGenerator) GetCmd(course *models.Course) (*exec.Cmd, error) {
	cUser, errc := user.Current()
	if errc != nil {
		utils.Error("failed to get current user: %v", errc)
		return nil, fmt.Errorf("failed to get current user: %w", errc)
	}

	pwd, err := os.Getwd()
	if err != nil {
		utils.Error("failed to get working directory: %v", err)
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	outputDir := config.COURSES_OUTPUT_DIR + course.Theme.Name
	srcFile := outputDir + "/" + course.GetFilename("md")
	destFile := outputDir + "/" + course.GetFilename()

	baseCmd := []string{"run", "--rm", "--init", "-e", "MARP_USER=" + cUser.Uid + ":" + cUser.Gid, "-v", pwd + ":/home/marp/app", DOCKER_IMAGE}
	cmdOptions := []string{srcFile, "-o", destFile, "--no-config", "--theme", course.Theme.Name}
	cmdOptions = append(cmdOptions, mcg.GetThemesSetOpts(course)...)
	cmdOptions = append(cmdOptions, []string{"--engine", ENGINE_CONFIGURATION}...)
	cmdOptions = append(cmdOptions, mcg.ParseDocType("html").GetTypeOpts()...)

	cmdFull := append(baseCmd, cmdOptions...)

	cmd := exec.Command("/usr/bin/docker", cmdFull...)

	return cmd, nil
}

func (mcg MarpCourseGenerator) Run(course *models.Course) error {
	cmd, err := mcg.GetCmd(course)
	if err != nil {
		return err
	}

	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	utils.Info("Command ready to be executed: %s", cmd.String())

	err = cmd.Run()

	if err != nil {
		utils.Error("%s", err.Error())
	}

	errByte := errb.String()
	if len(errb.Bytes()) > 0 {
		utils.Error("%s", errByte)
	}

	utils.Info("%s", outb.String())

	return nil
}

func (mcg MarpCourseGenerator) CompileResources(c *models.Course) error {
	outputDir := config.COURSES_OUTPUT_DIR + c.Theme.Name
	outputFolders := [2]string{"images", "theme"}

	for _, f := range outputFolders {
		err := os.MkdirAll(outputDir+"/"+f, os.ModePerm)
		if err != nil {
			utils.Error("failed to create output directory %s: %v", outputDir+"/"+f, err)
			return fmt.Errorf("failed to create output directory %s: %w", outputDir+"/"+f, err)
		}
	}

	// Copy Themes
	for _, t := range c.GetThemes() {
		themeSrc := config.THEMES_ROOT + t
		cptErr := models.CopyDir(themeSrc, outputDir+"/theme")
		if cptErr != nil {
			utils.Error("failed to copy theme %s: %v", t, cptErr)
			return fmt.Errorf("failed to copy theme %s: %w", t, cptErr)
		}
	}

	// Copy global images
	if _, err := os.Stat(config.IMAGES_ROOT); !os.IsNotExist(err) {
		cpiErr := models.CopyDir(config.IMAGES_ROOT, outputDir+"/images")
		if cpiErr != nil {
			utils.Error("failed to copy global images: %v", cpiErr)
			return fmt.Errorf("failed to copy global images: %w", cpiErr)
		}
	}

	// Copy course specifique images
	courseImages := config.COURSES_ROOT + c.Category + "/images"
	if _, ciiErr := os.Stat(courseImages); !os.IsNotExist(ciiErr) {
		cpic_err := models.CopyDir(courseImages, outputDir+"/images")
		if cpic_err != nil {
			utils.Error("failed to copy course images: %v", cpic_err)
			return fmt.Errorf("failed to copy course images: %w", cpic_err)
		}
	}

	return nil
}

func (mcg MarpCourseGenerator) GetPublicDir() string {
	return PUBLIC_DIR
}

func (mcg MarpCourseGenerator) ExportPDF(course *models.Course) error {
	// PDF export is not currently supported for Marp
	// Marp has its own PDF generation mechanism
	return fmt.Errorf("PDF export not implemented for Marp engine")
}
