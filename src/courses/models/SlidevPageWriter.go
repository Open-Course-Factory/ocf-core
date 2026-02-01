package models

import (
	"strings"
)

type SlidevPageWriter struct {
	Page    Page
	Section Section
	Chapter Chapter
}

func (spw *SlidevPageWriter) SetFrontMatter() string {
	frontMatter := "---\nchapter: " + spw.Chapter.Title + "\n"

	// Add class field if present
	if spw.Page.Class != "" {
		frontMatter += "class: " + spw.Page.Class + "\n"
	}

	frontMatter += "---\n\n"

	if spw.Page.Hide {
		frontMatter += "<!-- _hide: true -->\n\n"
		frontMatter += "<!-- _paginate: skip -->\n\n"
	}
	return frontMatter
}

func (spw *SlidevPageWriter) SetTitle() string {
	title := "## " + strings.ToUpper(spw.Section.Title) + "\n\n"
	return title
}

func (spw *SlidevPageWriter) SetToc() string {
	var tocBuilder strings.Builder
	tocBuilder.WriteString("<div class=\"toc\">\n\n")
	for _, lineOfToc := range spw.Page.Toc {
		tocBuilder.WriteString("- ")
		tocBuilder.WriteString(lineOfToc)
		tocBuilder.WriteString("\n")
	}
	tocBuilder.WriteString("\n</div>\n\n")
	return tocBuilder.String()
}

func (spw *SlidevPageWriter) SetContent() string {
	var contentBuilder strings.Builder
	for _, line := range spw.Page.Content {
		contentBuilder.WriteString(line)
		contentBuilder.WriteString("\n")
	}
	return contentBuilder.String()
}

func (spw *SlidevPageWriter) GetPage() string {
	return spw.SetFrontMatter() + spw.SetToc() + spw.SetTitle() + spw.SetContent()
}
