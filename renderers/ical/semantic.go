// Package ical implements bounded input, fetching and normalization for ARPRT.
package ical

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	ical "github.com/aanelson/iCalendar"
	"github.com/aanelson/iCalendar/civil"
)

const (
	MaxBody         = 4 << 20
	MaxLine         = 16 << 10
	MaxComponents   = 10000
	MaxOccurrences  = 20000
	MaxResult       = 8 << 20
	SemanticTimeout = 5 * time.Second
)

// Error exposes a closed classification, never a library error containing input.
type Error string

func (e Error) Error() string { return string(e) }

const (
	Invalid  Error = "invalid_calendar"
	Limit    Error = "resource_limit"
	Deadline Error = "deadline"
	Worker   Error = "semantic_worker"
)

// Request is private IPC. It must never be logged or passed to templates.
type Request struct {
	Calendar string
	Timezone string
	From, To time.Time
	Holiday  bool
}

// Instance keeps civil DATE values separate from instants. Source metadata is
// attached by the caller; URLs and authentication never enter this model.
type Instance struct {
	UID, RecurrenceID, Summary string
	Start, End                 time.Time
	StartDate, EndDate         string
	AllDay                     bool
}

// Supervise runs a fixed internal command, terminates it at the shared deadline
// and waits for it to exit. A stuck library call never outlives this function.
// command is chosen by the executable, never by calendar/configuration input.
func Supervise(ctx context.Context, command []string, req Request) ([]Instance, error) {
	if len(command) == 0 {
		return nil, Worker
	}
	ctx, cancel := context.WithTimeout(ctx, SemanticTimeout)
	defer cancel()
	if len(req.Calendar) > MaxBody {
		return nil, Limit
	}
	if !utf8.ValidString(req.Calendar) {
		return nil, Invalid
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, Invalid
	}
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stdin = bytes.NewReader(data)
	var out cappedBuffer
	cmd.Stdout = &out
	// Discard stderr: neither parser diagnostics nor panic stacks are public.
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, Deadline
	}
	if out.exceeded {
		return nil, Limit
	}
	if err != nil {
		return nil, Worker
	}
	var reply Reply
	if json.Unmarshal(out.Bytes(), &reply) != nil {
		return nil, Worker
	}
	if reply.Error != "" {
		switch reply.Error {
		case Invalid, Unsupported, Limit, Deadline:
			return nil, reply.Error
		}
		return nil, Worker
	}
	return reply.Instances, nil
}

type cappedBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxResult {
		b.exceeded = true
		return 0, Limit
	}
	return b.Buffer.Write(p)
}

// Reply is the bounded, private worker response.
type Reply struct {
	Instances []Instance
	Error     Error
}

// Serve performs a single worker request. The process supervisor owns its hard
// deadline; no goroutine is used to abandon a synchronous parser or iterator.
func Serve(r io.Reader, w io.Writer) error {
	data, err := io.ReadAll(io.LimitReader(r, 6*MaxBody+1))
	if err != nil || len(data) > 6*MaxBody {
		return Invalid
	}
	var req Request
	if json.Unmarshal(data, &req) != nil {
		return Invalid
	}
	instances, err := expand(req)
	reply := Reply{Instances: instances}
	if err != nil {
		reply.Error = Invalid
		var classified Error
		if errors.As(err, &classified) {
			reply.Error = classified
		}
	}
	return json.NewEncoder(w).Encode(reply)
}

func event(c *ical.Component, cal *ical.Calendar) ical.Event {
	return ical.Event{Item: ical.Item{Component: c, Calendar: cal}}
}

func text(c *ical.Component, name string) string {
	if p := c.Get(name); p != nil {
		return p.Text()
	}
	return ""
}

func key(t ical.DateTime, loc *time.Location) string {
	if t.IsDate {
		return "D:" + t.Date.String()
	}
	return "T:" + t.In(loc).UTC().Format(time.RFC3339)
}

// revision is ordered by SEQUENCE, DTSTAMP, then later input position.
func newer(a, b *ical.Component) bool {
	seq := func(c *ical.Component) int { n, _ := strconv.Atoi(text(c, "SEQUENCE")); return n }
	if seq(a) != seq(b) {
		return seq(a) > seq(b)
	}
	return text(a, "DTSTAMP") >= text(b, "DTSTAMP")
}

func expand(req Request) ([]Instance, error) {
	if len(req.Calendar) > MaxBody {
		return nil, Limit
	}
	if !utf8.ValidString(req.Calendar) {
		return nil, Invalid
	}
	for _, r := range req.Calendar {
		if (r < 32 && r != 9 && r != 10 && r != 13) || r == 127 {
			return nil, Invalid
		}
	}
	if !req.From.Before(req.To) || req.To.Sub(req.From) > 43*24*time.Hour {
		return nil, Invalid
	}
	loc, err := time.LoadLocation(req.Timezone)
	if err != nil || req.Timezone == "Local" || req.Timezone == "" {
		return nil, Invalid
	}
	if req.To.After(req.From.In(loc).AddDate(0, 0, 42)) {
		return nil, Limit
	}
	d := ical.NewDecoder(strings.NewReader(req.Calendar))
	d.MaxLineLength, d.MaxDepth = MaxLine, 16
	// Empty calendars are valid for this consumer. Every actual recovery is
	// fatal; Strict also rejects the empty calendar, so use Warn instead.
	warned := false
	d.Warn = func(error) { warned = true }
	cal, err := d.Decode()
	if err != nil || warned {
		return nil, Invalid
	}
	if _, err := d.Decode(); err != io.EOF || warned {
		return nil, Invalid
	}
	if cal.Version() != "2.0" {
		return nil, Invalid
	}
	components := 0
	var count func(*ical.Component) bool
	count = func(c *ical.Component) bool {
		components++
		if components > MaxComponents {
			return false
		}
		for _, child := range c.Children {
			if !count(child) {
				return false
			}
		}
		return true
	}
	if !count(cal.Component) {
		return nil, Limit
	}
	if err := validateCalendar(cal, req.Holiday, loc); err != nil {
		return nil, err
	}
	groups := map[string]map[string]*ical.Component{}
	var zones []*ical.Component
	for _, c := range cal.Children {
		if c.Name == "VTIMEZONE" {
			zones = append(zones, c)
			continue
		}
		if c.Name != "VEVENT" {
			continue
		}
		e := event(c, cal)
		if e.UID() == "" {
			return nil, Invalid
		}
		for _, name := range []string{"UID", "DTSTAMP", "SEQUENCE", "DTSTART", "DTEND", "DURATION", "RECURRENCE-ID", "SUMMARY", "STATUS", "RRULE"} {
			n := 0
			for range c.Props.All(name) {
				n++
			}
			if n > 1 {
				return nil, Invalid
			}
		}
		if p := c.Get("SEQUENCE"); p != nil {
			n, err := strconv.Atoi(p.Value)
			if err != nil || n < 0 {
				return nil, Invalid
			}
		}
		if p := c.Get("DTSTAMP"); p != nil {
			if _, err := time.Parse("20060102T150405Z", p.Value); err != nil {
				return nil, Invalid
			}
		}
		if c.Get("DTEND") != nil && c.Get("DURATION") != nil {
			return nil, Invalid
		}
		if e.Status() != "CANCELLED" {
			start, err := e.Start()
			if err != nil {
				return nil, Invalid
			}
			end, err := e.End()
			if err != nil || start.IsDate != end.IsDate || end.In(loc).Before(start.In(loc)) {
				return nil, Invalid
			}
			if start.IsDate && !end.In(loc).After(start.In(loc)) {
				return nil, Invalid
			}
		}
		rid, override, err := e.RecurrenceID()
		if err != nil {
			return nil, Invalid
		}
		k := ""
		if override {
			k = key(rid, loc)
		}
		if groups[e.UID()] == nil {
			groups[e.UID()] = map[string]*ical.Component{}
		}
		old := groups[e.UID()][k]
		if old == nil || newer(c, old) {
			groups[e.UID()][k] = c
		}
	}
	var result []Instance
	visited := 0
	for _, revisions := range groups {
		part, err := expandGroup(cal, zones, revisions, loc, req.From, req.To, &visited)
		if err != nil {
			return nil, err
		}
		if len(result)+len(part) > MaxOccurrences {
			return nil, Limit
		}
		result = append(result, part...)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if !a.Start.Equal(b.Start) {
			return a.Start.Before(b.Start)
		}
		if a.UID != b.UID {
			return a.UID < b.UID
		}
		return a.RecurrenceID < b.RecurrenceID
	})
	return result, nil
}

type rangeChange struct {
	rid      ical.DateTime
	start    ical.DateTime
	duration ical.Duration
	c        *ical.Component
}

func expandGroup(cal *ical.Calendar, zones []*ical.Component, revisions map[string]*ical.Component, loc *time.Location, from, to time.Time, visited *int) ([]Instance, error) {
	master := revisions[""]
	if master != nil && event(master, cal).Status() == "CANCELLED" {
		return nil, nil
	}
	var changes []rangeChange
	var result []Instance
	margin := time.Duration(0)
	for k, c := range revisions {
		if k == "" {
			continue
		}
		*visited += 1
		if *visited > MaxOccurrences {
			return nil, Limit
		}
		e := event(c, cal)
		rid, _, err := e.RecurrenceID()
		if err != nil {
			return nil, Invalid
		}
		p := c.Get("RECURRENCE-ID")
		scope := strings.ToUpper(p.Params.Get("RANGE"))
		if scope != "" && scope != "THISANDFUTURE" {
			return nil, Invalid
		}
		cancelled := e.Status() == "CANCELLED"
		if cancelled && scope == "" {
			continue
		}
		start, err := e.Start()
		if err != nil {
			if !cancelled {
				return nil, Invalid
			}
			start = rid
		}
		duration, err := e.Duration()
		if err != nil {
			if !cancelled {
				return nil, Invalid
			}
			duration = ical.Duration{}
		}
		if scope != "" {
			if master == nil {
				return nil, Invalid
			}
			mstart, err := event(master, cal).Start()
			if err != nil {
				return nil, Invalid
			}
			// A range shift is measured on the master's clock, not UTC.
			// Convert different TZID spellings to that clock before subtraction.
			if start.IsDate != rid.IsDate || rid.IsDate != mstart.IsDate {
				return nil, Invalid
			}
			if !start.IsDate && mstart.Location() != nil {
				start.DateTime = civil.DateTimeOf(start.In(loc).In(mstart.Location()))
				start.TZID = mstart.TZID
				start.UTC = mstart.UTC
				start.Loc = mstart.Loc
				rid.DateTime = civil.DateTimeOf(rid.In(loc).In(mstart.Location()))
				rid.TZID = mstart.TZID
				rid.UTC = mstart.UTC
				rid.Loc = mstart.Loc
			}
			shift := time.Duration(start.EpochSecond()-rid.EpochSecond()) * time.Second
			if shift < 0 {
				shift = -shift
			}
			margin = max(margin, shift+duration.Approx()+48*time.Hour)
			if margin > 366*24*time.Hour {
				return nil, Limit
			}
			changes = append(changes, rangeChange{rid, start, duration, c})
		}
		if cancelled {
			continue
		}
		end, err := e.End()
		if err != nil {
			return nil, Invalid
		}
		if overlaps(start, end, loc, from, to) {
			result = append(result, instance(c, rid, start, end, loc))
		}
		if len(result) > MaxOccurrences {
			return nil, Limit
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].rid.In(loc).Before(changes[j].rid.In(loc)) })
	if master == nil {
		return result, nil
	}
	mini := &ical.Calendar{Component: ical.NewComponent("VCALENDAR")}
	mini.Children = append(append([]*ical.Component{}, zones...), master)
	x := ical.Expander{Floating: loc}
	for occ, err := range x.Occurrences(mini, from.Add(-margin), to.Add(margin)) {
		if err != nil {
			return nil, Invalid
		}
		*visited += 1
		if *visited > MaxOccurrences {
			return nil, Limit
		}
		if revisions[key(occ.RecurrenceID, loc)] != nil {
			continue
		}
		c := master
		start, end := occ.Start, occ.End
		for _, change := range changes {
			if occ.RecurrenceID.In(loc).Before(change.rid.In(loc)) {
				break
			}
			c = change.c
			start = occ.Start
			start.DateTime = civil.DateTimeOfEpochSecond(occ.Start.EpochSecond() + change.start.EpochSecond() - change.rid.EpochSecond())
			end = start.Add(change.duration)
		}
		if event(c, cal).Status() == "CANCELLED" {
			continue
		}
		if overlaps(start, end, loc, from, to) {
			result = append(result, instance(c, occ.RecurrenceID, start, end, loc))
		}
	}
	return result, nil
}

func overlaps(start, end ical.DateTime, loc *time.Location, from, to time.Time) bool {
	s, e := start.In(loc), end.In(loc)
	if s.Equal(e) {
		return !s.Before(from) && s.Before(to)
	}
	return s.Before(to) && e.After(from)
}

func instance(c *ical.Component, rid, start, end ical.DateTime, loc *time.Location) Instance {
	summary := text(c, "SUMMARY")
	if summary == "" {
		summary = "名称未設定"
	}
	i := Instance{UID: text(c, "UID"), RecurrenceID: key(rid, loc), Summary: summary, Start: start.In(loc), End: end.In(loc), AllDay: start.IsDate}
	if start.IsDate {
		i.StartDate = start.Date.String()
		i.EndDate = end.Date.String()
	}
	return i
}

// Ongoing uses a half-open timed interval. Civil all-day events are retained by
// day normalization separately, without an ongoing badge.
func (i Instance) Ongoing(now time.Time) bool {
	return !i.AllDay && !now.Before(i.Start) && now.Before(i.End)
}

// Finished keeps an instantaneous event at exactly its start timestamp.
func (i Instance) Finished(now time.Time) bool {
	if i.AllDay {
		return false
	}
	if i.Start.Equal(i.End) {
		return now.After(i.Start)
	}
	return !now.Before(i.End)
}
