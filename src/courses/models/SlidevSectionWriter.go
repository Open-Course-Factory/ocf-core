package models

import (
	"log"
	"strings"
)

type SlidevSectionWriter struct {
	Section Section
	Chapter Chapter
}

func (ssw *SlidevSectionWriter) SetFrontMatter() string {
	frontMatter := "\n---\nlayout: cover\nchapter: " + ssw.Chapter.Title + "\n---\n\n"
	return frontMatter
}

func (ssw *SlidevSectionWriter) SetTitle() string {
	return "# " + strings.ToUpper(ssw.Chapter.Title) + "\n\n"
}

func (ssw *SlidevSectionWriter) SetToc() string {
	var toc string
	if len(ssw.Section.Pages) < 1 {
		log.Default().Println("Page should not be empty")
	} else {
		for _, lineOfToc := range ssw.Section.Pages[0].Toc {
			toc += "- " + lineOfToc + "\n"
		}
	}
	toc = toc + "\n"
	return toc
}

func (ssw *SlidevSectionWriter) SetContent() string {
	var pages string
	for _, page := range ssw.Section.Pages {
		pages += page.String(ssw.Section, ssw.Chapter) + "\n"
	}
	return pages
}

func (ssw *SlidevSectionWriter) GetSection() string {
	return ssw.SetFrontMatter() + ssw.SetTitle() + ssw.SetToc() + ssw.SetContent()
}
