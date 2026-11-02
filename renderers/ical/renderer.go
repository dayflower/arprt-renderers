package ical

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Run executes a single Job using one clock sample and a shared deadline.
// command is supplied only by the executable, never by mounted input.
func Run(parent context.Context, args []string, command []string, stdout io.Writer) error {
	now := time.Now()
	ctx, cancel := context.WithTimeout(parent, ExecutionTimeout)
	defer cancel()
	flags := flag.NewFlagSet("ical", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	contextPath := flags.String("context", "/arprt/context.json", "Render Context v1")
	output := flags.String("output", "/arprt/output.png", "Output PNG")
	config := flags.String("config", "", "Calendar configuration file")
	base := flags.String("webcappi-url", "", "webcappi base URL")
	tokenPath := flags.String("webcappi-token-file", "", "Capture Bearer file")
	if flags.Parse(args) != nil || flags.NArg() != 0 || !validPath(*config) || !validPath(*contextPath) || !validPath(*output) {
		return diagnostic("", "input", "invalid_config")
	}
	if _, err := captureURL(*base, 800, 480); err != nil {
		return diagnostic("", "input", "invalid_url")
	}
	token := ""
	if *tokenPath != "" {
		if !validPath(*tokenPath) {
			return diagnostic("", "input", "invalid_credentials")
		}
		data, err := readFile(*tokenPath, 8192)
		if err != nil {
			return diagnostic("", "input", "input_read")
		}
		var ok bool
		token, ok = terminalLine(data)
		if !ok || !bearer.MatchString(token) {
			return diagnostic("", "input", "invalid_credentials")
		}
	}
	data, err := readFile(*contextPath, 1<<20)
	if err != nil {
		return diagnostic("", "input", "input_read")
	}
	target, err := parseContext(data)
	if err != nil {
		return diagnostic("", "input", "invalid_config")
	}
	model, err := Prepare(ctx, *config, now, command, nil)
	if err != nil {
		return err
	}
	body, err := RenderHTML(model)
	if err != nil {
		return diagnostic("", "capture", "resource_limit")
	}
	w, h := target.logicalSize()
	data, err = capture(ctx, &http.Client{}, *base, token, body, w, h)
	if err != nil {
		return captureDiagnostic(ctx, err)
	}
	img, err := decodePNG(data, w, h)
	if err != nil {
		return diagnostic("", "capture", "invalid_png")
	}
	data, err = outputPNG(img, target.rotation)
	if err != nil {
		return diagnostic("", "capture", "resource_limit")
	}
	if err = writeOutput(ctx, *output, data); err != nil {
		if ctx.Err() != nil {
			return diagnostic("", "output", "cancelled")
		}
		return diagnostic("", "output", "output_write")
	}
	fmt.Fprintln(stdout, "Rendered iCalendar: 800x480 RGB")
	return nil
}
