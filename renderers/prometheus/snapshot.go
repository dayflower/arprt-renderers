package prometheus

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
)

type SnapshotEntry struct {
	Request Request `json:"request"`
	File    string  `json:"file"`
}
type Manifest struct {
	Version  int             `json:"version"`
	PlanHash string          `json:"planHash"`
	Entries  []SnapshotEntry `json:"entries"`
}

//go:embed snapshot.schema.json
var snapshotSchemaJSON []byte

var responseFilename = regexp.MustCompile(`^response-[0-9]{3}\.json$`)

// ExpectedManifest describes synthetic inputs; it does not record API traffic.
func ExpectedManifest(p Plan) Manifest {
	m := Manifest{Version: 1, PlanHash: hash(p), Entries: []SnapshotEntry{}}
	for i, r := range p.Requests {
		m.Entries = append(m.Entries, SnapshotEntry{r, formatResponseName(i)})
	}
	return m
}
func formatResponseName(i int) string { return fmt.Sprintf("response-%03d.json", i) }

// LoadSnapshot checks the complete plan before returning bounded raw responses.
// Response parsing/normalization is shared with the API client by its caller.
// os.Root permits mounted directory symlinks but prevents response path escapes.
func LoadSnapshot(dir string, p Plan) (map[string]json.RawMessage, error) {
	root, e := os.OpenRoot(dir)
	if e != nil {
		return nil, errors.New("snapshot open failure")
	}
	defer root.Close()
	read := func(name string, limit int) ([]byte, error) {
		f, e := root.Open(name)
		if e != nil {
			return nil, errors.New("snapshot file missing")
		}
		defer f.Close()
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() {
			return nil, errors.New("snapshot requires regular files")
		}
		return bounded(f, limit)
	}
	b, e := read("manifest.json", ManifestLimit)
	if e != nil {
		return nil, e
	}
	v, e := decodeJSON(b)
	if e != nil {
		return nil, e
	}
	var schema map[string]any
	if json.Unmarshal(snapshotSchemaJSON, &schema) != nil || validateSchema(schema, v, schema) != nil {
		return nil, errors.New("invalid snapshot manifest schema")
	}
	b, _ = json.Marshal(normalizeNumbers(v))
	var m Manifest
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(&m); e != nil {
		return nil, errors.New("invalid snapshot manifest")
	}
	if m.Version != 1 || m.PlanHash != hash(p) || len(m.Entries) != len(p.Requests) {
		return nil, errors.New("snapshot plan mismatch")
	}
	out := map[string]json.RawMessage{}
	files := map[string]bool{}
	total := 0
	for i, entry := range m.Entries {
		if !reflect.DeepEqual(entry.Request, p.Requests[i]) || !responseFilename.MatchString(entry.File) || filepath.Base(entry.File) != entry.File || files[entry.File] {
			return nil, errors.New("snapshot request mismatch")
		}
		files[entry.File] = true
		b, e := read(entry.File, ResponseLimit)
		if e != nil {
			return nil, e
		}
		total += len(b)
		if total > TotalBytesLimit {
			return nil, errors.New("snapshot byte budget exceeded")
		}
		out[entry.Request.ID] = b
	}
	return out, nil
}
