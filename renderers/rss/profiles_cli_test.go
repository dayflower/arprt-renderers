package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilesCLI(t *testing.T) { checkProfilesCLI(t, "") }

func checkProfilesCLI(t *testing.T, binary string) {
	t.Helper()
	for _, test := range []struct{ profile, item, want, absent string }{
		{"google-news", `<item><title>合成見出し - 合成媒体</title><source>合成媒体</source><description>Related story sentinel</description></item>`, `<h2>合成見出し</h2><div class="meta">合成媒体</div>`, "Related story sentinel"},
		{"livedoor", `<item><title>合成見出し</title><description><![CDATA[<p>ざっくり言うと</p><ul><li>第一の要点</li><li>第二の要点</li><li>第三の要点</li></ul><a href="x">記事を読む</a>]]></description></item>`, `class="block bullet">第一の要点`, "記事を読む"},
		{"auto", `<item xmlns:h="http://www.hatena.ne.jp/info/xmlns#"><title>合成見出し</title><h:bookmarkcount>0</h:bookmarkcount></item>`, "0 users", "<img"},
		{"generic", `<item><title>合成見出し - 合成媒体</title><source>合成媒体</source><description>Generic summary sentinel</description></item>`, "Generic summary sentinel", "0 users"},
	} {
		t.Run(test.profile, func(t *testing.T) {
			root := t.TempDir()
			input, contextPath, output := filepath.Join(root, "feed.xml"), filepath.Join(root, "context.json"), filepath.Join(root, "output.png")
			if err := os.WriteFile(input, rssItems(test.item), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(contextPath, contextBytes(0, "gray4"), 0o644); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || !bytes.Contains(body, []byte(test.want)) || bytes.Contains(body, []byte(test.absent)) {
					t.Errorf("profile %s HTML did not match expected text", test.profile)
				}
				w.Header().Set("Content-Type", "image/png")
				w.Write(pngBytes(t, 800, 480))
			}))
			defer server.Close()
			args := []string{"--context", contextPath, "--input", input, "--output", output, "--webcappi-url", server.URL, "--profile", test.profile}
			var log bytes.Buffer
			var err error
			if binary == "" {
				err = run(context.Background(), args, &log, &log)
			} else {
				cmd := exec.Command(binary, args...)
				cmd.Stdout, cmd.Stderr = &log, &log
				err = cmd.Run()
			}
			if err != nil {
				t.Fatalf("%v: %s", err, &log)
			}
			chosen := test.profile
			if chosen == "auto" {
				chosen = "hatena"
			}
			if !strings.Contains(log.String(), fmt.Sprintf("%s profile", chosen)) {
				t.Fatalf("missing selected profile: %s", &log)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			assertRotated(t, data, 0)
		})
	}
}
