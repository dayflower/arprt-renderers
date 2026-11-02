package ical

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ical "github.com/aanelson/iCalendar"
)

const Unsupported Error = "unsupported_semantics"

// validateCalendar runs only inside the killable worker, before revision
// selection or window extraction. Even discarded revisions must be valid.
func validateCalendar(cal *ical.Calendar, holiday bool, loc *time.Location) error {
	// Inspect every rule and budget before a typed accessor can compile a zone.
	if err := preflight(cal.Component); err != nil {
		return err
	}
	properties, dates := 0, 0
	var walk func(*ical.Component) error
	walk = func(c *ical.Component) error {
		counts := map[string]int{}
		for _, p := range c.Props {
			properties++
			if properties > 100000 || len(p.Params) > 64 {
				return Limit
			}
			counts[p.Name]++
			params := map[string]bool{}
			for _, param := range p.Params {
				name := strings.ToUpper(param.Name)
				if params[name] {
					return Invalid
				}
				params[name] = true
				if (name == "VALUE" || name == "TZID" || name == "RANGE") && len(param.Values) != 1 {
					return Invalid
				}
			}
			maxText := 0
			switch p.Name {
			case "UID", "SUMMARY":
				maxText = 1024
			case "TZID", "TZNAME":
				maxText = 4096
			}
			if maxText > 0 && utf8.RuneCountInString(p.Text()) > maxText {
				return Limit
			}
			if c.Name == "VEVENT" || c.Name == "STANDARD" || c.Name == "DAYLIGHT" {
				switch p.Name {
				case "EXRULE":
					return Unsupported
				case "RRULE":
					if err := validateRule(p, c); err != nil {
						return err
					}
				case "DTSTART", "DTEND", "RECURRENCE-ID", "RDATE", "EXDATE", "DTSTAMP":
					if err := validateDates(p, cal, c, &dates); err != nil {
						return err
					}
				case "UID", "SUMMARY", "STATUS", "DURATION", "SEQUENCE", "TZOFFSETFROM", "TZOFFSETTO", "TZNAME", "COMMENT", "DESCRIPTION", "LOCATION", "URL", "ATTACH", "ATTENDEE", "ORGANIZER", "CLASS", "CATEGORIES", "CONTACT", "CREATED", "LAST-MODIFIED", "PRIORITY", "RELATED-TO", "RESOURCES", "TRANSP", "REQUEST-STATUS", "COLOR", "CONFERENCE", "IMAGE":
				default:
					return Unsupported
				}
			}
		}
		if c.Name == "VCALENDAR" && (counts["VERSION"] != 1 || counts["PRODID"] != 1) {
			return Invalid
		}
		if c.Name == "VEVENT" {
			for _, name := range []string{"UID", "DTSTAMP", "SEQUENCE", "DTSTART", "DTEND", "DURATION", "RECURRENCE-ID", "SUMMARY", "STATUS", "RRULE"} {
				if counts[name] > 1 {
					return Invalid
				}
			}
			for _, name := range []string{"UID", "SUMMARY", "STATUS"} {
				if p := c.Get(name); p != nil && p.Params.Has("VALUE") && p.Params.Get("VALUE") != "TEXT" {
					return Invalid
				}
			}
			if c.Get("DTSTART") == nil && (c.Get("DTEND") != nil || c.Get("DURATION") != nil || c.Get("RRULE") != nil || c.Get("RDATE") != nil || c.Get("EXDATE") != nil) {
				return Invalid
			}
			for _, child := range c.Children {
				if child.Name != "VALARM" {
					return Unsupported
				}
			}
			e := event(c, cal)
			start, err := e.Start()
			if err != nil && (e.Status() != "CANCELLED" || c.Get("DTSTART") != nil) {
				return Invalid
			}
			if holiday {
				if c.Get("DTSTART") != nil && !start.IsDate {
					return Invalid
				}
				if rid, ok, err := e.RecurrenceID(); err != nil || (ok && !rid.IsDate) {
					return Invalid
				}
			}
			if p := c.Get("STATUS"); p != nil && p.Value != "TENTATIVE" && p.Value != "CONFIRMED" && p.Value != "CANCELLED" {
				return Invalid
			}
			if p := c.Get("DURATION"); p != nil {
				d, err := p.Duration()
				if err != nil || d.Negative || d.IsZero() {
					return Invalid
				}
				if start.IsDate && (d.Hours != 0 || d.Minutes != 0 || d.Seconds != 0) {
					return Invalid
				}
				if p.Params.Has("VALUE") && p.Params.Get("VALUE") != "DURATION" {
					return Invalid
				}
			}
			if c.Get("DTSTART") != nil && (c.Get("DTEND") != nil || c.Get("DURATION") != nil) {
				end, err := e.End()
				if err != nil || start.IsDate != end.IsDate || end.In(loc).Before(start.In(loc)) || (start.IsDate && !end.In(loc).After(start.In(loc))) {
					return Invalid
				}
			}
			if rid, ok, err := e.RecurrenceID(); err != nil || (ok && c.Get("DTSTART") != nil && rid.IsDate != start.IsDate) {
				return Invalid
			}
			if p := c.Get("RECURRENCE-ID"); p != nil {
				if r := p.Params.Get("RANGE"); r != "" && r != "THISANDFUTURE" {
					return Unsupported
				}
			}
			if _, err := e.Recurrence(); err != nil && c.Get("DTSTART") != nil {
				return Invalid
			}
		}
		if c.Name == "VTIMEZONE" {
			if counts["TZID"] != 1 || len(c.Children) == 0 {
				return Invalid
			}
			for _, child := range c.Children {
				if child.Name != "STANDARD" && child.Name != "DAYLIGHT" {
					return Unsupported
				}
			}
		}
		if c.Name == "STANDARD" || c.Name == "DAYLIGHT" {
			for _, name := range []string{"DTSTART", "TZOFFSETFROM", "TZOFFSETTO"} {
				if counts[name] != 1 {
					return Invalid
				}
			}
			if counts["RRULE"] > 1 {
				return Invalid
			}
			for _, name := range []string{"TZOFFSETFROM", "TZOFFSETTO"} {
				if _, err := c.Get(name).UTCOffset(); err != nil {
					return Invalid
				}
			}
		}
		for _, child := range c.Children {
			if (child.Name == "VEVENT" || child.Name == "VTIMEZONE") && c.Name != "VCALENDAR" {
				return Invalid
			}
			if (child.Name == "STANDARD" || child.Name == "DAYLIGHT") && c.Name != "VTIMEZONE" {
				return Invalid
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(cal.Component); err != nil {
		return err
	}
	zones := map[string]bool{}
	for z := range cal.Timezones() {
		if z.TZID() == "" || zones[z.TZID()] {
			return Invalid
		}
		zones[z.TZID()] = true
		if _, err := z.Location(); err != nil {
			return Invalid
		}
	}
	// Type consistency is checked across every revision, including cancelled
	// components without DTSTART, before the winning revisions are selected.
	types := map[string]bool{}
	for _, c := range cal.Children {
		if c.Name != "VEVENT" || c.Get("RECURRENCE-ID") != nil || c.Get("DTSTART") == nil {
			continue
		}
		start, err := event(c, cal).Start()
		if err != nil {
			return Invalid
		}
		if old, ok := types[text(c, "UID")]; ok && old != start.IsDate {
			return Invalid
		}
		types[text(c, "UID")] = start.IsDate
	}
	for _, c := range cal.Children {
		if c.Name != "VEVENT" || c.Get("RECURRENCE-ID") == nil {
			continue
		}
		rid, _, err := event(c, cal).RecurrenceID()
		if err != nil {
			return Invalid
		}
		if allDay, ok := types[text(c, "UID")]; ok && allDay != rid.IsDate {
			return Invalid
		}
	}
	return nil
}
func validateDates(p *ical.Property, cal *ical.Calendar, c *ical.Component, count *int) error {
	value := p.Params.Get("VALUE")
	period := p.Name == "RDATE" && value == "PERIOD"
	if value != "" && value != "DATE" && value != "DATE-TIME" && !period {
		return Unsupported
	}
	if p.Name == "DTSTAMP" && (value != "" && value != "DATE-TIME" || p.Params.Has("TZID")) {
		return Invalid
	}
	if p.Name != "RECURRENCE-ID" && p.Params.Has("RANGE") {
		return Unsupported
	}
	for _, param := range p.Params {
		switch strings.ToUpper(param.Name) {
		case "VALUE", "TZID", "RANGE":
		default:
			return Unsupported
		}
	}
	items := strings.Split(p.Value, ",")
	if p.Name != "RDATE" && p.Name != "EXDATE" && len(items) != 1 {
		return Invalid
	}
	if p.Name == "RDATE" || p.Name == "EXDATE" {
		*count += len(items)
		if *count > 20000 {
			return Limit
		}
	}
	for _, item := range items {
		pieces := []string{item}
		if period {
			pieces = strings.Split(item, "/")
			if len(pieces) != 2 {
				return Invalid
			}
		}
		for j, s := range pieces {
			if period && j == 1 && (strings.HasPrefix(s, "P") || strings.HasPrefix(s, "+P")) {
				continue
			}
			layout := "20060102T150405"
			if value == "DATE" {
				layout = "20060102"
			} else if strings.HasSuffix(s, "Z") {
				layout += "Z"
			}
			if _, err := time.Parse(layout, s); err != nil {
				return Invalid
			}
			if p.Params.Has("TZID") && (value == "DATE" || strings.HasSuffix(s, "Z")) {
				return Invalid
			}
		}
	}
	if (c.Name == "STANDARD" || c.Name == "DAYLIGHT") && (value == "DATE" || p.Params.Has("TZID") || strings.HasSuffix(p.Value, "Z")) {
		return Invalid
	}
	if period {
		if c.Name == "VEVENT" {
			start, err := event(c, cal).Start()
			if err != nil || start.IsDate {
				return Invalid
			}
		}
		periods, err := p.Periods(cal)
		if err != nil {
			return Invalid
		}
		for _, v := range periods {
			if !v.EndTime().In(time.UTC).After(v.Start.In(time.UTC)) {
				return Invalid
			}
		}
	} else {
		values, err := p.DateTimes(cal)
		if err != nil || len(values) != len(items) {
			return Invalid
		}
		if (p.Name == "RDATE" || p.Name == "EXDATE") && c.Name == "VEVENT" {
			start, err := event(c, cal).Start()
			if err != nil {
				return Invalid
			}
			for _, v := range values {
				if v.IsDate != start.IsDate {
					return Invalid
				}
			}
		}
	}
	return nil
}
func validateRule(p *ical.Property, c *ical.Component) error {
	seen := map[string]bool{}
	for _, part := range strings.Split(p.Value, ";") {
		k, v, ok := strings.Cut(part, "=")
		k = strings.ToUpper(k)
		if !ok || seen[k] {
			return Invalid
		}
		seen[k] = true
		switch k {
		case "FREQ", "UNTIL", "COUNT", "INTERVAL", "BYSECOND", "BYMINUTE", "BYHOUR", "BYDAY", "BYMONTHDAY", "BYYEARDAY", "BYWEEKNO", "BYMONTH", "BYSETPOS", "WKST":
		case "RSCALE":
			if v != "GREGORIAN" {
				return Unsupported
			}
		case "SKIP":
			if v != "OMIT" {
				return Unsupported
			}
		default:
			return Unsupported
		}
		if strings.HasPrefix(k, "BY") {
			entries := strings.Split(v, ",")
			if len(entries) > 366 {
				return Limit
			}
			values := map[string]bool{}
			for _, entry := range entries {
				canonical := strings.ToUpper(entry)
				if k == "BYDAY" && len(canonical) > 2 {
					if n, err := strconv.Atoi(canonical[:len(canonical)-2]); err == nil {
						canonical = strconv.Itoa(n) + canonical[len(canonical)-2:]
					}
				}
				if k != "BYDAY" {
					if n, err := strconv.Atoi(entry); err == nil {
						canonical = strconv.Itoa(n)
					}
				}
				if values[canonical] {
					return Invalid
				}
				values[canonical] = true
			}
		}
	}
	r, err := p.Recur()
	if err != nil || r.Validate() != nil {
		return Invalid
	}
	if start := c.Get("DTSTART"); start != nil {
		date := start.Params.Get("VALUE") == "DATE"
		if date && (seen["BYSECOND"] || seen["BYMINUTE"] || seen["BYHOUR"]) {
			return Invalid
		}
		if !r.Until.IsZero() {
			if date != r.Until.IsDate {
				return Invalid
			}
			zoned := start.Params.Has("TZID") || strings.HasSuffix(start.Value, "Z") || c.Name == "STANDARD" || c.Name == "DAYLIGHT"
			if !date && zoned != r.Until.UTC {
				return Invalid
			}
		}
	}
	return nil
}

func preflight(root *ical.Component) error {
	properties := 0
	var walk func(*ical.Component) error
	walk = func(c *ical.Component) error {
		for _, p := range c.Props {
			properties++
			if properties > 100000 || len(p.Params) > 64 {
				return Limit
			}
			if p.Name == "RRULE" && (c.Name == "VEVENT" || c.Name == "STANDARD" || c.Name == "DAYLIGHT") {
				if err := validateRule(p, c); err != nil {
					return err
				}
			}
		}
		for _, child := range c.Children {
			if (child.Name == "VEVENT" || child.Name == "VTIMEZONE") && c.Name != "VCALENDAR" {
				return Invalid
			}
			if (child.Name == "STANDARD" || child.Name == "DAYLIGHT") && c.Name != "VTIMEZONE" {
				return Invalid
			}
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}
