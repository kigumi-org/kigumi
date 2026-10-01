// Package run implements `kigumi run` and `kigumi test`: interpret the
// entry file or the test blocks of a module.
package run

import (
	"strings"

	"github.com/spf13/cobra"

	"kigumi/internal/cli/shared"
	"kigumi/internal/cliutil"
	"kigumi/internal/driver"
	"kigumi/internal/errors"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "run [flags] <module-root|entry.kg> [args...]",
		DisableFlagsInUseLine: true,
		Short:                 "Run the entry file in the interpreter",
		Args:                  cobra.MinimumNArgs(1),
		RunE:                  Script,
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().String("engine", "auto", "executor: auto (the VM when it can run the program), vm, or interp (tree walker)")
	return cmd
}

// Script interprets args[0] and passes the whole args slice to the program;
// called directly for a shebang line, where, unlike the `run` subcommand,
// anything after the entry file is script argv and must never be mistaken
// for a swallowed kigumi flag.
func Script(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "run" {
		if err := rejectSwallowedFlags(cmd, args); err != nil {
			return err
		}
		args = dropDashDash(args)
	}
	engine, _ := cmd.Flags().GetString("engine")
	if err := validateEngine(engine); err != nil {
		return err
	}
	m, err := shared.Load(cmd, args[0], false, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	code, err := driver.RunWith(m, driver.RunOptions{NoAccel: shared.NoAccel(), Engine: engine}, cmd.OutOrStdout(), cmd.ErrOrStderr(), args)
	if err != nil {
		return errors.WrapErr(err, "run module")
	}
	if code != 0 {
		return cliutil.Exit(code)
	}
	return nil
}

// rejectSwallowedFlags reports the first token in args[1:] that names a
// flag defined on cmd: with SetInterspersed(false), such a token stops
// being parsed as a flag and is forwarded to the script instead, which
// drops it (and its value, if any) with no warning. A literal "--" ends
// the scan, matching build's documented use of "--" to mark the rest as
// script argv.
func rejectSwallowedFlags(cmd *cobra.Command, args []string) error {
	for _, a := range args[1:] {
		if a == "--" {
			break
		}
		var name string
		switch {
		case strings.HasPrefix(a, "--"):
			name = strings.TrimPrefix(a, "--")
			if i := strings.IndexByte(name, '='); i >= 0 {
				name = name[:i]
			}
		case strings.HasPrefix(a, "-") && a != "-":
			name = a[1:2]
		default:
			continue
		}
		if f := cmd.Flags().Lookup(name); f != nil {
			return &cliutil.UsageError{Err: errors.NewErrf("--%s must come before %s: flags after the entry file are passed to the script, not to kigumi", f.Name, args[0])}
		}
		if len(name) == 1 {
			if f := cmd.Flags().ShorthandLookup(name); f != nil {
				return &cliutil.UsageError{Err: errors.NewErrf("-%s must come before %s: flags after the entry file are passed to the script, not to kigumi", name, args[0])}
			}
		}
	}
	return nil
}

// dropDashDash removes the first literal "--" in args[1:], if any. Cobra
// strips that marker itself when flags stay interspersed, but
// SetInterspersed(false) leaves it in args verbatim (pflag's parseArgs
// returns early on the first positional argument in that mode, before
// reaching its own "--" handling), so it would otherwise leak into the
// script's argv instead of just ending kigumi's own flag scan.
func dropDashDash(args []string) []string {
	for i, a := range args[1:] {
		if a == "--" {
			out := make([]string, 0, len(args)-1)
			out = append(out, args[:i+1]...)
			return append(out, args[i+2:]...)
		}
	}
	return args
}

func TestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <module-root>",
		Short: "Run the test blocks of a module",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, _ := cmd.Flags().GetString("engine")
			if err := validateEngine(engine); err != nil {
				return err
			}
			m, err := shared.Load(cmd, args[0], true, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			code, err := driver.RunTestsWith(m, driver.RunOptions{NoAccel: shared.NoAccel(), Engine: engine}, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err != nil {
				return errors.WrapErr(err, "run tests")
			}
			if code != 0 {
				return cliutil.Exit(code)
			}
			return nil
		},
	}
	cmd.Flags().String("engine", "auto", "executor: auto (the VM when it can run the tests), vm, or interp (tree walker)")
	return cmd
}

// validateEngine rejects any --engine value other than the three the flag
// documents; RunWith/RunTestsWith otherwise treat an unrecognized string
// exactly like "auto" with no indication the flag was misspelled.
func validateEngine(s string) error {
	switch s {
	case "", "auto", "vm", "interp":
		return nil
	default:
		return &cliutil.UsageError{Err: errors.NewErrf("unknown --engine %q: want auto, vm, or interp", s)}
	}
}
