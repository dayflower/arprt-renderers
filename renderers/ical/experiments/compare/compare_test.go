// These tests characterize pinned candidates, including known deficiencies.
// They do not claim that an observed incorrect result is RFC-conformant.
package compare

import (
	"os"
	"strings"
	"testing"
	"time"

	selected "github.com/aanelson/iCalendar"
	arran "github.com/arran4/golang-ical"
	emersion "github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../fixtures/" + name + ".ics")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTimezoneCandidates(t *testing.T) {
	for _, name := range []string{"dst-gap", "custom-zone"} {
		t.Run(name, func(t *testing.T) {
			input := fixture(t, name)
			cal, err := selected.Parse(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			var actual string
			for e := range cal.Events() {
				start, err := e.Start()
				if err != nil {
					t.Fatal(err)
				}
				actual = start.Time().UTC().Format(time.RFC3339)
			}
			want := "2026-03-08T07:30:00Z"
			if name == "custom-zone" {
				want = "2026-11-01T23:30:00Z"
			}
			if actual != want {
				t.Fatalf("selected: got %s want %s", actual, want)
			}
			old, err := emersion.NewDecoder(strings.NewReader(input)).Decode()
			if err != nil {
				t.Fatal(err)
			}
			e := old.Events()[0]
			start, err := e.DateTimeStart(time.UTC)
			if name == "custom-zone" {
				if err == nil {
					t.Fatal("recheck emersion custom-zone decision")
				}
			} else if err != nil || start.UTC().Format(time.RFC3339) != "2026-03-08T06:30:00Z" {
				t.Fatalf("recheck emersion gap result: %v %v", start, err)
			}
			other, err := arran.ParseCalendar(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			start, err = other.Events()[0].GetStartAt()
			if name == "custom-zone" {
				if err == nil {
					t.Fatal("recheck arran custom-zone decision")
				}
			} else if err != nil || start.UTC().Format(time.RFC3339) != "2026-03-08T06:30:00Z" {
				t.Fatalf("recheck arran gap result: %v %v", start, err)
			}
			t.Log("selected resolves RFC gap/custom zone; both alternatives require a timezone replacement")
		})
	}
}

func TestRecurrenceCandidates(t *testing.T) {
	start := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	for _, rule := range []string{"FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1;COUNT=4", "FREQ=MONTHLY;BYMONTHDAY=-1;COUNT=4"} {
		option, err := rrule.StrToROption(rule)
		if err != nil {
			t.Fatal(err)
		}
		option.Dtstart = start
		r, err := rrule.NewRRule(*option)
		if err != nil {
			t.Fatal(err)
		}
		got := r.All()
		want := []string{"2026-09-30", "2026-10-30", "2026-11-30", "2026-12-31"}
		if strings.Contains(rule, "BYMONTHDAY") {
			want[1] = "2026-10-31"
		}
		if len(got) != len(want) {
			t.Fatal(got)
		}
		for i, d := range got {
			if d.Format("2006-01-02") != want[i] {
				t.Fatal(got)
			}
		}
	}
	t.Log("rrule-go passes ordinary BYDAY/BYMONTHDAY/BYSETPOS/COUNT; these alone are insufficient for selection")
}

func TestSelectedNeedsRevisionAndRangeAdapter(t *testing.T) {
	for _, tc := range []struct {
		Name    string
		Count   int
		Summary string
	}{
		{"timestamp", 1, "old"},
		{"range-moved-in", 1, ""},
	} {
		cal, err := selected.Parse(strings.NewReader(fixture(t, tc.Name)))
		if err != nil {
			t.Fatal(err)
		}
		from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		count := 0
		for o, err := range cal.Occurrences(from, from.AddDate(0, 0, 7)) {
			if err != nil {
				t.Fatal(err)
			}
			count++
			if tc.Summary != "" && o.Component.Get("SUMMARY").Text() != tc.Summary {
				t.Fatal("recheck revision supplement")
			}
		}
		if count != tc.Count {
			t.Fatalf("%s: got %d, recheck range supplement", tc.Name, count)
		}
	}
}
