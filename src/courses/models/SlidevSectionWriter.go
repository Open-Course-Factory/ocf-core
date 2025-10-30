package models

import (
	"log"
	"sort"
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
	var tocBuilder strings.Builder
	if len(ssw.Section.Pages) < 1 {
		log.Default().Println("Page should not be empty")
	} else {
		for _, lineOfToc := range ssw.Section.Pages[0].Toc {
			tocBuilder.WriteString("- ")
			tocBuilder.WriteString(lineOfToc)
			tocBuilder.WriteString("\n")
		}
	}
	tocBuilder.WriteString("\n")
	return tocBuilder.String()
}

func (ssw *SlidevSectionWriter) SetContent() string {
	var pagesBuilder strings.Builder

	// Sort the pages by page.Order
	sort.Slice(ssw.Section.Pages, func(i, j int) bool {
		return ssw.Section.Pages[i].Order < ssw.Section.Pages[j].Order
	})

	for _, page := range ssw.Section.Pages {
		pagesBuilder.WriteString(page.String(ssw.Section, ssw.Chapter))
		pagesBuilder.WriteString("\n")
	}
	return pagesBuilder.String()
}

func (ssw *SlidevSectionWriter) GetSection() string {
	return ssw.SetFrontMatter() + ssw.SetTitle() + ssw.SetToc() + ssw.SetContent()
}
