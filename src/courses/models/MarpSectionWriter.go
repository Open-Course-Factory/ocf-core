package models

import (
	"strings"
)

type MarpSectionWriter struct {
	Section Section
	Chapter Chapter
}

func (msw *MarpSectionWriter) SetFrontMatter() string {
	firstLine := "---\n\n"
	localClass := "<!-- _class: lead -->\n\n"
	return firstLine + localClass
}

func (msw *MarpSectionWriter) SetTitle() string {
	return "# " + strings.ToUpper(msw.Section.ParentChapterTitle) + "\n\n"
}

func (msw *MarpSectionWriter) SetToc() string {
	var tocBuilder strings.Builder
	for _, lineOfToc := range msw.Section.Pages[0].Toc {
		tocBuilder.WriteString(lineOfToc)
		tocBuilder.WriteString("\n")
	}
	tocBuilder.WriteString("\n")
	return tocBuilder.String()
}

func (msw *MarpSectionWriter) SetContent() string {
	var pagesBuilder strings.Builder
	for _, page := range msw.Section.Pages {
		pagesBuilder.WriteString(page.String(msw.Section, msw.Chapter))
		pagesBuilder.WriteString("\n")
	}
	return pagesBuilder.String()
}

func (msw *MarpSectionWriter) GetSection() string {
	return msw.SetFrontMatter() + msw.SetTitle() + msw.SetToc() + msw.SetContent()
}
