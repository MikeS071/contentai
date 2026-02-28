package kb

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type ParsedFeed struct {
	Title string
	URL   string
	Items []FeedItem
}

type FeedItem struct {
	Title       string
	URL         string
	PublishedAt time.Time
	Content     string
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Link  string    `xml:"link"`
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title          string `xml:"title"`
	Link           string `xml:"link"`
	PubDate        string `xml:"pubDate"`
	Description    string `xml:"description"`
	ContentEncoded string `xml:"encoded"`
}

type atomDocument struct {
	Title   string      `xml:"title"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

var (
	tagPattern       = regexp.MustCompile(`(?is)<[^>]*>`)
	multiBlankLineRE = regexp.MustCompile(`\n{3,}`)
)

func ParseFeed(data []byte, baseURL string) (*ParsedFeed, error) {
	root, err := rootElementName(data)
	if err != nil {
		return nil, err
	}

	switch root {
	case "rss":
		return parseRSS(data, baseURL)
	case "feed":
		return parseAtom(data, baseURL)
	default:
		return nil, fmt.Errorf("unsupported feed root element: %s", root)
	}
}

func parseRSS(data []byte, baseURL string) (*ParsedFeed, error) {
	var doc rssDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse RSS: %w", err)
	}
	parsed := &ParsedFeed{
		Title: strings.TrimSpace(doc.Channel.Title),
		URL:   resolveURL(baseURL, doc.Channel.Link),
		Items: make([]FeedItem, 0, len(doc.Channel.Items)),
	}
	for _, item := range doc.Channel.Items {
		content := strings.TrimSpace(item.ContentEncoded)
		if content == "" {
			content = item.Description
		}
		parsed.Items = append(parsed.Items, FeedItem{
			Title:       strings.TrimSpace(item.Title),
			URL:         resolveURL(baseURL, item.Link),
			PublishedAt: parsePublishedDate(item.PubDate),
			Content:     HTMLToMarkdown(content),
		})
	}
	return parsed, nil
}

func parseAtom(data []byte, baseURL string) (*ParsedFeed, error) {
	var doc atomDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse Atom: %w", err)
	}
	feedURL := ""
	for _, l := range doc.Links {
		if l.Rel == "" || l.Rel == "alternate" || feedURL == "" {
			feedURL = l.Href
		}
	}

	parsed := &ParsedFeed{
		Title: strings.TrimSpace(doc.Title),
		URL:   resolveURL(baseURL, feedURL),
		Items: make([]FeedItem, 0, len(doc.Entries)),
	}
	for _, entry := range doc.Entries {
		entryURL := ""
		for _, l := range entry.Links {
			if l.Rel == "" || l.Rel == "alternate" || entryURL == "" {
				entryURL = l.Href
			}
		}
		content := strings.TrimSpace(entry.Content)
		if content == "" {
			content = entry.Summary
		}

		pub := parsePublishedDate(entry.Published)
		if pub.IsZero() {
			pub = parsePublishedDate(entry.Updated)
		}

		parsed.Items = append(parsed.Items, FeedItem{
			Title:       strings.TrimSpace(entry.Title),
			URL:         resolveURL(baseURL, entryURL),
			PublishedAt: pub,
			Content:     HTMLToMarkdown(content),
		})
	}
	return parsed, nil
}

func HTMLToMarkdown(input string) string {
	s := html.UnescapeString(input)
	replacer := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"</p>", "\n", "</div>", "\n", "</li>", "\n",
		"<li>", "- ",
	)
	s = replacer.Replace(s)
	s = tagPattern.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	s = strings.TrimSpace(strings.Join(lines, "\n"))
	s = multiBlankLineRE.ReplaceAllString(s, "\n\n")
	return s
}

func rootElementName(data []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", errors.New("empty XML document")
			}
			return "", fmt.Errorf("read XML root: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local, nil
		}
	}
}

func parsePublishedDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 02 Jan 2006 15:04:05 MST",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func resolveURL(baseURL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	if r.IsAbs() {
		return r.String()
	}
	b, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}
