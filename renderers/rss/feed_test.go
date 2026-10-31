package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/japanese"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestRSS2(t *testing.T) {
	f, err := parseFeed(fixture(t, "rss2.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "合成ニュース" || len(f.Articles) != 5 {
		t.Fatalf("unexpected model: %+v", f)
	}
	if len(f.Articles[0].Summary) != 0 || f.Articles[0].URL != "https://news.example.com/first?tracking=keep" {
		t.Fatal("headline-only item or URL changed")
	}
	a := f.Articles[1]
	if len(a.Summary) != 2 || a.Summary[0].Text != "合成した日本語の要約です。" || a.Summary[1].Text != "配信された改行と省略…" || a.Category != "科学" {
		t.Fatalf("summary: %+v", a)
	}
	if !f.Articles[0].Date.After(a.Date) {
		t.Fatal("dates or delivery order changed")
	}
	if f.Articles[2].Summary[0].Text != "Fallback & text." || !f.Articles[2].Summary[1].Bullet {
		t.Fatal("encoded fallback or bullet boundaries lost")
	}
	if f.Articles[3].Title != f.Articles[4].Title || !f.Articles[4].Date.IsZero() {
		t.Fatal("identifier-free items or invalid dates changed")
	}
}
func TestRDFOrderAndNamespaces(t *testing.T) {
	data := fixture(t, "rss1.xml")
	renamed := bytes.ReplaceAll(data, []byte("r:"), []byte("alternate:"))
	renamed = bytes.ReplaceAll(renamed, []byte("xmlns:r="), []byte("xmlns:alternate="))
	for _, data := range [][]byte{data, renamed} {
		f, err := parseFeed(data)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"urn:second", "urn:first", "urn:third"}
		for i, id := range want {
			if f.Articles[i].ID != id {
				t.Fatalf("order: %+v", f.Articles)
			}
		}
		if f.Articles[1].Date.IsZero() {
			t.Fatal("Dublin Core date missing")
		}
	}
	// Without a channel Seq, preserve document order despite taxonomy sequences.
	noSeq := bytes.Replace(data, []byte("<items><r:Seq>"), []byte("<other><r:Seq>"), 1)
	noSeq = bytes.Replace(noSeq, []byte("</r:Seq></items>"), []byte("</r:Seq></other>"), 1)
	f, err := parseFeed(noSeq)
	if err != nil || f.Articles[0].ID != "urn:first" {
		t.Fatalf("document order: %+v %v", f, err)
	}
}
func TestJapaneseEncodings(t *testing.T) {
	for _, test := range []struct {
		name   string
		encode func(string) (string, error)
	}{
		{"Shift_JIS", japanese.ShiftJIS.NewEncoder().String},
		{"EUC-JP", japanese.EUCJP.NewEncoder().String},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := fmt.Sprintf(`<?xml version="1.0" encoding="%s"?><rss version="2.0"><channel><title>日本語</title><item><title>合成記事</title><description>概要です。</description></item></channel></rss>`, test.name)
			encoded, err := test.encode(source)
			if err != nil {
				t.Fatal(err)
			}
			f, err := parseFeed([]byte(encoded))
			if err != nil {
				t.Fatal(err)
			}
			if f.Title != "日本語" || f.Articles[0].Summary[0].Text != "概要です。" {
				t.Fatalf("decoded: %+v", f)
			}
		})
	}
}
func TestInvalidAndBoundedFeeds(t *testing.T) {
	for name, input := range map[string]string{
		"atom":   `<feed xmlns="http://www.w3.org/2005/Atom"/>`,
		"broken": `<rss version="2.0"><channel>`,
		"empty":  "", "extra-root": `<rss version="2.0"><channel/></rss><extra/>`,
		"doctype":     `<!DOCTYPE rss SYSTEM "https://example.com/never-fetch"><rss version="2.0"><channel/></rss>`,
		"entity":      `<rss version="2.0"><channel><title>&external;</title></channel></rss>`,
		"no-channel":  `<rss version="2.0"/>`,
		"all-invalid": `<rss version="2.0"><channel><item><title> </title></item></channel></rss>`,
		"deep":        strings.Repeat("<a>", 65) + strings.Repeat("</a>", 65),
		"large":       strings.Repeat(" ", feedLimit+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseFeed([]byte(input)); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
	f, err := parseFeed(fixture(t, "empty.xml"))
	if err != nil || len(f.Articles) != 0 {
		t.Fatal("valid empty feed rejected")
	}
	items := strings.Repeat(`<item><title>Repeated title, no identifier</title></item>`, itemLimit+10)
	f, err = parseFeed([]byte(`<rss version="2.0"><channel>` + items + `</channel></rss>`))
	if err != nil || len(f.Articles) != itemLimit {
		t.Fatalf("item limit: %d %v", len(f.Articles), err)
	}
}
func TestHTMLTextSafety(t *testing.T) {
	source := `<p onclick="bad()">One &amp; &lt;two&gt; &amp;lt;three&amp;gt;<br>Second</p><ul><li>A <b>bold</b> item</li><li>Next</li></ul><script>bad-script</script><style>bad-style</style><svg>bad-svg</svg><iframe>bad-frame</iframe>`
	blocks := summaryHTML(source)
	if len(blocks) != 4 || blocks[0].Text != "One & <two> &lt;three&gt;" || blocks[2].Text != "A bold item" || !blocks[2].Bullet {
		t.Fatalf("text: %+v", blocks)
	}
	f := feed{Title: "<evil>", Articles: []article{{Title: "<img src='https://example.com/a' onerror='bad()'>", Summary: blocks, Date: parseDate("2026-10-31T02:00:00Z")}}}
	location, _ := time.LoadLocation("Asia/Tokyo")
	body, err := renderHTML(f, "", 8, true, location)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, bad := range []string{"bad-script", "bad-style", "bad-svg", "bad-frame", "<img ", "onclick="} {
		if strings.Contains(text, bad) {
			t.Fatalf("unsafe content: %s", bad)
		}
	}
	for _, want := range []string{"&lt;evil&gt;", "&lt;img", "10/31 11:00", "One &amp; &lt;two&gt; &amp;lt;three&amp;gt;"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing escaped content %s", want)
		}
	}
	body, err = renderHTML(f, "Override", 1, false, location)
	if err != nil || strings.Contains(string(body), "A bold item") || !strings.Contains(string(body), "Override") {
		t.Fatal("summary toggle or title override failed")
	}
}

func TestTextAndComplexityLimits(t *testing.T) {
	description := strings.Repeat("<p>合成段落。</p>", 10000)
	blocks := summaryHTML(description)
	if len(blocks) != 32 {
		t.Fatalf("unbounded summary: %d blocks", len(blocks))
	}
	blocks = summaryHTML(strings.Repeat("あ", 7000))
	if len([]rune(blocks[0].Text)) != 6000 {
		t.Fatal("summary character limit failed")
	}
	fragmented := strings.Repeat("<![CDATA[あ]]>", 20000)
	f, err := parseFeed([]byte(`<rss version="2.0"><channel><item><title>` + fragmented + `</title><link>javascript:bad()</link></item></channel></rss>`))
	if err != nil || len([]rune(f.Articles[0].Title)) != 1024 || f.Articles[0].URL != "" {
		t.Fatalf("fragmented title or URL validation: %v", err)
	}
	if _, err := parseXML([]byte("<root>" + strings.Repeat("<n/>", 50000) + "</root>")); err == nil {
		t.Fatal("XML element limit was ignored")
	}
}
