package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/dslh/gcal-transcripts/internal/gapi"
	"github.com/dslh/gcal-transcripts/internal/transcript"
)

// filterOpts choose events, for list and for pull --all.
type filterOpts struct {
	since, until string
	match        string
	attendee     string
	calendar     string
	out          string
	anyDoc       bool
}

func (f *filterOpts) register(fs *flag.FlagSet) {
	fs.StringVar(&f.since, "since", "7d",
		"start of the window: a `time` back from now (36h, 14d, 2w) or a date (2026-10-01)")
	fs.StringVar(&f.until, "until", "now",
		"end of the window: a `time` in the same forms as --since, where a date includes that whole day")
	fs.StringVar(&f.match, "match", "",
		"only events matching this `text`, by Calendar's own search over title, description and guests")
	fs.StringVar(&f.attendee, "attendee", "",
		"only events with a guest whose name or address contains this `text`")
	fs.StringVar(&f.calendar, "calendar", "primary", "the `id` of the calendar to read")
	fs.StringVar(&f.out, "out", ".", "the `dir` transcripts are saved in")
	fs.BoolVar(&f.anyDoc, "any-doc", false,
		"count every Google Doc attached as a transcript, not only those titled like one")
}

// events lists the events in the window that have a transcript attached.
func (f *filterOpts) events(ctx context.Context, svc *gapi.Service) ([]gapi.Event, error) {
	now := time.Now()
	from, err := parseWhen(f.since, now, false)
	if err != nil {
		return nil, usageError("--since: " + err.Error())
	}
	to, err := parseWhen(f.until, now, true)
	if err != nil {
		return nil, usageError("--until: " + err.Error())
	}
	if !from.Before(to) {
		return nil, usageError("--since must be earlier than --until")
	}
	all, err := svc.ListEvents(ctx, gapi.ListOptions{CalendarID: f.calendar, From: from, To: to, Query: f.match})
	if err != nil {
		return nil, err
	}
	var events []gapi.Event
	for _, ev := range all {
		if len(transcript.Find(ev, f.anyDoc)) > 0 && f.hasAttendee(ev) {
			events = append(events, ev)
		}
	}
	return events, nil
}

func (f *filterOpts) hasAttendee(ev gapi.Event) bool {
	if f.attendee == "" {
		return true
	}
	want := strings.ToLower(f.attendee)
	for _, p := range append(transcript.Guests(ev), ev.Organizer) {
		if strings.Contains(strings.ToLower(p.Email), want) || strings.Contains(strings.ToLower(p.DisplayName), want) {
			return true
		}
	}
	return false
}

// parseWhen reads a --since/--until value: "now", a duration back from now
// (Go's units plus d for days and w for weeks), an RFC 3339 time, or a local
// date. With endOfDay, a date means the end of that day.
func parseWhen(s string, now time.Time, endOfDay bool) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "now" {
		return now, nil
	}
	if t, err := time.ParseInLocation(time.DateOnly, s, time.Local); err == nil {
		if endOfDay {
			t = t.AddDate(0, 0, 1)
		}
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if n, unit := strings.TrimRight(s, "dw"), strings.TrimLeft(s, "0123456789"); unit == "d" || unit == "w" {
		days, err := strconv.Atoi(n)
		if err != nil {
			return time.Time{}, fmt.Errorf("can't read %q", s)
		}
		if unit == "w" {
			days *= 7
		}
		return now.AddDate(0, 0, -days), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("can't read %q: give a duration like 36h, 14d or 2w, or a date like 2026-10-01", s)
	}
	return now.Add(-d), nil
}

type listOpts struct {
	filterOpts
	json bool
}

func (o *listOpts) flags() *flag.FlagSet {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	o.register(fs)
	fs.BoolVar(&o.json, "json", false,
		"print JSON instead of a table, adding each event's organizer, guests and transcript Docs")
	return fs
}

var listCmd = &command{
	name:     "list",
	summary:  "list recent events that have a transcript attached",
	synopsis: []string{"[--since time] [--until time] [--match text] [--attendee text] [flags]"},
	about: `
Lists the events in the window that have a transcript Doc attached, oldest
first, with the ID to give 'gcal-transcripts pull'. The STATUS column says
whether each transcript is:

  ready              not saved yet, and you can export it
  saved              already in the --out directory (the file is named in --json)
  no access          attached, but the Doc isn't shared with you
  download disabled  the Doc's owner has stopped viewers downloading it

A transcript is a Google Doc attached to the event with "Gemini" or
"Transcript" in its title, as Gemini's notes ("Notes by Gemini", in the
organiser's language) and Meet's transcripts have. Gemini's notes only hold
a transcript tab if transcription was on. Use --any-doc to count every Doc
attached.`,
	flags: func() *flag.FlagSet { return new(listOpts).flags() },
	run:   runList,
}

type listRow struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Start       string    `json:"start"`
	Status      string    `json:"status"`
	SavedAs     string    `json:"saved_as,omitempty"`
	Organizer   string    `json:"organizer,omitempty"`
	Attendees   []string  `json:"attendees"`
	Transcripts []listDoc `json:"transcripts"`
	display     string    // the start, for the table
}

// newListRow describes an event for list, with the guests in the same form
// as a saved file's front matter, so they can be checked before pulling.
func newListRow(ev gapi.Event, docs []gapi.Attachment, savedAs string) listRow {
	r := listRow{ID: ev.ID, Title: ev.Summary, Start: ev.Start.String(), display: when(ev), SavedAs: savedAs,
		Attendees: []string{}}
	if ev.Organizer.Email != "" {
		r.Organizer = transcript.PersonLabel(ev.Organizer)
	}
	for _, p := range transcript.Guests(ev) {
		r.Attendees = append(r.Attendees, transcript.PersonLabel(p))
	}
	for _, a := range docs {
		r.Transcripts = append(r.Transcripts, listDoc{Title: a.Title, URL: a.FileURL})
	}
	return r
}

type listDoc struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Access string `json:"access,omitempty"`
}

func runList(ctx context.Context, args []string) error {
	var o listOpts
	rest, err := parseArgs(o.flags(), args)
	if err != nil {
		return err
	}
	if err := noArgs(rest); err != nil {
		return err
	}
	svc, err := newService(ctx)
	if err != nil {
		return err
	}
	events, err := o.events(ctx, svc)
	if err != nil {
		return err
	}
	saved, err := transcript.Saved(o.out)
	if err != nil {
		return err
	}

	rows := make([]listRow, len(events))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, ev := range events {
		docs := transcript.Find(ev, o.anyDoc)
		rows[i] = newListRow(ev, docs, saved[ev.ID])
		if rows[i].SavedAs != "" {
			rows[i].Status = "saved"
			continue
		}
		// Checking access costs a request per Doc, so a few run at once.
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			rows[i].Status = "ready"
			for j := range rows[i].Transcripts {
				doc := &rows[i].Transcripts[j]
				doc.Access = access(svc.CheckAccess(ctx, docs[j].FileID))
				if doc.Access != "ok" && rows[i].Status == "ready" {
					rows[i].Status = doc.Access
				}
			}
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}

	if o.json {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if rows == nil {
			rows = []listRow{}
		}
		return enc.Encode(rows)
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "No events with a transcript in that window.")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "WHEN\tTITLE\tSTATUS\tID")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.display, truncate(r.Title, 48), r.Status, r.ID)
	}
	return tw.Flush()
}

// access describes the result of CheckAccess for the STATUS column.
func access(err error) string {
	var apiErr *gapi.APIError
	switch {
	case err == nil:
		return "ok"
	case errors.As(err, &apiErr) && apiErr.IsNoAccess():
		return "no access"
	case errors.As(err, &apiErr) && apiErr.IsDownloadDisabled():
		return "download disabled"
	default:
		return "error: " + err.Error()
	}
}

func when(ev gapi.Event) string {
	t, allDay, err := ev.Start.Time()
	switch {
	case err != nil:
		return ev.Start.String()
	case allDay:
		return t.Format("Mon 02 Jan 2006")
	default:
		return t.Format("Mon 02 Jan 2006 15:04")
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
