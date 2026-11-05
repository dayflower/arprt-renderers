package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func syntheticForecast(now time.Time, zone, unit string) map[string]any {
	loc, _ := time.LoadLocation(zone)
	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	_, offset := now.In(loc).Zone()
	daily := map[string]any{"time": []int64{}, "weather_code": []any{}, "temperature_2m_max": []any{}, "temperature_2m_min": []any{}, "precipitation_probability_max": []any{}}
	for i := 0; i < 7; i++ {
		daily["time"] = append(daily["time"].([]int64), time.Date(local.Year(), local.Month(), local.Day()+i, 0, 0, 0, 0, time.UTC).Unix()-int64(offset))
		daily["weather_code"] = append(daily["weather_code"].([]any), []int{0, 2, 3, 61, 71, 95, 45}[i])
		daily["temperature_2m_max"] = append(daily["temperature_2m_max"].([]any), 24.5)
		daily["temperature_2m_min"] = append(daily["temperature_2m_min"].([]any), 12.3)
		daily["precipitation_probability_max"] = append(daily["precipitation_probability_max"].([]any), 80+i)
	}
	hourly := map[string]any{"time": []int64{}}
	for _, key := range hourlyFields {
		hourly[key] = []any{}
	}
	for at := start; at.Before(start.AddDate(0, 0, 1)); at = at.Add(time.Hour) {
		hourly["time"] = append(hourly["time"].([]int64), at.Unix())
		for _, key := range hourlyFields {
			value := map[string]any{"weather_code": 2, "temperature_2m": 18.5, "wind_speed_10m": 2.3, "wind_direction_10m": 0, "precipitation_probability": at.Hour() * 4}[key]
			if key == "temperature_2m" {
				value = float64(at.Hour())
			}
			hourly[key] = append(hourly[key].([]any), value)
		}
	}
	return map[string]any{"timezone": zone, "utc_offset_seconds": offset, "current": map[string]any{"time": now.Unix(), "temperature_2m": 18.5}, "hourly": hourly, "daily": daily, "current_units": map[string]string{"temperature_2m": unit}, "hourly_units": map[string]string{"temperature_2m": unit, "wind_speed_10m": "m/s", "precipitation_probability": "%"}, "daily_units": map[string]string{"temperature_2m_max": unit, "temperature_2m_min": unit, "precipitation_probability_max": "%"}}
}
func encoded(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestForecastURL(t *testing.T) {
	for _, model := range []string{"", "jma_seamless"} {
		u, _ := url.Parse(forecastURL(0, -180, model, "fahrenheit"))
		q := u.Query()
		for k, want := range map[string]string{"latitude": "0", "longitude": "-180", "models": model, "forecast_days": "7", "timeformat": "unixtime", "timezone": "auto", "temperature_unit": "fahrenheit", "wind_speed_unit": "ms"} {
			if q.Get(k) != want {
				t.Errorf("%s=%s", k, q.Get(k))
			}
		}
		if u.Host != "api.open-meteo.com" || u.Path != "/v1/forecast" || !strings.Contains(q.Get("hourly"), "wind_speed_10m") || strings.Contains(q.Get("hourly"), "relative_humidity_2m") {
			t.Fatal(u)
		}
	}
}
func TestForecastLocalDaysAndDST(t *testing.T) {
	for _, tc := range []struct{ zone, instant, date string }{{"Asia/Tokyo", "2026-11-04T23:00:00Z", "11/5 (Thu)"}, {"America/New_York", "2026-11-05T01:00:00Z", "11/4 (Wed)"}, {"America/New_York", "2026-03-06T16:00:00Z", "3/6 (Fri)"}, {"America/New_York", "2026-03-08T16:00:00Z", "3/8 (Sun)"}, {"America/New_York", "2026-10-30T16:00:00Z", "10/30 (Fri)"}, {"America/New_York", "2026-11-01T16:00:00Z", "11/1 (Sun)"}, {"Asia/Kathmandu", "2026-11-04T20:00:00Z", "11/5 (Thu)"}} {
		t.Run(tc.zone+tc.instant, func(t *testing.T) {
			now, _ := time.Parse(time.RFC3339, tc.instant)
			v, err := buildView(encoded(t, syntheticForecast(now, tc.zone, "°C")), now, "", "celsius")
			if err != nil {
				t.Fatal(err)
			}
			if v.Today.Date != tc.date || len(v.Days) != 7 || len(v.Hours) != 8 || v.Current != "18.5" {
				t.Fatal(v)
			}
			for _, day := range v.Days {
				if day.High != "24.5" || day.Low != "12.3" {
					t.Fatal("daily date shifted across DST", day)
				}
			}
			for i, h := range v.Hours {
				want := fmt.Sprintf("%.1f", float64(i*3))
				if h.Temperature != want || h.Time != fmt.Sprintf("%02d:00", i*3) {
					t.Fatal(h)
				}
			}
		})
	}
}
func TestForecastMissingAndMalformed(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	for _, name := range []string{"short", "missing", "unknown", "null-array"} {
		t.Run(name, func(t *testing.T) {
			f := syntheticForecast(now, "UTC", "°C")
			d := f["daily"].(map[string]any)
			switch name {
			case "short":
				for _, k := range dailyFields {
					d[k] = d[k].([]any)[:2]
				}
				d["time"] = d["time"].([]int64)[:2]
			case "missing":
				delete(d, "temperature_2m_min")
				delete(f["hourly"].(map[string]any), "wind_direction_10m")
			case "unknown":
				d["weather_code"].([]any)[0] = 999
			case "null-array":
				d["temperature_2m_min"] = nil
			}
			v, e := buildView(encoded(t, f), now, "jma_msm", "celsius")
			if e != nil {
				t.Fatal(e)
			}
			if name == "short" && (v.Days[2].High != "—" || len(v.Days) != 7) {
				t.Fatal(v)
			}
			if (name == "missing" || name == "null-array") && v.Today.Low != "—" {
				t.Fatal(v)
			}
			if name == "unknown" && v.Today.Name != "Unknown" {
				t.Fatal(v.Today)
			}
		})
	}
	for _, name := range []string{"timezone", "length", "order", "range", "inverted", "unit", "empty", "stale", "timestamp"} {
		t.Run(name, func(t *testing.T) {
			f := syntheticForecast(now, "UTC", "°C")
			h := f["hourly"].(map[string]any)
			d := f["daily"].(map[string]any)
			switch name {
			case "timezone":
				f["timezone"] = "Unknown/Zone"
			case "length":
				h["temperature_2m"] = []any{1}
			case "order":
				h["time"].([]int64)[1] = h["time"].([]int64)[0]
			case "range":
				h["precipitation_probability"].([]any)[0] = 101
			case "inverted":
				d["temperature_2m_min"].([]any)[0] = 100
			case "unit":
				f["current_units"].(map[string]string)["temperature_2m"] = "°F"
			case "empty":
				delete(f, "current")
				delete(f, "daily")
				delete(f, "hourly")
			case "stale":
				delete(f, "daily")
				delete(f, "hourly")
				f["current"].(map[string]any)["time"] = now.Add(-3 * time.Hour).Unix()
			case "timestamp":
				h["time"].([]int64)[0] = -1
			}
			if _, e := buildView(encoded(t, f), now, "", "celsius"); e == nil {
				t.Fatal("accepted malformed forecast")
			}
		})
	}
	f := syntheticForecast(now, "UTC", "°F")
	v, e := buildView(encoded(t, f), now, "", "fahrenheit")
	if e != nil || v.Unit != "°F" {
		t.Fatal(v, e)
	}
	f = syntheticForecast(now, "UTC", "°C")
	f["daily"].(map[string]any)["temperature_2m_min"].([]any)[0] = -0.01
	v, e = buildView(encoded(t, f), now, "", "celsius")
	if e != nil || v.Today.Low != "0.0" {
		t.Fatal(v, e)
	}
}
func TestWindAndWeather(t *testing.T) {
	speed := 2.0
	for _, degree := range []float64{0, 90, 180, 270, 360} {
		w := windDirection(&degree, &speed)
		want := degree + 180
		if want >= 360 {
			want -= 360
		}
		if w.Angle != want || w.Missing || w.Calm {
			t.Fatal(w)
		}
	}
	calm := 0.04
	if !windDirection(nil, &calm).Calm || !windDirection(nil, &speed).Missing {
		t.Fatal("calm or missing")
	}
	for _, code := range []int{0, 1, 2, 3, 45, 48, 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 71, 73, 75, 77, 80, 81, 82, 85, 86, 95, 96, 97, 99} {
		n := float64(code)
		if weatherIcon(&n).Name == "Unknown" {
			t.Fatal(code)
		}
	}
	if number(nil, "") != "—" {
		t.Fatal("missing")
	}
}
func TestForecastHTTP(t *testing.T) {
	var forwarded bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("capture credentials leaked")
		}
		switch r.URL.Path {
		case "/ok":
			io.WriteString(w, `{"timezone":"UTC"}`)
		case "/redirect":
			http.Redirect(w, r, "/destination", 302)
		case "/destination":
			forwarded = true
		case "/large":
			io.WriteString(w, strings.Repeat("x", forecastLimit+1))
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			z := gzip.NewWriter(w)
			io.WriteString(z, strings.Repeat("x", forecastLimit+1))
			z.Close()
		case "/cut":
			w.Header().Set("Content-Length", "999")
			io.WriteString(w, "x")
		case "/timeout":
			<-r.Context().Done()
		default:
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	if _, e := fetchForecast(context.Background(), newHTTPClient(), server.URL+"/ok"); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"/redirect", "/large", "/gzip", "/cut", "/timeout", "/failure"} {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, e := fetchForecast(ctx, newHTTPClient(), server.URL+p)
		cancel()
		if e == nil {
			t.Fatal(p)
		}
	}
	if forwarded {
		t.Fatal("followed redirect")
	}
}
func TestLayout(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	v, e := buildView(encoded(t, syntheticForecast(now, "UTC", "°C")), now, `<script>alert(1)</script>`, "celsius")
	if e != nil {
		t.Fatal(e)
	}
	b, e := renderHTML(v)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{`lang="en"`, `11/5 (Thu)`, `weather-ready`, `Wind m/s`, `Weather data by Open-Meteo`, `rotate(180 16 16)`} {
		if !bytes.Contains(b, []byte(s)) {
			t.Fatal(s)
		}
	}
	if bytes.Contains(b, []byte(v.Model)) || bytes.Count(b, []byte(`class="hour"`)) != 8 || bytes.Count(b, []byte(`class="day"`)) != 7 {
		t.Fatal("escaping or slot count")
	}
	if dir := os.Getenv("WEATHER_LAYOUT_DIR"); dir != "" {
		if e := os.MkdirAll(dir, 0o755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "weather.html"), b, 0o644); e != nil {
			t.Fatal(e)
		}
	}
}

func TestPrecipitationProbability(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	f := syntheticForecast(now, "UTC", "°C")
	v, err := buildView(encoded(t, f), now, "", "celsius")
	if err != nil {
		t.Fatal(err)
	}
	if v.Today.Precipitation != "80%" || v.Tomorrow.Precipitation != "81%" || v.Days[6].Precipitation != "86%" {
		t.Fatal("daily maximum", v.Days)
	}
	for i, h := range v.Hours {
		if h.Precipitation != fmt.Sprintf("%d%%", i*12) {
			t.Fatal("hourly selection", h)
		}
	}
	b, err := renderHTML(v)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(b, []byte(`class="precip"`)) != 17 || !bytes.Contains(b, []byte(">0%</div>")) || !bytes.Contains(b, []byte("Precip 80%")) {
		t.Fatal("probabilities missing from forecast sections")
	}
	for _, key := range []string{"precipitation_probability", "precipitation_probability_max"} {
		section, units := "hourly", "hourly_units"
		if key == "precipitation_probability_max" {
			section, units = "daily", "daily_units"
		}
		for _, bad := range []float64{-1, 101} {
			f := syntheticForecast(now, "UTC", "°C")
			f[section].(map[string]any)[key].([]any)[0] = bad
			if _, e := buildView(encoded(t, f), now, "", "celsius"); e == nil {
				t.Fatal("invalid probability", key, bad)
			}
		}
		f := syntheticForecast(now, "UTC", "°C")
		f[units].(map[string]string)[key] = "mm"
		if _, e := buildView(encoded(t, f), now, "", "celsius"); e == nil {
			t.Fatal("invalid probability units", key)
		}
		for _, missing := range []string{"absent", "null-array", "null-value"} {
			f := syntheticForecast(now, "UTC", "°C")
			m := f[section].(map[string]any)
			switch missing {
			case "absent":
				delete(m, key)
			case "null-array":
				m[key] = nil
			case "null-value":
				m[key].([]any)[0] = nil
			}
			v, e := buildView(encoded(t, f), now, "jma_msm", "celsius")
			if e != nil {
				t.Fatal(e)
			}
			got := v.Hours[0].Precipitation
			if section == "daily" {
				got = v.Today.Precipitation
			}
			if got != "—" {
				t.Fatal("missing probability became a number", got)
			}
		}
	}
	u, _ := url.Parse(forecastURL(0, 0, "", "celsius"))
	q := u.Query()
	if !strings.Contains(q.Get("hourly"), "precipitation_probability") || !strings.Contains(q.Get("daily"), "precipitation_probability_max") || q.Get("current") != "temperature_2m" {
		t.Fatal("incorrect precipitation API request", q)
	}
}
