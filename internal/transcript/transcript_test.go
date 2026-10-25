package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dslh/gcal-transcripts/internal/gapi"
)

func TestFind(t *testing.T) {
	ev := gapi.Event{Attachments: []gapi.Attachment{
		{Title: "Notes by Gemini", MimeType: googleDocMime},
		{Title: "Notizen von Gemini", MimeType: googleDocMime},
		{Title: "Weekly sync (2026-10-21 14:00 GMT+2) - Transcript", MimeType: googleDocMime},
		{Title: "Notes - Weekly sync", MimeType: googleDocMime},
		{Title: "Agenda", MimeType: googleDocMime},
		{Title: "Recording", MimeType: "video/mp4"},
	}}
	if got := Find(ev, false); len(got) != 3 {
		t.Errorf("Find = %d attachments, want the 3 titled like transcripts", len(got))
	}
	if got := Find(ev, true); len(got) != 5 {
		t.Errorf("Find with anyDoc = %d attachments, want every Doc (5)", len(got))
	}
}

func TestStripImages(t *testing.T) {
	in := "Hello ![][image1]\n\n[image1]: <data:image/png;base64,iVBORw0KGgo+/=>\n"
	want := "Hello ![][image1]\n\n[image1]: <<image>>\n"
	if got := StripImages(in); got != want {
		t.Errorf("StripImages = %q, want %q", got, want)
	}
}

func TestSplitTabs(t *testing.T) {
	md := "preamble\n# **📝 Notes**\nsummary\n# **📖 Transcript**\nwords\n"
	tabs := SplitTabs(md)
	if len(tabs) != 3 || tabs[0].Name != "" || tabs[1].Name != "Notes" || tabs[2].Name != "Transcript" {
		t.Fatalf("SplitTabs = %+v", tabs)
	}
	if strings.Join([]string{tabs[0].Markdown, tabs[1].Markdown, tabs[2].Markdown}, "") != md {
		t.Error("tabs don't add back up to the document")
	}
	if tab, ok := SelectTab(md, "trans"); !ok || tab.Markdown != "# **📖 Transcript**\nwords\n" {
		t.Errorf("SelectTab(trans) = %+v, %v", tab, ok)
	}
	german := "# **📝 Notizen**\nx\n# **📖 Transkript**\nWörter\n"
	if tab, ok := SelectTab(german, "Transcript"); !ok || tab.Name != "Transkript" {
		t.Errorf("SelectTab(Transcript) on a German Doc = %+v, %v", tab, ok)
	}
	if _, ok := SelectTab(md, "agenda"); ok {
		t.Error("SelectTab found a tab that isn't there")
	}
	if tabs := SplitTabs("no headings"); len(tabs) != 1 || tabs[0].Markdown != "no headings" {
		t.Errorf("SplitTabs without headings = %+v", tabs)
	}
}

func TestStem(t *testing.T) {
	start := time.Date(2026, 10, 21, 14, 0, 0, 0, time.Local)
	ev := gapi.Event{Summary: "Weekly sync: Jobstream / Café ☕", Start: gapi.EventTime{DateTime: start.Format(time.RFC3339)}}
	if got, want := Stem(ev), "2026-10-21_14-00_weekly-sync-jobstream-café"; got != want {
		t.Errorf("Stem = %q, want %q", got, want)
	}
	allDay := gapi.Event{Summary: "!!!", Start: gapi.EventTime{Date: "2026-10-21"}}
	if got, want := Stem(allDay), "2026-10-21_untitled"; got != want {
		t.Errorf("Stem = %q, want %q", got, want)
	}
}

func TestRenderAndSaved(t *testing.T) {
	dir := t.TempDir()
	ev := gapi.Event{
		ID:        "abc123_20261021T120000Z",
		Summary:   `Sync "quoted": a\b`,
		Start:     gapi.EventTime{DateTime: "2026-10-21T14:00:00+02:00"},
		End:       gapi.EventTime{DateTime: "2026-10-21T14:30:00+02:00"},
		Organizer: gapi.Person{Email: "org@example.com"},
		Attendees: []gapi.Person{
			{Email: "anna@example.com", DisplayName: "Anna"},
			{Email: "room@resource.calendar.google.com", Resource: true},
		},
	}
	data := Render(ev, []Doc{{Title: "T", URL: "https://docs.google.com/document/d/x", Markdown: "# Body\n"}},
		"", time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC))
	s := string(data)
	for _, want := range []string{
		"title: \"Sync \\\"quoted\\\": a\\\\b\"\n",
		"  - \"Anna <anna@example.com>\"\n",
		"---\n\n# Body\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered file lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "room@") {
		t.Error("rendered file lists a meeting room as a guest")
	}

	path := FreePath(dir, Stem(ev))
	if err := Write(path, data); err != nil {
		t.Fatal(err)
	}
	if second := FreePath(dir, Stem(ev)); second == path || !strings.HasSuffix(second, "_2.md") {
		t.Errorf("FreePath reused or misnamed: %s", second)
	}
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("just notes\n"), 0o644)
	saved, err := Saved(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[ev.ID] != filepath.Base(path) {
		t.Errorf("Saved = %v", saved)
	}
	if saved, err := Saved(filepath.Join(dir, "missing")); err != nil || len(saved) != 0 {
		t.Errorf("Saved on a missing dir = %v, %v", saved, err)
	}
}
