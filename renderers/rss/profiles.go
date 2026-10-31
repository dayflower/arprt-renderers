package main

import (
	"mime"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

func validProfile(profile string) bool {
	switch profile {
	case "auto", "generic", "hatena", "livedoor", "google-news":
		return true
	}
	return false
}

func selectProfile(profile, origin, space string, items []*xmlNode) string {
	if profile != "auto" {
		return profile
	}
	// Namespace-qualified item extensions identify mirrored Hatena feeds too.
	for _, n := range items {
		for _, c := range n.Children {
			if c.Name.Space == hatenaNS {
				return "hatena"
			}
		}
	}
	u, err := url.Parse(origin)
	if err != nil {
		return "generic"
	}
	host := strings.ToLower(u.Hostname())
	if host == "b.hatena.ne.jp" && space == rssNS {
		return "hatena"
	}
	if space == "" {
		for _, n := range items {
			if host == "news.google.com" && n.child("", "source") != nil {
				return "google-news"
			}
			if host == "news.livedoor.com" && n.child("", "description") != nil {
				return "livedoor"
			}
		}
	}
	return "generic"
}

func normalizeArticle(a *article, profile, space string) {
	n := a.Raw
	if profile == "generic" || profile == "livedoor" {
		for _, c := range n.Children {
			if c.Name.Space == space && c.Name.Local == "enclosure" {
				media, _, err := mime.ParseMediaType(c.attr("", "type"))
				if err == nil && strings.HasPrefix(media, "image/") {
					if candidate := httpURL(c.attr("", "url")); candidate != "" {
						a.ImageURL = candidate
						break
					}
				}
			}
		}
	}
	switch profile {
	case "hatena":
		a.Summary = summaryHTML(n.value(space, "description"))
		a.ImageURL = httpURL(n.value(hatenaNS, "imageurl"))
		a.BookmarkCount = nil
		count := strings.TrimSpace(n.value(hatenaNS, "bookmarkcount"))
		// ParseUint rejects negative values; require digits to reject a leading plus.
		if count != "" && strings.Trim(count, "0123456789") == "" {
			if value, err := strconv.ParseUint(count, 10, 64); err == nil {
				a.BookmarkCount = &value
			}
		}
		a.Category = ""
		for _, c := range n.Children {
			if c.Name.Space == dcNS && c.Name.Local == "subject" {
				if text := plain(c.Text, 128); text != "" {
					a.Category = text
					break
				}
			}
		}
		a.Date = parseDate(n.value(dcNS, "date"))
	case "livedoor":
		if desc := n.child(space, "description"); desc != nil {
			a.Summary = summaryHTMLMode(desc.Text, true)
		}
		hasBullet := false
		for _, b := range a.Summary {
			hasBullet = hasBullet || b.Bullet
		}
		if hasBullet {
			blocks := a.Summary[:0]
			for _, b := range a.Summary {
				if !b.Bullet && b.Text == "ざっくり言うと" {
					a.SummaryLabel = b.Text
					continue
				}
				blocks = append(blocks, b)
			}
			a.Summary = blocks
		}
	case "google-news":
		a.Summary = nil
		if a.Source != "" {
			suffix := " - " + a.Source
			if strings.HasSuffix(a.Title, suffix) {
				if title := strings.TrimSuffix(a.Title, suffix); strings.TrimSpace(title) != "" {
					a.Title = title
				}
			}
		}
	}
}

func hiddenElement(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.Data {
	case "script", "style", "iframe", "object", "embed", "svg", "math", "template", "noscript", "head":
		return true
	}
	return false
}

// Remove only a final standalone anchor, retaining the same words in prose.
func removeTrailingCTA(doc *html.Node) {
	var last *html.Node
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if hiddenElement(n) {
			return
		}
		if n.Type == html.TextNode && compact(n.Data) != "" {
			last = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if last == nil {
		return
	}
	anchor := last.Parent
	for anchor != nil && !(anchor.Type == html.ElementNode && anchor.Data == "a") {
		anchor = anchor.Parent
	}
	if anchor == nil {
		return
	}
	var text strings.Builder
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if hiddenElement(n) {
			return
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(anchor)
	if compact(text.String()) != "記事を読む" {
		return
	}
	// A block start or explicit line break must separate the link from prose.
	for n := anchor; n.Parent != nil; n = n.Parent {
		for prev := n.PrevSibling; prev != nil; prev = prev.PrevSibling {
			if prev.Type == html.ElementNode && (prev.Data == "br" || prev.Data == "p" || prev.Data == "div" || prev.Data == "ul" || prev.Data == "ol" || prev.Data == "section") {
				anchor.Parent.RemoveChild(anchor)
				return
			}
			text.Reset()
			collect(prev)
			if compact(text.String()) != "" {
				return
			}
		}
		switch n.Parent.Data {
		case "p", "div", "li", "section", "body":
			anchor.Parent.RemoveChild(anchor)
			return
		}
	}
}
