package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
)

// Set at release time by GoReleaser (-ldflags -X).
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var versionCmd = &command{
	name:     "version",
	summary:  "print the version",
	synopsis: []string{""},
	flags:    func() *flag.FlagSet { return flag.NewFlagSet("version", flag.ContinueOnError) },
	run: func(ctx context.Context, args []string) error {
		rest, err := parseArgs(flag.NewFlagSet("version", flag.ContinueOnError), args)
		if err != nil {
			return err
		}
		if err := noArgs(rest); err != nil {
			return err
		}
		printVersion(os.Stdout)
		return nil
	},
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "%s %s (%s, %s)\n", name, Version, Commit, Date)
}
