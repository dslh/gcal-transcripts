package main

import (
	"encoding/json"
	"flag"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dslh/gcal-transcripts/internal/gapi"
)

func TestParseArgsInterspersed(t *testing.T) {
	var o pullOpts
	fs := o.flags()
	pos, err := parseArgs(fs, []string{"id1", "--out", "dir", "id2", "--force", "--", "--weird-id"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pos, []string{"id1", "id2", "--weird-id"}) || o.out != "dir" || !o.force {
		t.Errorf("pos = %v, out = %q, force = %v", pos, o.out, o.force)
	}
	if _, err := parseArgs(flag.NewFlagSet("x", flag.ContinueOnError), []string{"--help"}); err != errHelp {
		t.Errorf("--help gave %v", err)
	}
	if _, err := parseArgs(flag.NewFlagSet("x", flag.ContinueOnError), []string{"--nope"}); err == nil {
		t.Error("an unknown flag parsed")
	}
}

func TestParseWhen(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local)
	for in, want := range map[string]time.Time{
		"now":        now,
		"36h":        now.Add(-36 * time.Hour),
		"14d":        now.AddDate(0, 0, -14),
		"2w":         now.AddDate(0, 0, -14),
		"2026-10-01": time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local),
	} {
		got, err := parseWhen(in, now, false)
		if err != nil || !got.Equal(want) {
			t.Errorf("parseWhen(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if got, _ := parseWhen("2026-10-01", now, true); !got.Equal(time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local)) {
		t.Errorf("an end date doesn't include its day: %v", got)
	}
	for _, bad := range []string{"", "d", "yesterday", "3x"} {
		if _, err := parseWhen(bad, now, false); err == nil {
			t.Errorf("parseWhen(%q) succeeded", bad)
		}
	}
}

func TestListRowGuests(t *testing.T) {
	ev := gapi.Event{
		ID:        "e",
		Organizer: gapi.Person{Email: "anna@example.com", DisplayName: "Anna"},
		Attendees: []gapi.Person{
			{Email: "anna@example.com", DisplayName: "Anna"},
			{Email: "ben@other.example"},
			{Email: "room@resource.calendar.google.com", Resource: true},
		},
	}
	r := newListRow(ev, nil, "")
	if r.Organizer != "Anna <anna@example.com>" {
		t.Errorf("organizer = %q", r.Organizer)
	}
	if !slices.Equal(r.Attendees, []string{"Anna <anna@example.com>", "ben@other.example"}) {
		t.Errorf("attendees = %q, want the people without the room", r.Attendees)
	}
	data, _ := json.Marshal(newListRow(gapi.Event{ID: "solo"}, nil, ""))
	if !strings.Contains(string(data), `"attendees":[]`) || strings.Contains(string(data), "organizer") {
		t.Errorf("an event without guests encodes as %s", data)
	}
}

func TestEveryCommandHasHelp(t *testing.T) {
	for _, c := range commands {
		if c.summary == "" || len(c.synopsis) == 0 || c.flags == nil {
			t.Errorf("%s lacks help", c.name)
		}
	}
}
