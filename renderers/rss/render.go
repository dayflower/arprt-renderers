package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"html/template"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

const pngLimit = 32 << 20
const htmlLimit = 1 << 20

type renderContext struct {
	Version string `json:"version"`
	Target  struct {
		Width    int             `json:"width"`
		Height   int             `json:"height"`
		Palette  string          `json:"palette"`
		Rotation json.RawMessage `json:"rotation"`
	} `json:"renderTarget"`
	Device struct {
		Name string `json:"name"`
	} `json:"device"`
	rotation int
}

func parseContext(data []byte) (renderContext, error) {
	var c renderContext
	if err := json.Unmarshal(data, &c); err != nil {
		return c, errors.New("invalid Context JSON")
	}
	if c.Version != "v1" || c.Target.Width != 800 || c.Target.Height != 480 || (c.Target.Palette != "gray4" && c.Target.Palette != "spectra6") || strings.TrimSpace(c.Device.Name) == "" {
		return c, errors.New("expected Context v1 with device.name and 800 x 480 gray4/spectra6 target")
	}
	if r := c.Target.Rotation; len(r) > 0 {
		if bytes.Equal(bytes.TrimSpace(r), []byte("null")) {
			return c, errors.New("rotation must be 0, 90, 180 or 270")
		}
		if err := json.Unmarshal(r, &c.rotation); err != nil {
			return c, errors.New("rotation must be an integer")
		}
	}
	if r := c.rotation; r != 0 && r != 90 && r != 180 && r != 270 {
		return c, errors.New("rotation must be 0, 90, 180 or 270")
	}
	return c, nil
}
func (c renderContext) logicalSize() (int, int) {
	if c.rotation == 90 || c.rotation == 270 {
		return 480, 800
	}
	return 800, 480
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; font-src 'none'; connect-src 'none'; img-src data:"><title>{{.Title}}</title>
<style>
*{box-sizing:border-box}html,body{margin:0;width:100%;height:100%;background:#fff;color:#171717}
body{padding:16px;font-family:"Noto Sans CJK JP","Noto Sans JP",sans-serif;overflow:hidden}
main{height:100%;overflow:hidden}h1{font-size:16px;line-height:1.4;font-weight:500;margin:0 0 8px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
article{display:flow-root;padding:8px 0;border-top:1px solid #888;break-inside:avoid}h2{font-size:23px;line-height:1.35;font-weight:700;margin:0;overflow-wrap:anywhere;display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:2;overflow:hidden}
.thumbnail{float:right;width:96px;height:72px;object-fit:contain;margin:0 0 4px 12px;background:white}.meta{font-size:14px;line-height:1.4;color:#444;margin-top:3px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.summary{font-size:17px;line-height:1.4;margin-top:4px;max-height:4.2em;overflow:hidden;overflow-wrap:anywhere}.block{display:-webkit-box;-webkit-box-orient:vertical;-webkit-line-clamp:3;overflow:hidden}.summary-label{font-size:14px;line-height:1.4;margin-top:4px}.bullet:before{content:"• "}.empty{font-size:20px}
</style></head><body><main><h1>{{.Title}}</h1>{{range .Articles}}<article>{{if .Image}}<img class="thumbnail" src="{{.Image}}" alt="" width="96" height="72">{{end}}<h2>{{.Title}}</h2>{{if .Meta}}<div class="meta">{{.Meta}}</div>{{end}}{{if .Summary}}{{if .SummaryLabel}}<div class="summary-label">{{.SummaryLabel}}</div>{{end}}<div class="summary">{{range .Summary}}<span class="block{{if .Bullet}} bullet{{end}}">{{.Text}}</span>{{end}}</div>{{end}}</article>{{else}}<p class="empty">No articles available.</p>{{end}}</main>
<script>
(async()=>{await document.fonts.ready;await Promise.all([...document.images].map(img=>img.decode().catch(()=>img.remove())));await new Promise(requestAnimationFrame);for(const summary of document.querySelectorAll('.summary')){let remaining=3;for(const block of [...summary.children]){if(!remaining){block.remove();continue}block.style.webkitLineClamp=String(remaining);const line=parseFloat(getComputedStyle(block).lineHeight);remaining-=Math.min(remaining,Math.ceil(block.getBoundingClientRect().height/line))}}await new Promise(requestAnimationFrame);const main=document.querySelector('main');let rows=[...main.querySelectorAll('article')];const fits=row=>row.getBoundingClientRect().bottom<=main.getBoundingClientRect().bottom;while(rows.length>1&&!fits(rows.at(-1)))rows.pop().remove();if(rows.length&&!fits(rows[0]))rows[0].querySelector('.thumbnail')?.remove();if(rows.length&&!fits(rows[0])){rows[0].querySelector('.summary')?.remove();rows[0].querySelector('.summary-label')?.remove();rows[0].querySelector('.meta')?.remove()}if(rows.length&&!fits(rows[0]))rows[0].remove();document.body.id='rss-ready'})();
</script></body></html>`))

type viewArticle struct {
	Title, Meta, SummaryLabel string
	Image                     template.URL
	Summary                   []summaryBlock
}
type viewFeed struct {
	Title    string
	Articles []viewArticle
}

func renderHTML(f feed, title string, maxItems int, summary bool, location *time.Location) ([]byte, error) {
	view := viewFeed{Title: f.Title}
	if title != "" {
		view.Title = clip(compact(title), 256)
	}
	for i, a := range f.Articles {
		if i >= maxItems {
			break
		}
		v := viewArticle{Title: a.Title}
		meta := []string{}
		if a.Category != "" {
			meta = append(meta, a.Category)
		}
		if a.BookmarkCount != nil {
			meta = append(meta, strconv.FormatUint(*a.BookmarkCount, 10)+" users")
		}
		source := a.Source
		if source == "" {
			source = f.Title
		}
		if source != view.Title {
			meta = append(meta, source)
		}
		if !a.Date.IsZero() {
			meta = append(meta, a.Date.In(location).Format("01/02 15:04"))
		}
		v.Meta = strings.Join(meta, " · ")
		if summary {
			v.SummaryLabel = a.SummaryLabel
			bullets := 0
			for _, block := range a.Summary {
				if block.Bullet {
					bullets++
					if bullets > 3 {
						continue
					}
				}
				v.Summary = append(v.Summary, block)
			}
		}
		view.Articles = append(view.Articles, v)
	}
	var b bytes.Buffer
	if err := page.Execute(&b, view); err != nil {
		return nil, err
	}
	if b.Len() > htmlLimit {
		return nil, errors.New("HTML exceeds size limit")
	}
	// Reserve all escaped text and template overhead before spending the remaining
	// budget on images. Earlier articles have priority; text is never discarded.
	remaining := htmlLimit - b.Len()
	for i := range view.Articles {
		data := f.Articles[i].ImageData
		if !validThumbnail(data) {
			continue
		}
		for attempt := 0; attempt < 2 && data != ""; attempt++ {
			view.Articles[i].Image = template.URL(data)
			var next bytes.Buffer
			if err := page.Execute(&next, view); err != nil {
				return nil, err
			}
			extra := next.Len() - b.Len()
			if extra <= remaining {
				remaining -= extra
				b = next
				break
			}
			view.Articles[i].Image = ""
			if attempt == 0 {
				data = reduceThumbnail(data)
			}
		}
	}
	return b.Bytes(), nil
}

func readBounded(r io.Reader, limit int) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, errors.New("incomplete input body")
	}
	if len(b) > limit {
		return nil, errors.New("input exceeds size limit")
	}
	return b, nil
}
func readFile(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readBounded(f, limit)
}
func newHTTPClient() *http.Client {
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || httpURL(req.URL.String()) == "" {
			return errors.New("redirect rejected")
		}
		return nil
	}}
}
func fetchFeed(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("invalid feed URL")
	}
	req.Header.Set("User-Agent", "arprt-rss/0.1")
	req.Header.Set("Accept", "application/rss+xml, application/rdf+xml, application/xml, text/xml")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("feed request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed: HTTP %d", resp.StatusCode)
	}
	return readBounded(resp.Body, feedLimit)
}
func captureURL(base string, width, height int) (string, error) {
	if httpURL(base) == "" {
		return "", errors.New("expected HTTP(S) webcappi base URL without credentials")
	}
	u, _ := url.Parse(base)
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("webcappi base URL must not have a query or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/capture"
	u.RawPath = ""
	q := url.Values{"width": {strconv.Itoa(width)}, "height": {strconv.Itoa(height)}, "html_mode": {"file"}, "ready_selector": {"#rss-ready"}}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func capture(ctx context.Context, client *http.Client, base, token string, body []byte, width, height int) ([]byte, error) {
	address, err := captureURL(base, width, height)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	// Never follow redirects carrying the service's Bearer token or HTML body.
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
		if err != nil {
			return nil, errors.New("invalid capture request")
		}
		req.Header.Set("Content-Type", "text/html; charset=utf-8")
		req.Header.Set("Accept", "image/png")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := noRedirect.Do(req)
		if err != nil {
			return nil, errors.New("capture request failed or timed out")
		}
		if (resp.StatusCode == 503 || resp.StatusCode == 504) && attempt == 0 {
			// Bound error reads and retry delays; all attempts share the same deadline.
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			delay := time.Second
			if v, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && v >= 0 {
				if v > 2 {
					v = 2
				}
				delay = time.Duration(v) * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, errors.New("capture deadline exceeded")
			case <-timer.C:
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("capture: HTTP %d", resp.StatusCode)
		}
		media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if err != nil || media != "image/png" {
			resp.Body.Close()
			return nil, errors.New("capture did not return image/png")
		}
		data, err := readBounded(resp.Body, pngLimit)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if _, err := decodePNG(data, width, height); err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, errors.New("capture failed")
}

func decodePNG(data []byte, width, height int) (image.Image, error) {
	if len(data) > pngLimit || len(data) < 8 || !bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return nil, errors.New("invalid PNG signature or size")
	}
	// Go's decoder stops at IEND. Check every chunk CRC and reject trailing bytes,
	// animation, and extra IEND payload before handing it to the decoder.
	ended := false
	for offset := 8; offset < len(data); {
		if len(data)-offset < 12 {
			return nil, errors.New("truncated PNG chunk")
		}
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if length > len(data)-offset-12 {
			return nil, errors.New("truncated PNG data")
		}
		chunk := data[offset+4 : offset+8+length]
		kind := string(chunk[:4])
		if crc32.ChecksumIEEE(chunk) != binary.BigEndian.Uint32(data[offset+8+length:offset+12+length]) {
			return nil, errors.New("invalid PNG checksum")
		}
		if kind == "acTL" || kind == "fcTL" || kind == "fdAT" {
			return nil, errors.New("capture must be a static PNG")
		}
		offset += length + 12
		if kind == "IEND" {
			if length != 0 || offset != len(data) {
				return nil, errors.New("invalid PNG ending")
			}
			ended = true
		}
	}
	if !ended {
		return nil, errors.New("missing PNG ending")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != width || config.Height != height {
		return nil, errors.New("capture PNG dimensions do not match logical canvas")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("invalid capture PNG")
	}
	return img, nil
}
func outputPNG(src image.Image, rotation int) ([]byte, error) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if rotation == 90 || rotation == 270 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch rotation {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			case 270:
				dx, dy = y, w-1-x
			}
			r, g, b, a := src.At(src.Bounds().Min.X+x, src.Bounds().Min.Y+y).RGBA()
			// RGBA returns premultiplied channels; composite transparency onto white.
			dst.SetRGBA(dx, dy, color.RGBA{uint8((r + 65535 - a) >> 8), uint8((g + 65535 - a) >> 8), uint8((b + 65535 - a) >> 8), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, dst); err != nil {
		return nil, err
	}
	if b.Len() > pngLimit {
		return nil, errors.New("output PNG exceeds controller input limit")
	}
	return b.Bytes(), nil
}
func writeOutput(ctx context.Context, path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".rss-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
