package ical

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func calendar(body string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//ARPRT//Synthetic//EN\r\n" + strings.ReplaceAll(body, "\n", "\r\n") + "END:VCALENDAR\r\n"
}
func testCommand(t *testing.T) []string {
	t.Helper()
	t.Setenv("ICAL_TEST_WORKER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return []string{executable, "-test.run=^TestWorker$"}
}
func TestPrepareMixedSources(t *testing.T) {
	var seen atomic.Int32
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("cookie leaked between sources")
		}
		http.SetCookie(w, &http.Cookie{Name: "private", Value: "cookie"})
		switch r.URL.Path {
		case "/basic":
			user, pass, ok := r.BasicAuth()
			if !ok || user != " user " || pass != " pass " {
				t.Error("basic changed")
			}
			fmt.Fprint(w, calendar("BEGIN:VEVENT\nUID:same\nDTSTART:20261102T090000\nDTEND:20261102T110000\nSUMMARY:日本語 <event>\nEND:VEVENT\n"))
		case "/bearer":
			if r.Header.Get("Authorization") != "Bearer private-token" {
				t.Error("bearer changed")
			}
			fmt.Fprint(w, calendar("BEGIN:VEVENT\nUID:same\nDTSTART;VALUE=DATE:20261102\nSUMMARY:終日\nEND:VEVENT\n"))
		case "/holiday":
			if r.Header.Get("Authorization") != "" {
				t.Error("auth leaked")
			}
			fmt.Fprint(w, calendar("BEGIN:VEVENT\nUID:holiday\nDTSTART;VALUE=DATE:20261102\nDTEND;VALUE=DATE:20261104\nSUMMARY:祝日\nEND:VEVENT\n"))
		case "/bad-holiday":
			fmt.Fprint(w, calendar("BEGIN:VEVENT\nUID:timed-holiday\nDTSTART:20000101T090000Z\nEND:VEVENT\n"))
		case "/empty":
			fmt.Fprint(w, calendar(""))
		default:
			t.Error("unexpected request")
		}
		seen.Add(1)
	}))
	defer tls.Close()
	basic := writeTestFile(t, "basic", `{"username":" user ","password":" pass "}`)
	token := writeTestFile(t, "token", "private-token\n")
	config := map[string]any{"version": 1, "view": "agenda", "calendars": []any{
		map[string]any{"id": "basic", "name": "仕事", "url": tls.URL + "/basic", "auth": map[string]string{"type": "basic", "credentialsFile": basic}},
		map[string]any{"id": "bearer", "name": "個人", "url": tls.URL + "/bearer", "auth": map[string]string{"type": "bearer", "credentialsFile": token}},
		map[string]any{"id": "empty", "name": "Empty", "url": tls.URL + "/empty"},
	}, "holidayCalendars": []any{map[string]string{"id": "holiday", "name": "祝日", "url": tls.URL + "/holiday"}}}
	data, _ := json.Marshal(config)
	filename := writeTestFile(t, "config", string(data))
	now := time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC)
	model, err := Prepare(context.Background(), filename, now, testCommand(t), tls.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	if seen.Load() != 4 || len(model.Days) != 7 || len(model.Days[0].Events) != 2 || len(model.Days[0].Holidays) != 1 {
		t.Fatalf("incorrect merged model: %+v", model)
	}
	if !model.Days[0].Events[0].AllDay || !model.Days[0].Events[1].Ongoing {
		t.Fatal("ordering/ongoing")
	}
	b, _ := json.Marshal(model)
	for _, secret := range []string{tls.URL, "private-token", " pass ", "same"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("private data entered display model")
		}
	}
	// A fatal holiday error outside the window still prevents any model.
	config["holidayCalendars"] = []any{map[string]string{"id": "holiday", "name": "Holiday", "url": tls.URL + "/bad-holiday"}}
	data, _ = json.Marshal(config)
	filename = writeTestFile(t, "invalid", string(data))
	if m, err := Prepare(context.Background(), filename, now, testCommand(t), tls.Client().Transport); m != nil || err == nil {
		t.Fatal("partial success")
	}
}
func TestFetchConcurrency(t *testing.T) {
	var active, peak atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{}, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		fmt.Fprint(w, calendar(""))
	}))
	defer server.Close()
	input := &Input{}
	for j := range 10 {
		input.sources = append(input.sources, source{id: fmt.Sprintf("source-%d", j), address: server.URL})
	}
	done := make(chan error, 1)
	go func() { _, err := input.fetch(context.Background(), nil); done <- err }()
	for range 3 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("fetches were not parallel")
		}
	}
	close(release)
	if err := <-done; err != nil || peak.Load() != 3 {
		t.Fatalf("concurrency=%d: %v", peak.Load(), err)
	}
}
func TestFetchPolicies(t *testing.T) {
	var foreignCalls atomic.Int32
	foreign := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1); fmt.Fprint(w, calendar("")) }))
	defer foreign.Close()
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/foreign":
			http.Redirect(w, r, foreign.URL, http.StatusFound)
		case "/downgrade":
			http.Redirect(w, r, "http://127.0.0.1:1/private", http.StatusFound)
		case "/userinfo":
			http.Redirect(w, r, "https://user:private@example.com", http.StatusFound)
		case "/fragment":
			http.Redirect(w, r, "/ok#private", http.StatusFound)
		case "/empty-fragment":
			http.Redirect(w, r, "/ok#", http.StatusFound)
		case "/status":
			http.Error(w, "private-body", http.StatusForbidden)
		case "/large":
			w.Header().Set("Content-Encoding", "gzip")
			z := gzip.NewWriter(w)
			_, _ = z.Write([]byte(strings.Repeat("x", MaxBody+1)))
			_ = z.Close()
		case "/wait":
			<-r.Context().Done()
		default:
			fmt.Fprint(w, calendar(""))
		}
	}))
	defer server.Close()
	for _, tc := range []struct{ path, auth, class string }{
		{"/redirect", "Bearer private", ""}, {"/loop", "", "redirect_policy"}, {"/foreign", "Bearer private", "redirect_policy"},
		{"/downgrade", "", "redirect_policy"}, {"/userinfo", "", "redirect_policy"}, {"/fragment", "", "redirect_policy"},
		{"/empty-fragment", "", "redirect_policy"}, {"/status", "", "http_status"}, {"/large", "", "resource_limit"}, {"/wait", "", "deadline"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			_, err := fetchSource(ctx, source{id: "work", address: server.URL + tc.path, authorization: tc.auth}, server.Client().Transport)
			if tc.class == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var d Diagnostic
			if !errors.As(err, &d) || d.Class != tc.class {
				t.Fatalf("got %v, want %s", err, tc.class)
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), server.URL) {
				t.Fatal("secret in error")
			}
		})
	}
	if foreignCalls.Load() != 0 {
		t.Fatal("authenticated cross-origin request sent")
	}
	// Caller cancellation stops the complete stage, including queued sources.
	input := &Input{sources: []source{{id: "work", address: server.URL + "/wait"}, {id: "next", address: server.URL + "/wait"}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b, err := input.fetch(ctx, server.Client().Transport); b != nil || err == nil {
		t.Fatal("cancellation returned partial bodies")
	}
}
func TestWholeExecutionAndInvalidInputBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, calendar("")) }))
	defer server.Close()
	config := fmt.Sprintf(`{"version":1,"calendars":[{"id":"valid","name":"Valid","url":%q},{"id":"invalid","name":"Invalid","url":"https://example.com","auth":{"type":"bearer","credentialsFile":"/missing-synthetic-file"}}]}`, server.URL)
	if m, err := Prepare(context.Background(), writeTestFile(t, "input", config), time.Now(), nil, nil); m != nil || err == nil || calls.Load() != 0 {
		t.Fatal("HTTP began before all input validation")
	}
	input := &Input{view: "agenda", timezone: "UTC", sources: []source{{id: "a", address: server.URL}, {id: "b", address: server.URL}}}
	t.Setenv("ICAL_STALL_WORKER", "1")
	exe, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	m, err := input.prepare(ctx, time.Now(), []string{exe, "-test.run=^TestWorkerStall$"}, nil)
	if m != nil || err == nil || time.Since(start) > 2*time.Second {
		t.Fatal("shared deadline failed")
	}
}
