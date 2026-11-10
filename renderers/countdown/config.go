package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const configLimit = 64 << 10

type config struct {
	Version    int    `json:"version"`
	TargetDate string `json:"targetDate"`
	Timezone   string `json:"timezone"`
	Title      string `json:"title"`
	target     time.Time
	location   *time.Location
}

// Decode the flat object explicitly so duplicate keys and nulls cannot hide
// behind encoding/json's usual last-key-wins and zero-value behavior.
func parseConfig(data []byte) (config, error) {
	c := config{Timezone: "Asia/Tokyo"}
	invalid := errors.New("invalid countdown configuration")
	if len(data) > configLimit || !utf8.Valid(data) {
		return c, invalid
	}
	d := json.NewDecoder(bytes.NewReader(data))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return c, invalid
	}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return c, invalid
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return c, invalid
		}
		seen[name] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return c, invalid
		}
		switch name {
		case "version":
			err = json.Unmarshal(raw, &c.Version)
		case "targetDate":
			err = json.Unmarshal(raw, &c.TargetDate)
		case "timezone":
			err = json.Unmarshal(raw, &c.Timezone)
		case "title":
			err = json.Unmarshal(raw, &c.Title)
		default:
			return c, invalid
		}
		if err != nil {
			return c, invalid
		}
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') {
		return c, invalid
	}
	if _, err := d.Token(); err != io.EOF || c.Version != 1 || !seen["targetDate"] {
		return c, invalid
	}
	c.target, err = time.Parse("2006-01-02", c.TargetDate)
	if err != nil || c.target.Year() < 1 || c.target.Format("2006-01-02") != c.TargetDate {
		return c, errors.New("targetDate must be a valid YYYY-MM-DD date in years 0001 through 9999")
	}
	if c.Timezone == "" || c.Timezone == "Local" {
		return c, errors.New("timezone must be an IANA identifier")
	}
	c.location, err = time.LoadLocation(c.Timezone)
	if err != nil {
		return c, errors.New("unknown timezone")
	}
	if strings.IndexFunc(c.Title, unicode.IsControl) >= 0 {
		return c, errors.New("title must not contain control characters")
	}
	c.Title = strings.TrimSpace(c.Title)
	if utf8.RuneCountInString(c.Title) > 128 {
		return c, errors.New("title must be at most 128 characters")
	}
	return c, nil
}

type countdownView struct {
	Title, Target, Today, Count, Status string
}

// Unix seconds for UTC date labels cover the whole supported range. Subtract
// those integer values rather than time.Duration, which saturates at 290 years.
func dateOrdinal(t time.Time) int64 {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

func buildView(c config, now time.Time) (countdownView, error) {
	today := now.In(c.location)
	if today.Year() < 1 || today.Year() > 9999 {
		return countdownView{}, errors.New("local current date is outside supported years")
	}
	v := countdownView{Title: c.Title, Target: c.TargetDate, Today: today.Format("2006-01-02")}
	delta := dateOrdinal(c.target) - dateOrdinal(today)
	switch {
	case delta > 1:
		v.Status = "DAYS TO GO"
	case delta == 1:
		v.Status = "DAY TO GO"
	case delta == 0:
		v.Status = "TODAY IS THE DAY"
	default:
		delta = 0
		v.Status = "TARGET PASSED"
	}
	v.Count = strconv.FormatInt(delta, 10)
	return v, nil
}
