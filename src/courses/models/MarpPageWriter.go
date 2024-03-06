package models

import (
	"strings"
)

type MarpPageWriter struct {
	Page Page
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
	title := "## " + strings.ToUpper(mpw.Page.Section.Title) + "\n\n"
	return title
}

func (mpw *MarpPageWriter) SetToc() string {
	var toc string
	toc = toc + "<div class=\"toc\">\n\n"
	for _, lineOfToc := range mpw.Page.Toc {
		toc += "- " + lineOfToc + "\n"
	}
	toc = toc + "\n</div>\n\n"
	return toc
}

func (mpw *MarpPageWriter) SetContent() string {
	var content string
	for _, line := range mpw.Page.Content {
		content += line + "\n"
	}
	return content
}

func (mpw *MarpPageWriter) GetPage() string {
	return mpw.SetFrontMatter() + mpw.SetToc() + mpw.SetTitle() + mpw.SetContent()
}
