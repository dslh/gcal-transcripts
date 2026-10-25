package transcript

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dslh/gcal-transcripts/internal/gapi"
)

// Stem names an event's file, without the extension: the start in local time
// then a slug of the title, e.g. 2026-10-21_14-00_weekly-sync. The time keeps
// two meetings on one day apart and makes a directory listing chronological.
func Stem(ev gapi.Event) string {
	start, allDay, err := ev.Start.Time()
	when := ev.Start.String()
	switch {
	case err != nil:
	case allDay:
		when = start.Format(time.DateOnly)
	default:
		when = start.Format("2006-01-02_15-04")
	}
	return when + "_" + slug(ev.Summary)
}

// slug keeps letters and digits from any script, which every filesystem
// accepts, and turns runs of anything else into a single hyphen.
func slug(title string) string {
	var b strings.Builder
	pendingDash := false
	n := 0
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(unicode.ToLower(r))
			if n++; n >= 60 {
				break
			}
		} else {
			pendingDash = true
		}
	}
	if b.Len() == 0 {
		return "untitled"
	}
	return b.String()
}

// Doc is one exported transcript Doc.
type Doc struct {
	Title    string
	URL      string
	Markdown string
}

// Render produces the file: YAML front matter describing the event, then the
// export of each Doc as Drive produced it. Strings are double-quoted with Go
// escaping, which is also valid YAML, so no title can break the front matter.
func Render(ev gapi.Event, docs []Doc, tab string, exported time.Time) []byte {
	var b strings.Builder
	q := strconv.Quote
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", q(ev.Summary))
	fmt.Fprintf(&b, "event_id: %s\n", q(ev.ID))
	fmt.Fprintf(&b, "start: %s\n", q(ev.Start.String()))
	fmt.Fprintf(&b, "end: %s\n", q(ev.End.String()))
	if ev.Organizer.Email != "" {
		fmt.Fprintf(&b, "organizer: %s\n", q(person(ev.Organizer)))
	}
	if guests := Guests(ev); guests != nil {
		b.WriteString("attendees:\n")
		for _, p := range guests {
			fmt.Fprintf(&b, "  - %s\n", q(person(p)))
		}
	}
	b.WriteString("transcripts:\n")
	for _, d := range docs {
		fmt.Fprintf(&b, "  - title: %s\n    url: %s\n", q(d.Title), q(d.URL))
	}
	if tab != "" {
		fmt.Fprintf(&b, "tab: %s\n", q(tab))
	}
	fmt.Fprintf(&b, "exported_at: %s\n", q(exported.Format(time.RFC3339)))
	b.WriteString("---\n\n")
	for i, d := range docs {
		// Only a second Doc needs a marker: with one, the front matter
		// already says where the body came from.
		if len(docs) > 1 {
			if i > 0 {
				b.WriteString("\n\n")
			}
			fmt.Fprintf(&b, "<!-- transcript: %s -->\n\n", strings.ReplaceAll(d.Title, "--", "- -"))
		}
		b.WriteString(d.Markdown)
	}
	return []byte(b.String())
}

// Guests are the people invited, leaving out meeting rooms and other
// resources.
func Guests(ev gapi.Event) []gapi.Person {
	var people []gapi.Person
	for _, p := range ev.Attendees {
		if !p.Resource {
			people = append(people, p)
		}
	}
	return people
}

func person(p gapi.Person) string {
	if p.DisplayName == "" {
		return p.Email
	}
	return fmt.Sprintf("%s <%s>", p.DisplayName, p.Email)
}

// Saved maps the event IDs already saved in dir to their file's name, read
// from each markdown file's front matter. Going by the event ID rather than
// the filename means a renamed meeting, or a renamed file, isn't fetched
// twice. A missing dir has nothing saved in it.
func Saved(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	saved := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		if id := eventID(filepath.Join(dir, e.Name())); id != "" {
			saved[id] = e.Name()
		}
	}
	return saved, nil
}

// eventID reads event_id out of a file's front matter, or "" if there is
// none.
func eventID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	if !sc.Scan() || sc.Text() != "---" {
		return ""
	}
	for sc.Scan() && sc.Text() != "---" {
		if v, ok := strings.CutPrefix(sc.Text(), "event_id: "); ok {
			if id, err := strconv.Unquote(v); err == nil {
				return id
			}
			return v
		}
	}
	return ""
}

// FreePath returns dir/stem.md, or dir/stem_2.md and so on if that is taken,
// so two meetings at the same time with the same title don't collide.
func FreePath(dir, stem string) string {
	for n := 1; ; n++ {
		name := stem + ".md"
		if n > 1 {
			name = fmt.Sprintf("%s_%d.md", stem, n)
		}
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			return p
		}
	}
}

// Write puts data at path through a temporary file, so an interrupted run
// never leaves half a transcript behind.
func Write(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
