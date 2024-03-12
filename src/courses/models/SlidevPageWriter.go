package models

import (
	"strings"
)

type SlidevPageWriter struct {
	Page Page
}

func (spw *SlidevPageWriter) SetFrontMatter() string {
	frontMatter := "---\nchapter: " + spw.Page.Section.Chapter.Title + "\n---\n\n"

	if spw.Page.Hide {
		frontMatter += "<!-- _hide: true -->\n\n"
		frontMatter += "<!-- _paginate: skip -->\n\n"
	}
	return frontMatter
}

func (spw *SlidevPageWriter) SetTitle() string {
	title := "## " + strings.ToUpper(spw.Page.Section.Title) + "\n\n"
	return title
}

func (spw *SlidevPageWriter) SetToc() string {
	var toc string
	toc = toc + "<div class=\"toc\">\n\n"
	for _, lineOfToc := range spw.Page.Toc {
		toc += "- " + lineOfToc + "\n"
	}
	toc = toc + "\n</div>\n\n"
	return toc
}

func (spw *SlidevPageWriter) SetContent() string {
	var content string
	for _, line := range spw.Page.Content {
		content += line + "\n"
	}
	return content
}

func (spw *SlidevPageWriter) GetPage() string {
	return spw.SetFrontMatter() + spw.SetToc() + spw.SetTitle() + spw.SetContent()
}
