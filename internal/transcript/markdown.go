// Package transcript turns Calendar events and Drive's markdown export into
// files on disk: which attachments count as transcripts, how the export is
// cleaned and split, and how the file is named and labelled.
package transcript

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/dslh/gcal-transcripts/internal/gapi"
)

const googleDocMime = "application/vnd.google-apps.document"

// Gemini's notes, which carry the transcript in a tab of their own when
// transcription was on, are titled "Notes by Gemini" in the organiser's
// language ("Notizen von Gemini"); a plain Meet transcript is titled
// "... - Transcript". Matching on the title keeps out agendas, and the
// running "Notes - <meeting>" Doc a recurring meeting accumulates.
var transcriptTitle = regexp.MustCompile(`(?i)\b(gemini|transcript|transkript)\b`)

// Find returns the event's attachments that look like a meeting transcript.
// With anyDoc, every Google Doc attached counts.
func Find(ev gapi.Event, anyDoc bool) []gapi.Attachment {
	var found []gapi.Attachment
	for _, a := range ev.Attachments {
		if a.MimeType == googleDocMime && (anyDoc || transcriptTitle.MatchString(a.Title)) {
			found = append(found, a)
		}
	}
	return found
}

// Gemini embeds meeting screenshots in the Doc, and the markdown export
// carries them inline as base64. On one real meeting that was 1.59 MB of a
// 1.65 MB document. Left in, the pictures alone exhaust an agent's context
// window before a word of the discussion is read.
var dataURI = regexp.MustCompile(`data:[^;,\s)>]+;base64,[A-Za-z0-9+/=]+`)

// ImagePlaceholder stands in for each embedded image.
const ImagePlaceholder = "<image>"

// StripImages replaces base64 image payloads with ImagePlaceholder, keeping
// everything else as exported.
func StripImages(markdown string) string {
	return dataURI.ReplaceAllString(markdown, ImagePlaceholder)
}

// Each tab of a multi-tab Doc arrives introduced by a level-1 heading holding
// the tab's name, e.g. `# **📖 Transcript**`.
var tabHeading = regexp.MustCompile(`(?m)^# +(.*)$`)

// Tab is one tab of an exported Doc.
type Tab struct {
	Name     string
	Markdown string
}

// tabName turns `**📖 Transcript**` into `Transcript`.
func tabName(heading string) string {
	name := strings.TrimSpace(strings.Trim(strings.TrimSpace(heading), "*"))
	return strings.TrimSpace(strings.TrimLeftFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}))
}

// SplitTabs splits an export into its tabs, in document order. The headings
// are Gemini's export format rather than a documented contract, so this
// degrades rather than breaks: a document with no level-1 heading comes back
// as a single unnamed tab holding everything.
func SplitTabs(markdown string) []Tab {
	matches := tabHeading.FindAllStringSubmatchIndex(markdown, -1)
	if matches == nil {
		return []Tab{{Markdown: markdown}}
	}
	var tabs []Tab
	// Anything above the first heading belongs to no tab, but dropping it
	// would silently lose content if the format ever grows a preamble.
	if pre := markdown[:matches[0][0]]; strings.TrimSpace(pre) != "" {
		tabs = append(tabs, Tab{Markdown: pre})
	}
	for i, m := range matches {
		end := len(markdown)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		tabs = append(tabs, Tab{Name: tabName(markdown[m[2]:m[3]]), Markdown: markdown[m[0]:end]})
	}
	return tabs
}

// tabAliases are the names Gemini gives a tab in the languages seen so far,
// so --tab transcript works whatever the organiser's language was.
var tabAliases = map[string][]string{
	"transcript": {"transkript"},
}

// SelectTab returns the named tab, matched case-insensitively: an exact
// match first, then a prefix, each also trying the name's aliases.
func SelectTab(markdown, name string) (Tab, bool) {
	tabs := SplitTabs(markdown)
	want := strings.ToLower(strings.TrimSpace(name))
	wants := append([]string{want}, tabAliases[want]...)
	for _, match := range []func(n, w string) bool{
		func(n, w string) bool { return n == w },
		strings.HasPrefix,
	} {
		for _, t := range tabs {
			for _, w := range wants {
				if t.Name != "" && match(strings.ToLower(t.Name), w) {
					return t, true
				}
			}
		}
	}
	return Tab{}, false
}

// TabNames lists the named tabs, for an error that says what was available.
func TabNames(markdown string) []string {
	var names []string
	for _, t := range SplitTabs(markdown) {
		if t.Name != "" {
			names = append(names, t.Name)
		}
	}
	return names
}
