package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

const captureCountQuery = "attributeValue:data-count:#rss-ready"
const captureMetadataHeader = "Webcappi-Capture-Metadata"

type rssCursor struct {
	Version string `json:"version"`
	After   string `json:"after"`
}

func articleKeys(articles []article) []string {
	keys := make([]string, len(articles))
	occurrences := make(map[[sha256.Size]byte]int)
	for i, a := range articles {
		if a.ID != "" {
			keys[i] = fmt.Sprintf("%x", sha256.Sum256([]byte("id:"+a.ID)))
			continue
		}
		fallback := fmt.Sprintf("article:%q %q %q", a.Title, a.Date.UTC().String(), a.Source)
		digest := sha256.Sum256([]byte(fallback))
		occurrences[digest]++
		keys[i] = fmt.Sprintf("%x:%d", digest, occurrences[digest])
	}
	return keys
}

func pageStart(keys []string, state json.RawMessage) int {
	var cursor rssCursor
	if json.Unmarshal(state, &cursor) == nil && cursor.Version == "v1" {
		for i, key := range keys {
			if key == cursor.After {
				return (i + 1) % len(keys)
			}
		}
	}
	return 0
}

func pageResult(remainingKeys []string, offered, displayed int) ([]byte, error) {
	if displayed > offered || (offered > 0 && displayed == 0) {
		return nil, errors.New("capture article count does not match the RSS page")
	}
	if offered == 0 {
		return []byte(`{"version":"v1"}`), nil
	}
	var next *rssCursor
	if displayed < len(remainingKeys) {
		next = &rssCursor{Version: "v1", After: remainingKeys[displayed-1]}
	}
	return json.Marshal(struct {
		Version   string     `json:"version"`
		NextState *rssCursor `json:"nextState"`
	}{Version: "v1", NextState: next})
}

func captureArticleCount(header http.Header) (int, error) {
	values := header.Values(captureMetadataHeader)
	if len(values) != 1 {
		return 0, errors.New("expected one webcappi capture metadata header")
	}
	encoded, ok := strings.CutPrefix(values[0], "b64url=")
	if !ok || len(encoded) < 2 || len(encoded) > 3 {
		return 0, errors.New("invalid webcappi article count metadata")
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return 0, errors.New("invalid webcappi article count metadata")
	}
	count, err := strconv.Atoi(string(data))
	if err != nil || count < 0 || strconv.Itoa(count) != string(data) {
		return 0, errors.New("invalid webcappi article count metadata")
	}
	return count, nil
}

func resultFile(path, output string, inputs ...string) (string, error) {
	outputPath, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	if path == "" {
		path = filepath.Join(filepath.Dir(outputPath), "result.json")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if path == outputPath || filepath.Dir(path) != filepath.Dir(outputPath) {
		return "", errors.New("result path must be a separate file beside the output PNG")
	}
	for _, input := range inputs {
		if input == "" {
			continue
		}
		inputPath, err := filepath.Abs(input)
		if err != nil {
			return "", err
		}
		if path == inputPath {
			return "", errors.New("result path must differ from all input files")
		}
	}
	return path, nil
}
