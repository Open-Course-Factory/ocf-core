package generator

import (
	"os/exec"
	config "soli/formations/src/configuration"
	"soli/formations/src/courses/models"
)

type Option string

type CourseGenerationEngine interface {
	GetThemesSetOpts(course *models.Course) []string
	GetCmd(course *models.Course, docType *string) *exec.Cmd
	Run(configuration *config.Configuration, course *models.Course, docType *string) error
	CompileResources(c *models.Course, configuration *config.Configuration) error
	GetPublicDir() string
}

var SLIDE_ENGINE CourseGenerationEngine

type SlideEngine int
