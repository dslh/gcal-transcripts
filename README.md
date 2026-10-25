# gcal-transcripts

Download the meeting transcripts attached to your Google Calendar events and
save them as markdown files, ready to commit to a repository or hand to an
agent.

Google Meet attaches a transcript, or Gemini's notes with the transcript in a
tab of their own, to the calendar event as a Google Doc. `gcal-transcripts`
finds those events, exports each Doc through Google Drive's own markdown
conversion, and writes it out with front matter describing the meeting.

```
$ gcal-transcripts list --since 2w
WHEN                   TITLE                    STATUS     ID
Tue 14 Oct 2026 10:00  Discovery: Acme hiring   saved      5k2h…
Tue 21 Oct 2026 14:00  Weekly sync              ready      7qfn…_20261021T120000Z
Thu 23 Oct 2026 16:00  Career site review       no access  1d9s…

$ gcal-transcripts pull --all --since 2w --out transcripts
skip  "Discovery: Acme hiring" (Tue 14 Oct 2026 10:00): already saved as 2026-10-14_10-00_discovery-acme-hiring.md
transcripts/2026-10-21_14-00_weekly-sync.md
fail  "Career site review" (Thu 23 Oct 2026 16:00): the transcript isn't shared with you; ask the meeting organiser for access (404: File not found)
saved 1, skipped 1, failed 1
```

It's a single binary with no runtime dependencies, for macOS, Linux and Windows.

## Install

macOS, with [Homebrew](https://brew.sh):

```sh
brew install --cask dslh/tap/gcal-transcripts
```

Windows, with [Scoop](https://scoop.sh):

```sh
scoop bucket add dslh https://github.com/dslh/scoop-bucket
scoop install gcal-transcripts
```

Linux, or anywhere else: download the archive for your platform from the
[releases page](https://github.com/dslh/gcal-transcripts/releases), or
`go install github.com/dslh/gcal-transcripts/cmd/gcal-transcripts@latest`
(a build from source has no OAuth client built in; see below).

## Use

```sh
gcal-transcripts login              # once: approve read-only access in your browser
gcal-transcripts list               # events from the last 7 days with a transcript
gcal-transcripts pull <event-id>    # save one, by the ID list printed
gcal-transcripts pull --all --out transcripts --since 30d
```

Every command has its own help: `gcal-transcripts help pull`, or
`gcal-transcripts pull --help`.

- **Choosing events.** `--since` and `--until` take a duration back from now
  (`36h`, `14d`, `2w`) or a date (`2026-10-01`). `--match` uses Calendar's own
  search over titles, descriptions and guests; `--attendee` keeps events with
  a guest whose name or address contains the text.
- **What counts as a transcript.** A Google Doc attached to the event with
  "Gemini" or "Transcript" in its title: Gemini's notes are titled "Notes by
  Gemini" in the organiser's language ("Notizen von Gemini"), and a plain Meet
  transcript "… - Transcript". That leaves out agendas, and the running
  "Notes - <meeting>" Doc a recurring meeting collects. `--any-doc` counts
  every attached Doc instead.
- **Only the transcript.** Gemini's notes hold a summary and, when
  transcription was on, the transcript in separate tabs. `--tab transcript`
  saves just that tab (it also finds the German "Transkript"); an event whose
  notes have no such tab is reported as failed.
- **Already saved.** `pull` reads the `event_id` in the front matter of the
  markdown files in `--out`, and skips events it finds there, so renaming a
  file is safe. `--force` fetches them again.
- **Scripting.** `pull` prints each saved path on stdout and everything else
  on stderr; `list --json` gives the same rows as JSON. The exit status is 0
  on success, 1 if anything failed, 2 for a usage mistake and 3 when you need
  to sign in again.

## The file

```markdown
---
title: "Weekly sync"
event_id: "7qfn…_20261021T120000Z"
start: "2026-10-21T14:00:00+02:00"
end: "2026-10-21T14:30:00+02:00"
organizer: "anna@example.com"
attendees:
  - "Anna Example <anna@example.com>"
  - "ben@example.com"
transcripts:
  - title: "Notes by Gemini"
    url: "https://docs.google.com/document/d/…/edit"
exported_at: "2026-10-25T21:30:00+01:00"
---

# 📝 Notes
…
```

After the front matter comes the export exactly as Drive produced it, with
one exception: embedded images are replaced with `<image>`. Gemini's notes
carry meeting screenshots inline as base64, which can be over 95% of the file
and would swamp any agent that reads it.

## Access and privacy

`gcal-transcripts` asks for read-only access to your calendar events and your
Drive files, and talks only to Google. The session is kept in your user config
directory (`gcal-transcripts status` shows where), readable by you alone;
`gcal-transcripts logout` revokes it.

A transcript attached to an event isn't necessarily shared with every guest.
`list` checks each one and shows `no access` or `download disabled` where you
can't export it; ask the meeting organiser to share the Doc.

## The OAuth client

Signing in to Google needs an OAuth client of the *Desktop app* type. Release
builds have one built in, injected at build time from repository secrets
(`GCAL_TRANSCRIPTS_CLIENT_ID`, `GCAL_TRANSCRIPTS_CLIENT_SECRET`). Google
doesn't treat a desktop client's secret as confidential, since it can be read
out of any binary; PKCE protects the sign-in itself.

The built-in client's consent screen may be limited to one Google Workspace
organisation. To use your own client instead, or with a build from source:

1. In [Google Cloud](https://console.cloud.google.com), enable the **Google
   Calendar API** and **Google Drive API** for a project.
2. Under **Google Auth Platform → Clients**, create a client of type
   **Desktop app** and download its JSON.
3. `gcal-transcripts login --client-file client.json`, or set
   `GCAL_TRANSCRIPTS_CLIENT` to the file's path.

The session remembers its client, so later commands need neither.

## Development

```sh
go test ./...
go build -o bin/gcal-transcripts ./cmd/gcal-transcripts
bin/gcal-transcripts login --client-file path/to/client.json
```

`GCAL_TRANSCRIPTS_SESSION` keeps the session somewhere other than the default.
Releases are cut by pushing a `v*` tag; GoReleaser publishes the archives, the
Homebrew cask and the Scoop manifest.
