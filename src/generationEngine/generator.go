package generator

import (
	"os/exec"
	"soli/formations/src/courses/models"
)

type Option string

type CourseGenerationEngine interface {
	GetThemesSetOpts(course *models.Course) []string
	GetCmd(course *models.Course) (*exec.Cmd, error)
	Run(course *models.Course) error
	CompileResources(c *models.Course) error
	GetPublicDir() string
	ExportPDF(course *models.Course) error
}

var SLIDE_ENGINE CourseGenerationEngine

type SlideEngine int
