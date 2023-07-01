package models

import "strings"

// Part of a Section
type Page struct {
	Number             int
	ParentSectionTitle string
	Toc                []string
	Content            []string
	Hide               bool
}

func (p Page) String() string {
	firstLine := "---\n\n"

	title := "## " + strings.ToUpper(p.ParentSectionTitle) + "\n\n"
	var toc string
	toc = toc + "<div class=\"toc\">\n\n"
	for _, lineOfToc := range p.Toc {
		toc += "- " + lineOfToc + "\n"
	}
	toc = toc + "\n</div>\n\n"

	var content string
	for _, line := range p.Content {
		content += line + "\n"
	}

	if p.Hide {
		firstLine += "<!-- _hide: true -->\n\n"
		firstLine += "<!-- _paginate: skip -->\n\n"
	}

	return firstLine + toc + title + content
}

func createPage(number int, pageContent []string, parentSectionTitle string, hide bool) (p Page) {
	p.Number = number
	p.Content = pageContent
	p.ParentSectionTitle = parentSectionTitle
	p.Hide = hide
	return
}
