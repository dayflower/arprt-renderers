package ical

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"
)

type layoutEvent struct{ Text, Meta string }

type upcomingEvent struct {
	Date, Weekday, Text, Meta string
	HolidayOnly               bool
	HolidayExtra              int
}
type layoutDay struct {
	Month                                 bool
	HolidayExtra                          int
	Date, Number, Weekday, Class, Section string
	Holidays                              []string
	Events                                []layoutEvent
	Omitted                               int
}
type layoutPage struct {
	Title, Period, View        string
	FutureOmitted, FutureTotal int
	Priorities                 []layoutDay
	Upcoming                   []upcomingEvent
	Days                       []layoutDay
	Weekdays                   []string
	Weeks                      int
}

func shortened(s string, limit int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	return string(r)
}

// RenderHTML bounds template work separately from semantic normalization.
// Layout never mutates the complete day model; removed rows remain counted.
func RenderHTML(m *Model) ([]byte, error) {
	p := layoutPage{Title: shortened(m.Title, 128), View: m.View, Period: m.Now.Format("January 2006"), Weeks: len(m.Days) / 7}
	if m.View != "month" && m.View != "agenda" {
		return nil, errors.New("invalid layout model")
	}
	if (m.View == "month" && (len(m.Days) < 28 || len(m.Days) > 42 || len(m.Days)%7 != 0)) || (m.View == "agenda" && len(m.Days) != 7) {
		return nil, errors.New("invalid layout window")
	}

	names := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	for i := range 7 {
		p.Weekdays = append(p.Weekdays, names[(int(m.Days[0].Weekday)+i)%7])
	}
	if m.View == "agenda" {
		p.Period = m.Now.Format("Jan 2") + " – " + m.Now.AddDate(0, 0, 6).Format("Jan 2")
	}
	for i, d := range m.Days {
		date, err := time.Parse(time.DateOnly, d.Date)
		if err != nil {
			return nil, errors.New("invalid layout date")
		}
		v := layoutDay{Month: m.View == "month", Date: d.Date, Number: fmt.Sprint(date.Day()), Weekday: names[d.Weekday], Class: "day", Omitted: d.Omitted}
		if !d.InMonth {
			v.Class += " outside"
		}
		if d.Today {
			v.Class += " today"
		}
		if d.Weekday == time.Sunday {
			v.Class += " sunday"
		}
		if d.Weekday == time.Saturday {
			v.Class += " saturday"
		}
		if len(d.Holidays) > 0 {
			v.Class += " holiday"
		}
		if m.View == "agenda" {
			v.Number = date.Format("Jan 2")
			if i == 0 {
				v.Section = "Today"
			} else if i == 1 {
				v.Section = "Tomorrow"

			}
			if i < 2 {
				v.Class += " priority"
			} else {
				v.Class += " future"
			}
		}
		for _, h := range d.Holidays {
			if len(v.Holidays) == 0 {
				v.Holidays = append(v.Holidays, shortened(h, 96))
			}
		}
		v.HolidayExtra = max(0, len(d.Holidays)-1)
		maxRows := 8
		if m.View == "agenda" {
			maxRows = 16
		}
		for j, e := range d.Events {
			if j >= maxRows {
				v.Omitted++
				continue
			}
			meta := "All day"
			prefix := ""
			if !e.AllDay {
				meta = e.Start.Format("15:04") + "–" + e.End.Format("15:04")
				prefix = e.Start.Format("15:04") + " "
			}
			text := shortened(e.Summary, 180)
			if m.View == "month" {
				if prefix != "" {
					text += " · " + strings.TrimSpace(prefix)
				}
			}
			if e.ContinuesBefore {
				text = "← " + text
			}
			if e.ContinuesAfter {
				text += " →"
			}

			v.Events = append(v.Events, layoutEvent{Text: text, Meta: meta})
		}
		p.Days = append(p.Days, v)
	}
	if m.View == "agenda" {
		for i, d := range p.Days {
			if i < 2 {
				p.Priorities = append(p.Priorities, d)
				continue
			}
			p.FutureOmitted += d.Omitted
			p.FutureTotal += len(d.Events) + d.Omitted
			if len(d.Events) == 0 && d.Omitted == 0 && len(d.Holidays) > 0 {
				name := d.Holidays[0]
				p.FutureTotal++
				p.Upcoming = append(p.Upcoming, upcomingEvent{Date: d.Date, Weekday: d.Number + " " + d.Weekday, Text: name, HolidayOnly: true, HolidayExtra: d.HolidayExtra})
			}
			for _, e := range d.Events {
				meta := e.Meta
				if len(d.Holidays) > 0 {
					meta += " · " + d.Holidays[0]
					if d.HolidayExtra > 0 {
						meta += fmt.Sprintf(" (+%d)", d.HolidayExtra)
					}
				}
				p.Upcoming = append(p.Upcoming, upcomingEvent{Date: d.Date, Weekday: d.Number + " " + d.Weekday, Text: e.Text, Meta: meta})
			}
		}
		// Keep appointment candidates before holiday-only rows, preserving date order.
		for len(p.Upcoming) > 32 {
			remove := len(p.Upcoming) - 1
			for j := len(p.Upcoming) - 1; j >= 0; j-- {
				if p.Upcoming[j].HolidayOnly {
					remove = j
					break
				}
			}
			p.Upcoming = append(p.Upcoming[:remove], p.Upcoming[remove+1:]...)
			p.FutureOmitted++
		}
	}

	var b bytes.Buffer
	if err := calendarPage.Execute(&b, p); err != nil {
		return nil, errors.New("layout template failed")
	}
	if b.Len() > htmlLimit {
		return nil, errors.New("HTML exceeds size limit")
	}
	return b.Bytes(), nil
}

var calendarPage = template.Must(template.New("calendar").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; font-src 'none'"><title>{{.Title}}</title><style>
*{box-sizing:border-box}html,body{margin:0;width:100%;height:100%;background:#fff;color:#000}
body{padding:8px;font-family:"Noto Sans CJK JP","Noto Sans JP",sans-serif;font-size:18px;font-weight:500;overflow:hidden}
main{height:100%;min-height:0;display:flex;flex-direction:column;gap:4px}
header{height:32px;flex-shrink:0;display:flex;align-items:center;gap:12px}
h1{font-size:24px;line-height:32px;font-weight:700;margin:0;flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
header span{font-size:22px;line-height:32px;white-space:nowrap;font-weight:700}
.calendar{flex:1;min-height:0;display:grid;grid-template-columns:repeat(7,minmax(0,1fr));grid-template-rows:26px repeat({{.Weeks}},minmax(0,1fr));border-top:2px solid #000;border-left:2px solid #000}
.weekday{font-size:18px;line-height:24px;text-align:center;font-weight:700;border-right:1px solid #000;border-bottom:2px solid #000}.weekday.weekend{background:#000;color:#fff}
.day{position:relative;min-width:0;min-height:0;overflow:hidden;display:flex;flex-direction:column;border-right:1px solid #000;border-bottom:1px solid #000;padding:2px}
.date{height:26px;line-height:26px;flex-shrink:0;font-size:22px;font-weight:700;display:flex;align-items:center;gap:4px;white-space:nowrap;min-width:0}
.day-number{min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;line-height:24px}
.today .day-number{background:#000;color:#fff;padding:0 3px;border-radius:3px}
.sunday .day-number,.saturday .day-number{text-decoration:underline;text-underline-offset:3px}
.holiday .day-number{border-bottom:3px double #000}.today.holiday .day-number{outline:3px double #000;outline-offset:1px}
.outside .day-number{font-style:italic;border:1px dotted #000}
.holidays{height:22px;line-height:22px;font-size:16px;flex-shrink:0;display:flex;min-width:0;font-weight:600}
.holiday-name{flex:1;min-width:0;overflow:hidden;white-space:nowrap;text-overflow:ellipsis}.holiday-more{flex-shrink:0}
.events{flex:1;min-height:0;overflow:hidden}.event{height:24px;line-height:24px;font-size:18px;overflow:hidden;min-width:0}
.event-title{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.event-meta{display:none}
.omitted{font-size:16px;font-weight:700;line-height:22px;height:22px;flex-shrink:0;white-space:nowrap}.date .omitted{margin-left:auto}
.agenda{flex:1;min-height:0;display:flex;flex-direction:row;gap:24px}
.primary-days{display:flex;flex-direction:column;gap:20px;flex:1;min-width:0;min-height:0;max-height:100%}.agenda.has-upcoming .primary-days{flex:0 0 40%;max-height:100%}
.agenda .day{flex:1;border:0;padding:0;min-height:34px}.agenda .day.holiday{min-height:56px}.agenda .date{font-size:22px;height:34px;line-height:34px}.agenda .day-number{line-height:28px}
.agenda .today .day-number{background:transparent;color:#000;padding:0;border-radius:0}.agenda .today.holiday .day-number{outline:0}
.agenda .event{height:56px;line-height:30px;font-size:22px}.agenda .event-meta{display:block;font-size:16px;line-height:22px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.agenda .event-title{line-height:30px}
.upcoming{flex:1;min-width:0;min-height:0;display:flex;flex-direction:column;gap:4px}
h2{font-size:22px;font-weight:700;line-height:30px;height:30px;flex-shrink:0;margin:0;display:flex;gap:12px;align-items:center}
.upcoming-list{flex:1;min-height:0;overflow:hidden}
.upcoming-event{height:56px;line-height:30px;display:grid;grid-template-columns:108px minmax(0,1fr);grid-template-rows:30px 22px;gap:0 10px;overflow:hidden}
.upcoming-date{grid-column:1;grid-row:1/3;font-size:18px;font-weight:700;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.upcoming-title{font-size:22px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;grid-column:2;grid-row:1}
.upcoming-meta{font-size:16px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;grid-column:2;grid-row:2;line-height:22px}
@media(orientation:portrait){header{height:60px;flex-direction:column;gap:0;align-items:start}h1{flex:none;max-width:100%;height:32px}header span{font-size:22px;line-height:28px}
.calendar .date{font-size:20px;gap:0}.calendar .day-number{line-height:24px}.calendar .day{padding:1px}.calendar .outside .day-number{font-size:18px}.calendar .omitted{font-size:16px}
.agenda{flex-direction:column;gap:16px}.primary-days{flex:0 1 auto;gap:14px}.agenda.has-upcoming .primary-days{flex:0 1 auto;max-height:44%}
.upcoming-event{height:56px;grid-template-columns:110px minmax(0,1fr);grid-template-rows:30px 22px;gap:0 10px}
.upcoming-date{grid-column:1;grid-row:1/3;line-height:30px}.upcoming-title{grid-column:2;grid-row:1;line-height:30px}.upcoming-meta{grid-column:2;grid-row:2;line-height:22px}
}

.upcoming-event.holiday-only{height:34px;grid-template-rows:30px}.holiday-only .upcoming-title{display:flex}.holiday-only .upcoming-date{grid-row:1;text-decoration:underline double;text-underline-offset:3px}
</style></head><body><main><header><h1>{{.Title}}</h1><span>{{.Period}}</span></header>
{{if eq .View "month"}}<div class="calendar">{{range .Weekdays}}<div class="weekday{{if or (eq . "Sat") (eq . "Sun")}} weekend{{end}}">{{.}}</div>{{end}}{{range .Days}}{{template "day" .}}{{end}}</div>{{else}}<div class="agenda{{if .FutureTotal}} has-upcoming{{end}}">{{if .Priorities}}<div class="primary-days">{{range .Priorities}}{{template "day" .}}{{end}}</div>{{end}}{{if .FutureTotal}}<section class="upcoming" data-total="{{.FutureTotal}}" data-omitted="{{.FutureOmitted}}"><h2>Upcoming<span class="omitted"></span></h2><div class="upcoming-list">{{range .Upcoming}}<div class="upcoming-event{{if .HolidayOnly}} holiday-only{{end}}" data-date="{{.Date}}"><div class="upcoming-date">{{.Weekday}}</div><div class="upcoming-title">{{if .HolidayOnly}}<span class="holiday-name">{{.Text}}</span><span class="holiday-more">{{if .HolidayExtra}} +{{.HolidayExtra}}{{end}}</span>{{else}}{{.Text}}{{end}}</div>{{if not .HolidayOnly}}<div class="upcoming-meta">{{.Meta}}</div>{{end}}</div>{{end}}</div></section>{{end}}</div>{{end}}</main>

<script>
(async()=>{await document.fonts.ready;await new Promise(requestAnimationFrame);
const fit=(container,boxSelector,rowSelector)=>{const box=container.querySelector(boxSelector),rows=[...box.querySelectorAll(rowSelector)];let hidden=Number(container.dataset.omitted);while(rows.length&&rows.at(-1).getBoundingClientRect().bottom>box.getBoundingClientRect().bottom+0.1){const holiday=rows.findLastIndex(row=>row.classList.contains('holiday-only'));rows.splice(holiday<0?rows.length-1:holiday,1)[0].remove();hidden++}container.querySelector('.omitted').textContent=hidden?'+'+hidden+(container.classList.contains('month-day')?'':' more'):'';container.dataset.hidden=String(hidden)};
for(const day of document.querySelectorAll('.day')){fit(day,'.events','.event');const holiday=day.querySelector('.holiday-name');if(holiday&&holiday.scrollWidth>holiday.clientWidth)day.querySelector('.holiday-more').textContent+='…'}
const upcoming=document.querySelector('.upcoming');if(upcoming)fit(upcoming,'.upcoming-list','.upcoming-event');await new Promise(requestAnimationFrame);document.body.id='ical-ready'})();
</script></body></html>
{{define "day"}}<section class="{{.Class}}{{if .Month}} month-day{{end}}" data-date="{{.Date}}" data-omitted="{{.Omitted}}"><div class="date"><span class="day-number">{{if .Section}}{{.Section}} · {{end}}{{.Number}}{{if not .Month}}<span class="day-weekday"> {{.Weekday}}</span>{{end}}</span><span class="omitted"></span></div>{{if .Holidays}}<div class="holidays"><span class="holiday-name">{{index .Holidays 0}}</span><span class="holiday-more">{{if .HolidayExtra}} +{{.HolidayExtra}}{{end}}</span></div>{{end}}<div class="events">{{range .Events}}<div class="event"><div class="event-title">{{.Text}}</div><div class="event-meta">{{.Meta}}</div></div>{{end}}</div></section>{{end}}`))
