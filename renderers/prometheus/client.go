package prometheus

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Diagnostic exposes only an identifier and a fixed classification.
// Never wrap transport, JSON, filesystem or API errors in public errors.
type Diagnostic struct{ Panel, Code string }

func (d Diagnostic) Error() string { return fmt.Sprintf("panel=%s code=%s", d.Panel, d.Code) }

type FetchStats struct {
	Attempts, Bytes, Warnings, Infos int
	Elapsed                          time.Duration
}

// Client owns its transport and a frozen credential. It is never used for capture.
// Its private fields prevent accidental URL/credential serialization.
type Client struct {
	base          string
	authorization string
	http          *http.Client
}

var bearerToken = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

// LoadConfig bounds startup configuration reads before any HTTP work.
func LoadConfig(path string, override *string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, Diagnostic{Code: "config_read"}
	}
	defer f.Close()
	return ParseConfig(f, override)
}

func NewClient(p Prometheus) (*Client, error) {
	if _, err := APIURL(p.URL, "/api/v1/query"); err != nil {
		return nil, Diagnostic{Code: "invalid_url"}
	}
	auth, err := loadAuthorization(p.Auth)
	if err != nil {
		return nil, Diagnostic{Code: "invalid_credentials"}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true // Decode explicitly to account for every byte.
	transport.MaxConnsPerHost = Concurrency
	transport.ResponseHeaderTimeout = FetchBudget
	transport.MaxResponseHeaderBytes = 64 << 10
	return &Client{p.URL, auth, &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func loadAuthorization(a *Auth) (string, error) {
	bad := errors.New("invalid_credentials")
	if a == nil {
		return "", nil
	}
	f, err := os.Open(a.CredentialsFile)
	if err != nil {
		return "", bad
	}
	defer f.Close()
	b, err := bounded(f, 8<<10)
	if err != nil || !utf8.Valid(b) {
		return "", bad
	}
	if a.Type == "bearer" {
		token := strings.TrimSpace(string(b))
		if !bearerToken.MatchString(token) {
			return "", bad
		}
		return "Bearer " + token, nil
	}
	if a.Type != "basic" {
		return "", bad
	}
	v, err := decodeJSON(b)
	m, ok := v.(map[string]any)
	if err != nil || !ok || len(m) != 2 {
		return "", bad
	}
	user, uok := m["username"].(string)
	pass, pok := m["password"].(string)
	valid := func(s string) bool {
		if s == "" {
			return false
		}
		for _, r := range s {
			if r < 32 || r == 127 {
				return false
			}
		}
		return true
	}
	if !uok || !pok || !valid(user) || !valid(pass) || strings.Contains(user, ":") {
		return "", bad
	}
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)), nil
}

type fetchBudget struct {
	mu       sync.Mutex
	changed  *sync.Cond
	reserved int
	exceeded bool
	stats    FetchStats
}

func (b *fetchBudget) attempt() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stats.Attempts >= RequestLimit {
		return false
	}
	b.stats.Attempts++
	return true
}

// budgetReader charges partial reads and retry bodies, including limit probes.
type budgetReader struct {
	r      io.Reader
	budget *fetchBudget
	bytes  int
}

func (b *budgetReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	b.budget.mu.Lock()
	if b.budget.changed == nil {
		b.budget.changed = sync.NewCond(&b.budget.mu)
	}
	remaining := TotalBytesLimit - b.budget.stats.Bytes - b.budget.reserved
	for remaining <= 0 && b.budget.reserved > 0 && !b.budget.exceeded {
		b.budget.changed.Wait()
		remaining = TotalBytesLimit - b.budget.stats.Bytes - b.budget.reserved
	}
	if b.budget.exceeded {
		b.budget.mu.Unlock()
		return 0, errors.New("byte_limit")
	}
	// Reserve before reading so concurrent workers cannot each spend the same
	// remaining allowance. One probe byte detects an exact-limit overflow.
	allowance := min(len(p), ResponseLimit-b.bytes+1, max(1, remaining))
	b.budget.reserved += allowance
	b.budget.mu.Unlock()
	n, err := b.r.Read(p[:allowance])
	b.budget.mu.Lock()
	b.budget.reserved -= allowance
	b.bytes += n
	b.budget.stats.Bytes += n
	exceeded := b.bytes > ResponseLimit || b.budget.stats.Bytes > TotalBytesLimit
	b.budget.exceeded = b.budget.exceeded || exceeded
	b.budget.changed.Broadcast()
	b.budget.mu.Unlock()
	if exceeded {
		return n, errors.New("byte_limit")
	}
	return n, err
}

// Fetch shares one deadline and budget across all tasks, retries and auxiliaries.
// A fatal transport or semantic error cancels workers and returns no partial data.
func (c *Client) Fetch(parent context.Context, p Plan) (map[string]Response, FetchStats, error) {
	start := time.Now()
	if len(p.Requests)*2 > RequestLimit {
		return nil, FetchStats{}, Diagnostic{Code: "request_limit"}
	}
	ctx, cancel := context.WithTimeout(parent, FetchBudget)
	defer cancel()
	defer c.http.CloseIdleConnections()
	budget := &fetchBudget{}
	out := map[string]Response{}
	var mu sync.Mutex
	var first error
	next := 0
	var wg sync.WaitGroup
	for range min(Concurrency, len(p.Requests)) {
		wg.Go(func() {
			for {
				mu.Lock()
				if ctx.Err() != nil || next == len(p.Requests) {
					mu.Unlock()
					return
				}
				r := p.Requests[next]
				next++
				mu.Unlock()
				response, err := c.fetchOne(ctx, r, budget)
				mu.Lock()
				if err != nil {
					if first == nil {
						first = Diagnostic{r.Panel, err.Error()}
						cancel()
					}
					mu.Unlock()
					return
				}
				out[r.ID] = response
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if first == nil && ctx.Err() != nil {
		first = Diagnostic{Code: "canceled"}
	}
	stats := budget.stats
	stats.Elapsed = time.Since(start)
	if first != nil {
		return nil, stats, first
	}
	for _, r := range out {
		stats.Warnings += r.Warnings
		stats.Infos += r.Infos
	}
	return out, stats, nil
}

func (c *Client) fetchOne(ctx context.Context, r Request, budget *fetchBudget) (Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Response{}, errors.New("canceled")
			case <-timer.C:
			}
		}
		deadline, _ := ctx.Deadline()
		left := time.Until(deadline)
		if ctx.Err() != nil || left < time.Millisecond {
			return Response{}, errors.New("canceled")
		}
		if !budget.attempt() {
			return Response{}, errors.New("request_limit")
		}
		endpoint := "/api/v1/query"
		form := url.Values{"query": {r.Query}, "timeout": {strconv.FormatInt(left.Milliseconds(), 10) + "ms"}}
		if r.Role == "range" {
			endpoint = "/api/v1/query_range"
			form.Set("start", seconds(r.Start))
			form.Set("end", seconds(r.End))
			form.Set("step", seconds(r.Step))
		} else {
			form.Set("time", seconds(r.Time))
		}
		u, err := APIURL(c.base, endpoint)
		if err != nil {
			return Response{}, errors.New("invalid_url")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
		if err != nil {
			return Response{}, errors.New("invalid_request")
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Encoding", "gzip")
		if c.authorization != "" {
			req.Header.Set("Authorization", c.authorization)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return Response{}, errors.New("canceled")
			}
			// Only failures to establish a connection qualify, not TLS, HTTP or body errors.
			var op *net.OpError
			if attempt == 0 && errors.As(err, &op) && op.Op == "dial" {
				continue
			}
			return Response{}, errors.New("transport_error")
		}
		body, code := readHTTPBody(resp, budget)
		if code != "" {
			return Response{}, errors.New(code)
		}
		if resp.StatusCode == http.StatusServiceUnavailable && attempt == 0 {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return Response{}, errors.New("http_status")
		}
		return ParseResponse(body, r)
	}
	return Response{}, errors.New("http_status")
}

func readHTTPBody(resp *http.Response, budget *fetchBudget) ([]byte, string) {
	defer resp.Body.Close()
	var reader io.Reader = resp.Body
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		if encoding != "gzip" {
			return nil, "unsupported_encoding"
		}
		z, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, "invalid_compression"
		}
		defer z.Close()
		reader = z
	}
	b, err := io.ReadAll(&budgetReader{r: reader, budget: budget})
	if err != nil {
		return nil, "body_read_or_limit"
	}
	return b, ""
}
