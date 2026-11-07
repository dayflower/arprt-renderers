package prometheus

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func credentialFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testRequest(i int) Request {
	return Request{ID: fmt.Sprintf("p/value/%d/-1", i), Panel: "p", Role: "value", Series: i, Bucket: -1, Endpoint: "/api/v1/query", Query: "synthetic_secret", Time: 100}
}

func TestClientFormsAndFrozenAuth(t *testing.T) {
	for _, auth := range []struct{ kind, body, header string }{
		{"bearer", "  synthetic-token\n", "Bearer synthetic-token"},
		{"basic", `{"username":" user ","password":" password "}`, "Basic IHVzZXIgOiBwYXNzd29yZCA="},
		{"", "", ""},
	} {
		t.Run(auth.kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.Header.Get("Authorization") != auth.header {
					t.Error("method, query or headers")
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				budget, err := time.ParseDuration(r.Form.Get("timeout"))
				if err != nil || budget <= 0 || budget > FetchBudget || r.Form.Get("query") != "synthetic_secret" {
					t.Error("query or timeout")
				}
				if r.URL.Path == "/prefix/api/v1/query_range" {
					if r.Form.Get("start") != "90" || r.Form.Get("end") != "100" || r.Form.Get("step") != "5" || r.Form.Has("time") {
						t.Error("range parameters")
					}
					_, _ = w.Write(apiResponse("matrix", []any{}))
				} else {
					if r.URL.Path != "/prefix/api/v1/query" || r.Form.Get("time") != "100" || r.Form.Has("start") {
						t.Error("instant parameters")
					}
					_, _ = w.Write(apiResponse("scalar", []any{100, "1"}))
				}
			}))
			defer server.Close()
			p := Prometheus{URL: server.URL + "/prefix"}
			if auth.kind != "" {
				p.Auth = &Auth{auth.kind, credentialFile(t, auth.body)}
			}
			client, err := NewClient(p)
			if err != nil {
				t.Fatal(err)
			}
			if p.Auth != nil {
				if err := os.WriteFile(p.Auth.CredentialsFile, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			q := testRequest(0)
			q.ID = "p/range/0/-1"
			q.Role = "range"
			q.Start = 90
			q.End = 100
			q.Step = 5
			q.Time = 0
			responses, stats, err := client.Fetch(context.Background(), Plan{Requests: []Request{testRequest(0), q}})
			if err != nil || len(responses) != 2 || stats.Attempts != 2 {
				t.Fatalf("fetch: %v %+v", err, stats)
			}
		})
	}
}

func TestInvalidCredentials(t *testing.T) {
	for _, tc := range []struct{ kind, body string }{
		{"bearer", ""}, {"bearer", "secret\nextra"}, {"bearer", "x:y"}, {"bearer", strings.Repeat("a", 8193)},
		{"basic", `{"username":"a:b","password":"private"}`},
		{"basic", `{"username":"a","password":""}`},
		{"basic", `{"username":"a","username":"b","password":"private"}`},
		{"basic", `{"username":"a","password":"private","extra":1}`},
		{"basic", `{"username":"a","password":"private\n"}`},
		{"basic", `{"username":1,"password":"private"}`},
		{"basic", `{"username":"a","password":"private"} {}`},
	} {
		_, err := NewClient(Prometheus{URL: "http://example.invalid", Auth: &Auth{tc.kind, credentialFile(t, tc.body)}})
		if err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "example.invalid") {
			t.Fatalf("credential rejection: %v", err)
		}
	}
}

func TestClientStatusRetriesAndRedaction(t *testing.T) {
	for _, status := range []int{200, 400, 401, 422, 429, 500, 503, 302} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls, redirected atomic.Int32
			dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
			defer dest.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if status == 302 {
					w.Header().Set("Location", dest.URL+"/private")
				}
				if n == 1 || status != 503 {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "private response synthetic_secret")
					return
				}
				_, _ = w.Write(apiResponse("scalar", []any{100, "2"}))
			}))
			defer server.Close()
			client, err := NewClient(Prometheus{URL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			_, stats, err := client.Fetch(context.Background(), Plan{Requests: []Request{testRequest(0)}})
			want := int32(1)
			if status == 503 {
				want = 2
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("accepted failure")
			}
			if calls.Load() != want || stats.Attempts != int(want) || redirected.Load() != 0 {
				t.Fatalf("attempts: %+v", stats)
			}
			if err != nil {
				for _, secret := range []string{server.URL, "synthetic_secret", "private"} {
					if strings.Contains(err.Error(), secret) {
						t.Fatal("error leaked")
					}
				}
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConnectionRetryClassification(t *testing.T) {
	for _, op := range []string{"dial", "read", "tls"} {
		client, err := NewClient(Prometheus{URL: "http://example.invalid"})
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return nil, &net.OpError{Op: op, Net: "tcp", Err: errors.New("private transport")}
			}
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(apiResponse("scalar", []any{100, "1"})))}, nil
		})
		_, _, err = client.Fetch(context.Background(), Plan{Requests: []Request{testRequest(0)}})
		if op == "dial" {
			if err != nil || calls != 2 {
				t.Fatal("dial not retried")
			}
		} else if err == nil || calls != 1 {
			t.Fatal("retried nonconnection failure")
		}
	}
}

func TestFetchConcurrencyAndFatalCancellation(t *testing.T) {
	var active, peak atomic.Int32
	started := make(chan struct{}, Concurrency)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(apiResponse("scalar", []any{100, "1"}))
	}))
	defer server.Close()
	client, err := NewClient(Prometheus{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	requests := []Request{}
	for i := 0; i < 12; i++ {
		requests = append(requests, testRequest(i))
	}
	done := make(chan error, 1)
	go func() { _, _, err := client.Fetch(context.Background(), Plan{Requests: requests}); done <- err }()
	for range Concurrency {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	close(release)
	// Drain start notifications so later batches cannot block the handlers.
	go func() {
		for range 8 {
			<-started
		}
	}()
	if err := <-done; err != nil || peak.Load() != Concurrency {
		t.Fatalf("concurrency=%d err=%v", peak.Load(), err)
	}

	var blocked atomic.Int32
	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("query") == "bad" {
			deadline := time.Now().Add(2 * time.Second)
			for blocked.Load() < 3 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			_, _ = io.WriteString(w, `{"status":"error","error":"private"}`)
			return
		}
		blocked.Add(1)
		<-r.Context().Done()
	}))
	defer fail.Close()
	client, err = NewClient(Prometheus{URL: fail.URL})
	if err != nil {
		t.Fatal(err)
	}
	requests[0].Query = "bad"
	begin := time.Now()
	out, _, err := client.Fetch(context.Background(), Plan{Requests: requests})
	if err == nil || out != nil || time.Since(begin) > 3*time.Second || blocked.Load() != 3 {
		t.Fatalf("fatal cancellation: %v blocked=%d", err, blocked.Load())
	}
}

func TestFetchCancelWaitingAndBackoff(t *testing.T) {
	for _, retry := range []bool{false, true} {
		started := make(chan struct{}, 4)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			started <- struct{}{}
			if retry {
				w.WriteHeader(503)
				return
			}
			<-r.Context().Done()
		}))
		client, err := NewClient(Prometheus{URL: server.URL})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { _, _, err := client.Fetch(ctx, Plan{Requests: []Request{testRequest(0)}}); done <- err }()
		<-started
		if retry {
			time.Sleep(20 * time.Millisecond)
		}
		begin := time.Now()
		cancel()
		if err := <-done; err == nil || time.Since(begin) > time.Second {
			t.Fatal("cancel did not stop fetch")
		}
		server.Close()
	}
}

func TestFetchBudgetsAndGzip(t *testing.T) {
	for _, tc := range []struct {
		name        string
		size, tasks int
		retry, gzip bool
		fails       bool
	}{
		{"gzip", 1024, 1, false, true, false},
		{"per response", ResponseLimit + 1, 1, false, true, true},
		{"cumulative", ResponseLimit - 128, 9, false, false, true},
		{"retry cumulative", ResponseLimit - 128, 5, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls sync.Map
			body := append(apiResponse("scalar", []any{100, "1"}), bytes.Repeat([]byte(" "), tc.size)...)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				counter, _ := calls.LoadOrStore(r.Form.Get("query"), &atomic.Int32{})
				n := counter.(*atomic.Int32).Add(1)
				if tc.gzip {
					w.Header().Set("Content-Encoding", "gzip")
				}
				if tc.retry && n == 1 {
					w.WriteHeader(503)
				}
				if tc.gzip {
					z := gzip.NewWriter(w)
					_, _ = z.Write(body)
					_ = z.Close()
				} else {
					_, _ = w.Write(body)
				}
			}))
			defer server.Close()
			client, err := NewClient(Prometheus{URL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			requests := []Request{}
			for i := 0; i < tc.tasks; i++ {
				r := testRequest(i)
				r.Query = fmt.Sprint(i)
				requests = append(requests, r)
			}
			out, stats, err := client.Fetch(context.Background(), Plan{Requests: requests})
			if (err != nil) != tc.fails {
				t.Fatalf("err=%v stats=%+v", err, stats)
			}
			if tc.fails && out != nil {
				t.Fatal("returned partial results")
			}
			if strings.Contains(tc.name, "cumulative") && stats.Bytes <= TotalBytesLimit {
				t.Fatalf("did not reach cumulative limit: %+v err=%v", stats, err)
			}
			if stats.Bytes > TotalBytesLimit+1 {
				t.Fatalf("concurrent budget overshoot: %+v", stats)
			}
			if tc.name == "gzip" && stats.Bytes != len(body) {
				t.Fatal("compressed bytes charged instead of decompressed bytes")
			}
		})
	}
	client, err := NewClient(Prometheus{URL: "http://example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	requests := make([]Request, 129)
	if _, stats, err := client.Fetch(context.Background(), Plan{Requests: requests}); err == nil || stats.Attempts != 0 {
		t.Fatal("plan budget not checked")
	}
	b := &fetchBudget{}
	for range RequestLimit {
		if !b.attempt() {
			t.Fatal("early attempt limit")
		}
	}
	if b.attempt() {
		t.Fatal("attempt limit exceeded")
	}
}

func TestSnapshotAndHTTPModelEquivalence(t *testing.T) {
	c := config(t)
	response := func(timestamp int64, start, step int64, rangeQuery bool) []byte {
		if !rangeQuery {
			return apiResponse("scalar", []any{timestamp, "12"})
		}
		return apiResponse("matrix", []any{
			map[string]any{"metric": map[string]string{"instance": "b"}, "values": []any{[]any{start, "-1"}, []any{start + step, "NaN"}, []any{start + 3*step, "3"}}},
			map[string]any{"metric": map[string]string{"instance": "a"}, "values": []any{[]any{start, "1"}, []any{start + step, "2"}}},
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		parse := func(key string) int64 { v, _ := json.Number(r.Form.Get(key)).Int64(); return v }
		_, _ = w.Write(response(parse("time"), parse("start"), parse("step"), r.URL.Path == "/api/v1/query_range"))
	}))
	defer server.Close()
	c.Prometheus.URL = server.URL
	p, err := BuildPlan(c, contextFixture(t), instant(t, "2026-11-07T12:34:56+09:00"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifest, _ := json.Marshal(ExpectedManifest(p))
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	for i, r := range p.Requests {
		b := response(r.Time, r.Start, r.Step, r.Role == "range")
		if err := os.WriteFile(filepath.Join(dir, formatResponseName(i)), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	live, _, err := LoadModel(context.Background(), c, p, "")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := LoadModel(context.Background(), c, p, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(live, snapshot) {
		t.Fatal("live and snapshot differ")
	}
	// Snapshot acquisition must not open live credentials, even when configured.
	c.Prometheus.Auth = &Auth{Type: "bearer", CredentialsFile: filepath.Join(dir, "missing-credential")}
	p.ConfigHash = hash(c)
	manifest, _ = json.Marshal(ExpectedManifest(p))
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if got, _, err := LoadModel(context.Background(), c, p, dir); err != nil || !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("snapshot read live credentials: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "response-000.json"), []byte(`{"status":"error","error":"private"}`), 0600); err != nil {
		t.Fatal(err)
	}
	failed, _, err := LoadModel(context.Background(), c, p, dir)
	if err == nil || len(failed.Panels) != 0 {
		t.Fatal("invalid snapshot produced a model")
	}
}

func TestSharedDeadlineAndQueue(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		d, err := time.ParseDuration(r.Form.Get("timeout"))
		if err != nil || d > 200*time.Millisecond {
			t.Error("server timeout exceeded caller budget")
		}
		calls.Add(1)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewClient(Prometheus{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	requests := []Request{}
	for i := 0; i < 20; i++ {
		requests = append(requests, testRequest(i))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	begin := time.Now()
	out, stats, err := client.Fetch(ctx, Plan{Requests: requests})
	if err == nil || out != nil || time.Since(begin) > time.Second || calls.Load() != Concurrency || stats.Attempts != Concurrency {
		t.Fatalf("deadline/queue: %+v calls=%d err=%v", stats, calls.Load(), err)
	}
}

func TestBudgetReaderExactBoundaries(t *testing.T) {
	b := &fetchBudget{stats: FetchStats{Bytes: TotalBytesLimit - 8}}
	reader := &budgetReader{r: strings.NewReader("12345678"), budget: b, bytes: ResponseLimit - 8}
	if data, err := io.ReadAll(reader); err != nil || string(data) != "12345678" || b.stats.Bytes != TotalBytesLimit {
		t.Fatalf("exact limit: %v", err)
	}
	reader = &budgetReader{r: strings.NewReader("x"), budget: b}
	if _, err := io.ReadAll(reader); err == nil || b.stats.Bytes != TotalBytesLimit+1 {
		t.Fatal("overflow probe")
	}
}

func TestMalformedHTTPBodiesDoNotRetry(t *testing.T) {
	for _, encoding := range []string{"gzip", "br", ""} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Content-Encoding", encoding)
			_, _ = io.WriteString(w, "private malformed response")
		}))
		client, err := NewClient(Prometheus{URL: server.URL})
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = client.Fetch(context.Background(), Plan{Requests: []Request{testRequest(0)}})
		if err == nil || calls.Load() != 1 || strings.Contains(err.Error(), "private") {
			t.Fatal("malformed body retry or leak")
		}
		server.Close()
	}
}

func TestStartupConfigRead(t *testing.T) {
	body, err := os.ReadFile("fixtures/config.json")
	if err != nil {
		t.Fatal(err)
	}
	path := credentialFile(t, string(body))
	c, err := LoadConfig(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if c.Version != 1 {
		t.Fatal("configuration not frozen")
	}
	if _, err := LoadConfig(path, nil); err == nil {
		t.Fatal("accepted malformed config")
	}
	if _, err := LoadConfig(credentialFile(t, strings.Repeat(" ", ConfigLimit+1)), nil); err == nil {
		t.Fatal("accepted oversized config")
	}
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "private-missing"), nil); err == nil || strings.Contains(err.Error(), "private-missing") {
		t.Fatal("read error leaked path")
	}
}
