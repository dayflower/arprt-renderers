// Weather renders Open-Meteo forecasts for an ARPRT Kubernetes Job.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

var modelID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)

func run(parent context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("weather", flag.ContinueOnError)
	flags.SetOutput(stderr)
	contextPath := flags.String("context", "/arprt/context.json", "Render Context v1 JSON")
	output := flags.String("output", "/arprt/output.png", "Output PNG path")
	latitude := flags.Float64("latitude", math.NaN(), "Required latitude (-90 to 90)")
	longitude := flags.Float64("longitude", math.NaN(), "Required longitude (-180 to 180)")
	locationName := flags.String("location-name", "", "Optional display name below the current local time")
	model := flags.String("model", "", "Optional single Open-Meteo model identifier")
	unit := flags.String("temperature-unit", "celsius", "celsius or fahrenheit")
	input := flags.String("input", "", "Read an Open-Meteo JSON snapshot instead of requesting the API")
	webcappi := flags.String("webcappi-url", "", "Required webcappi base URL")
	tokenFile := flags.String("webcappi-token-file", "", "Read-only Bearer token file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if math.IsNaN(*latitude) || math.IsInf(*latitude, 0) || *latitude < -90 || *latitude > 90 || math.IsNaN(*longitude) || math.IsInf(*longitude, 0) || *longitude < -180 || *longitude > 180 {
		return errors.New("valid latitude and longitude are required")
	}
	if !utf8.ValidString(*locationName) || strings.IndexFunc(*locationName, unicode.IsControl) >= 0 {
		return errors.New("location-name must be UTF-8 text without control characters")
	}
	*locationName = strings.TrimSpace(*locationName)
	if utf8.RuneCountInString(*locationName) > 128 {
		return errors.New("location-name must be at most 128 characters")
	}
	if *model != "" && !modelID.MatchString(*model) {
		return errors.New("expected one model identifier")
	}
	if *unit != "celsius" && *unit != "fahrenheit" {
		return errors.New("temperature-unit must be celsius or fahrenheit")
	}
	if _, err := captureURL(*webcappi, 800, 480); err != nil {
		return err
	}
	now := time.Now()
	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	data, err := readFile(*contextPath, 1<<20)
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
	client := newHTTPClient()
	defer client.CloseIdleConnections()
	if *input != "" {
		data, err = readFile(*input, forecastLimit)
	} else {
		data, err = fetchForecast(ctx, client, forecastURL(*latitude, *longitude, *model, *unit))
	}
	if err != nil {
		return errors.New("weather input failed")
	}
	view, err := buildView(data, now, *model, *unit)
	if err != nil {
		return err
	}
	view.LocationName = *locationName
	view.Palette = target.Target.Palette
	body, err := renderHTML(view)
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
		return errors.New("cannot replace output")
	}
	fmt.Fprintf(stdout, "Rendered weather: 800x480 RGB, %d bytes\n", len(data))
	return nil
}

func validToken(s string) bool {
	if s == "" {
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
	return s[0] != '='
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
		fmt.Fprintln(os.Stderr, "weather:", err)
		os.Exit(1)
	}
}
