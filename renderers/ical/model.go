package ical

import (
	"sort"
	"time"
)

// Model is safe for templates: it contains no subscription or credentials.
// Event text is plain text and must be HTML-escaped by the rendering template.
type Model struct {
	Title, View, Timezone string
	Now                   time.Time
	Sources               []Label
	Days                  []Day
}
type Label struct {
	ID, Name string
	Holiday  bool
}
type Day struct {
	Date                     string
	Weekday                  time.Weekday
	Today, InMonth           bool
	Holidays                 []string
	Events                   []DayEvent
	Omitted, OmittedHolidays int
}
type DayEvent struct {
	SourceID, SourceName, Summary                    string
	Start, End                                       time.Time
	AllDay, ContinuesBefore, ContinuesAfter, Ongoing bool
	// Ordering identities stay private, including when Model is serialized.
	order    int
	uid, rid string
}

func midnight(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
func (input *Input) window(now time.Time) (time.Time, time.Time) {
	loc, _ := time.LoadLocation(input.timezone)
	today := midnight(now.In(loc))
	if input.view == "agenda" {
		return today, today.AddDate(0, 0, 7)
	}
	from := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)
	end := from.AddDate(0, 1, 0)
	week := 0
	if input.weekStartsOn == "monday" {
		week = 1
	}
	from = from.AddDate(0, 0, -(int(from.Weekday())-week+7)%7)
	if n := (week - int(end.Weekday()) + 7) % 7; n != 0 {
		end = end.AddDate(0, 0, n)
	}
	return from, end
}
func (input *Input) model(now, from, to time.Time, all [][]Instance) *Model {
	loc, _ := time.LoadLocation(input.timezone)
	now = now.In(loc)
	today := now.Format(time.DateOnly)
	m := &Model{Title: input.title, View: input.view, Timezone: input.timezone, Now: now, Days: []Day{}, Sources: []Label{}}
	for _, s := range input.sources {
		m.Sources = append(m.Sources, Label{s.id, s.name, s.holiday})
	}
	for day := from; day.Before(to); day = day.AddDate(0, 0, 1) {
		end := day.AddDate(0, 0, 1)
		date := day.Format(time.DateOnly)
		d := Day{Date: date, Weekday: day.Weekday(), Today: date == today, InMonth: day.Month() == now.Month() && day.Year() == now.Year(), Events: []DayEvent{}, Holidays: []string{}}
		holidays := map[string]bool{}
		for j, instances := range all {
			s := input.sources[j]
			for _, i := range instances {
				start, finish := i.Start.In(loc), i.End.In(loc)
				if i.AllDay {
					if date < i.StartDate || date >= i.EndDate {
						continue
					}
					start = day
					finish = end
				} else {
					if start.Equal(finish) {
						if start.Before(day) || !start.Before(end) {
							continue
						}
					} else if !start.Before(end) || !finish.After(day) {
						continue
					}
					if input.view == "agenda" && d.Today && i.Finished(now) {
						continue
					}
				}
				if s.holiday {
					if !holidays[i.Summary] {
						d.Holidays = append(d.Holidays, i.Summary)
						holidays[i.Summary] = true
					}
					continue
				}
				before, after := i.Start.Before(day), i.End.After(end)
				if i.AllDay {
					before = i.StartDate < date
					after = i.EndDate > end.Format(time.DateOnly)
				}
				d.Events = append(d.Events, DayEvent{SourceID: s.id, SourceName: s.name, Summary: i.Summary, Start: maxTime(start, day), End: minTime(finish, end), AllDay: i.AllDay, ContinuesBefore: before, ContinuesAfter: after, Ongoing: d.Today && i.Ongoing(now), order: j, uid: i.UID, rid: i.RecurrenceID})
			}
		}
		sort.Slice(d.Events, func(a, b int) bool {
			x, y := d.Events[a], d.Events[b]
			if x.AllDay != y.AllDay {
				return x.AllDay
			}
			if !x.Start.Equal(y.Start) {
				return x.Start.Before(y.Start)
			}
			if x.order != y.order {
				return x.order < y.order
			}
			if x.Summary != y.Summary {
				return x.Summary < y.Summary
			}
			if x.uid != y.uid {
				return x.uid < y.uid
			}
			return x.rid < y.rid
		})
		m.Days = append(m.Days, d)
	}
	return m
}
func maxTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}
func minTime(a, b time.Time) time.Time {
	if a.After(b) {
		return b
	}
	return a
}
