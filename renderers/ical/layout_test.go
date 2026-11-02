package ical

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The browser matrix uses only synthetic models and deliberately long Japanese.
// Set ICAL_WEBCAPPI and ICAL_LAYOUT_DIR for real browser captures; ordinary tests
// never require a browser or external networking.
func TestLayouts(t *testing.T) {
	var cases []struct {
		Name                             string
		Events, Holidays, Sources, Month int
	}
	data, err := fixtures.ReadFile("fixtures/layout-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		for _, view := range []string{"month", "agenda"} {
			month := c.Month
			if month == 0 {
				month = 11
			}
			now := time.Date(2026, time.Month(month), 2, 12, 0, 0, 0, time.FixedZone("JST", 9*3600))
			input := &Input{view: view, timezone: "Asia/Tokyo", weekStartsOn: "sunday"}
			from, to := input.window(now)
			m := &Model{Title: "合成カレンダー 長い日本語の表示を検証するためのタイトル", View: view, Now: now}
			if c.Name == "sample" {
				m.Title = "Calendar"
			}
			for i := range c.Sources {
				m.Sources = append(m.Sources, Label{ID: fmt.Sprint(i), Name: strings.Repeat("長い日本語ソース名", 5)})
			}
			if c.Name == "sample" {
				m.Sources[0].Name = "Work"
				m.Sources[1].Name = "Personal"
			}
			for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
				day := Day{Date: d.Format(time.DateOnly), Weekday: d.Weekday(), Today: d.Day() == 2 && d.Month() == now.Month(), InMonth: d.Month() == now.Month()}
				for range c.Holidays {
					day.Holidays = append(day.Holidays, strings.Repeat("非常に長い祝日名称と振替休日", 10))
				}
				for j := range c.Events {
					day.Events = append(day.Events, DayEvent{Summary: fmt.Sprintf("%d 長い日本語の予定 <script>alert(1)</script> & ", j) + strings.Repeat("テキスト", 90), SourceID: fmt.Sprint(j % c.Sources), SourceName: "合成ソース", Start: d.Add(9 * time.Hour), End: d.Add(11 * time.Hour), AllDay: j == 0, ContinuesBefore: j == 1, ContinuesAfter: j == 1, Ongoing: day.Today && j == 2})
				}
				if c.Name == "sample" {
					day.Holidays = nil
					if d.Month() == 11 && d.Day() == 3 {
						day.Holidays = []string{"Culture Day"}
					}
					if d.Month() == 11 && d.Day() == 23 {
						day.Holidays = []string{"勤労感謝の日"}
					}
					if d.Day() != 2 && d.Day() != 5 && d.Day() != 11 && d.Day() != 18 {
						day.Events = nil
					}
					for j := range day.Events {
						e := &day.Events[j]
						e.Summary = []string{"定例ミーティング", "Project review", "歯科予約"}[j]
						e.SourceName = m.Sources[j%c.Sources].Name
						e.AllDay = false
						e.ContinuesBefore = false
						e.ContinuesAfter = false
						e.Start = d.Add(time.Duration(9+j*3) * time.Hour)
						e.End = e.Start.Add(time.Hour)
						e.Ongoing = day.Today && j == 1
					}
					if view == "agenda" && day.Today && len(day.Events) > 0 {
						day.Events = day.Events[1:]
					}
				}
				m.Days = append(m.Days, day)
			}
			body, err := RenderHTML(m)
			if err != nil {
				t.Fatal(err)
			}
			if len(body) > htmlLimit || bytes.Contains(body, []byte("<script>alert(1)</script>")) {
				t.Fatal("unsafe or excessive HTML")
			}
			entries := len(m.Days)
			if view == "agenda" {
				entries = 0
				for _, d := range m.Days[:2] {
					if len(d.Events) > 0 {
						entries++
					}
				}
				futureCandidates := 0
				for _, d := range m.Days[2:] {
					futureCandidates += min(16, len(d.Events))
				}
				entries += min(32, futureCandidates)
			}
			if got := bytes.Count(body, []byte("data-date=")); got != entries {
				t.Fatalf("incorrect rendered event/date entries: got %d want %d", got, entries)
			}
			for _, removed := range []string{`class="legend"`, `class="key"`, `class="source"`, "No events", "Now · "} {
				if bytes.Contains(body, []byte(removed)) {
					t.Fatalf("unwanted interface content: %s", removed)
				}
			}

			if c.Events > 8 && view == "month" && !bytes.Contains(body, []byte(`data-omitted="72"`)) {
				t.Fatal("pre-layout omission not counted")
			}
			if c.Holidays > 1 && !bytes.Contains(body, []byte(fmt.Sprintf("+%d", c.Holidays-1))) {
				t.Fatal("holiday omissions not counted")
			}
			if c.Name != "sample" && c.Events > 0 && (!bytes.Contains(body, []byte("&lt;script&gt;")) || !bytes.Contains(body, []byte("←")) || !bytes.Contains(body, []byte("→"))) {
				t.Fatal("missing event meaning or escaping")
			}

			if bytes.Contains(body, []byte("予定なし")) || bytes.Contains(body, []byte("開催中")) || bytes.Contains(body, []byte("この先5日")) {
				t.Fatal("non-English interface label")
			}
			directory := os.Getenv("ICAL_LAYOUT_DIR")
			if directory != "" {
				if err := os.MkdirAll(directory, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, c.Name+"-"+view+".html"), body, 0644); err != nil {
					t.Fatal(err)
				}
			}
			base := os.Getenv("ICAL_WEBCAPPI")
			if base == "" {
				continue
			}
			for _, size := range [][2]int{{800, 480}, {480, 800}} {
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				png, err := capture(ctx, &http.Client{}, base, "", body, size[0], size[1])
				cancel()
				if err != nil {
					t.Fatal(err)
				}
				if directory != "" {
					name := fmt.Sprintf("%s-%s-%dx%d.png", c.Name, view, size[0], size[1])
					if err := os.WriteFile(filepath.Join(directory, name), png, 0644); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func TestAgendaUpcomingDates(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	m := &Model{Title: "Calendar", View: "agenda", Now: now}
	for i := range 7 {
		d := now.AddDate(0, 0, i)
		day := Day{Date: d.Format(time.DateOnly), Weekday: d.Weekday(), InMonth: true, Today: i == 0}
		if i == 1 {
			day.Holidays = []string{"Holiday without appointments"}
		}
		if i == 2 {
			day.Events = []DayEvent{{Summary: "日本語の予定", AllDay: true, SourceName: "Hidden source"}}
		}
		if i == 6 {
			day.Events = []DayEvent{{Summary: "Last day", AllDay: true}}
		}
		m.Days = append(m.Days, day)
	}
	body, err := RenderHTML(m)
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if strings.Contains(html, `class="primary-days"`) {
		t.Fatal("empty priority sections must be absent")
	}
	if !strings.Contains(html, `data-total="2"`) || !strings.Contains(html, "日本語の予定") || !strings.Contains(html, "Last day") {
		t.Fatal("missing upcoming events")
	}
	for _, date := range []string{"2026-11-02", "2026-11-03", "2026-11-05", "2026-11-06", "2026-11-07"} {
		if strings.Contains(html, `data-date="`+date+`"`) {
			t.Fatal("empty date became a displayed row", date)
		}
	}
	first := strings.Index(html, `data-date="2026-11-04"`)
	last := strings.Index(html, `data-date="2026-11-08"`)
	if first < 0 || last < first {
		t.Fatal("upcoming dates are missing or out of order")
	}
	if strings.Contains(html, "Hidden source") {
		t.Fatal("source labels should not be displayed")
	}
}
