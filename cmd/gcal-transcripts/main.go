// gcal-transcripts lists Google Calendar events that have a meeting
// transcript attached, and saves those transcripts as markdown files.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"golang.org/x/oauth2"

	"github.com/dslh/gcal-transcripts/internal/auth"
	"github.com/dslh/gcal-transcripts/internal/gapi"
)

const name = "gcal-transcripts"

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		usageTo(os.Stderr)
		return 2
	}
	switch args[0] {
	case "help":
		return printHelp(args[1:])
	case "-h", "-help", "--help":
		usageTo(os.Stdout)
		return 0
	case "-v", "-version", "--version":
		printVersion(os.Stdout)
		return 0
	}

	c := lookup(args[0])
	if c == nil {
		fmt.Fprintf(os.Stderr, "%s: unknown command %q\n\n", name, args[0])
		usageTo(os.Stderr)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return exitCode(c, c.run(ctx, args[1:]))
}

// exitCode reports err and maps it to the exit status: 2 for a usage
// mistake, 3 when signing in again would fix it, 1 for anything else.
func exitCode(c *command, err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, errHelp) {
		commandHelp(os.Stdout, c)
		return 0
	}
	var ue usageError
	if errors.As(err, &ue) {
		fmt.Fprintf(os.Stderr, "%s %s: %s\nRun '%s help %s' for usage.\n", name, c.name, ue, name, c.name)
		return 2
	}
	var silent silentError
	if !errors.As(err, &silent) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
	}
	if needsLogin(err) {
		fmt.Fprintf(os.Stderr, "Run '%s login' to sign in.\n", name)
		return 3
	}
	return 1
}

func needsLogin(err error) bool {
	var apiErr *gapi.APIError
	var scopeErr *auth.MissingScopesError
	var refreshErr *oauth2.RetrieveError
	return errors.Is(err, auth.ErrNotLoggedIn) ||
		errors.As(err, &scopeErr) ||
		errors.As(err, &refreshErr) ||
		errors.As(err, &apiErr) && apiErr.IsAuth()
}

type usageError string

func (u usageError) Error() string { return string(u) }

// silentError is a failure that has already been reported in detail, and
// only needs to set the exit status.
type silentError string

func (s silentError) Error() string { return string(s) }

// newService loads the session and returns a client for the APIs.
func newService(ctx context.Context) (*gapi.Service, error) {
	path, err := auth.SessionPath()
	if err != nil {
		return nil, err
	}
	s, err := auth.LoadSession(path)
	if err != nil {
		return nil, err
	}
	if err := s.Check(); err != nil {
		return nil, err
	}
	return gapi.New(s.HTTPClient(ctx)), nil
}
