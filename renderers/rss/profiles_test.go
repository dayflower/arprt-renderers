package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func rssItems(items string) []byte {
	return []byte(`<rss version="2.0" xmlns:c="http://purl.org/rss/1.0/modules/content/"><channel><title>合成ニュース</title>` + items + `</channel></rss>`)
}

func TestProfileSelection(t *testing.T) {
	google := rssItems(`<item><title>見出し - 合成媒体</title><source>合成媒体</source><description>関連見出し</description></item>`)
	for _, test := range []struct{ profile, origin, want string }{
		{"auto", "https://news.google.com/rss", "google-news"},
		{"generic", "https://news.google.com/rss", "generic"},
		{"auto", "https://news.google.com.example.com/rss", "generic"},
		{"auto", "https://example.com/news.google.com", "generic"},
		{"auto", "", "generic"},
		{"google-news", "https://mirror.example.com/rss", "google-news"},
		{"auto", "https://news.livedoor.com/topics/rss/top.xml", "livedoor"},
		{"hatena", "", "hatena"}, {"livedoor", "", "livedoor"},
	} {
		f, err := parseFeedProfile(google, test.profile, test.origin)
		if err != nil || f.Profile != test.want {
			t.Fatalf("%+v: %s %v", test, f.Profile, err)
		}
	}
	for _, origin := range []string{"https://news.google.com/rss", "https://news.livedoor.com/rss"} {
		f, err := parseFeedProfile(rssItems(`<item><title>Google News はてな livedoor</title></item>`), "auto", origin)
		if err != nil || f.Profile != "generic" {
			t.Fatal("selected profile without required structure")
		}
	}
	if _, err := parseFeedProfile(google, "unknown", ""); err == nil {
		t.Fatal("unknown profile accepted")
	}
}

func TestHatenaMetadata(t *testing.T) {
	data := []byte(`<r:RDF xmlns:r="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:h="http://www.hatena.ne.jp/info/xmlns#" xmlns:d="http://purl.org/dc/elements/1.1/" xmlns:c="http://purl.org/rss/1.0/modules/content/">
 <channel><title>合成ニュース</title><items><r:Seq><r:li r:resource="urn:b"/><r:li r:resource="urn:a"/></r:Seq></items></channel>
 <item r:about="urn:a"><title>空の抜粋</title><description/><c:encoded>decorated fallback</c:encoded><h:bookmarkcount>0</h:bookmarkcount><h:imageurl>javascript:bad()</h:imageurl></item>
 <item r:about="urn:b"><title>長い見出し</title><description>抜粋です。</description><d:subject> </d:subject><d:subject>技術</d:subject><d:subject>除外カテゴリ</d:subject><d:date>2026-10-31T11:00:00+09:00</d:date><h:bookmarkcount>190</h:bookmarkcount><h:imageurl>https://images.example.com/thumbnail.png</h:imageurl></item>
 </r:RDF>`)
	for _, data := range [][]byte{data, bytes.ReplaceAll(bytes.ReplaceAll(data, []byte("h:"), []byte("alternate:")), []byte("xmlns:h="), []byte("xmlns:alternate="))} {
		f, err := parseFeedProfile(data, "auto", "")
		if err != nil {
			t.Fatal(err)
		}
		a, b := f.Articles[0], f.Articles[1]
		if f.Profile != "hatena" || a.ID != "urn:b" || a.Category != "技術" || a.Date.IsZero() || a.BookmarkCount == nil || *a.BookmarkCount != 190 || a.ImageURL != "https://images.example.com/thumbnail.png" || a.Summary[0].Text != "抜粋です。" {
			t.Fatalf("metadata: %+v", a)
		}
		if b.BookmarkCount == nil || *b.BookmarkCount != 0 || b.Category != "" || b.ImageURL != "" || len(b.Summary) != 0 {
			t.Fatalf("missing metadata: %+v", b)
		}
		body, err := renderHTML(f, "", 8, true, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"190 users", "0 users", "技術", "10/31 02:00"} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("missing %s", want)
			}
		}
		for _, bad := range []string{"decorated fallback", "除外カテゴリ", "<img", "https://images.example.com/thumbnail.png"} {
			if strings.Contains(string(body), bad) {
				t.Fatalf("unexpected %s", bad)
			}
		}
		generic, err := parseFeedProfile(data, "generic", "https://b.hatena.ne.jp/hotentry.rss")
		if err != nil || generic.Articles[0].BookmarkCount != nil || generic.Articles[0].Category != "" {
			t.Fatal("generic did not disable corrections")
		}
	}
	for _, value := range []string{"", "-1", "+1", "bad", "1.5", "18446744073709551616"} {
		data := rssItems(`<item xmlns:h="http://www.hatena.ne.jp/info/xmlns#"><title>Title</title><h:bookmarkcount>` + value + `</h:bookmarkcount></item>`)
		f, err := parseFeedProfile(data, "hatena", "")
		if err != nil || f.Articles[0].BookmarkCount != nil {
			t.Fatalf("invalid count %q", value)
		}
	}
}

func TestLivedoorSummaries(t *testing.T) {
	for _, test := range []struct {
		description string
		want        []summaryBlock
		label       string
	}{
		{`<a href="https://example.com/article"> 記事を読む </a>`, nil, ""},
		{`<p><a href="x"><b>記事を読む</b></a></p><p> </p>`, nil, ""},
		{`<p>通常の要約…</p><div><a href="x">記事を読む</a></div>`, []summaryBlock{{Text: "通常の要約…"}}, ""},
		{`要約<br><a href="x">記事を読む</a>`, []summaryBlock{{Text: "要約"}}, ""},
		{`<p>文章中で記事を読むこともある。</p>`, []summaryBlock{{Text: "文章中で記事を読むこともある。"}}, ""},
		{`<p>ここで<a href="x">記事を読む</a></p>`, []summaryBlock{{Text: "ここで記事を読む"}}, ""},
		{`<a href="x">記事を読む</a><p>後の文章</p>`, []summaryBlock{{Text: "記事を読む"}, {Text: "後の文章"}}, ""},
		{`<p>ざっくり言うと</p><ul><li>第一の要点</li><li>第二の要点</li><li>第三の要点</li></ul><a href="x">記事を読む</a>`, []summaryBlock{{Text: "第一の要点", Bullet: true}, {Text: "第二の要点", Bullet: true}, {Text: "第三の要点", Bullet: true}}, "ざっくり言うと"},
	} {
		data := rssItems(`<item><title>合成見出し</title><description><![CDATA[` + test.description + `]]></description></item>`)
		f, err := parseFeedProfile(data, "livedoor", "")
		if err != nil {
			t.Fatal(err)
		}
		a := f.Articles[0]
		if !reflect.DeepEqual(a.Summary, test.want) || a.SummaryLabel != test.label {
			t.Fatalf("%s: %+v", test.description, a)
		}
	}
}

func TestGoogleNewsCorrections(t *testing.T) {
	for _, description := range []string{`<ol><li>関連見出し</li></ol>`, `<a href="x">関連見出し</a>`} {
		for _, test := range []struct{ title, source, want string }{
			{"話題 - 詳細 - 合成媒体", "合成媒体", "話題 - 詳細"},
			{"話題 - 合成媒体別版", "合成媒体", "話題 - 合成媒体別版"},
			{"話題 - 合成媒体", "", "話題 - 合成媒体"},
			{"話題-合成媒体", "合成媒体", "話題-合成媒体"},
		} {
			data := rssItems(fmt.Sprintf(`<item><title>%s</title><link>https://news.google.com/rss/articles/synthetic?keep=1</link><source>%s</source><description><![CDATA[%s]]></description><c:encoded>encoded fallback</c:encoded></item>`, test.title, test.source, description))
			f, err := parseFeedProfile(data, "google-news", "")
			if err != nil {
				t.Fatal(err)
			}
			a := f.Articles[0]
			if a.Title != test.want || len(a.Summary) != 0 || a.Source != test.source || a.URL != "https://news.google.com/rss/articles/synthetic?keep=1" {
				t.Fatalf("correction: %+v", a)
			}
			body, err := renderHTML(f, "", 8, true, time.UTC)
			if err != nil || strings.Contains(string(body), "関連見出し") || strings.Contains(string(body), "encoded fallback") {
				t.Fatal("related stories leaked")
			}
			if test.source != "" && !strings.Contains(string(body), `<div class="meta">合成媒体</div>`) {
				t.Fatal("source not displayed")
			}
			generic, err := parseFeedProfile(data, "generic", "")
			if err != nil || generic.Articles[0].Title != test.title || len(generic.Articles[0].Summary) == 0 {
				t.Fatal("generic corrections enabled")
			}
		}
	}
}

func TestMixedSummaryDisplay(t *testing.T) {
	f, err := parseFeed(fixture(t, "rss2.xml"))
	if err != nil {
		t.Fatal(err)
	}
	f.Articles[0].Title = strings.Repeat("長い日本語見出し", 128)
	f.Articles[2].SummaryLabel = "ざっくり言うと"
	f.Articles[2].Summary = []summaryBlock{{Text: "One", Bullet: true}, {Text: "Two", Bullet: true}, {Text: "Three", Bullet: true}, {Text: "Hidden fourth", Bullet: true}}
	body, err := renderHTML(f, "", 8, true, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "配信された改行と省略…") || strings.Contains(text, "Hidden fourth") || strings.Count(text, ">ざっくり言うと</div>") != 1 {
		t.Fatal("mixed summaries changed")
	}
	if !strings.Contains(text, `</h2><div class="meta">10/31 02:00</div></article>`) {
		t.Fatal("headline-only article reserved summary space")
	}
	body, err = renderHTML(f, "", 8, false, time.UTC)
	if err != nil || strings.Contains(string(body), ">ざっくり言うと</div>") || strings.Contains(string(body), "合成した日本語の要約です。") {
		t.Fatal("summary switch leaked content")
	}
}
