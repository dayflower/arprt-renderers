package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestConfig(t *testing.T) {
	for _, input := range []string{`{"version":1,"targetDate":"2027-01-01"}`, `{"version":1,"targetDate":"0001-01-01","timezone":"UTC","title":"   "}`, `{"version":1,"targetDate":"9999-12-31","title":"  Example & 湾  "}`} {
		c, err := parseConfig([]byte(input))
		if err != nil || c.location == nil || c.Title != strings.TrimSpace(c.Title) {
			t.Fatal(c, err)
		}
	}
	base := `{"version":1,"targetDate":"2027-01-01"}`
	for _, input := range []string{
		`{}`, `[]`, `null`, base + `{}`, strings.Replace(base, `1`, `1.0`, 1),
		`{"targetDate":"2027-01-01"}`, `{"version":2,"targetDate":"2027-01-01"}`,
		`{"version":1,"targetDate":null}`, `{"version":1,"targetDate":123}`,
		`{"version":1,"targetDate":"2027-01-01","targetDate":"2028-01-01"}`,
		`{"version":1,"version":1,"targetDate":"2027-01-01"}`,
		`{"version":1,"targetDate":"2027-01-01","extra":false}`,
		`{"version":1,"targetDate":"2027-01-01","timezone":null}`,
		`{"version":1,"targetDate":"2027-01-01","title":null}`,
		`{"version":1,"targetDate":"2027-01-01","title":[]}`,
		`{"version":1,"targetDate":"2027-01-01","title":"a\nb"}`,
		`{"version":1,"targetDate":"2027-01-01","title":"\ta"}`,
		`{"version":1,"targetDate":"2027-01-01","title":"` + strings.Repeat("湾", 129) + `"}`,
		base[:len(base)-1] + `,"title":"` + string([]byte{0xff}) + `"}`,
		strings.Repeat(" ", configLimit) + base,
	} {
		if _, err := parseConfig([]byte(input)); err == nil {
			t.Errorf("accepted configuration %q", input[:min(len(input), 180)])
		}
	}
	for _, date := range []string{"0000-01-01", "10000-01-01", "2027-02-29", "1900-02-29", "2027-04-31", "2027-1-01", "2027-01-1", " 2027-01-01", "2027-01-01T00:00:00Z", "2027-00-01", "2027-01-00"} {
		if _, err := parseConfig([]byte(fmt.Sprintf(`{"version":1,"targetDate":%q}`, date))); err == nil {
			t.Errorf("accepted date %q", date)
		}
	}
	for _, zone := range []string{"", "Local", "+09:00", "unknown/zone", "/etc/localtime"} {
		if _, err := parseConfig([]byte(fmt.Sprintf(`{"version":1,"targetDate":"2027-01-01","timezone":%q}`, zone))); err == nil {
			t.Errorf("accepted timezone %q", zone)
		}
	}
}

func TestCalendarDays(t *testing.T) {
	for _, tc := range []struct{ today, target, zone, count, status string }{
		{"2026-11-10T12:00:00Z", "2027-01-01", "Asia/Tokyo", "52", "DAYS TO GO"},
		{"2026-12-31T00:00:00Z", "2027-01-01", "UTC", "1", "DAY TO GO"},
		{"2027-01-01T23:59:59Z", "2027-01-01", "UTC", "0", "TODAY IS THE DAY"},
		{"2027-01-02T00:00:00Z", "2027-01-01", "UTC", "0", "TARGET PASSED"},
		{"2028-02-28T12:00:00Z", "2028-03-01", "UTC", "2", "DAYS TO GO"},
		{"2100-02-28T12:00:00Z", "2100-03-01", "UTC", "1", "DAY TO GO"},
		{"2000-02-28T12:00:00Z", "2000-03-01", "UTC", "2", "DAYS TO GO"},
		{"2026-03-08T05:00:00Z", "2026-03-09", "America/New_York", "1", "DAY TO GO"},
		{"2026-11-01T04:00:00Z", "2026-11-02", "America/New_York", "1", "DAY TO GO"},
		{"2011-12-29T22:00:00Z", "2011-12-31", "Pacific/Apia", "2", "DAYS TO GO"},
		{"2026-12-31T15:00:00Z", "2027-01-01", "Asia/Tokyo", "0", "TODAY IS THE DAY"},
		{"2026-12-31T15:00:00Z", "2027-01-01", "America/Los_Angeles", "1", "DAY TO GO"},
		{"0001-01-01T00:00:00Z", "9999-12-31", "UTC", "3652058", "DAYS TO GO"},
		{"9999-12-31T00:00:00Z", "0001-01-01", "UTC", "0", "TARGET PASSED"},
	} {
		t.Run(tc.today+"/"+tc.zone+"/"+tc.target, func(t *testing.T) {
			cfg, err := parseConfig([]byte(fmt.Sprintf(`{"version":1,"targetDate":%q,"timezone":%q}`, tc.target, tc.zone)))
			if err != nil {
				t.Fatal(err)
			}
			now, err := time.Parse(time.RFC3339, tc.today)
			if err != nil {
				t.Fatal(err)
			}
			view, err := buildView(cfg, now)
			if err != nil || view.Count != tc.count || view.Status != tc.status || view.Today != now.In(cfg.location).Format("2006-01-02") {
				t.Fatal(view, err)
			}
		})
	}
	cfg, _ := parseConfig([]byte(`{"version":1,"targetDate":"2027-01-01","timezone":"Asia/Tokyo"}`))
	if _, err := buildView(cfg, time.Date(9999, 12, 31, 23, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("out-of-range local year accepted")
	}
}

func TestLayoutEscaping(t *testing.T) {
	cfg, _ := parseConfig([]byte(`{"version":1,"targetDate":"2027-01-01","title":"  <script>湾 & \"event\"</script>  "}`))
	view, _ := buildView(cfg, time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC))
	body, err := renderHTML(view)
	if err != nil || strings.Contains(string(body), `<script>湾`) || !strings.Contains(string(body), `&lt;script&gt;湾 &amp;`) || !strings.Contains(string(body), `>52</div>`) {
		t.Fatal("incorrect text escaping or reference model", err)
	}
	view.Title = ""
	body, _ = renderHTML(view)
	if strings.Contains(string(body), `class="title"`) || !strings.Contains(string(body), `>2027-01-01</time>`) || !strings.Contains(string(body), `>2026-11-10</time>`) {
		t.Fatal("optional title or date format")
	}
}
