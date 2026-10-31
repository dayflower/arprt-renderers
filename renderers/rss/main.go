// RSS renders a feed through webcappi for an ARPRT Kubernetes Job.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func run(parent context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("rss", flag.ContinueOnError)
	flags.SetOutput(stderr)
	contextPath := flags.String("context", "/arprt/context.json", "Render Context v1 JSON")
	output := flags.String("output", "/arprt/output.png", "Output PNG path")
	input := flags.String("input", "", "Read a local RSS file (exclusive with --feed-url)")
	feedURL := flags.String("feed-url", "", "HTTP(S) RSS feed URL")
	webcappi := flags.String("webcappi-url", "", "Required webcappi base URL; uses its default browser profile")
	tokenFile := flags.String("webcappi-token-file", "", "Read-only Bearer token file")
	title := flags.String("title", "", "Override feed display title")
	maxItems := flags.Int("max-items", 8, "Maximum displayed articles (1-20)")
	summary := flags.Bool("summary", true, "Display summaries; use --summary=false to disable")
	timezone := flags.String("timezone", "Asia/Tokyo", "Article date display timezone")
	profile := flags.String("profile", "generic", "Feed normalization profile: generic (separate from webcappi browser profiles)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if (*input == "") == (*feedURL == "") {
		return errors.New("provide exactly one of --input or --feed-url")
	}
	if *feedURL != "" && httpURL(*feedURL) == "" {
		return errors.New("expected HTTP(S) feed URL without credentials")
	}
	if _, err := captureURL(*webcappi, 800, 480); err != nil {
		return err
	}
	if *maxItems < 1 || *maxItems > 20 {
		return errors.New("max-items must be between 1 and 20")
	}
	if *profile != "generic" {
		return errors.New("only the generic feed profile is supported")
	}
	location, err := time.LoadLocation(*timezone)
	if err != nil {
		return errors.New("unknown timezone")
	}
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	data, err := readFile(*contextPath, 1<<20)
	if err != nil {
		return fmt.Errorf("context: %w", err)
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
		if token == "" || strings.ContainsAny(token, "\r\n\t ") {
			return errors.New("invalid Bearer token file")
		}
	}
	client := newHTTPClient()
	if *input != "" {
		data, err = readFile(*input, feedLimit)
	} else {
		data, err = fetchFeed(ctx, client, *feedURL)
	}
	if err != nil {
		return fmt.Errorf("feed input: %w", err)
	}
	f, err := parseFeed(data)
	if err != nil {
		return err
	}
	body, err := renderHTML(f, *title, *maxItems, *summary, location)
	if err != nil {
		return err
	}
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
		return err
	}
	if err := writeOutput(ctx, *output, data); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Rendered RSS: %d parsed articles, generic profile, 800x480 RGB, %d bytes\n", len(f.Articles), len(data))
	return nil
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "rss:", err)
		os.Exit(1)
	}
}
