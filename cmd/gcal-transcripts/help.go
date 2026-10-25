package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// command is a subcommand. Its flags are built by flags, so that help can
// list them without running the command.
type command struct {
	name     string
	summary  string
	synopsis []string
	about    string
	flags    func() *flag.FlagSet
	run      func(ctx context.Context, args []string) error
}

var commands = []*command{loginCmd, statusCmd, logoutCmd, listCmd, pullCmd, versionCmd}

func lookup(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
	}
	return nil
}

func usageTo(w io.Writer) {
	fmt.Fprintf(w, "%s saves Google Calendar meeting transcripts as markdown.\n\n", name)
	fmt.Fprintf(w, "Usage:\n  %s <command> [flags]\n\nCommands:\n", name)
	for _, c := range commands {
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.summary)
	}
	fmt.Fprintf(w, "\nRun '%s help <command>' or '%s <command> --help' for details.\n", name, name)
}

// printHelp handles `help [command]`. Help asked for goes to stdout.
func printHelp(args []string) int {
	if len(args) == 0 {
		usageTo(os.Stdout)
		return 0
	}
	c := lookup(args[0])
	if c == nil {
		fmt.Fprintf(os.Stderr, "%s: no help for unknown command %q\n\n", name, args[0])
		usageTo(os.Stderr)
		return 2
	}
	commandHelp(os.Stdout, c)
	return 0
}

func commandHelp(w io.Writer, c *command) {
	fmt.Fprintf(w, "%s %s: %s\n\nUsage:\n", name, c.name, c.summary)
	for _, s := range c.synopsis {
		fmt.Fprintf(w, "  %s %s %s\n", name, c.name, s)
	}
	if c.about != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(c.about))
	}
	fs := c.flags()
	var lines []string
	fs.VisitAll(func(f *flag.Flag) {
		arg, usage := flag.UnquoteUsage(f)
		head := "  --" + f.Name
		if arg != "" {
			head += " " + arg
		}
		if f.DefValue != "" && f.DefValue != "false" {
			usage += fmt.Sprintf(" (default %q)", f.DefValue)
		}
		lines = append(lines, head+"\n      "+usage)
	})
	lines = append(lines, "  -h, --help\n      show this help")
	fmt.Fprintf(w, "\nFlags:\n%s\n", strings.Join(lines, "\n"))
}

// errHelp means --help was among a command's flags.
var errHelp = errors.New("help requested")

// parseArgs parses fs's flags from anywhere in args, not only before the
// first positional argument as the flag package does on its own, and returns
// the positional arguments.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, errHelp
			}
			return nil, usageError(err.Error())
		}
		rest := fs.Args()
		// After a bare --, everything is positional.
		if consumed := len(args) - len(rest); consumed > 0 && args[consumed-1] == "--" {
			return append(positional, rest...), nil
		}
		if len(rest) == 0 {
			return positional, nil
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
}

func noArgs(args []string) error {
	if len(args) > 0 {
		return usageError(fmt.Sprintf("unexpected argument %q", args[0]))
	}
	return nil
}
