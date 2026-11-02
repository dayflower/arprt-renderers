package ical

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ExecutionTimeout = 60 * time.Second

// Prepare is the all-or-nothing boundary before rendering. The executable owns
// command (its private worker mode); configuration can never supply a command.
// transport may supply a private trust store; nil uses Go's default transport.
func Prepare(ctx context.Context, filename string, now time.Time, command []string, transport http.RoundTripper) (*Model, error) {
	ctx, cancel := context.WithTimeout(ctx, ExecutionTimeout)
	defer cancel()
	input, err := LoadInput(filename)
	if err != nil {
		return nil, err
	}
	return input.prepare(ctx, now, command, transport)
}
func (input *Input) prepare(ctx context.Context, now time.Time, command []string, transport http.RoundTripper) (*Model, error) {
	from, to := input.window(now)
	bodies, err := input.fetch(ctx, transport)
	if err != nil {
		return nil, err
	}
	semanticCtx, cancel := context.WithTimeout(ctx, SemanticTimeout)
	defer cancel()
	all := make([][]Instance, len(bodies))
	total := 0
	for j, body := range bodies {
		instances, err := Supervise(semanticCtx, command, Request{Calendar: body, Timezone: input.timezone, From: from, To: to, Holiday: input.sources[j].holiday})
		if err != nil {
			class := "semantic_worker"
			var e Error
			if errors.As(err, &e) {
				class = string(e)
			}
			return nil, diagnostic(input.sources[j].id, "semantics", class)
		}
		total += len(instances)
		if total > MaxOccurrences {
			return nil, diagnostic(input.sources[j].id, "semantics", "resource_limit")
		}
		all[j] = instances
	}
	if ctx.Err() != nil {
		return nil, diagnostic("", "semantics", "deadline")
	}
	model := input.model(now, from, to, all)
	if semanticCtx.Err() != nil {
		return nil, diagnostic("", "semantics", "deadline")
	}
	return model, nil
}
func origin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if n, err := strconv.Atoi(port); err == nil {
		port = strconv.Itoa(n)
	}
	return u.Scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
}
func (input *Input) fetch(ctx context.Context, transport http.RoundTripper) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if transport == nil {
		transport = http.DefaultTransport
	}
	bodies := make([]string, len(input.sources))
	errs := make([]error, len(input.sources))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(3, len(input.sources)) {
		wg.Go(func() {
			for index := range jobs {
				bodies[index], errs[index] = fetchSource(ctx, input.sources[index], transport)
			}
		})
	}
	for index := range input.sources {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	// Configuration order, rather than scheduling order, determines diagnostics.
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return bodies, nil
}
func fetchSource(ctx context.Context, s source, transport http.RoundTripper) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	first, _ := url.Parse(s.address)
	policy := false
	client := &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		u, err := validateURL(req.URL.String(), s.authorization != "")
		if err != nil || (req.Response != nil && strings.Contains(req.Response.Header.Get("Location"), "#")) || len(via) > 5 || (via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https") || (s.authorization != "" && origin(u) != origin(first)) {
			policy = true
			return http.ErrUseLastResponse
		}
		if s.authorization != "" {
			req.Header.Set("Authorization", s.authorization)
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.address, nil)
	if err != nil {
		return "", diagnostic(s.id, "fetch", "transport")
	}
	if s.authorization != "" {
		req.Header.Set("Authorization", s.authorization)
	}
	resp, err := client.Do(req)
	if err != nil {
		class := "transport"
		if ctx.Err() != nil {
			class = "deadline"
		}
		return "", diagnostic(s.id, "fetch", class)
	}
	defer resp.Body.Close()
	if policy {
		return "", diagnostic(s.id, "fetch", "redirect_policy")
	}
	if resp.StatusCode != http.StatusOK {
		return "", Diagnostic{Source: s.id, Stage: "fetch", Class: "http_status", Status: resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	if ctx.Err() != nil {
		return "", diagnostic(s.id, "fetch", "deadline")
	}
	if len(b) > MaxBody {
		return "", diagnostic(s.id, "fetch", "resource_limit")
	}
	if err != nil {
		return "", diagnostic(s.id, "fetch", "transport")
	}
	return string(b), nil
}
