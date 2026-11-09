package main

import (
	"bytes"
	"errors"
	"html/template"
	"math"
)

// Icon markup is selected only from these embedded paths, never API text.
type icon struct{ Name, Kind string }

func weatherIcon(code *float64) icon {
	if code == nil {
		return icon{englishWeatherNames[-1], "circle-question-mark"}
	}
	name := englishWeatherNames[int(*code)]
	switch int(*code) {
	case 0:
		return icon{name, "sun"}
	case 1, 2:
		return icon{name, "cloud-sun"}
	case 3:
		return icon{name, "cloud"}
	case 45, 48:
		return icon{name, "cloud-fog"}
	case 51, 53, 55, 56, 57:
		return icon{name, "cloud-drizzle"}
	case 61, 66, 80:
		return icon{name, "cloud-hail"}
	case 63, 81:
		return icon{name, "cloud-rain"}
	case 65, 67, 82:
		return icon{name, "cloud-rain-wind"}
	case 71, 73, 75, 77, 85, 86:
		return icon{name, "snowflake"}
	case 95, 97:
		return icon{name, "cloud-lightning"}
	case 96, 99:
		return icon{name, "regex"}
	}
	return icon{englishWeatherNames[-1], "circle-question-mark"}
}

type windIcon struct {
	Missing, Calm bool
	Angle         float64
}

func windDirection(direction, speed *float64) windIcon {
	if speed != nil && math.Round(*speed*10) == 0 {
		return windIcon{Calm: true}
	}
	if direction == nil {
		return windIcon{Missing: true}
	}
	return windIcon{Angle: math.Mod(*direction+180, 360)}
}

var page = template.Must(template.New("weather").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; font-src 'none'; connect-src 'none'; img-src 'none'"><title>{{.Labels.Title}}</title>
<style>
* { box-sizing:border-box }
html,body { margin:0; width:100%; height:100%; background:white; color:black }
body { padding:10px 18px; font-family:Arial,sans-serif; font-variant-numeric:tabular-nums }
main { height:100%; display:flex; flex-direction:column; gap:4px }
.summary,.hourly,.weekly,footer { flex-shrink:0 }
h1,h2,p { margin:0 }
h1,h2 { font-size:18px }
.card h1,.card h2 { font-size:22px }
svg { width:36px; height:36px; vertical-align:middle; flex-shrink:0 }
svg.wind { width:23px; height:23px }
.summary { display:grid; grid-template-columns:1fr 164px 1fr; gap:16px; align-items:center; min-height:140px }
.card { display:flex; flex-direction:column; gap:7px; min-width:0 }
.card-heading { display:flex; align-items:baseline; gap:8px; white-space:nowrap }
.card .date { font-size:17px }
.card-body { display:grid; grid-template-columns:96px minmax(0,1fr); grid-template-rows:auto auto auto; column-gap:12px; row-gap:4px; align-items:center; width:100% }
.card-body>svg { grid-column:1; grid-row:1 / 4; width:96px; height:96px }
.condition-name { font-size:20px; overflow-wrap:normal }
.card .range { grid-column:2; font-size:25px; line-height:1; display:flex; flex-wrap:wrap; align-items:baseline; gap:0 3px }
.card .range>b,.card .range>.low { white-space:nowrap }
.forecast-unit { font-size:.5em; font-weight:normal }
.card .range .low { font-weight:normal }
.card .precip { grid-column:2; font-size:17px; white-space:nowrap }
.tomorrow .card-heading { justify-content:flex-end }
.tomorrow .card-body { grid-template-columns:minmax(0,1fr) 96px }
.tomorrow .card-body>svg { grid-column:2 }
.tomorrow .condition-name,.tomorrow .range,.tomorrow .precip { grid-column:1; text-align:right }
.tomorrow .range { justify-content:flex-end }
.precip { font-size:17px }
.precip { font-size:15px; white-space:nowrap }
.low { font-weight:normal }
.now { display:flex; flex-direction:column; min-width:0; text-align:center; border-left:1px solid; border-right:1px solid }
.now .details { display:contents }
.now .value { order:1 }
.now .stamp { order:2 }
.now .location { order:3; max-width:100% }
.now .value { font-size:40px; font-weight:bold; line-height:1.15 }
.unit { font-size:18px; font-weight:normal }
.stamp { font-size:15px }
.now .location { font-size:16px; margin-top:4px; padding:0 6px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; min-width:0 }
.hourly,.weekly { border-top:2px solid; padding-top:7px }
.hours { display:grid; grid-template-columns:repeat(8,minmax(0,1fr)); margin-top:6px }
.hour { text-align:center; border-right:1px dotted; display:flex; flex-direction:column; align-items:center; gap:2px; font-size:16px }
.hour:last-child { border:0 }
.hour>svg { width:32px; height:32px }
.temperature { font-size:22px; font-weight:bold; line-height:1.15 }
.windrow { display:flex; align-items:center; justify-content:center; gap:4px; font-size:16px }
.days { display:grid; grid-template-columns:repeat(7,minmax(0,1fr)); gap:4px; margin-top:7px }
.day { text-align:center; display:flex; flex-direction:column; align-items:center; gap:3px; font-size:15px }
.day .range { font-size:17px; white-space:nowrap; display:flex; flex-direction:row; gap:4px; line-height:1.05 }
.day>svg { width:28px; height:28px }
.day .range .low:before { content:"/ " }
.day time { white-space:nowrap }
.weekly h2 span,.hourly h2 span { font-size:14px; font-weight:normal }
footer { margin-top:auto; font-size:12px; display:flex; gap:10px; justify-content:space-between; flex-wrap:wrap }
footer a { color:black }
.model { overflow-wrap:anywhere }
@media(orientation:portrait) {
 body { padding:14px }
 main { gap:8px }
 .summary { grid-template-columns:1fr 1fr; gap:12px; min-height:0 }
 .now { grid-column:1 / -1; grid-row:1; border:0; border-bottom:2px solid; padding-bottom:7px; display:grid; grid-template-columns:1fr 1fr; align-items:center }
 .now .details { display:block; grid-column:1; min-width:0 }
 .now .value { grid-column:2; grid-row:1; font-size:36px }
 .card { gap:6px }
 .card h1,.card h2 { font-size:20px }
 .card .date { font-size:16px }
 .card-heading { gap:8px }
 .card-body { grid-template-columns:88px minmax(0,1fr); column-gap:8px; row-gap:4px }
 .tomorrow .card-body { grid-template-columns:minmax(0,1fr) 88px }
 .card-body>svg { width:88px; height:88px }
 .condition-name { font-size:17px }
 .card .range { font-size:22px }
 .card .precip { font-size:16px }
 .hours { grid-template-columns:repeat(4,minmax(0,1fr)); row-gap:10px }
 .hour:nth-child(4n) { border:0 }
 .hour { font-size:15px }
 .hour>svg { width:30px; height:30px }
 .hour .temperature { font-size:22px }
 .windrow { font-size:16px }
 .days { display:flex; flex-direction:column; gap:2px }
 .day { display:grid; grid-template-columns:1.1fr 32px 52px 1.2fr; text-align:left; gap:5px }
 .day .range { text-align:right; font-size:18px; flex-direction:row; justify-content:flex-end; gap:4px }
 .day .range .low:before { content:"/ " }
 .day svg { width:26px; height:26px }
 .hourly h2,.weekly h2 { font-size:17px }
 .hourly h2 span,.weekly h2 span { font-size:13px }
 footer { font-size:11px; gap:3px }
}

{{if eq .Palette "spectra6"}}
/* Limit accents to forecast values and artwork; keep structure and headings black. */
:root { --warm:#c00000; --cool:#0000c0; --lightning:#e0b000; --ice:#40a8d8 }
.range b { color:var(--warm) }
.low,.precip { color:var(--cool) }
.weather-icon[data-kind="sun"] { color:var(--warm) }
.weather-icon[data-kind="cloud-lightning"] { color:var(--lightning) }
.weather-icon[data-kind="cloud-drizzle"],.weather-icon[data-kind="cloud-hail"],
.weather-icon[data-kind="cloud-rain"],.weather-icon[data-kind="cloud-rain-wind"] { color:var(--cool) }
.weather-icon[data-kind="snowflake"],.weather-icon[data-kind="regex"] { color:var(--ice) }
{{end}}
</style></head><body data-palette="{{.Palette}}"><main>
<section class="summary"><div class="card"><div class="card-heading"><h1>{{.Labels.Today}}</h1><time class="date">{{.Today.Date}}</time></div><div class="card-body">{{template "icon" .Today.Icon}}<span class="condition-name">{{.Today.Name}}</span><div class="range"><b>{{.Today.High}}{{if ne .Today.High "—"}}<span class="forecast-unit">{{.Unit}}</span>{{end}}</b><span>/</span><span class="low">{{.Today.Low}}{{if ne .Today.Low "—"}}<span class="forecast-unit">{{.Unit}}</span>{{end}}</span></div><div class="precip" title="{{.Labels.DailyPrecipitation}}">{{.Labels.Precipitation}} {{.Today.Precipitation}}</div></div></div>
<div class="now"><div class="details"><h2>{{.Labels.Now}}</h2><div class="stamp">{{.CurrentTime}}</div>{{if .LocationName}}<div class="location" title="{{.LocationName}}">{{.LocationName}}</div>{{end}}</div><div class="value">{{.Current}}<span class="unit"> {{.Unit}}</span></div></div>
<div class="card tomorrow"><div class="card-heading"><time class="date">{{.Tomorrow.Date}}</time><h2>{{.Labels.Tomorrow}}</h2></div><div class="card-body">{{template "icon" .Tomorrow.Icon}}<span class="condition-name">{{.Tomorrow.Name}}</span><div class="range"><b>{{.Tomorrow.High}}{{if ne .Tomorrow.High "—"}}<span class="forecast-unit">{{.Unit}}</span>{{end}}</b><span>/</span><span class="low">{{.Tomorrow.Low}}{{if ne .Tomorrow.Low "—"}}<span class="forecast-unit">{{.Unit}}</span>{{end}}</span></div><div class="precip" title="{{.Labels.DailyPrecipitation}}">{{.Labels.Precipitation}} {{.Tomorrow.Precipitation}}</div></div></div></section>
<section class="hourly"><h2>{{.Labels.Hourly}} <span>{{.Labels.Today}} · {{.Unit}} · {{.Labels.Wind}} m/s · {{.Labels.Precipitation}} %</span></h2><div class="hours">{{range .Hours}}<div class="hour"><time>{{.Time}}</time>{{template "icon" .Icon}}<div class="temperature">{{.Temperature}}{{if ne .Temperature "—"}}<span class="forecast-unit">{{$.Unit}}</span>{{end}}</div><div class="precip">{{.Precipitation}}</div><div class="windrow">{{template "wind" .Direction}}<span>{{.Wind}}</span></div></div>{{end}}</div></section>
<section class="weekly"><h2>{{.Labels.Weekly}} <span>{{.Labels.HighLow}} · {{.Unit}} · {{.Labels.Precipitation}} %</span></h2><div class="days">{{range .Days}}<div class="day"><time>{{.Date}}</time>{{template "icon" .Icon}}<div class="precip" title="{{$.Labels.DailyPrecipitation}}">{{.Precipitation}}</div><div class="range"><b>{{.High}}</b><span class="low">{{.Low}}</span></div></div>{{end}}</div></section>
<footer><span>{{.Labels.Updated}} {{.Updated}}</span><span>{{if .Model}}<span class="model">{{.Labels.Model}}: {{.Model}} · </span>{{end}}<a href="https://open-meteo.com/">{{.Labels.Attribution}}</a></span></footer>
</main><script>(async()=>{await document.fonts.ready;for(const svg of document.querySelectorAll('svg.weather-icon')){const art=svg.querySelector('.weather-art'),b=art.getBBox();art.setAttribute('transform','translate('+(12-b.x-b.width/2)+' '+(12-b.y-b.height/2)+')')}await new Promise(requestAnimationFrame);await new Promise(requestAnimationFrame);const main=document.querySelector('main'),bounds=main.getBoundingClientRect();if(main.scrollHeight>main.clientHeight+1||[...main.querySelectorAll('main>*,.now h2,.now .value,.now .stamp,.now .location,.card-heading,.card-body>*,.hour>*,.day>*')].some(el=>{const r=el.getBoundingClientRect();return r.left<bounds.left-1||r.right>bounds.right+1||r.top<bounds.top-1||r.bottom>bounds.bottom+1})){document.body.dataset.layoutError='overflow';return}document.body.id='weather-ready'})();</script></body></html>
{{define "wind"}}{{if .Missing}}<span aria-label="Wind direction unavailable">—</span>{{else}}<svg class="wind" viewBox="0 0 32 32" role="img" aria-label="{{if .Calm}}Calm{{else}}Wind travel direction{{end}}">{{if .Calm}}<circle cx="16" cy="16" r="7" fill="none" stroke="black" stroke-width="2.5"/>{{else}}<path d="M16 3 L24 15 L18 13 L18 28 L14 28 L14 13 L8 15 Z" transform="rotate({{.Angle}} 16 16)" fill="black"/>{{end}}</svg>{{end}}{{end}}
{{define "icon"}}<svg class="weather-icon" data-kind="{{.Kind}}" viewBox="0 0 24 24" role="img" aria-label="{{.Name}}"><title>{{.Name}}</title><g class="weather-art" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">{{.Artwork}}</g></svg>{{end}}`))

func renderHTML(v dashboard) ([]byte, error) {
	var b bytes.Buffer
	if err := page.Execute(&b, v); err != nil {
		return nil, err
	}
	if b.Len() > htmlLimit {
		return nil, errors.New("HTML exceeds size limit")
	}
	return b.Bytes(), nil
}
