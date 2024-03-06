package models

import (
	"strings"
)

type MarpSectionWriter struct {
	Section Section
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
	var toc string
	for _, lineOfToc := range msw.Section.Pages[0].Toc {
		toc += lineOfToc + "\n"
	}
	toc = toc + "\n"
	return toc
}

func (msw *MarpSectionWriter) SetContent() string {
	var pages string
	for _, page := range msw.Section.Pages {
		pages += page.String() + "\n"
	}
	return pages
}

func (msw *MarpSectionWriter) GetSection() string {
	return msw.SetFrontMatter() + msw.SetTitle() + msw.SetToc() + msw.SetContent()
}
