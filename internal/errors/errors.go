// Package errors is the one place the error backend lives: application
// code imports it instead of the standard library or cockroachdb/errors,
// so the wrapping strategy stays swappable. Stack traces are captured once,
// at the origin, by NewErr and WrapErr.
package errors

import (
	stderrors "errors"

	crdb "github.com/cockroachdb/errors"
)

var (
	New  = stderrors.New
	Is   = stderrors.Is
	As   = stderrors.As
	Join = stderrors.Join
)

// NewErr creates an error carrying the current stack.
func NewErr(msg string) error { return crdb.New(msg) }

// NewErrf formats an error carrying the current stack.
func NewErrf(format string, args ...any) error { return crdb.Newf(format, args...) }

// WrapErr adds context; the stack is recorded only if err has none yet.
func WrapErr(err error, msg string) error {
	if err == nil {
		return nil
	}
	if crdb.GetReportableStackTrace(err) != nil {
		return crdb.WithMessage(err, msg)
	}
	return crdb.Wrap(err, msg)
}

// WrapErrf is WrapErr with a formatted message.
func WrapErrf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if crdb.GetReportableStackTrace(err) != nil {
		return crdb.WithMessagef(err, format, args...)
	}
	return crdb.Wrapf(err, format, args...)
}
