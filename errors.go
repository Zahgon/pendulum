package pendulum

import "github.com/python-pendulum/pendulum/internal/pyrt"

// Error is a Python exception raised by the original: Type is the exception's class
// name (ValueError, ParserError, InvalidTimezone, ...) and Msg its str(). Error()
// joins them with ": ".
type Error = pyrt.Error

func valueError(format string, args ...any) error { return pyrt.ValueError(format, args...) }

func typeError(msg string) error { return pyrt.NewError("TypeError", msg) }

func newError(class, msg string) error { return pyrt.NewError(class, msg) }
