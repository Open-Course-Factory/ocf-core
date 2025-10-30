package models

import (
	"strings"
)

type MarpPageWriter struct {
	Page    Page
	Section Section
}

func (mpw *MarpPageWriter) SetFrontMatter() string {
	frontMatter := "---\n\n"

	if mpw.Page.Hide {
		frontMatter += "<!-- _hide: true -->\n\n"
		frontMatter += "<!-- _paginate: skip -->\n\n"
	}
	return frontMatter
}

func (mpw *MarpPageWriter) SetTitle() string {
	title := "## " + strings.ToUpper(mpw.Section.Title) + "\n\n"
	return title
}

func (mpw *MarpPageWriter) SetToc() string {
	var tocBuilder strings.Builder
	tocBuilder.WriteString("<div class=\"toc\">\n\n")
	for _, lineOfToc := range mpw.Page.Toc {
		tocBuilder.WriteString("- ")
		tocBuilder.WriteString(lineOfToc)
		tocBuilder.WriteString("\n")
	}
	tocBuilder.WriteString("\n</div>\n\n")
	return tocBuilder.String()
}

func (mpw *MarpPageWriter) SetContent() string {
	var contentBuilder strings.Builder
	for _, line := range mpw.Page.Content {
		contentBuilder.WriteString(line)
		contentBuilder.WriteString("\n")
	}
	return contentBuilder.String()
}

func (mpw *MarpPageWriter) GetPage() string {
	return mpw.SetFrontMatter() + mpw.SetToc() + mpw.SetTitle() + mpw.SetContent()
}
