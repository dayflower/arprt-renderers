// Countdown renders calendar-day countdowns for an ARPRT Kubernetes Job.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func run(parent context.Context, args []string, stdout, stderr io.Writer) error {
	return runWithClock(parent, args, stdout, stderr, time.Now)
}

func runWithClock(parent context.Context, args []string, stdout, stderr io.Writer, clock func() time.Time) error {
	now := clock()
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	flags := flag.NewFlagSet("countdown", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "Required countdown configuration JSON")
	contextPath := flags.String("context", "/arprt/context.json", "Render Context v1 JSON")
	output := flags.String("output", "/arprt/output.png", "Output PNG path")
	webcappi := flags.String("webcappi-url", "", "Required webcappi base URL")
	tokenFile := flags.String("webcappi-token-file", "", "Read-only Bearer token file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" {
		return errors.New("config is required; positional arguments are not accepted")
	}
	if _, err := captureURL(*webcappi, 800, 480); err != nil {
		return err
	}
	data, err := readFile(*configPath, configLimit)
	if err != nil {
		return errors.New("cannot read configuration")
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return err
	}
	data, err = readFile(*contextPath, 1<<20)
	if err != nil {
		return errors.New("cannot read Context")
	}
	target, err := parseContext(data)
	if err != nil {
		return err
	}
	token := ""
	if *tokenFile != "" {
		data, err := readFile(*tokenFile, 8192)
		if err != nil {
			return errors.New("cannot read Bearer token file")
		}
		token = strings.TrimSpace(string(data))
		if !validToken(token) {
			return errors.New("invalid Bearer token file")
		}
	}
	view, err := buildView(cfg, now)
	if err != nil {
		return err
	}
	body, err := renderHTML(view)
	if err == nil {
		body, err = target.layoutHTML(body)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errors.New("execution cancelled")
	}
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	width, height := target.logicalSize()
	data, err = capture(ctx, client, *webcappi, token, body, width, height)
	if err != nil {
		return err
	}
	img, err := decodePNG(data, width, height)
	if err != nil {
		return err
	}
	data, err = outputPNG(img, target.rotation)
	if err != nil {
		return errors.New("cannot encode output")
	}
	if err := writeOutput(ctx, *output, data); err != nil {
		return errors.New("cannot replace output")
	}
	fmt.Fprintf(stdout, "Rendered countdown: 800x480 RGB, %d bytes\n", len(data))
	return nil
}

func validToken(s string) bool {
	if s == "" || s[0] == '=' {
		return false
	}
	padding := false
	for _, c := range s {
		if c == '=' {
			padding = true
			continue
		}
		if padding || !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~+/", c)) {
			return false
		}
	}
	return true
}

func httpURL(s string) string {
	req, err := http.NewRequest(http.MethodGet, s, nil)
	if err != nil || req.URL.Hostname() == "" || req.URL.User != nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
		return ""
	}
	return s
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "countdown:", err)
		os.Exit(1)
	}
}
