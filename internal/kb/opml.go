package kb

import (
	"encoding/xml"
	"fmt"
	"strings"
)

type OPMLFeed struct {
	Title string
	URL   string
}

type opmlDocument struct {
	Body opmlBody `xml:"body"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	Outlines []opmlOutline `xml:"outline"`
}

func ParseOPML(data []byte) ([]OPMLFeed, error) {
	var doc opmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse OPML: %w", err)
	}
	feeds := make([]OPMLFeed, 0)
	for _, outline := range doc.Body.Outlines {
		collectOPML(outline, &feeds)
	}
	return feeds, nil
}

func collectOPML(node opmlOutline, out *[]OPMLFeed) {
	if strings.TrimSpace(node.XMLURL) != "" {
		title := strings.TrimSpace(node.Title)
		if title == "" {
			title = strings.TrimSpace(node.Text)
		}
		*out = append(*out, OPMLFeed{
			Title: title,
			URL:   strings.TrimSpace(node.XMLURL),
		})
	}
	for _, child := range node.Outlines {
		collectOPML(child, out)
	}
}
