package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dslh/gcal-transcripts/internal/gapi"
	"github.com/dslh/gcal-transcripts/internal/transcript"
)

type pullOpts struct {
	filterOpts
	all    bool
	force  bool
	dryRun bool
	tab    string
}

func (o *pullOpts) flags() *flag.FlagSet {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	o.register(fs)
	fs.BoolVar(&o.all, "all", false, "pull every transcript in the window not saved yet, instead of naming events")
	fs.BoolVar(&o.force, "force", false, "fetch transcripts again even if already saved, overwriting their files")
	fs.BoolVar(&o.dryRun, "dry-run", false, "say what would be saved without fetching or writing anything")
	fs.StringVar(&o.tab, "tab", "",
		"save only this `tab` of each Doc (e.g. transcript), matched by name or its start")
	return fs
}

var pullCmd = &command{
	name:    "pull",
	summary: "save transcripts as markdown files",
	synopsis: []string{
		"[flags] <event-id>...",
		"--all [--since time] [--until time] [--match text] [--attendee text] [flags]",
	},
	about: `
Exports each event's transcript Doc as markdown, using Google Drive's own
conversion, and saves it in the --out directory as
YYYY-MM-DD_HH-MM_<title>.md. The file starts with YAML front matter
recording the event (title, ID, times, organizer, guests, transcript links),
followed by the export as Drive produced it, except that embedded images,
which Gemini's notes are full of, are replaced with <image>.

Name events by the IDs 'gcal-transcripts list' prints, or use --all with the
same filters as list. An event already saved in --out is skipped unless
--force; saved files are recognised by the event_id in their front matter,
so renaming a file doesn't make it be fetched again.

Each saved file's path is printed on stdout. A transcript that can't be
fetched (it isn't shared with you, say) is reported and skipped, the rest
still saved, and the exit status is 1.`,
	flags: func() *flag.FlagSet { return new(pullOpts).flags() },
	run:   runPull,
}

func runPull(ctx context.Context, args []string) error {
	var o pullOpts
	ids, err := parseArgs(o.flags(), args)
	if err != nil {
		return err
	}
	if o.all == (len(ids) > 0) {
		return usageError("name the events to pull, or use --all")
	}
	svc, err := newService(ctx)
	if err != nil {
		return err
	}
	saved, err := transcript.Saved(o.out)
	if err != nil {
		return err
	}

	var events []gapi.Event
	if o.all {
		if events, err = o.events(ctx, svc); err != nil {
			return err
		}
	} else {
		for _, id := range ids {
			ev, err := svc.GetEvent(ctx, o.calendar, id)
			if err != nil {
				return fmt.Errorf("event %s: %w", id, err)
			}
			events = append(events, ev)
		}
	}
	if !o.dryRun {
		if err := os.MkdirAll(o.out, 0o755); err != nil {
			return err
		}
	}

	var pulled, skipped, failed int
	for _, ev := range events {
		if name, ok := saved[ev.ID]; ok && !o.force {
			fmt.Fprintf(os.Stderr, "skip  %s: already saved as %s\n", label(ev), name)
			skipped++
			continue
		}
		path, err := o.pull(ctx, svc, ev, saved[ev.ID])
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Fprintf(os.Stderr, "fail  %s: %s\n", label(ev), describe(err))
			if needsLogin(err) {
				return err
			}
			failed++
			continue
		}
		if o.dryRun {
			fmt.Fprintf(os.Stderr, "would save %s as %s\n", label(ev), path)
		} else {
			fmt.Println(path)
		}
		pulled++
	}

	verb := "saved"
	if o.dryRun {
		verb = "would save"
	}
	fmt.Fprintf(os.Stderr, "%s %d, skipped %d, failed %d\n", verb, pulled, skipped, failed)
	if failed > 0 {
		return silentError(fmt.Sprintf("%d transcripts failed", failed))
	}
	return nil
}

// pull saves one event's transcripts to a file and returns its path. An
// event already saved as existing is written back to the same file. Either
// every Doc on the event is fetched or nothing is written: a partial file
// would count as saved, and the missing part would never be fetched.
func (o *pullOpts) pull(ctx context.Context, svc *gapi.Service, ev gapi.Event, existing string) (string, error) {
	path := filepath.Join(o.out, existing)
	if existing == "" {
		path = transcript.FreePath(o.out, transcript.Stem(ev))
	}
	atts := transcript.Find(ev, o.anyDoc)
	if len(atts) == 0 {
		return "", errors.New("no transcript attached")
	}
	if o.dryRun {
		return path, nil
	}

	var docs []transcript.Doc
	for _, a := range atts {
		raw, err := svc.ExportMarkdown(ctx, a.FileID)
		if err != nil {
			return "", fmt.Errorf("%q: %w", a.Title, err)
		}
		md := transcript.StripImages(string(raw))
		if o.tab != "" {
			t, ok := transcript.SelectTab(md, o.tab)
			if !ok {
				return "", fmt.Errorf("%q has no tab %q (tabs: %s)", a.Title, o.tab, strings.Join(transcript.TabNames(md), ", "))
			}
			md = t.Markdown
		}
		docs = append(docs, transcript.Doc{Title: a.Title, URL: a.FileURL, Markdown: md})
	}
	return path, transcript.Write(path, transcript.Render(ev, docs, o.tab, time.Now()))
}

// describe explains the failures a guest is likely to meet in plain words.
func describe(err error) string {
	var apiErr *gapi.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.IsNoAccess():
			return "the transcript isn't shared with you; ask the meeting organiser for access (" + err.Error() + ")"
		case apiErr.IsDownloadDisabled():
			return "the transcript's owner has disabled downloading for viewers (" + err.Error() + ")"
		}
	}
	return err.Error()
}

func label(ev gapi.Event) string {
	return fmt.Sprintf("%q (%s)", ev.Summary, when(ev))
}
