package pendulum

import (
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/python-pendulum/pendulum/internal/pydt"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// Duration is pendulum.Duration (a timedelta that also counts years and months), or
// the AbsoluteDuration that Time.diff(abs=True) returns.
type Duration struct {
	td        pydt.Timedelta // the timedelta value the standard library compares and counts
	total     float64        // _total
	years     int
	months    int
	weeks     int
	days      int // _days
	remDays   int
	seconds   int
	micro     int
	absolute  bool
	sig       addArgs // _signature
	hasSig    bool
	interval  bool
	invertSet *bool // Interval's own invert flag
}

// DurationArgs are the keyword arguments of pendulum.duration(). A float64 whose value
// is whole stands for a Python int, any other for a float (and a float year or month
// count is the ValueError the original raises).
type DurationArgs struct {
	Days, Seconds, Microseconds, Milliseconds, Minutes, Hours, Weeks float64
	Years, Months                                                    float64
}

type durationArgs struct {
	days, seconds, microseconds, milliseconds, minutes, hours, weeks num
	years, months                                                    num
}

// pyNum maps a float64 argument to the Python number it stands for.
func pyNum(v float64) num {
	if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
		return ni(int(v))
	}
	return nf(v)
}

// NewDuration is pendulum.duration(...).
func NewDuration(a DurationArgs) (Duration, error) {
	return newDuration(durationArgs{pyNum(a.Days), pyNum(a.Seconds), pyNum(a.Microseconds),
		pyNum(a.Milliseconds), pyNum(a.Minutes), pyNum(a.Hours), pyNum(a.Weeks), pyNum(a.Years), pyNum(a.Months)})
}

var errFloatYears = pyrt.ValueError("Float year and months are not supported")

func newDuration(a durationArgs) (Duration, error) {
	if a.years.isFloat || a.months.isFloat {
		return Duration{}, errFloatYears
	}
	extra := a.years.mul(ni(365)).add(a.months.mul(ni(30)))
	days := a.days.add(a.years.mul(ni(365))).add(a.months.mul(ni(30)))
	td, err := pydt.NewDelta(days.arg(), a.seconds.arg(), a.microseconds.arg(),
		a.milliseconds.arg(), a.minutes.arg(), a.hours.arg(), a.weeks.arg())
	if err != nil {
		return Duration{}, err
	}
	total := td.TotalSecondsExact() - float64(extra.i*86400)
	d := Duration{td: td, total: total, years: int(a.years.i), months: int(a.months.i)}
	m := 1.0
	if total < 0 {
		m = -1
	}
	_, mod := pyrt.FloatDivmod(total, m)
	d.micro = int(math.RoundToEven(mod * 1e6))
	whole := int(math.Abs(math.Trunc(total)))
	mi := int(m)
	d.seconds = whole % 86400 * mi
	d.days = whole / 86400 * mi
	ad := d.days
	if ad < 0 {
		ad = -ad
	}
	d.remDays = ad % 7 * mi
	d.weeks = ad / 7 * mi
	d.sig = addArgs{years: a.years, months: a.months, weeks: a.weeks, days: a.days, hours: a.hours,
		minutes: a.minutes, seconds: a.seconds, microseconds: a.microseconds.add(a.milliseconds.mul(ni(1000)))}
	d.hasSig = true
	return d, nil
}

func newAbsoluteDuration(a durationArgs) (Duration, error) {
	if a.years.isFloat || a.months.isFloat {
		return Duration{}, errFloatYears
	}
	td, err := pydt.NewDelta(a.days.arg(), a.seconds.arg(), a.microseconds.arg(),
		a.milliseconds.arg(), a.minutes.arg(), a.hours.arg(), a.weeks.arg())
	if err != nil {
		return Duration{}, err
	}
	d := Duration{td: td, total: td.TotalSecondsExact(), absolute: true}
	total := math.Abs(d.total)
	_, mod := pyrt.FloatDivmod(total, 1)
	d.micro = int(math.RoundToEven(mod * 1e6))
	whole := int(math.Trunc(total))
	days := whole / 86400
	d.seconds = whole % 86400
	d.days = iabs(days + int(a.years.i)*365 + int(a.months.i)*30)
	d.weeks, d.remDays = days/7, days%7
	d.months, d.years = iabs(int(a.months.i)), iabs(int(a.years.i))
	return d, nil
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func isign(v int) int {
	if v < 0 {
		return -1
	}
	return 1
}

// className is the Python class of the value.
func (d Duration) className() string {
	if d.absolute {
		return "AbsoluteDuration"
	}
	return "Duration"
}

// Years and the other getters are the Duration's properties.
func (d Duration) Years() int         { return d.years }
func (d Duration) Months() int        { return d.months }
func (d Duration) Weeks() int         { return d.weeks }
func (d Duration) RemainingDays() int { return d.remDays }
func (d Duration) Seconds() int       { return d.seconds }
func (d Duration) Microseconds() int  { return d.micro }

// Days is d.days: the timedelta's days, which count years as 365 days and months as 30.
func (d Duration) Days() int { return d.td.Days() }

// Hours is d.hours: the hours within the day.
func (d Duration) Hours() int {
	if iabs(d.seconds) >= 3600 {
		return iabs(d.seconds) / 3600 % 24 * isign(d.seconds)
	}
	return 0
}

// Minutes is d.minutes: the minutes within the hour.
func (d Duration) Minutes() int {
	if iabs(d.seconds) >= 60 {
		return iabs(d.seconds) / 60 % 60 * isign(d.seconds)
	}
	return 0
}

// RemainingSeconds is d.remaining_seconds: the seconds within the minute.
func (d Duration) RemainingSeconds() int { return iabs(d.seconds) % 60 * isign(d.seconds) }

// Invert is d.invert: the duration is negative.
func (d Duration) Invert() bool {
	if d.invertSet != nil {
		return *d.invertSet
	}
	if d.absolute {
		return d.total < 0
	}
	return d.TotalSeconds() < 0
}

// TotalSeconds is d.total_seconds().
func (d Duration) TotalSeconds() float64 {
	if d.absolute {
		return math.Abs(d.total)
	}
	return d.td.TotalSecondsExact()
}

// TotalMinutes, TotalHours, TotalDays and TotalWeeks divide TotalSeconds.
func (d Duration) TotalMinutes() float64 { return d.TotalSeconds() / 60 }
func (d Duration) TotalHours() float64   { return d.TotalSeconds() / 3600 }
func (d Duration) TotalDays() float64    { return d.TotalSeconds() / 86400 }
func (d Duration) TotalWeeks() float64   { return d.TotalDays() / 7 }

// InWeeks and the other In* methods truncate the totals to ints.
func (d Duration) InWeeks() int   { return int(d.TotalWeeks()) }
func (d Duration) InDays() int    { return int(d.TotalDays()) }
func (d Duration) InHours() int   { return int(d.TotalHours()) }
func (d Duration) InMinutes() int { return int(d.TotalMinutes()) }
func (d Duration) InSeconds() int { return int(d.TotalSeconds()) }

// AsTimedelta is d.as_timedelta() as days, seconds and microseconds.
func (d Duration) AsTimedelta() (days, seconds, microseconds int, err error) {
	td, err := pydt.NewDelta(pydt.I(0), pydt.F(d.TotalSeconds()), pydt.I(0), pydt.I(0), pydt.I(0), pydt.I(0), pydt.I(0))
	if err != nil {
		return 0, 0, 0, err
	}
	return td.Days(), td.Seconds(), td.Microseconds(), nil
}

// TimedeltaParts are the stdlib timedelta's own days, seconds and microseconds.
func (d Duration) TimedeltaParts() (days, seconds, microseconds int) {
	return d.td.Days(), d.td.Seconds(), d.td.Microseconds()
}

// InWords is d.in_words(locale, separator); nil locale is the current locale.
func (d Duration) InWords(locale *string, separator string) (string, error) {
	return inWords(locale, separator, [7]int{d.years, d.months, d.weeks, d.remDays, d.Hours(), d.Minutes(), d.RemainingSeconds()}, d.micro, 0)
}

// String is str(d), the duration in words in the current locale.
func (d Duration) String() string {
	s, err := d.InWords(nil, " ")
	if err != nil {
		return err.Error()
	}
	return s
}

// Repr is repr(d): Duration(weeks=1, days=2, hours=3).
func (d Duration) Repr() string {
	var b strings.Builder
	b.WriteString(d.className() + "(")
	add := func(name string, v int) {
		if v != 0 {
			fmt.Fprintf(&b, "%s=%d, ", name, v)
		}
	}
	add("years", d.years)
	add("months", d.months)
	add("weeks", d.weeks)
	if d.days != 0 {
		fmt.Fprintf(&b, "days=%d, ", d.remDays)
	}
	add("hours", d.Hours())
	add("minutes", d.Minutes())
	add("seconds", d.RemainingSeconds())
	add("microseconds", d.micro)
	b.WriteString(")")
	return strings.Replace(b.String(), ", )", ")", 1)
}

func (d Duration) signature() addArgs {
	return d.sig
}

// sameKind builds a value of d's class from seconds, as self.__class__(seconds=...).
func (d Duration) sameKind(a durationArgs) (Duration, error) {
	if d.absolute {
		return newAbsoluteDuration(a)
	}
	return newDuration(a)
}

// Add is d + other and Sub d - other: totals in seconds, as floats.
func (d Duration) Add(other Duration) (Duration, error) {
	return d.sameKind(durationArgs{seconds: nf(d.TotalSeconds() + other.TotalSeconds())})
}

func (d Duration) Sub(other Duration) (Duration, error) {
	return d.sameKind(durationArgs{seconds: nf(d.TotalSeconds() - other.TotalSeconds())})
}

// Neg is -d.
func (d Duration) Neg() (Duration, error) {
	return d.sameKind(durationArgs{years: ni(-d.years), months: ni(-d.months), weeks: ni(-d.weeks),
		days: ni(-d.remDays), seconds: ni(-d.seconds), microseconds: ni(-d.micro)})
}

func (d Duration) toMicroseconds() *big.Int {
	v := big.NewInt(int64(d.days)*86400 + int64(d.seconds))
	v.Mul(v, big.NewInt(1000000))
	return v.Add(v, big.NewInt(int64(d.micro)))
}

func bigArg(v *big.Int) num {
	if v.IsInt64() {
		return num{i: v.Int64()}
	}
	return num{i: math.MaxInt64}
}

func microArgs(us *big.Int) durationArgs {
	return durationArgs{microseconds: bigArg(us)}
}

// MulInt is d * n.
func (d Duration) MulInt(n int) (Duration, error) {
	return d.sameKind(durationArgs{years: ni(d.years * n), months: ni(d.months * n), seconds: nf(d.total * float64(n))})
}

// MulFloat is d * f: the microseconds scaled with round-half-even (years and months
// are dropped, as the original drops them).
func (d Duration) MulFloat(f float64) (Duration, error) {
	a, b, err := asIntegerRatio(f)
	if err != nil {
		return Duration{}, err
	}
	us := new(big.Int).Mul(d.toMicroseconds(), a)
	q, err := divideAndRound(us, b)
	if err != nil {
		return Duration{}, err
	}
	return d.sameKind(microArgs(q))
}

var errIntDivZero = pyrt.NewError("ZeroDivisionError", "integer division or modulo by zero")

// FloorDivInt is d // n.
func (d Duration) FloorDivInt(n int) (Duration, error) {
	if n == 0 {
		return Duration{}, errIntDivZero
	}
	us := floorDivBig(d.toMicroseconds(), big.NewInt(int64(n)))
	return d.sameKind(durationArgs{microseconds: bigArg(us), years: ni(floorDiv(d.years, n)), months: ni(floorDiv(d.months, n))})
}

// FloorDivDuration is d // other.
func (d Duration) FloorDivDuration(other Duration) (int, error) {
	o := other.toMicroseconds()
	if o.Sign() == 0 {
		return 0, errIntDivZero
	}
	return int(floorDivBig(d.toMicroseconds(), o).Int64()), nil
}

// TrueDivInt is d / n.
func (d Duration) TrueDivInt(n int) (Duration, error) {
	if n == 0 {
		return Duration{}, errIntDivZero
	}
	bn := big.NewInt(int64(n))
	us, _ := divideAndRound(d.toMicroseconds(), bn)
	y, _ := divideAndRound(big.NewInt(int64(d.years)), bn)
	m, _ := divideAndRound(big.NewInt(int64(d.months)), bn)
	return d.sameKind(durationArgs{microseconds: bigArg(us), years: bigArg(y), months: bigArg(m)})
}

// TrueDivFloat is d / f.
func (d Duration) TrueDivFloat(f float64) (Duration, error) {
	a, b, err := asIntegerRatio(f)
	if err != nil {
		return Duration{}, err
	}
	if a.Sign() == 0 {
		return Duration{}, errIntDivZero
	}
	us, _ := divideAndRound(new(big.Int).Mul(b, d.toMicroseconds()), a)
	y, _ := divideAndRound(new(big.Int).Mul(big.NewInt(int64(d.years)), b), a)
	m := divideAndRoundFloat(float64(d.months), f)
	return d.sameKind(durationArgs{microseconds: bigArg(us), years: bigArg(y), months: ni(m)})
}

// TrueDivDuration is d / other as a float.
func (d Duration) TrueDivDuration(other Duration) (float64, error) {
	o := other.toMicroseconds()
	if o.Sign() == 0 {
		return 0, pyrt.NewError("ZeroDivisionError", "division by zero")
	}
	return pydt.TrueDiv(d.toMicroseconds(), o), nil
}

// Mod is d % other.
func (d Duration) Mod(other Duration) (Duration, error) {
	o := other.toMicroseconds()
	if o.Sign() == 0 {
		return Duration{}, pyrt.NewError("ZeroDivisionError", "integer modulo by zero")
	}
	_, r := floorDivModBig(d.toMicroseconds(), o)
	return d.sameKind(microArgs(r))
}

// DivMod is divmod(d, other).
func (d Duration) DivMod(other Duration) (int, Duration, error) {
	o := other.toMicroseconds()
	if o.Sign() == 0 {
		return 0, Duration{}, errIntDivZero
	}
	q, r := floorDivModBig(d.toMicroseconds(), o)
	rd, err := d.sameKind(microArgs(r))
	return int(q.Int64()), rd, err
}

// Equal is d == other and Compare orders two durations, both on their timedelta values.
func (d Duration) Equal(other Duration) bool  { return d.td == other.td }
func (d Duration) Compare(other Duration) int { return d.td.Compare(other.td) }

func floorDivBig(a, b *big.Int) *big.Int { q, _ := floorDivModBig(a, b); return q }

func floorDivModBig(a, b *big.Int) (*big.Int, *big.Int) {
	q, r := new(big.Int).QuoRem(a, b, new(big.Int))
	if r.Sign() != 0 && (r.Sign() < 0) != (b.Sign() < 0) {
		q.Sub(q, big.NewInt(1))
		r.Add(r, b)
	}
	return q, r
}

// divideAndRound is pendulum's _divide_and_round for ints: a / b rounded half to even.
func divideAndRound(a, b *big.Int) (*big.Int, error) {
	if b.Sign() == 0 {
		return nil, errIntDivZero
	}
	q, r := floorDivModBig(a, b)
	r.Mul(r, big.NewInt(2))
	c := r.Cmp(b)
	greater := (b.Sign() > 0 && c > 0) || (b.Sign() < 0 && c < 0)
	if greater || (c == 0 && q.Bit(0) == 1) {
		q.Add(q, big.NewInt(1))
	}
	return q, nil
}

// divideAndRoundFloat is _divide_and_round(a, b) with a float divisor.
func divideAndRoundFloat(a, b float64) int {
	q, r := pyrt.FloatDivmod(a, b)
	qi := int(q)
	r *= 2
	greater := (b > 0 && r > b) || (b <= 0 && r < b)
	if greater || (r == b && pyMod(qi, 2) == 1) {
		qi++
	}
	return qi
}

// asIntegerRatio is float.as_integer_ratio().
func asIntegerRatio(f float64) (*big.Int, *big.Int, error) {
	if math.IsInf(f, 0) {
		return nil, nil, pyrt.NewError("OverflowError", "cannot convert Infinity to integer ratio")
	}
	if math.IsNaN(f) {
		return nil, nil, pyrt.NewError("ValueError", "cannot convert NaN to integer ratio")
	}
	r := new(big.Rat)
	r.SetFloat64(f)
	return new(big.Int).Set(r.Num()), new(big.Int).Set(r.Denom()), nil
}
