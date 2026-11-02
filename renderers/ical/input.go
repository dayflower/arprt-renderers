package ical

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

// Diagnostic contains only public, closed classifications and validated IDs.
type Diagnostic struct {
	Source, Stage, Class string
	Status               int
}

func (e Diagnostic) Error() string {
	s := e.Stage + ":" + e.Class
	if e.Source != "" {
		s = e.Source + ":" + s
	}
	if e.Status != 0 {
		s += ":" + strconv.Itoa(e.Status)
	}
	return s
}
func diagnostic(id, stage, class string) error {
	return Diagnostic{Source: id, Stage: stage, Class: class}
}

type configFile struct {
	Version          int          `json:"version"`
	Title            string       `json:"title"`
	View             string       `json:"view"`
	Timezone         string       `json:"timezone"`
	WeekStartsOn     string       `json:"weekStartsOn"`
	Calendars        []sourceFile `json:"calendars"`
	HolidayCalendars []sourceFile `json:"holidayCalendars"`
}
type sourceFile struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	URL     *string   `json:"url"`
	URLFile *string   `json:"urlFile"`
	Auth    *authFile `json:"auth"`
}
type authFile struct {
	Type            string `json:"type"`
	CredentialsFile string `json:"credentialsFile"`
}

// Input is an immutable execution snapshot. Private fields prevent accidental
// serialization of URLs and credentials. LoadInput is its only constructor.
type Input struct {
	title, view, timezone, weekStartsOn string
	sources                             []source
}
type source struct {
	id, name, address, authorization string
	holiday                          bool
}

var sourceID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var bearer = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

// strictJSON rejects duplicates and null at every depth before typed decoding.
func strictJSON(data []byte, dst any) error {
	if !utf8.Valid(data) {
		return Invalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	var value func() error
	value = func() error {
		t, err := d.Token()
		if err != nil || t == nil {
			return Invalid
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, err := d.Token()
					if err != nil {
						return Invalid
					}
					key, ok := k.(string)
					if !ok || seen[key] {
						return Invalid
					}
					seen[key] = true
					if err := value(); err != nil {
						return err
					}
				}
			case '[':
				for d.More() {
					if err := value(); err != nil {
						return err
					}
				}
			default:
				return Invalid
			}
			if _, err := d.Token(); err != nil {
				return Invalid
			}
		}
		return nil
	}
	if value() != nil {
		return Invalid
	}
	if _, err := d.Token(); err != io.EOF {
		return Invalid
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil {
		return Invalid
	}
	return nil
}
func validText(s string, limit int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > limit || strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func validPath(s string) bool {
	return len(s) <= 512 && strings.HasPrefix(s, "/") && s != "/" && path.Clean(s) == s && validText(s, 512)
}
func readFile(name string, limit int) ([]byte, error) {
	if !validPath(name) {
		return nil, Invalid
	}
	f, err := os.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, Invalid
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, Invalid
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil || len(b) > limit {
		return nil, Invalid
	}
	return b, nil
}
func terminalLine(b []byte) (string, bool) {
	s := string(b)
	if strings.HasSuffix(s, "\r\n") {
		s = strings.TrimSuffix(s, "\r\n")
	} else {
		s = strings.TrimSuffix(s, "\n")
	}
	return s, utf8.ValidString(s) && !strings.ContainsAny(s, "\r\n")
}
func validateURL(raw string, authenticated bool) (*url.URL, error) {
	if len(raw) > 8192 || !utf8.ValidString(raw) || strings.Contains(raw, "#") {
		return nil, Invalid
	}
	for _, r := range raw {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return nil, Invalid
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, Invalid
	}
	if u.Scheme == "webcal" {
		u.Scheme = "https"
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (authenticated && u.Scheme != "https") {
		return nil, Invalid
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, Invalid
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, Invalid
		}
	}
	return u, nil
}

// LoadInput reads all files once, before any network access. Symlinks are followed.
func LoadInput(filename string) (*Input, error) {
	data, err := readFile(filename, 64<<10)
	if err != nil {
		return nil, diagnostic("", "input", "input_read")
	}
	c := configFile{Title: "カレンダー", View: "month", Timezone: "Asia/Tokyo", WeekStartsOn: "sunday"}
	if strictJSON(data, &c) != nil || c.Version != 1 || !validText(c.Title, 256) || (c.View != "month" && c.View != "agenda") || (c.WeekStartsOn != "sunday" && c.WeekStartsOn != "monday") || !validText(c.Timezone, 128) || c.Timezone == "Local" {
		return nil, diagnostic("", "input", "invalid_config")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return nil, diagnostic("", "input", "invalid_config")
	}
	n := len(c.Calendars) + len(c.HolidayCalendars)
	if n < 1 || n > 10 {
		return nil, diagnostic("", "input", "invalid_config")
	}
	input := &Input{title: c.Title, view: c.View, timezone: c.Timezone, weekStartsOn: c.WeekStartsOn}
	seen := map[string]bool{}
	for index, s := range append(c.Calendars, c.HolidayCalendars...) {
		if !sourceID.MatchString(s.ID) || seen[s.ID] {
			return nil, diagnostic("", "input", "invalid_config")
		}
		seen[s.ID] = true
		if !validText(s.Name, 128) || (s.URL == nil) == (s.URLFile == nil) {
			return nil, diagnostic(s.ID, "input", "invalid_config")
		}
		if s.URLFile != nil && !validPath(*s.URLFile) {
			return nil, diagnostic(s.ID, "input", "invalid_config")
		}
		raw := ""
		if s.URL != nil {
			raw = *s.URL
		} else {
			b, err := readFile(*s.URLFile, 8194)
			if err != nil {
				return nil, diagnostic(s.ID, "input", "input_read")
			}
			var ok bool
			raw, ok = terminalLine(b)
			if !ok {
				return nil, diagnostic(s.ID, "input", "invalid_url")
			}
		}
		u, err := validateURL(raw, s.Auth != nil)
		if err != nil {
			return nil, diagnostic(s.ID, "input", "invalid_url")
		}
		auth := ""
		if s.Auth != nil {
			if (s.Auth.Type != "basic" && s.Auth.Type != "bearer") || !validPath(s.Auth.CredentialsFile) {
				return nil, diagnostic(s.ID, "input", "invalid_credentials")
			}
			b, err := readFile(s.Auth.CredentialsFile, 8<<10)
			if err != nil {
				return nil, diagnostic(s.ID, "input", "input_read")
			}
			if s.Auth.Type == "basic" {
				var v struct {
					Username string `json:"username"`
					Password string `json:"password"`
				}
				if strictJSON(b, &v) != nil || !credentialText(v.Username) || !credentialText(v.Password) || strings.Contains(v.Username, ":") {
					return nil, diagnostic(s.ID, "input", "invalid_credentials")
				}
				auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(v.Username+":"+v.Password))
			} else {
				token, ok := terminalLine(b)
				if !ok || !bearer.MatchString(token) {
					return nil, diagnostic(s.ID, "input", "invalid_credentials")
				}
				auth = "Bearer " + token
			}
		}
		input.sources = append(input.sources, source{s.ID, s.Name, u.String(), auth, index >= len(c.Calendars)})
	}
	return input, nil
}
func credentialText(s string) bool {
	if s == "" || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
