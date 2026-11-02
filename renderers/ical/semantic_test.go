package ical

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

//go:embed fixtures/*
var fixtures embed.FS

func TestWorker(t *testing.T) {
	if os.Getenv("ICAL_TEST_WORKER") != "1" {
		return
	}
	if err := Serve(os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func run(t *testing.T, req Request) ([]Instance, error) {
	t.Helper()
	t.Setenv("ICAL_TEST_WORKER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Supervise(context.Background(), []string{executable, "-test.run=^TestWorker$"}, req)
}

func TestFixtures(t *testing.T) {
	data, err := fixtures.ReadFile("fixtures/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Timezone, From, To string
		Want                     []string
		Error                    Error
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			data, err := fixtures.ReadFile("fixtures/" + tc.Name + ".ics")
			if err != nil {
				t.Fatal(err)
			}
			from, err := time.Parse(time.RFC3339, tc.From)
			if err != nil {
				t.Fatal(err)
			}
			to, err := time.Parse(time.RFC3339, tc.To)
			if err != nil {
				t.Fatal(err)
			}
			instances, err := run(t, Request{string(data), tc.Timezone, from, to})
			if tc.Error != "" {
				if !errors.Is(err, tc.Error) {
					t.Fatalf("got %v, want %v", err, tc.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, i := range instances {
				start, end := i.Start.UTC().Format(time.RFC3339), i.End.UTC().Format(time.RFC3339)
				if i.AllDay {
					start, end = i.StartDate, i.EndDate
				}
				got = append(got, strings.Join([]string{i.UID, i.RecurrenceID, i.Summary, start, end}, "|"))
			}
			if !reflect.DeepEqual(got, tc.Want) {
				t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tc.Want, "\n"))
			}
		})
	}
}

func TestWorkerStall(t *testing.T) {
	if os.Getenv("ICAL_STALL_WORKER") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
}

func TestSupervisorStopsComputation(t *testing.T) {
	t.Setenv("ICAL_STALL_WORKER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = Supervise(ctx, []string{executable, "-test.run=^TestWorkerStall$"}, Request{})
	if !errors.Is(err, Deadline) || time.Since(start) > 2*time.Second {
		t.Fatalf("worker was not reaped promptly: %v", err)
	}
}

func TestInputBounds(t *testing.T) {
	_, err := run(t, Request{Calendar: strings.Repeat("x", MaxBody+1)})
	if !errors.Is(err, Limit) {
		t.Fatal(err)
	}
	var b cappedBuffer
	if _, err := b.Write(make([]byte, MaxResult+1)); !errors.Is(err, Limit) || b.Len() != 0 {
		t.Fatal("unbounded result")
	}
}

func TestStructuralBounds(t *testing.T) {
	for _, tc := range []struct {
		Name, Body string
		Want       Error
	}{
		{"components", strings.Repeat("BEGIN:VTODO\r\nEND:VTODO\r\n", MaxComponents), Limit},
		{"line", "BEGIN:VEVENT\r\nUID:line\r\nDTSTART:20261102T090000Z\r\nSUMMARY:" + strings.Repeat("x", MaxLine) + "\r\nEND:VEVENT\r\n", Invalid},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			body := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//ARPRT//Bound test//EN\r\n" + tc.Body + "END:VCALENDAR\r\n"
			from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			_, err := run(t, Request{body, "Asia/Tokyo", from, from.AddDate(0, 0, 7)})
			if !errors.Is(err, tc.Want) {
				t.Fatal(err)
			}
		})
	}
}

func TestAgendaTimeBoundaries(t *testing.T) {
	start := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		Name              string
		Span              time.Duration
		Offset            time.Duration
		Ongoing, Finished bool
	}{
		{"before", time.Hour, -time.Second, false, false},
		{"start", time.Hour, 0, true, false},
		{"end", time.Hour, time.Hour, false, true},
		{"instant-before", 0, -time.Second, false, false},
		{"instant-at", 0, 0, false, false},
		{"instant-after", 0, time.Second, false, true},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			i := Instance{Start: start, End: start.Add(tc.Span)}
			now := start.Add(tc.Offset)
			if i.Ongoing(now) != tc.Ongoing || i.Finished(now) != tc.Finished {
				t.Fatal("incorrect ongoing/finished boundary")
			}
		})
	}
	if (Instance{AllDay: true, Start: start, End: start.AddDate(0, 0, 1)}).Finished(start.AddDate(0, 0, 2)) {
		t.Fatal("day normalization owns all-day visibility")
	}
}
