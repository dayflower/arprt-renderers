package ical

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCivilWindows(t *testing.T) {
	for _, tc := range []struct {
		view, week, now, from, to string
		days                      int
	}{
		{"month", "sunday", "2026-11-02", "2026-11-01", "2026-12-06", 35},
		{"month", "monday", "2026-11-02", "2026-10-26", "2026-12-07", 42},
		{"month", "sunday", "2026-02-10", "2026-02-01", "2026-03-01", 28},
		{"agenda", "sunday", "2026-03-07", "2026-03-07", "2026-03-14", 7},
		{"agenda", "sunday", "2026-10-31", "2026-10-31", "2026-11-07", 7},
	} {
		loc, _ := time.LoadLocation("America/New_York")
		now, _ := time.ParseInLocation(time.DateOnly, tc.now, loc)
		input := &Input{view: tc.view, weekStartsOn: tc.week, timezone: loc.String()}
		from, to := input.window(now)
		model := input.model(now, from, to, nil)
		if from.Format(time.DateOnly) != tc.from || to.Format(time.DateOnly) != tc.to || len(model.Days) != tc.days {
			t.Fatalf("%+v: %s %s %d", tc, from, to, len(model.Days))
		}
		if tc.now == "2026-03-07" && to.Sub(from) != 167*time.Hour {
			t.Fatal("DST spring civil-day window")
		}
		if tc.now == "2026-10-31" && to.Sub(from) != 169*time.Hour {
			t.Fatal("DST autumn civil-day window")
		}
	}
}
func TestDayModel(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, loc)
	at := func(day, hour int) time.Time { return time.Date(2026, 11, day, hour, 0, 0, 0, loc) }
	input := &Input{title: "Calendar", view: "agenda", timezone: loc.String(), sources: []source{
		{id: "first", name: "First"}, {id: "second", name: "Second"}, {id: "holiday", name: "Holiday", holiday: true}, {id: "other", name: "Other", holiday: true},
	}}
	all := [][]Instance{
		{
			{UID: "span", Summary: "Across midnight", Start: at(1, 23), End: at(3, 1)},
			{UID: "finished", Summary: "Finished", Start: at(2, 8), End: at(2, 10)},
			{UID: "instant", Summary: "Instant", Start: now, End: now},
			{UID: "past-instant", Summary: "Past instant", Start: now.Add(-time.Second), End: now.Add(-time.Second)},
			{UID: "date", Summary: "All day", AllDay: true, StartDate: "2026-11-01", EndDate: "2026-11-03"},
		},
		{{UID: "same", Summary: "Instant second", Start: now, End: now}},
		{{Summary: "休日", AllDay: true, StartDate: "2026-11-02", EndDate: "2026-11-04"}, {Summary: "休日", AllDay: true, StartDate: "2026-11-02", EndDate: "2026-11-03"}},
		{{Summary: "別の祝日", AllDay: true, StartDate: "2026-11-02", EndDate: "2026-11-03"}},
	}
	from, to := input.window(now)
	m := input.model(now, from, to, all)
	d := m.Days[0]
	if len(d.Events) != 4 || len(d.Holidays) != 2 || strings.Join(d.Holidays, "|") != "休日|別の祝日" {
		t.Fatalf("wrong day: %+v", d)
	}
	if !d.Events[0].AllDay || !d.Events[0].ContinuesBefore || d.Events[0].ContinuesAfter {
		t.Fatal("all-day civil continuation")
	}
	e := d.Events[1]
	if e.Summary != "Across midnight" || !e.ContinuesBefore || !e.ContinuesAfter || !e.Ongoing || !e.Start.Equal(at(2, 0)) || !e.End.Equal(at(3, 0)) {
		t.Fatalf("split interval: %+v", e)
	}
	if d.Events[2].Ongoing || d.Events[2].SourceID != "first" || d.Events[3].SourceID != "second" {
		t.Fatal("instant ordering/ongoing")
	}
	if len(m.Days[1].Events) != 1 || len(m.Days[1].Holidays) != 1 || len(m.Days[2].Events) != 0 || len(m.Days[2].Holidays) != 0 {
		t.Fatal("exclusive date end/empty days")
	}
	input.view = "month"
	from, to = input.window(now)
	m = input.model(now, from, to, all)
	if len(m.Days[1].Events) != 6 {
		t.Fatal("month hid finished events")
	}
}
func TestNetworkSemanticValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		holiday    bool
		want       Error
	}{
		{"unsupported-rule", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nRRULE:FREQ=DAILY;RSCALE=CHINESE\nEND:VEVENT\n", false, Unsupported},
		{"unknown-extension", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nX-UNKNOWN-RULE:anything\nEND:VEVENT\n", false, Unsupported},
		{"old-invalid", "BEGIN:VEVENT\nUID:private\nDTSTART:20000101T090000Z\nRRULE:FREQ=DAILY;COUNT=2;UNTIL=20010101T090000Z\nEND:VEVENT\n", false, Invalid},
		{"wrong-value", "BEGIN:VEVENT\nUID:private\nDTSTART;VALUE=DATE:20261102T090000Z\nEND:VEVENT\n", false, Invalid},
		{"missing-date-type", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102\nEND:VEVENT\n", false, Invalid},
		{"date-with-zone", "BEGIN:VEVENT\nUID:private\nDTSTART;VALUE=DATE;TZID=Asia/Tokyo:20261102\nEND:VEVENT\n", false, Invalid},
		{"empty-rdate", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nRDATE:20261103T090000Z,\nEND:VEVENT\n", false, Invalid},
		{"rid-type", "BEGIN:VEVENT\nUID:private\nDTSTART;VALUE=DATE:20261102\nEND:VEVENT\nBEGIN:VEVENT\nUID:private\nRECURRENCE-ID:20261103T090000Z\nSTATUS:CANCELLED\nEND:VEVENT\n", false, Invalid},
		{"negative-duration", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nDURATION:-PT1H\nEND:VEVENT\n", false, Invalid},
		{"both-end", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nDTEND:20261102T110000Z\nDURATION:PT1H\nEND:VEVENT\n", false, Invalid},
		{"duplicate-rule-list", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nRRULE:FREQ=DAILY;BYHOUR=1,01\nEND:VEVENT\n", false, Invalid},
		{"duplicate-rule-part", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nRRULE:FREQ=DAILY;COUNT=1;COUNT=1\nEND:VEVENT\n", false, Invalid},
		{"utf8", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nSUMMARY:\xff\nEND:VEVENT\n", false, Invalid},
		{"weekday-duplicate", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nRRULE:FREQ=MONTHLY;BYDAY=1MO,+1MO\nEND:VEVENT\n", false, Invalid},
		{"parameter-bound", "BEGIN:VEVENT\nUID:private\nDTSTART:20261102T090000Z\nSUMMARY" + strings.Repeat(";X-PARAM=x", 65) + ":Title\nEND:VEVENT\n", false, Limit},
		{"property-bound", strings.Repeat("COMMENT:ignored\n", 100001), false, Limit},
		{"old-timed-holiday", "BEGIN:VEVENT\nUID:private\nDTSTART:20000101T090000Z\nEND:VEVENT\n", true, Invalid},
		{"text-bound", "BEGIN:VEVENT\nUID:private\nDTSTART:20000101T090000Z\nSUMMARY:" + strings.Repeat("日", 1025) + "\nEND:VEVENT\n", false, Limit},
		{"invalid-unreferenced-zone", "BEGIN:VTIMEZONE\nTZID:Private\nBEGIN:STANDARD\nDTSTART:20000101T000000\nTZOFFSETTO:+0900\nEND:STANDARD\nEND:VTIMEZONE\n", false, Invalid},
		{"zone-rule-extension", "BEGIN:VTIMEZONE\nTZID:Private\nBEGIN:STANDARD\nDTSTART:20000101T000000\nTZOFFSETFROM:+0900\nTZOFFSETTO:+0900\nRRULE:FREQ=YEARLY;X-SECRET=VALUE\nEND:STANDARD\nEND:VTIMEZONE\n", false, Unsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
			result, err := Supervise(context.Background(), testCommand(t), Request{Calendar: calendar(tc.body), Timezone: "Asia/Tokyo", From: from, To: from.AddDate(0, 0, 7), Holiday: tc.holiday})
			if !errors.Is(err, tc.want) || result != nil {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}
