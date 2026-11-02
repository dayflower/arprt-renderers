package ical

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, name, data string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func loadTestInput(t *testing.T, text string) (*Input, error) {
	t.Helper()
	return LoadInput(writeTestFile(t, "config.json", text))
}
func TestStrictInput(t *testing.T) {
	good := `{"version":1,"calendars":[{"id":"work","name":"仕事","url":"https://calendar.example/work?private=value"}]}`
	input, err := loadTestInput(t, good)
	if err != nil || input.title != "カレンダー" || input.view != "month" || input.timezone != "Asia/Tokyo" || input.weekStartsOn != "sunday" {
		t.Fatalf("defaults: %v", err)
	}
	for _, bad := range []string{
		strings.Replace(good, `"version":1`, `"version":2`, 1),
		strings.Replace(good, `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(good, `"name":"仕事"`, `"name":"仕事","name":"仕事"`, 1),
		strings.Replace(good, `"version":1`, `"version":1,"extra":1`, 1),
		strings.Replace(good, `"version":1`, `"version":1,"title":null`, 1),
		strings.Replace(good, `"version":1`, `"version":1,"title":" "`, 1),
		strings.Replace(good, `"version":1`, `"version":1,"view":""`, 1),
		strings.Replace(good, `"version":1`, `"version":1,"timezone":"Local"`, 1),
		strings.Replace(good, `"id":"work"`, `"id":"private/unsafe"`, 1),
		strings.Replace(good, `"name":"仕事"`, `"name":"仕事","unknown":true`, 1),
		strings.Replace(good, `"name":"仕事"`, `"name":"仕事","auth":{}`, 1),
		good + `{}`, `{"version":1}`, `{"version":1,"calendars":null}`,
		`{"version":1,"calendars":[{"id":"a","name":"A","url":"https://example.com"}],"holidayCalendars":[{"id":"a","name":"B","url":"https://example.com"}]}`,
	} {
		if input, err := loadTestInput(t, bad); err == nil || input != nil {
			t.Fatalf("accepted invalid configuration: %s", bad)
		}
	}
	if input, err := loadTestInput(t, strings.Replace(good, `"calendars"`, `"holidayCalendars"`, 1)); err != nil || !input.sources[0].holiday {
		t.Fatalf("holiday only: %v", err)
	}
}
func TestURLs(t *testing.T) {
	for _, raw := range []string{"https://example.com/a?q=private", "http://example.com", "webcal://example.com/a?q=private", "https://[::1]:8443/a"} {
		u, err := validateURL(raw, false)
		if err != nil {
			t.Fatal(raw)
		}
		if strings.HasPrefix(raw, "webcal:") && u.String() != "https://example.com/a?q=private" {
			t.Fatal(u)
		}
	}
	for _, raw := range []string{"https://example.com/#", "https://user:password@example.com", "file:///etc/test", "https://example.com:99999", "https://example.com:", "https://example.com/has space", "//example.com", "https:///path", "https://example.com/\n", "https://example.com/%zz"} {
		if _, err := validateURL(raw, false); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if _, err := validateURL("http://example.com", true); err == nil {
		t.Fatal("plaintext authentication")
	}
}
func TestCredentialFilesAndSnapshot(t *testing.T) {
	for _, tc := range []struct {
		kind, body, want string
		valid            bool
	}{
		{"basic", `{"username":" 日本語 ","password":" p:ass "}`, "", true},
		{"basic", `{"username":"a:b","password":"private"}`, "", false},
		{"basic", `{"username":"a","password":""}`, "", false},
		{"basic", `{"username":"a","password":"x","password":"y"}`, "", false},
		{"basic", `{"username":"a","password":"x","other":true}`, "", false},
		{"basic", `{"username":"a","password":"x\n"}`, "", false},
		{"bearer", "private.token+/==\r\n", "Bearer private.token+/==", true},
		{"bearer", "private\n\n", "", false}, {"bearer", " private", "", false},
		{"bearer", "pri vate", "", false}, {"bearer", "", "", false}, {"bearer", "private=token", "", false},
	} {
		t.Run(tc.kind+tc.body, func(t *testing.T) {
			cred := writeTestFile(t, "credential", tc.body)
			address := writeTestFile(t, "url", "webcal://calendar.example/secret?key=private\r\n")
			// Projected-volume style symlinks must remain usable.
			link := filepath.Join(t.TempDir(), "url")
			if err := os.Symlink(address, link); err != nil {
				t.Fatal(err)
			}
			config := map[string]any{"version": 1, "calendars": []any{map[string]any{"id": "work", "name": "Work", "urlFile": link, "auth": map[string]string{"type": tc.kind, "credentialsFile": cred}}}}
			b, _ := json.Marshal(config)
			input, err := loadTestInput(t, string(b))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
			if err != nil {
				if strings.Contains(err.Error(), "private") {
					t.Fatal("credential leaked")
				}
				return
			}
			if tc.want != "" && input.sources[0].authorization != tc.want {
				t.Fatal("token changed")
			}
			if tc.kind == "basic" && input.sources[0].authorization != "Basic IOaXpeacrOiqniA6IHA6YXNzIA==" {
				t.Fatal("basic whitespace/UTF-8 changed")
			}
			before := input.sources[0]
			if err := os.WriteFile(cred, []byte("replaced"), 0600); err != nil {
				t.Fatal(err)
			}
			if input.sources[0] != before {
				t.Fatal("input was not snapshotted")
			}
			serialized, _ := json.Marshal(input)
			if strings.Contains(string(serialized), "private") {
				t.Fatal("serialized secret")
			}
		})
	}
	for _, s := range []string{"url\n\n", "url\r", " url", "url\ninside"} {
		path := writeTestFile(t, "url", s)
		b, _ := json.Marshal(map[string]any{"version": 1, "calendars": []any{map[string]string{"id": "a", "name": "A", "urlFile": path}}})
		if _, err := loadTestInput(t, string(b)); err == nil {
			t.Fatal("invalid URL file")
		}
	}
}
func TestInputBoundsAndPaths(t *testing.T) {
	for _, p := range []string{"relative", "/a/../b", "/a/./b", "/a//b", "/", "/a\x00b"} {
		if validPath(p) {
			t.Fatalf("accepted %q", p)
		}
	}
	if _, err := LoadInput(writeTestFile(t, "large", strings.Repeat(" ", 64<<10+1))); err == nil {
		t.Fatal("unbounded config")
	}
	sources := make([]map[string]string, 11)
	for j := range sources {
		sources[j] = map[string]string{"id": string(rune('a' + j)), "name": "Calendar", "url": "https://example.com"}
	}
	b, _ := json.Marshal(map[string]any{"version": 1, "calendars": sources})
	if _, err := loadTestInput(t, string(b)); err == nil {
		t.Fatal("too many sources")
	}
}
