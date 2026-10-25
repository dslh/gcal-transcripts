package main

import (
	"flag"
	"slices"
	"testing"
	"time"
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

func TestEveryCommandHasHelp(t *testing.T) {
	for _, c := range commands {
		if c.summary == "" || len(c.synopsis) == 0 || c.flags == nil {
			t.Errorf("%s lacks help", c.name)
		}
	}
}
