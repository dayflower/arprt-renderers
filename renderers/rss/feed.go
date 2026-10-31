package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

const (
	feedLimit = 4 << 20
	itemLimit = 200
	rssNS     = "http://purl.org/rss/1.0/"
	rdfNS     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	dcNS      = "http://purl.org/dc/elements/1.1/"
	hatenaNS  = "http://www.hatena.ne.jp/info/xmlns#"
	contentNS = "http://purl.org/rss/1.0/modules/content/"
)

type xmlNode struct {
	Name     xml.Name
	Attr     []xml.Attr
	Text     string
	Children []*xmlNode
	text     strings.Builder
}

func (n *xmlNode) child(space, local string) *xmlNode {
	if n != nil {
		for _, c := range n.Children {
			if c.Name.Space == space && c.Name.Local == local {
				return c
			}
		}
	}
	return nil
}
func (n *xmlNode) value(space, local string) string {
	if c := n.child(space, local); c != nil {
		return c.Text
	}
	return ""
}
func (n *xmlNode) attr(space, local string) string {
	for _, a := range n.Attr {
		if a.Name.Space == space && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// The bounded tree preserves namespace-qualified extensions for feed adapters.
func parseXML(data []byte) (*xmlNode, error) {
	if len(data) > feedLimit {
		return nil, errors.New("feed exceeds size limit")
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	d.CharsetReader = charset.NewReaderLabel
	var root *xmlNode
	var stack []*xmlNode
	nodes := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid feed XML or unsupported encoding")
		}
		switch t := token.(type) {
		case xml.StartElement:
			nodes++
			if len(stack) >= 64 || nodes > 50000 {
				return nil, errors.New("feed XML exceeds complexity limit")
			}
			n := &xmlNode{Name: t.Name, Attr: t.Attr}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("multiple XML roots")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			n := stack[len(stack)-1]
			n.Text = n.text.String()
			n.text = strings.Builder{}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				n := stack[len(stack)-1]
				n.text.Write(t)
			} else if strings.TrimSpace(string(t)) != "" {
				return nil, errors.New("text outside XML root")
			}
		case xml.Directive:
			// No DTDs, entities, or external resources are needed for RSS.
			return nil, errors.New("XML directives are unsupported")
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, errors.New("empty or incomplete XML")
	}
	return root, nil
}

type summaryBlock struct {
	Text   string
	Bullet bool
}
type article struct {
	ID, Title, URL, Source, Category  string
	ImageURL, ImageData, SummaryLabel string
	BookmarkCount                     *uint64
	Summary                           []summaryBlock
	Date                              time.Time
	Raw                               *xmlNode
}
type feed struct {
	Title    string
	Profile  string
	Articles []article
}

func clip(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}
func compact(s string) string { return strings.Join(strings.Fields(s), " ") }
func httpURL(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.String()
}

// Only text crosses into templates; feed markup and attributes are discarded.
func summaryHTML(s string) []summaryBlock {
	return summaryHTMLMode(s, false)
}
func summaryHTMLMode(s string, trimCTA bool) []summaryBlock {
	doc, err := html.Parse(strings.NewReader(clip(s, 65536)))
	if err != nil {
		return nil
	}
	if trimCTA {
		removeTrailingCTA(doc)
	}
	var blocks []summaryBlock
	var current strings.Builder
	bullet := false
	budget := 6000
	flush := func() {
		if len(blocks) < 32 && budget > 0 {
			if text := compact(current.String()); text != "" {
				text = clip(text, budget)
				blocks = append(blocks, summaryBlock{Text: text, Bullet: bullet})
				budget -= utf8.RuneCountInString(text)
			}
		}
		current.Reset()
	}
	var walk func(*html.Node, int)
	walk = func(n *html.Node, depth int) {
		if depth > 64 || len(blocks) >= 32 || budget == 0 {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "iframe", "object", "embed", "svg", "math", "template", "noscript", "head":
				return
			}
			if n.Data == "br" {
				flush()
				return
			}
			boundary := n.Data == "p" || n.Data == "div" || n.Data == "li" || n.Data == "ul" || n.Data == "ol" || n.Data == "section" || n.Data == "h1" || n.Data == "h2"
			previous := bullet
			if boundary {
				flush()
			}
			if n.Data == "li" {
				bullet = true
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, depth+1)
			}
			if boundary {
				flush()
			}
			bullet = previous
			return
		}
		if n.Type == html.TextNode {
			// Raw feed newlines remain paragraph boundaries, including plain descriptions.
			lines := strings.Split(n.Data, "\n")
			for i, line := range lines {
				if i > 0 {
					flush()
				}
				current.WriteString(line)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, depth+1)
		}
	}
	walk(doc, 0)
	flush()
	return blocks
}
func plain(s string, limit int) string {
	blocks := summaryHTML(s)
	values := make([]string, 0, len(blocks))
	for _, b := range blocks {
		values = append(values, b.Text)
	}
	return clip(strings.Join(values, " "), limit)
}
func parseDate(s string) time.Time {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339Nano, "2006-01-02T15:04:05-0700", "2006-01-02"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseFeed(data []byte) (feed, error) {
	return parseFeedProfile(data, "generic", "")
}

func parseFeedProfile(data []byte, profile, origin string) (feed, error) {
	var result feed
	if !validProfile(profile) {
		return result, errors.New("unknown feed profile")
	}
	root, err := parseXML(data)
	if err != nil {
		return result, err
	}
	var channel *xmlNode
	var items []*xmlNode
	space := ""
	switch root.Name {
	case xml.Name{Local: "rss"}:
		if root.attr("", "version") != "2.0" {
			return result, errors.New("expected RSS 2.0")
		}
		channel = root.child("", "channel")
		if channel != nil {
			for _, n := range channel.Children {
				if n.Name == (xml.Name{Local: "item"}) {
					items = append(items, n)
				}
			}
		}
	case xml.Name{Space: rdfNS, Local: "RDF"}:
		space = rssNS
		channel = root.child(space, "channel")
		for _, n := range root.Children {
			if n.Name == (xml.Name{Space: space, Local: "item"}) {
				items = append(items, n)
			}
		}
		// Only channel/items/Seq controls order, never a taxonomy Seq.
		if channel != nil {
			if container := channel.child(space, "items"); container != nil {
				if seq := container.child(rdfNS, "Seq"); seq != nil {
					byID := map[string]*xmlNode{}
					for _, n := range items {
						id := n.attr(rdfNS, "about")
						if _, exists := byID[id]; id != "" && !exists {
							byID[id] = n
						}
					}
					ordered := make([]*xmlNode, 0, len(items))
					used := map[*xmlNode]bool{}
					for _, ref := range seq.Children {
						if ref.Name != (xml.Name{Space: rdfNS, Local: "li"}) {
							continue
						}
						n := byID[ref.attr(rdfNS, "resource")]
						if n != nil && !used[n] {
							ordered = append(ordered, n)
							used[n] = true
						}
					}
					for _, n := range items {
						if !used[n] {
							ordered = append(ordered, n)
						}
					}
					items = ordered
				}
			}
		}
	default:
		return result, errors.New("unsupported feed format; expected RSS 2.0 or RSS 1.0/RDF (Atom is unsupported)")
	}
	if channel == nil {
		return result, errors.New("missing RSS channel")
	}
	result.Profile = selectProfile(profile, origin, space, items)
	result.Title = plain(channel.value(space, "title"), 256)
	if result.Title == "" {
		result.Title = "RSS feed"
	}
	count := len(items)
	if count > itemLimit {
		items = items[:itemLimit]
	}
	seen := map[string]bool{}
	for _, n := range items {
		title := plain(n.value(space, "title"), 1024)
		if title == "" {
			continue
		}
		a := article{Title: title, URL: httpURL(n.value(space, "link")), Raw: n}
		a.ID = strings.TrimSpace(n.value("", "guid"))
		if a.ID == "" {
			a.ID = n.attr(rdfNS, "about")
		}
		if a.ID == "" {
			a.ID = a.URL
		}
		if a.ID != "" {
			if seen[a.ID] {
				continue
			}
			seen[a.ID] = true
		}
		desc := n.child(space, "description")
		if desc == nil {
			desc = n.child(contentNS, "encoded")
		}
		if desc != nil {
			a.Summary = summaryHTML(desc.Text)
		}
		date := n.value(space, "pubDate")
		if date == "" {
			date = n.value(dcNS, "date")
		}
		a.Date = parseDate(date)
		a.Source = plain(n.value(space, "source"), 256)
		for _, c := range n.Children {
			if c.Name == (xml.Name{Space: space, Local: "category"}) {
				if v := plain(c.Text, 128); v != "" {
					a.Category = v
					break
				}
			}
		}
		normalizeArticle(&a, result.Profile, space)
		result.Articles = append(result.Articles, a)
	}
	if count > 0 && len(result.Articles) == 0 {
		return result, errors.New("feed has no valid titled items within the parsing limit")
	}
	return result, nil
}
