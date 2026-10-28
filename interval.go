package pendulum

import (
	"fmt"

	"github.com/python-pendulum/pendulum/internal/pydt"
)

// Interval is pendulum.Interval: the span between two DateTimes or two Dates. It is a
// Duration whose calendar units come from a precise difference of its ends.
type Interval struct {
	dur      Duration
	start    any // DateTime or Date
	end      any
	absolute bool
	invert   bool
	delta    preciseDiff
}

// NewInterval is pendulum.interval(start, end, absolute): start and end are both
// DateTime or both Date.
func NewInterval(start, end any, absolute bool) (Interval, error) {
	return newInterval(start, end, absolute)
}

func asPoint(v any, withFold bool) (point, bool) {
	switch x := v.(type) {
	case DateTime:
		d := x.d
		if !withFold {
			d = d.WithFold(0)
		}
		return point{dt: d}, true
	case Date:
		return point{isDate: true, date: x.d}, true
	}
	return point{}, false
}

func newInterval(start, end any, absolute bool) (Interval, error) {
	_, sdt := start.(DateTime)
	_, edt := end.(DateTime)
	if sdt != edt {
		return Interval{}, valueError("Both start and end of an Interval must have the same type")
	}
	if sdt {
		s, e := start.(DateTime), end.(DateTime)
		if (s.d.Tzinfo() == nil) != (e.d.Tzinfo() == nil) {
			return Interval{}, typeError("can't compare offset-naive and offset-aware datetimes")
		}
	}
	// __new__: the timedelta value.
	s0, _ := asPoint(start, true)
	e0, _ := asPoint(end, true)
	if absolute {
		c, err := s0.compare(e0)
		if err != nil {
			return Interval{}, err
		}
		if c > 0 {
			s0, e0 = e0, s0
		}
	}
	var delta pydt.Timedelta
	if s0.isDate {
		delta = s0.date.Diff(e0.date)
		delta, _ = delta.Neg()
	} else {
		a, b := s0.dt, e0.dt
		if a.Tzinfo() != nil && a.Tzinfo() == b.Tzinfo() {
			var err error
			if off, ok := a.UtcOffset(); ok {
				if a, err = a.SubDelta(off); err != nil {
					return Interval{}, err
				}
				a = a.Naive()
			}
			if off, ok := b.UtcOffset(); ok {
				if b, err = b.SubDelta(off); err != nil {
					return Interval{}, err
				}
				b = b.Naive()
			}
		}
		var err error
		if delta, err = b.Diff(a); err != nil {
			return Interval{}, err
		}
	}
	dur, err := newDuration(durationArgs{seconds: nf(delta.TotalSecondsExact())})
	if err != nil {
		return Interval{}, err
	}
	// __init__: the ends and the precise difference.
	s1, _ := asPoint(start, false)
	e1, _ := asPoint(end, false)
	iv := Interval{dur: dur, start: start, end: end, absolute: absolute}
	c, err := pointOf(start).compare(pointOf(end))
	if err != nil {
		return Interval{}, err
	}
	if c > 0 {
		iv.invert = true
		if absolute {
			iv.start, iv.end = end, start
			s1, e1 = e1, s1
		}
	}
	inv := iv.invert
	iv.dur.invertSet = &inv
	if iv.delta, err = computePreciseDiff(s1, e1); err != nil {
		return Interval{}, err
	}
	return iv, nil
}

// pointOf is the pendulum value itself, compared with its fold.
func pointOf(v any) point { p, _ := asPoint(v, true); return p }

// Start is iv.start and End iv.end: a DateTime or a Date.
func (iv Interval) Start() any { return iv.start }
func (iv Interval) End() any   { return iv.end }

// Absolute reports whether the interval was built with absolute=True.
func (iv Interval) Absolute() bool { return iv.absolute }

// Years and Months are the precise difference's years and months.
func (iv Interval) Years() int  { return iv.delta.years }
func (iv Interval) Months() int { return iv.delta.months }

// Weeks is iv.weeks: whole weeks of the precise difference's days.
func (iv Interval) Weeks() int { return iabs(iv.delta.days) / 7 * isign(iv.delta.days) }

// Days is iv.days: the whole days of the total span.
func (iv Interval) Days() int { return iv.dur.days }

// RemainingDays is iv.remaining_days.
func (iv Interval) RemainingDays() int { return iabs(iv.delta.days) % 7 * isign(iv.dur.days) }

// Hours and Minutes are the precise difference's hours and minutes.
func (iv Interval) Hours() int   { return iv.delta.hours }
func (iv Interval) Minutes() int { return iv.delta.minutes }

// Seconds, RemainingSeconds and Microseconds come from the total span, as the
// original's Duration properties do.
func (iv Interval) Seconds() int          { return iv.dur.seconds }
func (iv Interval) RemainingSeconds() int { return iv.dur.RemainingSeconds() }
func (iv Interval) Microseconds() int     { return iv.dur.micro }

// Invert is iv.invert: the start was after the end.
func (iv Interval) Invert() bool { return iv.invert }

// TotalSeconds and the other totals are those of the span.
func (iv Interval) TotalSeconds() float64 { return iv.dur.TotalSeconds() }
func (iv Interval) TotalMinutes() float64 { return iv.dur.TotalMinutes() }
func (iv Interval) TotalHours() float64   { return iv.dur.TotalHours() }
func (iv Interval) TotalDays() float64    { return iv.dur.TotalDays() }
func (iv Interval) TotalWeeks() float64   { return iv.dur.TotalWeeks() }

// InYears is iv.in_years() and InMonths iv.in_months().
func (iv Interval) InYears() int  { return iv.Years() }
func (iv Interval) InMonths() int { return iv.Years()*12 + iv.Months() }

// InWeeks is iv.in_weeks(): whole weeks of InDays.
func (iv Interval) InWeeks() int {
	d := iv.InDays()
	return isign(d) * (iabs(d) / 7)
}

// InDays is iv.in_days(): the difference in calendar days.
func (iv Interval) InDays() int { return iv.delta.totalDays }

// InHours, InMinutes and InSeconds truncate the span's totals.
func (iv Interval) InHours() int   { return iv.dur.InHours() }
func (iv Interval) InMinutes() int { return iv.dur.InMinutes() }
func (iv Interval) InSeconds() int { return iv.dur.InSeconds() }

// InWords is iv.in_words(locale, separator).
func (iv Interval) InWords(locale *string, separator string) (string, error) {
	return inWords(locale, separator, [7]int{iv.Years(), iv.Months(), iv.Weeks(), iv.RemainingDays(),
		iv.Hours(), iv.Minutes(), iv.RemainingSeconds()}, iv.Microseconds(), 1)
}

// AsDuration is iv.as_duration().
func (iv Interval) AsDuration() (Duration, error) {
	return newDuration(durationArgs{seconds: nf(iv.TotalSeconds())})
}

// Range is list(iv.range(unit, amount)): the start, then start.add(unit=amount*k) while
// within the end (stepping back for an inverted, non-absolute interval). amount must be
// positive: the original never stops otherwise.
func (iv Interval) Range(unit string, amount int) ([]any, error) {
	if amount <= 0 {
		return nil, valueError("amount must be positive")
	}
	back := !iv.absolute && iv.invert
	var out []any
	cur := iv.start
	for i := amount; ; i += amount {
		c, err := pointOf(cur).compare(pointOf(iv.end))
		if err != nil {
			return nil, err
		}
		if (!back && c > 0) || (back && c < 0) {
			return out, nil
		}
		out = append(out, cur)
		n := i
		if back {
			n = -i
		}
		if cur, err = addUnit(iv.start, unit, n); err != nil {
			return nil, err
		}
	}
}

func addUnit(v any, unit string, n int) (any, error) {
	switch x := v.(type) {
	case DateTime:
		var a AddArgs
		switch unit {
		case "years":
			a.Years = n
		case "months":
			a.Months = n
		case "weeks":
			a.Weeks = n
		case "days":
			a.Days = n
		case "hours":
			a.Hours = n
		case "minutes":
			a.Minutes = n
		case "seconds":
			a.Seconds = float64(n)
		case "microseconds":
			a.Microseconds = n
		default:
			return nil, typeError(fmt.Sprintf("DateTime.add() got an unexpected keyword argument '%s'", unit))
		}
		return x.Add(a)
	case Date:
		switch unit {
		case "years":
			return x.Add(n, 0, 0, 0)
		case "months":
			return x.Add(0, n, 0, 0)
		case "weeks":
			return x.Add(0, 0, n, 0)
		case "days":
			return x.Add(0, 0, 0, n)
		}
		return nil, typeError(fmt.Sprintf("Date.add() got an unexpected keyword argument '%s'", unit))
	}
	return nil, typeError("bad interval end")
}

// Contains is item in iv: start <= item <= end.
func (iv Interval) Contains(item any) (bool, error) {
	p, ok := asPoint(item, true)
	if !ok {
		return false, typeError("unsupported item")
	}
	c, err := pointOf(iv.start).compare(p)
	if err != nil || c > 0 {
		return false, err
	}
	c, err = p.compare(pointOf(iv.end))
	return c <= 0, err
}

// Add is iv + d and Sub iv - d, as Durations.
func (iv Interval) Add(d Duration) (Duration, error) {
	a, err := iv.AsDuration()
	if err != nil {
		return Duration{}, err
	}
	return a.Add(d)
}

func (iv Interval) Sub(d Duration) (Duration, error) {
	a, err := iv.AsDuration()
	if err != nil {
		return Duration{}, err
	}
	return a.Sub(d)
}

// MulInt, MulFloat, FloorDivInt, TrueDivInt and TrueDivFloat act on the Duration.
func (iv Interval) MulInt(n int) (Duration, error) {
	a, err := iv.AsDuration()
	if err != nil {
		return Duration{}, err
	}
	return a.MulInt(n)
}

func (iv Interval) MulFloat(f float64) (Duration, error) {
	a, err := iv.AsDuration()
	if err != nil {
		return Duration{}, err
	}
	return a.MulFloat(f)
}

func (iv Interval) FloorDivInt(n int) (Duration, error) {
	a, err := iv.AsDuration()
	if err != nil {
		return Duration{}, err
	}
	return a.FloorDivInt(n)
}

func (iv Interval) TrueDivInt(n int) (Duration, error) {
	a, err := iv.AsDuration()
	if err != nil {
		return Duration{}, err
	}
	return a.TrueDivInt(n)
}

func (iv Interval) TrueDivFloat(f float64) (Duration, error) {
	a, err := iv.AsDuration()
	if err != nil {
		return Duration{}, err
	}
	return a.TrueDivFloat(f)
}

// Neg is -iv: the interval from end to start.
func (iv Interval) Neg() (Interval, error) { return newInterval(iv.end, iv.start, iv.absolute) }

// Abs is abs(iv): the same ends, absolute.
func (iv Interval) Abs() (Interval, error) { return newInterval(iv.start, iv.end, true) }

// Equal is iv == other: same start, end and absoluteness.
func (iv Interval) Equal(other Interval) bool {
	return pointOf(iv.start).equal(pointOf(other.start)) && pointOf(iv.end).equal(pointOf(other.end)) &&
		iv.absolute == other.absolute
}

// Repr is repr(iv) (also str(iv)): <Interval [start -> end]>.
func (iv Interval) Repr() string {
	return fmt.Sprintf("<Interval [%s -> %s]>", strOf(iv.start), strOf(iv.end))
}

// String is str(iv).
func (iv Interval) String() string { return iv.Repr() }

func strOf(v any) string {
	switch x := v.(type) {
	case DateTime:
		return x.String()
	case Date:
		return x.String()
	}
	return ""
}

// Duration returns the interval's Duration view (its timedelta value and the
// properties a Duration has).
func (iv Interval) Duration() Duration { return iv.dur }
