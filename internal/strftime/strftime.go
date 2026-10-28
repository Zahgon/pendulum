// Package strftime reproduces, byte for byte, the strftime methods of the
// datetime, date and time types of CPython 3.12.15 running on glibc 2.36 with
// LC_TIME left at the C locale.
//
// The work happens in the same three layers as in CPython:
//
//   - wrap_strftime (Modules/_datetimemodule.c) replaces %z, %:z, %Z and %f
//     with the object's utcoffset(), tzname() and microsecond, and passes every
//     other directive, including flagged forms such as %-z or %#Z, through;
//   - time.strftime (Modules/timemodule.c) validates the time tuple, clamps
//     tm_isdst, splits the format into runs of ASCII characters, which it hands
//     to wcsftime, and runs that start at a NUL or non-ASCII character and end
//     before the next '%', which it copies unchanged, and turns a run whose
//     output does not fit in a buffer of 256 times its length into "";
//   - the C library's strftime expands each ASCII run in the C locale, with the
//     flags _ - 0 ^ #, field widths, the E and O modifiers, and its rules for
//     unknown conversions; glibc.go reproduces glibc 2.36's observable results,
//     as recorded from the reference interpreter, in its own code.
package strftime

import (
	"fmt"
	"math"
	"strings"
)

// Tm is what CPython hands to strftime for one object.
type Tm struct {
	// Year .. Second are the timetuple() fields; Microsecond feeds %f.
	Year, Month, Day, Hour, Minute, Second, Microsecond int

	Weekday int // Python weekday(): 0 = Monday .. 6 = Sunday
	Yday    int // 1..366
	IsDST   int // timetuple().tm_isdst: -1, 0 or 1 (it comes from dst(): None -> -1)
	// UTCOffset is utcoffset() in microseconds, nil when it is None (naive objects).
	UTCOffset *int64
	// TZName is tzname(), nil when it is None.
	TZName *string
}

// Env is the process's C-library local time, which glibc consults for %Z/%z it is
// asked to expand itself and for %s.
type Env struct {
	// TZName is time.tzname: the C library's tzname[], which glibc prints for a
	// %Z that CPython does not intercept (%#Z, %^Z, %10Z, %EZ ...). glibc
	// rewrites tzname[] on every localtime()/mktime() call, so after one for
	// another era of the zone it can differ from the time.tzname that Python
	// computed at startup; the caller supplies the value current at the call.
	TZName [2]string
	// Mktime is glibc mktime() of the fields (Year..Second, IsDST as tm_isdst) in the
	// process zone; ok is false where mktime returns -1 as an error.
	Mktime func(t Tm) (secs int64, ok bool)
}

// Error is a Python exception: Class is the exception class name, Message its str().
type Error struct{ Class, Message string }

// Error returns the exception as Python's traceback prints its last line.
func (e *Error) Error() string { return e.Class + ": " + e.Message }

// maxOffset bounds utcoffset(): CPython accepts only offsets strictly between
// -24h and +24h, in microseconds.
const maxOffset = 24 * 3600 * 1_000_000

// Format is obj.strftime(format) for a datetime, date or time whose timetuple and
// tzinfo answers are t (the caller builds t: a date has zero time fields and IsDST -1;
// a time object uses 1900-01-01, Weekday 0, Yday 1, IsDST -1). A failure returns *Error.
//
// A %s that glibc expands calls env.Mktime once; a nil Mktime counts as a failed
// mktime, which glibc prints as -1. A %Z or flagged %z that glibc expands reads
// env.TZName, which must hold the process's tzname[] as glibc sees it at the call.
func Format(format string, t Tm, env Env) (string, error) {
	newFormat, err := wrapStrftime(format, &t)
	if err != nil {
		return "", err
	}
	tm, err := timeTuple(&t)
	if err != nil {
		return "", err
	}
	// glibc's %s hands mktime the struct tm after checktm and the tm_isdst clamp.
	mt := t
	mt.Month, mt.Day, mt.IsDST = int(tm.mon)+1, int(tm.mday), int(tm.isdst)
	g := &glibc{tm: tm, t: mt, env: &env}
	return g.timeStrftime(newFormat), nil
}

// wrapStrftime is the preprocessing step of CPython's wrap_strftime: it
// replaces %z, %:z, %Z and %f and leaves every other byte alone. Working on
// UTF-8 bytes rather than code points is safe because '%' and the letters it
// looks for never occur inside a multi-byte sequence.
func wrapStrftime(format string, t *Tm) (string, *Error) {
	var b strings.Builder
	start, i, n := 0, 0, len(format)
	for i < n {
		j := strings.IndexByte(format[i:], '%')
		if j < 0 {
			break
		}
		end := i + j
		i = end + 1
		if i == n {
			break
		}
		ch := format[i]
		i++
		var repl string
		switch {
		case ch == 'z':
			s, err := formatUTCOffset(t.UTCOffset, "")
			if err != nil {
				return "", err
			}
			repl = s
		case ch == ':' && i < n && format[i] == 'z':
			i++
			s, err := formatUTCOffset(t.UTCOffset, ":")
			if err != nil {
				return "", err
			}
			repl = s
		case ch == 'Z':
			if t.TZName != nil {
				// The name is pasted into the format, so its '%' are doubled.
				repl = strings.ReplaceAll(*t.TZName, "%", "%%")
			}
		case ch == 'f':
			repl = fmt.Sprintf("%06d", t.Microsecond)
		default:
			continue
		}
		b.WriteString(format[start:end])
		b.WriteString(repl)
		start = i
	}
	if start == 0 {
		return format, nil
	}
	b.WriteString(format[start:])
	return b.String(), nil
}

// formatUTCOffset is CPython's format_utcoffset: sign, hours and minutes, then
// seconds and microseconds only when they are not zero.
func formatUTCOffset(off *int64, sep string) (string, *Error) {
	if off == nil {
		return "", nil
	}
	us := *off
	if us <= -maxOffset || us >= maxOffset {
		return "", &Error{"ValueError", "offset must be a timedelta strictly between " +
			"-timedelta(hours=24) and timedelta(hours=24)."}
	}
	sign := byte('+')
	if us < 0 {
		sign = '-'
		us = -us
	}
	micro, secs := us%1_000_000, us/1_000_000
	hours, minutes, seconds := secs/3600, secs%3600/60, secs%60
	switch {
	case micro != 0:
		return fmt.Sprintf("%c%02d%s%02d%s%02d.%06d", sign, hours, sep, minutes, sep, seconds, micro), nil
	case seconds != 0:
		return fmt.Sprintf("%c%02d%s%02d%s%02d", sign, hours, sep, minutes, sep, seconds), nil
	}
	return fmt.Sprintf("%c%02d%s%02d", sign, hours, sep, minutes), nil
}

// cTm is the struct tm that time.strftime passes to wcsftime. tm_zone is
// NULL and tm_gmtoff is 0 because timetuple() builds a 9-field struct_time.
type cTm struct {
	year, mon, mday, hour, min, sec, wday, yday, isdst int32
}

// timeTuple is time.strftime's gettmarg and checktm followed by its tm_isdst
// clamping, with the same exceptions in the same order.
func timeTuple(t *Tm) (*cTm, *Error) {
	for _, v := range [...]int{t.Year, t.Month, t.Day, t.Hour, t.Minute, t.Second, t.Weekday, t.Yday, t.IsDST} {
		if v > math.MaxInt32 {
			return nil, &Error{"OverflowError", "signed integer is greater than maximum"}
		}
		if v < math.MinInt32 {
			return nil, &Error{"OverflowError", "signed integer is less than minimum"}
		}
	}
	if t.Year < math.MinInt32+1900 {
		return nil, &Error{"OverflowError", "year out of range"}
	}
	// C int arithmetic: the conversions below may wrap exactly as they do there.
	tm := &cTm{
		year:  int32(t.Year) - 1900,
		mon:   int32(t.Month) - 1,
		mday:  int32(t.Day),
		hour:  int32(t.Hour),
		min:   int32(t.Minute),
		sec:   int32(t.Second),
		wday:  (int32(t.Weekday) + 1) % 7,
		yday:  int32(t.Yday) - 1,
		isdst: int32(t.IsDST),
	}
	valueError := func(msg string) (*cTm, *Error) { return nil, &Error{"ValueError", msg} }
	switch {
	case tm.mon == -1:
		tm.mon = 0
	case tm.mon < 0 || tm.mon > 11:
		return valueError("month out of range")
	}
	switch {
	case tm.mday == 0:
		tm.mday = 1
	case tm.mday < 0 || tm.mday > 31:
		return valueError("day of month out of range")
	}
	if tm.hour < 0 || tm.hour > 23 {
		return valueError("hour out of range")
	}
	if tm.min < 0 || tm.min > 59 {
		return valueError("minute out of range")
	}
	if tm.sec < 0 || tm.sec > 61 {
		return valueError("seconds out of range")
	}
	if tm.wday < 0 {
		return valueError("day of week out of range")
	}
	switch {
	case tm.yday == -1:
		tm.yday = 0
	case tm.yday < 0 || tm.yday > 365:
		return valueError("day of year out of range")
	}
	tm.isdst = max(-1, min(1, tm.isdst))
	return tm, nil
}

// timeStrftime is the formatting loop of time.strftime: ASCII runs go through
// glibc with a buffer that starts at 1024 characters and doubles while the
// result is empty and the buffer is shorter than 256 times the run; the
// buffer size carries over from one run to the next. Runs that start at a NUL
// or non-ASCII character are copied up to the next '%'.
func (g *glibc) timeStrftime(format string) string {
	var out strings.Builder
	bufsize := int64(1024)
	i := 0
	for i < len(format) {
		start := i
		for i < len(format) && format[i] != 0 && format[i] < 0x80 {
			i++
		}
		if chunk := format[start:i]; chunk != "" {
			limit := 256 * int64(len(chunk))
			largest := bufsize
			for largest < limit {
				largest += largest
			}
			s, n, ok := g.run(chunk, largest)
			if ok && n > 0 {
				for n >= bufsize {
					bufsize += bufsize
				}
				out.WriteString(s)
			} else {
				bufsize = largest
			}
		}
		start = i
		if j := strings.IndexByte(format[i:], '%'); j >= 0 {
			i += j
		} else {
			i = len(format)
		}
		out.WriteString(format[start:i])
	}
	return out.String()
}
