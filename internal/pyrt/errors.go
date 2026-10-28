package pyrt

import "fmt"

// Error is a Python exception. Error() is "<class name>: <str(exception)>".
type Error struct {
	Type string
	Msg  string
	// Cause is the exception the original chained this one from (raise ... from ...).
	Cause error
}

func (e *Error) Error() string { return e.Type + ": " + e.Msg }

func (e *Error) Unwrap() error { return e.Cause }

// Errorf builds a Python exception of the given class.
func Errorf(typ, format string, args ...any) *Error {
	return &Error{Type: typ, Msg: fmt.Sprintf(format, args...)}
}

// NewError builds a Python exception with a literal message.
func NewError(typ, msg string) *Error { return &Error{Type: typ, Msg: msg} }

// parents holds each class's bases as Python 3.12 declares them; calendar's
// IllegalMonthError is both a ValueError and an IndexError.
var parents = map[string][]string{
	"ParserError":            {"ValueError"},
	"IllegalMonthError":      {"ValueError", "IndexError"},
	"IllegalWeekdayError":    {"ValueError"},
	"UnicodeDecodeError":     {"ValueError"},
	"UnicodeEncodeError":     {"ValueError"},
	"ValueError":             {"Exception"},
	"TypeError":              {"Exception"},
	"OverflowError":          {"ArithmeticError"},
	"ZeroDivisionError":      {"ArithmeticError"},
	"InvalidOperation":       {"ArithmeticError"},
	"ArithmeticError":        {"Exception"},
	"IndexError":             {"LookupError"},
	"KeyError":               {"LookupError"},
	"LookupError":            {"Exception"},
	"AttributeError":         {"Exception"},
	"NotImplementedError":    {"RuntimeError"},
	"RuntimeError":           {"Exception"},
	"StopIteration":          {"Exception"},
	"UnknownTimezoneWarning": {"RuntimeWarning"},
}

func subclass(t, typ string) bool {
	if t == typ {
		return true
	}
	for _, p := range parents[t] {
		if subclass(p, typ) {
			return true
		}
	}
	return false
}

// IsInstance reports whether err is a Python exception of class typ or a subclass.
func IsInstance(err error, typ string) bool {
	e, ok := err.(*Error)
	if !ok {
		return false
	}
	return subclass(e.Type, typ) || typ == "Exception" || typ == "BaseException"
}

// ValueError and the other helpers name the common built-in classes.
func ValueError(format string, args ...any) *Error { return Errorf("ValueError", format, args...) }

func TypeError(format string, args ...any) *Error { return Errorf("TypeError", format, args...) }

func OverflowError(format string, args ...any) *Error {
	return Errorf("OverflowError", format, args...)
}

func IndexError(msg string) *Error { return NewError("IndexError", msg) }

// Raise panics with err when it is not nil. Code that mirrors Python control flow
// raises this way and an API boundary turns the panic back into an error with Recover.
func Raise(err error) {
	if err != nil {
		panic(err)
	}
}

// Recover stores a raised *Error in *err. Any other panic is a bug and continues.
func Recover(err *error) {
	if r := recover(); r != nil {
		e, ok := r.(*Error)
		if !ok {
			panic(r)
		}
		*err = e
	}
}
