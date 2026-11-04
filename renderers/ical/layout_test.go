package ical

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
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
				entries = 2
				futureCandidates := 0
				for _, d := range m.Days[2:] {
					futureCandidates += min(16, len(d.Events))
					if len(d.Events) == 0 && len(d.Holidays) > 0 {
						futureCandidates++
					}
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
		if i == 1 || i == 3 {
			day.Holidays = []string{"Holiday without appointments", "Another holiday"}
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
	markup := html.UnescapeString(string(body))
	if !strings.Contains(markup, `class="primary-days"`) || !strings.Contains(markup, "Tomorrow") || !strings.Contains(markup, "Holiday without appointments") {
		t.Fatal("empty priority days and their holidays must be present")
	}
	if !strings.Contains(markup, `data-total="3"`) || !strings.Contains(markup, "日本語の予定") || !strings.Contains(markup, "Last day") {
		t.Fatal("missing upcoming events")
	}
	for _, date := range []string{"2026-11-06", "2026-11-07"} {
		if strings.Contains(markup, `data-date="`+date+`"`) {
			t.Fatal("empty date became a displayed row", date)
		}
	}
	first := strings.Index(markup, `data-date="2026-11-04"`)
	last := strings.Index(markup, `data-date="2026-11-08"`)
	holiday := strings.Index(markup, `class="upcoming-event holiday-only" data-date="2026-11-05"`)
	if holiday < first || holiday > last || !strings.Contains(markup, `<span class="holiday-name">Holiday without appointments</span><span class="holiday-more"> +1</span>`) {
		t.Fatal("holiday-only row missing, out of order or missing extra holiday count")
	}
	if first < 0 || last < first {
		t.Fatal("upcoming dates are missing or out of order")
	}
	if strings.Contains(markup, "Hidden source") {
		t.Fatal("source labels should not be displayed")
	}
}

func TestAgendaHolidayCandidateLimits(t *testing.T) {
	for _, tc := range []struct {
		name                                           string
		events                                         [5]int
		omitted                                        [5]int
		wantTotal, wantOmitted, wantRows, wantHolidays int
	}{
		{"holidays only", [5]int{}, [5]int{}, 5, 0, 5, 5},
		{"room for holidays", [5]int{0, 16, 0, 14, 0}, [5]int{}, 33, 1, 32, 2},
		{"appointments take all candidates", [5]int{0, 20, 0, 20, 0}, [5]int{}, 43, 11, 32, 0},
		{"daily and combined overflow", [5]int{0, 80, 1, 0, 80}, [5]int{}, 163, 131, 32, 0},
		{"already omitted appointments", [5]int{}, [5]int{2, 0, 0, 0, 0}, 6, 2, 4, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
			m := &Model{View: "agenda", Now: now}
			for i := range 7 {
				d := now.AddDate(0, 0, i)
				day := Day{Date: d.Format(time.DateOnly), Weekday: d.Weekday(), InMonth: true, Today: i == 0, Holidays: []string{"First <holiday>", "Second", "Third"}}
				if i >= 2 {
					day.Omitted = tc.omitted[i-2]
					for range tc.events[i-2] {
						day.Events = append(day.Events, DayEvent{Summary: "Appointment", AllDay: true})
					}
				}
				m.Days = append(m.Days, day)
			}
			body, err := RenderHTML(m)
			if err != nil {
				t.Fatal(err)
			}
			markup := html.UnescapeString(string(body))
			want := fmt.Sprintf(`class="upcoming" data-total="%d" data-omitted="%d"`, tc.wantTotal, tc.wantOmitted)
			if !strings.Contains(markup, want) {
				t.Fatalf("missing totals: %s", want)
			}
			if got := strings.Count(markup, `class="upcoming-event`); got != tc.wantRows {
				t.Fatalf("rows: got %d want %d", got, tc.wantRows)
			}
			if got := strings.Count(markup, `class="upcoming-event holiday-only"`); got != tc.wantHolidays {
				t.Fatalf("holiday rows: got %d want %d", got, tc.wantHolidays)
			}
			if bytes.Contains(body, []byte("First <holiday>")) || !bytes.Contains(body, []byte("First &lt;holiday&gt;")) {
				t.Fatal("holiday name escaping")
			}
			if tc.wantHolidays > 0 && !strings.Contains(markup, `<span class="holiday-name">First <holiday></span><span class="holiday-more"> +2</span>`) {
				t.Fatal("missing holiday-only extra count")
			}
			if tc.wantRows > tc.wantHolidays && !strings.Contains(markup, "All day · First <holiday> (+2)") {
				t.Fatal("missing appointment holiday metadata")
			}
			if tc.name == "room for holidays" && strings.Contains(markup, `data-date="2026-11-08"`) {
				t.Fatal("latest holiday should be omitted first")
			}
		})
	}
}
