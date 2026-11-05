package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const forecastLimit = 2 << 20

var hourlyFields = []string{"weather_code", "temperature_2m", "wind_direction_10m", "wind_speed_10m", "precipitation_probability"}
var dailyFields = []string{"weather_code", "temperature_2m_max", "temperature_2m_min", "precipitation_probability_max"}

func forecastURL(lat, lon float64, model, unit string) string {
	q := url.Values{"latitude": {strconv.FormatFloat(lat, 'f', -1, 64)}, "longitude": {strconv.FormatFloat(lon, 'f', -1, 64)}, "current": {"temperature_2m"}, "daily": {strings.Join(dailyFields, ",")}, "hourly": {strings.Join(hourlyFields, ",")}, "forecast_days": {"7"}, "timezone": {"auto"}, "timeformat": {"unixtime"}, "temperature_unit": {unit}, "wind_speed_unit": {"ms"}}
	if model != "" {
		q.Set("models", model)
	}
	return "https://api.open-meteo.com/v1/forecast?" + q.Encode()
}
func fetchForecast(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("invalid forecast request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "arprt-weather/0.1")
	// Forecast requests never carry capture credentials and do not follow redirects.
	bounded := *client
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := bounded.Do(req)
	if err != nil {
		return nil, errors.New("forecast request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("forecast: HTTP %d", resp.StatusCode)
	}
	return readBounded(resp.Body, forecastLimit)
}

type forecast struct {
	UTCOffset    *int                       `json:"utc_offset_seconds"`
	Timezone     string                     `json:"timezone"`
	Current      map[string]json.RawMessage `json:"current"`
	Hourly       map[string]json.RawMessage `json:"hourly"`
	Daily        map[string]json.RawMessage `json:"daily"`
	CurrentUnits map[string]string          `json:"current_units"`
	HourlyUnits  map[string]string          `json:"hourly_units"`
	DailyUnits   map[string]string          `json:"daily_units"`
}
type series struct {
	Times  []int64
	Values map[string][]*float64
}

func parseSeries(raw map[string]json.RawMessage, fields []string) (series, error) {
	s := series{Values: map[string][]*float64{}}
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw["time"], &s.Times); err != nil || len(s.Times) > 400 {
		return s, errors.New("invalid forecast times")
	}
	for i, v := range s.Times {
		if v < 0 || v > 253402214400 || i > 0 && v <= s.Times[i-1] {
			return s, errors.New("invalid forecast time order")
		}
	}
	for _, key := range fields {
		v, ok := raw[key]
		if !ok {
			continue
		}
		var values []*float64
		if err := json.Unmarshal(v, &values); err != nil {
			return s, errors.New("invalid forecast values")
		}
		if values == nil {
			continue
		}
		if len(values) != len(s.Times) {
			return s, errors.New("forecast array length mismatch")
		}
		for _, n := range values {
			if n != nil && !validValue(key, *n) {
				return s, errors.New("invalid forecast value range")
			}
		}
		s.Values[key] = values
	}
	return s, nil
}
func validValue(key string, n float64) bool {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return false
	}
	switch key {
	case "weather_code":
		return n >= 0 && n <= 999 && n == math.Trunc(n)
	case "precipitation_probability", "precipitation_probability_max":
		return n >= 0 && n <= 100
	case "wind_direction_10m":
		return n >= 0 && n <= 360
	case "wind_speed_10m":
		return n >= 0 && n <= 1000
	default:
		return n >= -500 && n <= 500
	}
}
func (s series) value(key string, i int) *float64 {
	if i < 0 || i >= len(s.Values[key]) {
		return nil
	}
	return s.Values[key][i]
}
func number(n *float64, suffix string) string {
	if n == nil {
		return "—"
	}
	value := math.Round(*n*10) / 10
	if value == 0 {
		value = 0
	}
	return fmt.Sprintf("%.1f%s", value, suffix)
}

func percent(n *float64) string {
	if n == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", *n)
}

type dayView struct {
	Date, Name, High, Low, Precipitation string
	Icon                                 icon
}
type hourView struct {
	Time, Temperature, Wind, Precipitation string
	Icon                                   icon
	Direction                              windIcon
}
type dashboard struct {
	Labels                                               labels
	Today, Tomorrow                                      dayView
	Days                                                 []dayView
	Hours                                                []hourView
	Current, CurrentTime, Updated, Timezone, Model, Unit string
}

func buildView(data []byte, now time.Time, model, unit string) (dashboard, error) {
	var f forecast
	var v dashboard
	if len(data) > forecastLimit || json.Unmarshal(data, &f) != nil {
		return v, errors.New("invalid forecast JSON")
	}
	location, err := time.LoadLocation(f.Timezone)
	if err != nil || f.Timezone == "" || len(f.Timezone) > 100 {
		return v, errors.New("invalid forecast timezone")
	}
	if f.UTCOffset == nil || *f.UTCOffset < -86400 || *f.UTCOffset > 86400 {
		return v, errors.New("invalid forecast UTC offset")
	}
	hourly, err := parseSeries(f.Hourly, hourlyFields)
	if err != nil {
		return v, err
	}
	daily, err := parseSeries(f.Daily, dailyFields)
	if err != nil {
		return v, err
	}
	v.Unit = "°C"
	if unit == "fahrenheit" {
		v.Unit = "°F"
	}
	// Validate supplied unit metadata to prevent mislabeled offline snapshots.
	for _, group := range []struct {
		units  map[string]string
		fields []string
	}{{f.HourlyUnits, hourlyFields}, {f.DailyUnits, dailyFields}, {f.CurrentUnits, []string{"temperature_2m"}}} {
		for _, key := range group.fields {
			expected := ""
			switch key {
			case "temperature_2m", "temperature_2m_min", "temperature_2m_max":
				expected = v.Unit
			case "wind_speed_10m":
				expected = "m/s"
			case "wind_direction_10m":
				expected = "°"
			case "precipitation_probability", "precipitation_probability_max":
				expected = "%"
			}
			if u, ok := group.units[key]; ok && expected != "" && u != expected {
				return v, errors.New("forecast unit mismatch")
			}
		}
	}
	v.Labels = englishLabels
	v.Timezone = f.Timezone
	v.Model = model
	v.Current = "—"
	v.CurrentTime = "—"
	v.Updated = now.In(location).Format("1/2 (Mon) 15:04")
	local := now.In(location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	meaningful := false
	for day := 0; day < 7; day++ {
		date := today.AddDate(0, 0, day)
		d := dayView{Date: date.Format("1/2 (Mon)"), Name: englishWeatherNames[-1], High: "—", Low: "—", Precipitation: "—", Icon: weatherIcon(nil)}
		// Daily UNIX timestamps encode civil dates with the API request-time offset.
		// Using an IANA offset here would shift dates across DST transitions.
		for i, stamp := range daily.Times {
			if time.Unix(stamp+int64(*f.UTCOffset), 0).UTC().Format("2006-01-02") != date.Format("2006-01-02") {
				continue
			}
			code, high, low := daily.value("weather_code", i), daily.value("temperature_2m_max", i), daily.value("temperature_2m_min", i)
			if high != nil && low != nil && *high < *low {
				return v, errors.New("daily high is below low")
			}
			d.Icon = weatherIcon(code)
			d.Name = d.Icon.Name
			d.High = number(high, "")
			d.Low = number(low, "")
			probability := daily.value("precipitation_probability_max", i)
			d.Precipitation = percent(probability)
			meaningful = meaningful || code != nil || high != nil || low != nil || probability != nil
			break
		}
		v.Days = append(v.Days, d)
	}
	v.Today = v.Days[0]
	v.Tomorrow = v.Days[1]
	for hour := 0; hour < 24; hour += 3 {
		h := hourView{Time: fmt.Sprintf("%02d:00", hour), Temperature: "—", Wind: "—", Precipitation: "—", Icon: weatherIcon(nil), Direction: windIcon{Missing: true}}
		for i, stamp := range hourly.Times {
			at := time.Unix(stamp, 0).In(location)
			if at.Format("2006-01-02") != today.Format("2006-01-02") || at.Hour() != hour || at.Minute() != 0 {
				continue
			}
			code, temp, speed, direction := hourly.value("weather_code", i), hourly.value("temperature_2m", i), hourly.value("wind_speed_10m", i), hourly.value("wind_direction_10m", i)
			probability := hourly.value("precipitation_probability", i)
			h.Precipitation = percent(probability)
			h.Icon = weatherIcon(code)
			h.Temperature = number(temp, "")
			h.Wind = number(speed, "")
			h.Direction = windDirection(direction, speed)
			meaningful = meaningful || code != nil || temp != nil || speed != nil || direction != nil || probability != nil
			break
		}
		v.Hours = append(v.Hours, h)
	}
	if len(f.Current) > 0 {
		var stamp *int64
		var temp *float64
		if json.Unmarshal(f.Current["time"], &stamp) != nil || stamp == nil || *stamp < 0 || *stamp > 253402214400 {
			return v, errors.New("invalid current time")
		}
		if raw, ok := f.Current["temperature_2m"]; ok {
			if json.Unmarshal(raw, &temp) != nil || temp != nil && !validValue("temperature_2m", *temp) {
				return v, errors.New("invalid current temperature")
			}
		}
		at := time.Unix(*stamp, 0)
		if temp != nil && at.In(location).Format("2006-01-02") == today.Format("2006-01-02") && math.Abs(now.Sub(at).Seconds()) <= 2*3600 {
			v.Current = number(temp, "")
			v.CurrentTime = at.In(location).Format("15:04")
			meaningful = true
		}
	}
	if !meaningful {
		return v, errors.New("no displayable forecast data")
	}
	return v, nil
}
