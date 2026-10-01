// Package cliutil holds the small helpers every command shares: running
// the root command, mapping errors to exit codes, and rendering errors.
package cliutil

import (
	"fmt"
	"io"
	"os"
	rtdebug "runtime/debug"

	"github.com/spf13/cobra"

	"kigumi/internal/errors"
)

// UsageError marks a mistake in flags or arguments; it exits with 2.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// ExitError ends the process with a code after the command has already
// reported everything it wanted to (diagnostics, program output).
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

func Exit(code int) error { return &ExitError{Code: code} }

// InternalErrorExit is the exit code for a panic that escaped a command's
// RunE (a compiler bug, not a usage mistake): kept apart from 2 so a
// caller can tell "bad invocation" from "the toolchain itself crashed".
const InternalErrorExit = 70

// Execute runs root and returns the process exit code: 0 on success, 2
// for usage errors, the requested code for ExitError, InternalErrorExit
// for an unrecovered panic, 1 otherwise. Other errors are printed to
// stderr, with their stack trace when debug is on.
func Execute(root *cobra.Command, debug func() bool) (code int) {
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &UsageError{Err: err} })
	markUsageErrors(root)
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "kigumi: internal error: %v\n", r)
			if debug != nil && debug() {
				os.Stderr.Write(rtdebug.Stack())
			}
			code = InternalErrorExit
		}
	}()
	err := root.Execute()
	if err == nil {
		return 0
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		fmt.Fprintf(os.Stderr, "kigumi: %v\n", err)
		return 2
	}
	PrintError(os.Stderr, err, debug != nil && debug())
	return 1
}

// markUsageErrors makes positional-argument validation failures usage
// errors too, on every command of the tree.
func markUsageErrors(cmd *cobra.Command) {
	if validate := cmd.Args; validate != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := validate(c, args); err != nil {
				return &UsageError{Err: err}
			}
			return nil
		}
	}
	for _, sub := range cmd.Commands() {
		markUsageErrors(sub)
	}
}

// PrintError renders an error; verbose adds the stack trace.
func PrintError(w io.Writer, err error, verbose bool) {
	if verbose {
		fmt.Fprintf(w, "kigumi: %+v\n", err)
		return
	}
	fmt.Fprintf(w, "kigumi: %v\n", err)
}
