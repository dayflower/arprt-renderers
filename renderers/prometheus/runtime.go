package prometheus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func readRuntimeFile(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, Diagnostic{Code: "input_read"}
	}
	defer f.Close()
	b, err := bounded(f, limit)
	if err != nil {
		return nil, Diagnostic{Code: "input_read"}
	}
	return b, nil
}

// Run owns the single execution deadline. It writes only the final output and a
// uniquely named temporary sibling, after every acquisition/layout/image check.
func Run(parent context.Context, args []string, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, ExecutionBudget)
	defer cancel()
	o, err := ParseOptions(args, time.Now)
	if err != nil {
		return err
	}
	c, err := LoadConfig(o.Config, o.PrometheusURL)
	if err != nil {
		return err
	}
	b, err := readRuntimeFile(o.Context, ContextLimit)
	if err != nil {
		return err
	}
	target, err := ParseContext(b)
	if err != nil {
		return err
	}
	w, h := target.LogicalSize()
	address, err := captureURL(o.WebcappiURL, w, h)
	if err != nil {
		return err
	}
	var auth *Auth
	if o.WebcappiTokenFile != "" {
		auth = &Auth{Type: "bearer", CredentialsFile: o.WebcappiTokenFile}
	}
	authorization, err := loadAuthorization(auth)
	if err != nil {
		return Diagnostic{Code: "capture_credentials"}
	}
	plan, err := BuildPlan(c, target, o.Now)
	if err != nil {
		return err
	}
	model, _, err := LoadModel(ctx, c, plan, o.Input)
	if err != nil {
		return err
	}
	body, err := RenderHTML(c, target, model)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	captureCtx, stop := context.WithDeadline(ctx, deadline.Add(-OutputReserve))
	data, err := capture(captureCtx, address, authorization, body)
	stop()
	if err != nil {
		return err
	}
	img, err := decodePNG(data, w, h)
	if err != nil {
		return Diagnostic{Code: "capture_png"}
	}
	data, err = outputPNG(img, target.Rotation)
	if err != nil {
		return Diagnostic{Code: "output_png"}
	}
	if err = writeOutput(ctx, o.Output, data); err != nil {
		return Diagnostic{Code: "output_replace"}
	}
	fmt.Fprintf(stdout, "Rendered Prometheus: 800x480 RGB, %d bytes\n", len(data))
	return nil
}

func captureURL(base string, w, h int) (string, error) {
	u, err := BaseURL(base)
	if err != nil {
		return "", Diagnostic{Code: "capture_url"}
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/capture"
	u.RawPath = ""
	u.RawQuery = url.Values{"width": {strconv.Itoa(w)}, "height": {strconv.Itoa(h)}, "html_mode": {"file"}, "ready_selector": {"#prometheus-ready"}}.Encode()
	return u.String(), nil
}

func capture(ctx context.Context, address, authorization string, body []byte) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, Diagnostic{Code: "canceled"}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	transport.MaxResponseHeaderBytes = 64 << 10
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return nil, Diagnostic{Code: "capture_request"}
	}
	req.Header.Set("Content-Type", "text/html; charset=utf-8")
	req.Header.Set("Accept", "image/png")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, Diagnostic{Code: "capture_request"}
	}
	defer resp.Body.Close()
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != 200 || err != nil || media != "image/png" || resp.Header.Get("Content-Encoding") != "" {
		return nil, Diagnostic{Code: "capture_response"}
	}
	b, err := bounded(resp.Body, pngLimit)
	if err != nil {
		return nil, errors.New("capture_body")
	}
	return b, nil
}
