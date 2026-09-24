package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	config "soli/formations/src/configuration"
)

// TocLayout describes how much of the table of contents fits on one slide.
// Pagination is based on an estimate of the rendered height of each chapter entry
// (title line + introduction), so long titles or introductions get fewer chapters per page.
type TocLayout struct {
	// LinesPerPage is the number of text lines available on a TOC slide (0 disables pagination)
	LinesPerPage int `json:"lines_per_page"`
	// CharsPerLine is the average number of characters per rendered line
	CharsPerLine int `json:"chars_per_line"`
}

// Default for 16:9 themes: 12 lines of 75 characters, i.e. 6 chapters whose title and introduction are one-liners.
var defaultTocLayout = TocLayout{LinesPerPage: 12, CharsPerLine: 75}

// loadTocLayout returns the TOC layout of a theme. A theme can override the defaults with a toc.json
// file at its root ({"lines_per_page": 12, "chars_per_line": 75}). A4 themes are not paginated.
func loadTocLayout(themeName string) TocLayout {
	layout := defaultTocLayout
	if strings.Contains(themeName, "A4") {
		layout.LinesPerPage = 0
	}
	if themeName == "" {
		return layout
	}
	for _, dir := range []string{config.COURSES_OUTPUT_DIR + themeName, config.THEMES_ROOT + themeName} {
		content, err := os.ReadFile(filepath.Join(dir, "toc.json"))
		if err != nil {
			continue
		}
		var custom TocLayout
		if json.Unmarshal(content, &custom) == nil {
			if custom.LinesPerPage >= 0 {
				layout.LinesPerPage = custom.LinesPerPage
			}
			if custom.CharsPerLine > 0 {
				layout.CharsPerLine = custom.CharsPerLine
			}
		}
		break
	}
	return layout
}

// textLines estimates how many rendered lines a text takes.
func (l TocLayout) textLines(text string) int {
	count := utf8.RuneCountInString(strings.TrimSpace(text))
	if count == 0 {
		return 0
	}
	cpl := l.CharsPerLine
	if cpl <= 0 {
		cpl = defaultTocLayout.CharsPerLine
	}
	return (count + cpl - 1) / cpl
}

// chapterLines estimates the height of a chapter entry: "Chapitre N : title" plus its introduction.
func (l TocLayout) chapterLines(chapter *Chapter) int {
	title := "Chapitre " + strconv.Itoa(chapter.Number) + " : " + chapter.Title
	lines := l.textLines(title)
	if lines == 0 {
		lines = 1
	}
	return lines + l.textLines(chapter.Introduction)
}
