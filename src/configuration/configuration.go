package config

const COURSES_ROOT = "./courses/"
const COURSES_OUTPUT_DIR = "./dist/"

const THEMES_ROOT = "./themes/"
const IMAGES_ROOT = "./images/"

var DRY_RUN *bool

// Part of a Section
type Configuration struct {
	AuthorTrigram    string `json:"author_trigram"`
	AuthorFullname   string `json:"author_fullname"`
	AuthorEmail      string `json:"author_email"`
	SecretJwt        string `mapstructure:"SECRET_JWT"`
	SecretRefreshJwt string `mapstructure:"SECRET_REFRESH_JWT"`
	Worker           WorkerConfig
}

type Format int

const (
	HTML Format = iota
	PDF
)

func (s Format) String() string {
	switch s {
	case HTML:
		return "html"
	case PDF:
		return "pdf"
	}
	return "unknown"
}
