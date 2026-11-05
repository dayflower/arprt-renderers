package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This opt-in test uses a real browser; ordinary tests remain offline.
func TestBrowserLayouts(t *testing.T) {
	endpoint := os.Getenv("WEATHER_WEBCAPPI")
	if endpoint == "" {
		t.Skip("set WEATHER_WEBCAPPI for real-browser checks")
	}
	directory := os.Getenv("WEATHER_LAYOUT_DIR")
	if directory == "" {
		t.Fatal("WEATHER_LAYOUT_DIR is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	for _, name := range []string{"normal", "missing", "fahrenheit", "precipitation"} {
		unit, symbol := "celsius", "°C"
		if name == "fahrenheit" {
			unit, symbol = "fahrenheit", "°F"
		}
		f := syntheticForecast(now, "America/New_York", symbol)
		f["daily"].(map[string]any)["precipitation_probability_max"].([]any)[0] = 100
		f["hourly"].(map[string]any)["precipitation_probability"].([]any)[0] = 100
		if name == "precipitation" {
			codes := []int{61, 63, 65, 96, 71, 45, 51}
			d := f["daily"].(map[string]any)
			for i, code := range codes {
				d["weather_code"].([]any)[i] = code
			}
			h := f["hourly"].(map[string]any)
			for i := range h["weather_code"].([]any) {
				h["weather_code"].([]any)[i] = []int{61, 63, 65, 96, 71, 45, 51, 82}[i/3]
			}
		}
		if name == "missing" {
			delete(f, "hourly")
			delete(f["daily"].(map[string]any), "precipitation_probability_max")
			delete(f["daily"].(map[string]any), "weather_code")
			delete(f["daily"].(map[string]any), "temperature_2m_min")
			delete(f, "current")
		}
		if name == "fahrenheit" {
			d := f["daily"].(map[string]any)
			for i := range 7 {
				d["weather_code"].([]any)[i] = 97
				d["temperature_2m_max"].([]any)[i] = 123.4
				d["temperature_2m_min"].([]any)[i] = -12.3
			}
			f["current"].(map[string]any)["temperature_2m"] = 101.2
		}
		v, err := buildView(encoded(t, f), now, "ecmwf_ifs025", unit)
		if err != nil {
			t.Fatal(err)
		}
		switch name {
		case "normal", "precipitation":
			v.LocationName = "Example City"
		case "fahrenheit":
			v.LocationName = strings.Repeat("W", 128)
		}
		for _, palette := range []string{"gray4", "spectra6"} {
			v.Palette = palette
			body, err := renderHTML(v)
			if err != nil {
				t.Fatal(err)
			}
			// Fail readiness if a visible slot extends outside the canvas.
			check := `for(const svg of document.querySelectorAll('svg.weather-icon')){const b=svg.getBBox();if(Math.abs(b.x+b.width/2-12)>.1||Math.abs(b.y+b.height/2-12)>.1)throw new Error('Weather art is not centered');}for(const e of document.querySelectorAll('main>*,.now h2,.now .value,.now .stamp,.now .location,.card-heading,.card-body>*,.hour>*,.day>*')){const r=e.getBoundingClientRect();if(r.bottom>innerHeight||r.right>innerWidth||r.left<0||r.top<0)throw new Error('Layout overflow');}for(const s of document.querySelectorAll('.summary,.hourly,.weekly')){const r=s.getBoundingClientRect();for(const e of s.querySelectorAll('.card-heading,.card-body>*,.hour>*,.day>*'))if(e.getBoundingClientRect().bottom>r.bottom+1)throw new Error('Section overflow');}for(const e of document.querySelectorAll('.day .range,.hour>*,.card-body>*')){const r=e.getBoundingClientRect(),p=e.parentElement.getBoundingClientRect();if(r.left<p.left-1||r.right>p.right+1)throw new Error('Slot overflow');}`
			check += `const place=document.querySelector('.now .location');if(place){const r=place.getBoundingClientRect(),s=document.querySelector('.now .stamp').getBoundingClientRect();if(r.top<s.bottom-1)throw new Error('Location must be below time');const p=document.querySelector('.now').getBoundingClientRect();if(r.left<p.left||r.right>p.right)throw new Error('Location overflow');}if(document.querySelector('footer').textContent.includes('America/New_York'))throw new Error('Timezone is still visible');`
			check += `const colored=document.body.dataset.palette==='spectra6';const black='rgb(0, 0, 0)',red='rgb(192, 0, 0)',blue='rgb(0, 0, 192)',yellow='rgb(224, 176, 0)',ice='rgb(64, 168, 216)';for(const e of document.querySelectorAll('.range b,.low,.precip,.now .value,.hour .temperature,.windrow,footer')){const expected=colored?(e.matches('.range b')?red:e.matches('.low,.precip')?blue:black):black;if(getComputedStyle(e).color!==expected)throw new Error('Incorrect text color');}for(const e of document.querySelectorAll('.weather-icon')){const kind=e.dataset.kind;const expected=colored?(kind==='sun'?red:kind==='cloud-lightning'?yellow:['snowflake','regex'].includes(kind)?ice:['cloud-drizzle','cloud-hail','cloud-rain','cloud-rain-wind'].includes(kind)?blue:black):black;if(getComputedStyle(e.querySelector('.weather-art')).stroke!==expected)throw new Error('Incorrect weather stroke');}`
			body = []byte(strings.Replace(string(body), "document.body.id='weather-ready'", check+"document.body.id='weather-ready'", 1))
			for _, size := range [][2]int{{800, 480}, {480, 800}} {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
				data, err := capture(ctx, newHTTPClient(), endpoint, "", body, size[0], size[1])
				cancel()
				if err != nil {
					t.Fatalf("%s %v: %v", name, size, err)
				}
				path := filepath.Join(directory, fmt.Sprintf("%s-%s-%dx%d.png", name, palette, size[0], size[1]))
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
