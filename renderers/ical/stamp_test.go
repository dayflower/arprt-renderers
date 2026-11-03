package ical

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestStampValidation(t *testing.T) {
	for _, tc := range []struct {
		name, property string
		want           error
	}{
		{"date", "DTSTAMP;VALUE=DATE:19760401", nil},
		{"implicit-date", "DTSTAMP:19760401", nil},
		{"leap-date", "DTSTAMP;VALUE=DATE:20240229", nil},
		{"utc", "DTSTAMP:20261101T090000Z", nil},
		{"explicit-utc", "DTSTAMP;VALUE=DATE-TIME:20261101T090000Z", nil},
		{"invalid-date", "DTSTAMP;VALUE=DATE:20260229", Invalid},
		{"invalid-implicit-date", "DTSTAMP:20260230", Invalid},
		{"datetime-as-date", "DTSTAMP;VALUE=DATE:20261101T090000Z", Invalid},
		{"date-as-datetime", "DTSTAMP;VALUE=DATE-TIME:20261101", Invalid},
		{"zoned", "DTSTAMP;TZID=Asia/Tokyo:20261101T090000", Invalid},
		{"zoned-date", "DTSTAMP;VALUE=DATE;TZID=Asia/Tokyo:20261101", Invalid},
		{"floating", "DTSTAMP:20261101T090000", Invalid},
		{"duplicate", "DTSTAMP;VALUE=DATE:19760401\nDTSTAMP:20261101T090000Z", Invalid},
		{"date-list", "DTSTAMP;VALUE=DATE:20261101,20261102", Invalid},
		{"unknown-type", "DTSTAMP;VALUE=TEXT:ignored", Unsupported},
		{"unknown-parameter", "DTSTAMP;X-ARPRT=ignored:19760401", Unsupported},
		{"range", "DTSTAMP;RANGE=THISANDFUTURE:19760401", Unsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			_, err := run(t, Request{Calendar: calendar("BEGIN:VEVENT\nUID:synthetic\nDTSTART;VALUE=DATE:20261102\n" + tc.property + "\nEND:VEVENT\n"), Timezone: "Asia/Tokyo", From: from, To: from.AddDate(0, 0, 7)})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDateStampRevisions(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		want                string
	}{
		{"date-order", "DTSTAMP;VALUE=DATE:20990101\n", "DTSTAMP:19760401\n", "second"},
		{"date-missing", "DTSTAMP;VALUE=DATE:20990101\n", "", "second"},
		{"missing-date", "", "DTSTAMP:19760401\n", "second"},
		{"utc-date", "DTSTAMP:20260101T000000Z\n", "DTSTAMP;VALUE=DATE:20990101\n", "first"},
		{"date-utc", "DTSTAMP:20990101\n", "DTSTAMP:20260101T000000Z\n", "second"},
		{"sequence-first", "DTSTAMP:19760401\nSEQUENCE:2\n", "DTSTAMP:20260101T000000Z\nSEQUENCE:1\n", "first"},
		{"sequence-second", "DTSTAMP:20260101T000000Z\nSEQUENCE:1\n", "DTSTAMP;VALUE=DATE:19760401\nSEQUENCE:2\n", "second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			component := func(stamp, summary string) string {
				return "BEGIN:VEVENT\nUID:synthetic\nDTSTART;VALUE=DATE:20261102\n" + stamp + "SUMMARY:" + summary + "\nEND:VEVENT\n"
			}
			from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			got, err := run(t, Request{Calendar: calendar(component(tc.first, "first") + component(tc.second, "second")), Timezone: "Asia/Tokyo", From: from, To: from.AddDate(0, 0, 7)})
			if err != nil || len(got) != 1 || got[0].Summary != tc.want {
				t.Fatalf("got %+v, %v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestHolidayMetadataExpansion(t *testing.T) {
	data, err := fixtures.ReadFile("fixtures/holiday-metadata.ics")
	if err != nil {
		t.Fatal(err)
	}
	for _, year := range []int{2024, 2025, 2026, 2027, 2028, 2029} {
		for _, month := range []time.Month{time.January, time.February} {
			t.Run(fmt.Sprintf("%d-%02d", year, month), func(t *testing.T) {
				from := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
				got, err := run(t, Request{Calendar: string(data), Timezone: "Asia/Tokyo", From: from, To: from.AddDate(0, 1, 0), Holiday: true})
				if err != nil {
					t.Fatal(err)
				}
				want := ""
				if year <= 2028 && month == time.January {
					want = fmt.Sprintf("%d-01-%02d", year, 8+(8-int(from.Weekday()))%7)
				}
				if year <= 2028 && month == time.February && year != 2026 && year != 2027 {
					want = fmt.Sprintf("%d-02-11", year)
				}
				if want == "" {
					if len(got) != 0 {
						t.Fatalf("unexpected occurrences: %+v", got)
					}
					return
				}
				summary := "合成の祝日"
				if month == time.January {
					summary = "合成の月曜祝日"
				}
				date, _ := time.Parse(time.DateOnly, want)
				if len(got) != 1 || !got[0].AllDay || got[0].StartDate != want || got[0].EndDate != date.AddDate(0, 0, 1).Format(time.DateOnly) || got[0].Summary != summary {
					t.Fatalf("incorrect holiday: %+v", got)
				}
			})
		}
	}
	// The same fixture must remain valid as an ordinary calendar too.
	from := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	got, err := run(t, Request{Calendar: string(data), Timezone: "America/Los_Angeles", From: from, To: from.AddDate(0, 1, 0)})
	if err != nil || len(got) != 1 || !strings.HasPrefix(got[0].StartDate, "2025-02-11") {
		t.Fatalf("ordinary calendar: %+v, %v", got, err)
	}
}
