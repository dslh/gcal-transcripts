package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/dslh/gcal-transcripts/internal/auth"
)

type loginOpts struct {
	clientFile string
	noBrowser  bool
}

func (o *loginOpts) flags() *flag.FlagSet {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.StringVar(&o.clientFile, "client-file", "",
		"sign in with the OAuth client in this `file` (a Desktop app client JSON from Google Cloud) "+
			"instead of $"+auth.ClientEnv+" or the one built in")
	fs.BoolVar(&o.noBrowser, "no-browser", false, "print the sign-in URL instead of opening a browser")
	return fs
}

var loginCmd = &command{
	name:     "login",
	summary:  "sign in to Google in your browser",
	synopsis: []string{"[--client-file file] [--no-browser]"},
	about: `
Opens Google's consent screen in your browser and keeps the session it
returns, so the other commands can read your calendar and the transcript Docs
attached to it. Access is read-only: Calendar events, and Drive files.

Release builds have an OAuth client built in. A local build has none, so pass
--client-file, or set $GCAL_TRANSCRIPTS_CLIENT to the file's path. The
session remembers its client, so later commands need neither.

The session is kept under your user config directory; run 'gcal-transcripts
status' to see where, and set $GCAL_TRANSCRIPTS_SESSION to keep it elsewhere.
Signing in again replaces it.`,
	flags: func() *flag.FlagSet { return new(loginOpts).flags() },
	run:   runLogin,
}

func runLogin(ctx context.Context, args []string) error {
	var o loginOpts
	rest, err := parseArgs(o.flags(), args)
	if err != nil {
		return err
	}
	if err := noArgs(rest); err != nil {
		return err
	}
	client, err := auth.ResolveClient(o.clientFile)
	if err != nil {
		return err
	}
	path, err := auth.SessionPath()
	if err != nil {
		return err
	}

	open := openBrowser
	if o.noBrowser {
		open = nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	s, err := auth.Login(ctx, client, open, os.Stderr)
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("gave up waiting for the browser to finish signing in")
	}
	if err != nil {
		return err
	}
	if err := s.Save(path); err != nil {
		return err
	}
	if err := s.Check(); err != nil {
		return fmt.Errorf("signed in, but %w; sign in again and leave every box ticked", err)
	}
	fmt.Fprintf(os.Stderr, "Signed in as %s.\n", orUnknown(s.Account))
	return nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

var statusCmd = &command{
	name:     "status",
	summary:  "show who is signed in and where the session is kept",
	synopsis: []string{""},
	flags:    func() *flag.FlagSet { return flag.NewFlagSet("status", flag.ContinueOnError) },
	run:      runStatus,
}

func runStatus(ctx context.Context, args []string) error {
	rest, err := parseArgs(flag.NewFlagSet("status", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := noArgs(rest); err != nil {
		return err
	}
	path, err := auth.SessionPath()
	if err != nil {
		return err
	}
	fmt.Printf("session: %s\n", path)
	s, err := auth.LoadSession(path)
	if err != nil {
		if errors.Is(err, auth.ErrNotLoggedIn) {
			fmt.Println("signed in: no")
			if c, err := auth.ResolveClient(""); err == nil {
				fmt.Printf("login would use the client: %s\n", c.Source)
			} else {
				fmt.Println("login needs --client-file: this build has no client built in")
			}
		}
		return err
	}
	fmt.Printf("signed in: %s\n", orUnknown(s.Account))
	fmt.Printf("client: %s\n", orUnknown(s.ClientSource))
	if err := s.Check(); err != nil {
		return err
	}
	fmt.Println("scopes: ok")
	return nil
}

var logoutCmd = &command{
	name:     "logout",
	summary:  "revoke the session and forget it",
	synopsis: []string{""},
	about: `
Tells Google to revoke the session's access, then deletes the session file.
If Google can't be reached the file is deleted anyway, and the access can be
removed at https://myaccount.google.com/connections.`,
	flags: func() *flag.FlagSet { return flag.NewFlagSet("logout", flag.ContinueOnError) },
	run:   runLogout,
}

func runLogout(ctx context.Context, args []string) error {
	rest, err := parseArgs(flag.NewFlagSet("logout", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if err := noArgs(rest); err != nil {
		return err
	}
	path, err := auth.SessionPath()
	if err != nil {
		return err
	}
	s, err := auth.LoadSession(path)
	if errors.Is(err, auth.ErrNotLoggedIn) {
		fmt.Fprintln(os.Stderr, "Not signed in.")
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.Revoke(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v (deleting the session anyway)\n", name, err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	fmt.Fprintf(os.Stderr, "Signed out of %s.\n", orUnknown(s.Account))
	return nil
}

func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}
