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
	paginate := flags.Bool("paginate", false, "Continue from the last displayed article using ARPRT Job state")
	result := flags.String("result", "", "Job state result path with --paginate (default: result.json beside the output PNG)")
	input := flags.String("input", "", "Read a local RSS file (exclusive with --feed-url)")
	feedURL := flags.String("feed-url", "", "HTTP(S) RSS feed URL")
	webcappi := flags.String("webcappi-url", "", "Required webcappi base URL; uses its default browser profile")
	tokenFile := flags.String("webcappi-token-file", "", "Read-only Bearer token file")
	title := flags.String("title", "", "Override feed display title")
	maxItems := flags.Int("max-items", 8, "Maximum displayed articles (1-20)")
	summary := flags.Bool("summary", true, "Display summaries; use --summary=false to disable")
	images := flags.Bool("images", true, "Display optional thumbnails; use --images=false to disable")
	publicImagesOnly := flags.Bool("public-images-only", true, "Restrict image destinations to public IPs; use --public-images-only=false to allow internal images")
	timezone := flags.String("timezone", "Asia/Tokyo", "Article date display timezone")
	profile := flags.String("profile", "auto", "Feed normalization profile: auto, generic, hatena, livedoor, google-news")
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
	if _, err := captureURL(*webcappi, 800, 480, *paginate); err != nil {
		return err
	}
	if *maxItems < 1 || *maxItems > 20 {
		return errors.New("max-items must be between 1 and 20")
	}
	if !validProfile(*profile) {
		return errors.New("unknown feed profile")
	}
	resultPath := ""
	if *paginate {
		var err error
		resultPath, err = resultFile(*result, *output, *contextPath, *input, *tokenFile)
		if err != nil {
			return err
		}
	} else if *result != "" {
		return errors.New("--result requires --paginate")
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
	f, err := parseFeedProfile(data, *profile, *feedURL)
	if err != nil {
		return err
	}
	parsedCount := len(f.Articles)
	var remainingKeys []string
	if *paginate {
		keys := articleKeys(f.Articles)
		start := pageStart(keys, target.RendererState)
		remainingKeys = keys[start:]
		f.Articles = f.Articles[start:]
	}
	if *images {
		imageClient := newImageClient(*publicImagesOnly)
		defer imageClient.CloseIdleConnections()
		loadImages(ctx, imageClient, &f, *maxItems)
	}
	body, err := renderHTML(f, *title, *maxItems, *summary, location)
	if err == nil {
		body, err = target.layoutHTML(body)
	}
	if err != nil {
		return err
	}
	width, height := target.logicalSize()
	shot, err := capture(ctx, client, *webcappi, token, body, width, height, *paginate)
	if err != nil {
		return err
	}
	var state []byte
	if *paginate {
		state, err = pageResult(remainingKeys, min(len(f.Articles), *maxItems), shot.displayed)
		if err != nil {
			return err
		}
	}
	data, err = outputPNG(shot.image, target.rotation)
	if err != nil {
		return err
	}
	if err := writeOutput(ctx, *output, data, resultPath, state); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Rendered RSS: %d parsed articles, %s profile, 800x480 RGB, %d bytes\n", parsedCount, f.Profile, len(data))
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
