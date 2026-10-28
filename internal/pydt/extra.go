package pydt

import (
	"fmt"
	"math"
	"math/big"

	"github.com/python-pendulum/pendulum/internal/clocal"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// epochOrdinal is datetime(1970, 1, 1).toordinal().
const epochOrdinal = 719163

// maxFoldSeconds is CPython's max_fold_seconds.
const maxFoldSeconds = 24 * 3600

// NewDatetime is datetime.datetime(year, ..., microsecond, tzinfo=tz, fold=fold).
func NewDatetime(year, month, day, hour, minute, second, microsecond int, tz Tzinfo, fold int) (Datetime, error) {
	return newDatetime([7]int{year, month, day, hour, minute, second, microsecond}, tz, fold)
}

// Fields returns year, month, day, hour, minute, second and microsecond.
func (d Datetime) Parts() [7]int {
	return [7]int{d.year, d.month, d.day, d.hour, d.minute, d.second, d.microsecond}
}

// Naive is d with its tzinfo removed (replace(tzinfo=None)).
func (d Datetime) Naive() Datetime { d.tz = nil; return d }

// LocalSeconds is _zoneinfo's _get_local_timestamp: the naive reading as seconds since
// the epoch, microseconds ignored.
func (d Datetime) LocalSeconds() int64 {
	return int64(d.Toordinal()-epochOrdinal)*86400 + int64(d.hour*3600+d.minute*60+d.second)
}

// FromLocalSeconds is the naive datetime whose LocalSeconds is s, with microsecond us.
func FromLocalSeconds(s int64, us int) (Datetime, error) {
	days, rem := s/86400, s%86400
	if rem < 0 {
		rem += 86400
		days--
	}
	ord := days + epochOrdinal
	if ord < 1 || ord > maxOrdinal {
		return Datetime{}, errDateRange
	}
	y, m, dd := ordToYMD(int(ord))
	return Datetime{y, m, dd, int(rem / 3600), int(rem % 3600 / 60), int(rem % 60), us, 0, nil}, nil
}

// Dst is datetime.dst().
func (d Datetime) Dst() (Timedelta, bool) {
	if d.tz == nil {
		return Timedelta{}, false
	}
	return d.tz.Dst(d)
}

// Isocalendar is date.isocalendar(): ISO year, week and weekday.
func (d Date) Isocalendar() (int, int, int) {
	year := d.year
	week1 := isoWeek1Monday(year)
	today := ymdToOrd(d.year, d.month, d.day)
	week, day := cDivmod(today-week1, 7)
	if week < 0 {
		year--
		week1 = isoWeek1Monday(year)
		week, day = cDivmod(today-week1, 7)
	} else if week >= 52 {
		if today >= isoWeek1Monday(year+1) {
			year++
			week = 0
		}
	}
	return year, week + 1, day + 1
}

func isoWeek1Monday(year int) int {
	first := ymdToOrd(year, 1, 1)
	firstWeekday := (first + 6) % 7
	monday := first - firstWeekday
	if firstWeekday > 3 {
		monday += 7
	}
	return monday
}

// Isocalendar is datetime.isocalendar().
func (d Datetime) Isocalendar() (int, int, int) { return d.Date().Isocalendar() }

// Isoweekday is datetime.isoweekday().
func (d Datetime) Isoweekday() int { return d.Weekday() + 1 }

// Isoformat is date.isoformat().
func (d Date) Isoformat() string { return fmt.Sprintf("%04d-%02d-%02d", d.year, d.month, d.day) }

// Isoformat is time.isoformat() for a naive time.
func (t Time) Isoformat() string {
	s := fmt.Sprintf("%02d:%02d:%02d", t.hour, t.minute, t.second)
	if t.microsecond != 0 {
		s += fmt.Sprintf(".%06d", t.microsecond)
	}
	return s
}

// Isoformat is datetime.isoformat(sep).
func (d Datetime) Isoformat(sep string) string {
	s := fmt.Sprintf("%04d-%02d-%02d%s%02d:%02d:%02d", d.year, d.month, d.day, sep, d.hour, d.minute, d.second)
	if d.microsecond != 0 {
		s += fmt.Sprintf(".%06d", d.microsecond)
	}
	if off, ok := d.UtcOffset(); ok {
		s += FormatOffset(off, ":")
	}
	return s
}

// FormatOffset is CPython's format_utcoffset: +HH:MM, with :SS and .ffffff when present.
func FormatOffset(off Timedelta, sep string) string {
	sign := "+"
	if off.days < 0 {
		sign = "-"
		off, _ = off.Neg()
	}
	secs := off.days*86400 + off.seconds
	h, m, s := secs/3600, secs%3600/60, secs%60
	out := fmt.Sprintf("%s%02d%s%02d", sign, h, sep, m)
	if s != 0 || off.microseconds != 0 {
		out += fmt.Sprintf("%s%02d", sep, s)
		if off.microseconds != 0 {
			out += fmt.Sprintf(".%06d", off.microseconds)
		}
	}
	return out
}

// TotalMicroseconds is the delta as an integer number of microseconds.
func (t Timedelta) TotalMicroseconds() int64 {
	return (int64(t.days)*86400+int64(t.seconds))*1000000 + int64(t.microseconds)
}

// TotalSecondsExact is total_seconds() computed as Python does: the microsecond total
// divided by 10**6 with a correctly rounded true division.
func (t Timedelta) TotalSecondsExact() float64 {
	return TrueDiv(big.NewInt(t.TotalMicroseconds()), big.NewInt(1000000))
}

// TrueDiv is a / b for Python ints: the correctly rounded float quotient.
func TrueDiv(a, b *big.Int) float64 {
	f, _ := new(big.Rat).SetFrac(a, b).Float64()
	return f
}

// DeltaFromMicroseconds is timedelta(microseconds=x) for an int x.
func DeltaFromMicroseconds(x *big.Int) (Timedelta, error) { return microsecondsToDelta(x) }

// DeltaArg is one argument of the timedelta constructor: a Python int or float.
type DeltaArg struct {
	Int     *big.Int
	Float   float64
	IsFloat bool
}

// I and F build int and float constructor arguments.
func I(v int64) DeltaArg       { return DeltaArg{Int: big.NewInt(v)} }
func F(v float64) DeltaArg     { return DeltaArg{Float: v, IsFloat: true} }
func BigI(v *big.Int) DeltaArg { return DeltaArg{Int: v} }

func (a DeltaArg) truthy() bool {
	if a.IsFloat {
		return a.Float != 0
	}
	return a.Int != nil && a.Int.Sign() != 0
}

// NewDelta is datetime.timedelta(days, seconds, microseconds, milliseconds, minutes,
// hours, weeks), accumulated in CPython's order with its float rounding.
func NewDelta(days, seconds, microseconds, milliseconds, minutes, hours, weeks DeltaArg) (Timedelta, error) {
	sum := new(big.Int)
	leftover := 0.0
	parts := []struct {
		v      DeltaArg
		factor int64
	}{
		{microseconds, 1}, {milliseconds, 1000}, {seconds, 1000000}, {minutes, 60000000},
		{hours, 3600000000}, {days, 86400000000}, {weeks, 604800000000},
	}
	for _, p := range parts {
		if !p.v.truthy() {
			continue
		}
		if !p.v.IsFloat {
			sum.Add(sum, new(big.Int).Mul(p.v.Int, big.NewInt(p.factor)))
			continue
		}
		if err := accum(sum, p.v.Float, p.factor, &leftover); err != nil {
			return Timedelta{}, err
		}
	}
	if leftover != 0 {
		whole := math.Round(leftover)
		if math.Abs(whole-leftover) == 0.5 {
			odd := float64(sum.Bit(0))
			if sum.Sign() < 0 {
				odd = float64(new(big.Int).Abs(sum).Bit(0))
			}
			whole = 2.0*math.Round((leftover+odd)*0.5) - odd
		}
		sum.Add(sum, big.NewInt(int64(whole)))
	}
	return microsecondsToDelta(sum)
}

// AddDelta is datetime + timedelta (fold reset, tzinfo kept).
func (d Datetime) AddDelta(td Timedelta) (Datetime, error) { return d.addDelta(td, 1) }

// SubDelta is datetime - timedelta.
func (d Datetime) SubDelta(td Timedelta) (Datetime, error) { return d.addDelta(td, -1) }

// Astimezone is datetime.astimezone(tz) for a non-None tz.
func (d Datetime) Astimezone(tz Tzinfo) (Datetime, error) {
	selfTz := d.tz
	var off Timedelta
	if selfTz != nil {
		if sameTzinfo(selfTz, tz) {
			return d, nil
		}
		o, ok := selfTz.UtcOffset(d)
		if ok {
			off = o
		} else {
			selfTz = nil
		}
	}
	if selfTz == nil {
		secs, err := localToSeconds(d)
		if err != nil {
			return Datetime{}, err
		}
		goff, _, _ := clocal.Local(secs)
		off, _ = NewTimedelta(0, goff, 0)
	}
	utc, err := d.addDelta(off, -1)
	if err != nil {
		return Datetime{}, err
	}
	utc.fold = d.fold
	utc.tz = tz
	return tz.FromUtc(utc)
}

// local is CPython's local(u): the local reading, in seconds since 0001-01-01, of the
// instant u (same scale).
func local(u int64) (int64, error) {
	t := u - epochSecondsOrd
	off, _, _ := clocal.Local(t)
	return utcToSeconds(t + int64(off))
}

// epochSecondsOrd is the epoch in seconds since 0001-01-01 on the ordinal scale.
const epochSecondsOrd = int64(epochOrdinal) * 86400

func utcToSeconds(localEpoch int64) (int64, error) {
	days := localEpoch / 86400
	if localEpoch%86400 < 0 {
		days--
	}
	ord := days + epochOrdinal
	if ord < 1 {
		y := yearOfDays(days)
		return 0, pyrt.ValueError("year %d is out of range", y)
	}
	if ord > maxOrdinal {
		y := yearOfDays(days)
		return 0, pyrt.ValueError("year %d is out of range", y)
	}
	return localEpoch + epochSecondsOrd, nil
}

// yearOfDays is the proleptic Gregorian year of a day count from the epoch, for any day.
func yearOfDays(days int64) int64 {
	z := days + 719468
	era := z / 146097
	if z < 0 && z%146097 != 0 {
		era--
	}
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	if mp >= 10 {
		y++
	}
	return y
}

// localToSeconds is CPython's local_to_seconds for a naive reading, returned as seconds
// since the epoch.
func localToSeconds(d Datetime) (int64, error) {
	t := int64(d.Toordinal())*86400 + int64(d.hour*3600+d.minute*60+d.second)
	lt, err := local(t)
	if err != nil {
		return 0, err
	}
	a := lt - t
	u1 := t - a
	t1, err := local(u1)
	if err != nil {
		return 0, err
	}
	var b int64
	if t1 == t {
		u2 := u1 - maxFoldSeconds
		if d.fold == 1 {
			u2 = u1 + maxFoldSeconds
		}
		lt, err = local(u2)
		if err != nil {
			return 0, err
		}
		b = lt - u2
		if a == b {
			return u1 - epochSecondsOrd, nil
		}
	} else {
		b = t1 - u1
	}
	u2 := t - b
	t2, err := local(u2)
	if err != nil {
		return 0, err
	}
	if t2 == t {
		return u2 - epochSecondsOrd, nil
	}
	if t1 == t {
		return u1 - epochSecondsOrd, nil
	}
	if d.fold == 1 {
		return min(u1, u2) - epochSecondsOrd, nil
	}
	return max(u1, u2) - epochSecondsOrd, nil
}

// Timestamp is datetime.timestamp().
func (d Datetime) Timestamp() (float64, error) {
	if off, ok := d.UtcOffset(); ok {
		_ = off
		epoch := Datetime{year: 1970, month: 1, day: 1, tz: utcMarker{}}
		delta, err := d.Diff(epoch)
		if err != nil {
			return 0, err
		}
		return delta.TotalSecondsExact(), nil
	}
	secs, err := localToSeconds(d)
	if err != nil {
		return 0, err
	}
	return float64(secs) + float64(d.microsecond)/1e6, nil
}

// utcMarker is datetime.timezone.utc for internal arithmetic.
type utcMarker struct{}

func (utcMarker) UtcOffset(Datetime) (Timedelta, bool) { return Timedelta{}, true }
func (utcMarker) Dst(Datetime) (Timedelta, bool)       { return Timedelta{}, true }
func (utcMarker) TzName(Datetime) (string, bool)       { return "UTC", true }
func (utcMarker) FromUtc(d Datetime) (Datetime, error) { return d, nil }
func (utcMarker) String() string                       { return "datetime.timezone.utc" }

// Timestamp splits a timestamp argument as _PyTime_ObjectToTimeval does with
// ROUND_HALF_EVEN: whole seconds and microseconds.
type Timestamp struct {
	Int     int64
	Float   float64
	IsFloat bool
}

var errTimeT = pyrt.OverflowError("timestamp out of range for platform time_t")

func (ts Timestamp) split() (int64, int, error) {
	if !ts.IsFloat {
		return ts.Int, 0, nil
	}
	f := ts.Float
	if math.IsNaN(f) {
		return 0, 0, pyrt.NewError("ValueError", "Invalid value NaN (not a number)")
	}
	intpart, frac := math.Modf(f)
	frac *= 1e6
	r := math.Round(frac)
	if math.Abs(frac-r) == 0.5 {
		r = 2.0 * math.Round(frac/2.0)
	}
	frac = r
	if frac >= 1e6 {
		frac -= 1e6
		intpart++
	} else if frac < 0 {
		frac += 1e6
		intpart--
	}
	if !(intpart >= -9223372036854775808.0 && intpart < 9223372036854775808.0) {
		return 0, 0, errTimeT
	}
	return int64(intpart), int(frac), nil
}

// FromTimestamp is datetime.datetime.fromtimestamp(ts, tz): local time when tz is nil.
func FromTimestamp(ts float64, tz Tzinfo) (Datetime, error) {
	return FromTimestampArg(Timestamp{Float: ts, IsFloat: true}, tz)
}

// FromTimestampArg is FromTimestamp for an int or float timestamp.
func FromTimestampArg(ts Timestamp, tz Tzinfo) (Datetime, error) {
	timet, us, err := ts.split()
	if err != nil {
		return Datetime{}, err
	}
	off := 0
	if tz == nil {
		off, _, _ = clocal.Local(timet)
	}
	wall := timet + int64(off)
	days := wall / 86400
	if wall%86400 < 0 {
		days--
	}
	y := yearOfDays(days)
	if y > math.MaxInt32+1900 || y < math.MinInt32+1900 {
		if tz == nil {
			return Datetime{}, pyrt.NewError("OSError", "[Errno 75] Value too large for defined data type")
		}
		return Datetime{}, pyrt.NewError("OSError", "[Errno 75] Value too large for defined data type")
	}
	if y < MinYear || y > MaxYear {
		return Datetime{}, pyrt.ValueError("year %d is out of range", y)
	}
	d, err := FromLocalSeconds(wall, us)
	if err != nil {
		return Datetime{}, err
	}
	if tz == nil {
		resultSeconds := d.LocalSeconds()
		probe, err := localEpoch(timet - maxFoldSeconds)
		if err != nil {
			return Datetime{}, err
		}
		transition := resultSeconds - probe - maxFoldSeconds
		if transition < 0 {
			probe, err = localEpoch(timet + transition)
			if err != nil {
				return Datetime{}, err
			}
			if probe == resultSeconds {
				d.fold = 1
			}
		}
		return d, nil
	}
	d.tz = tz
	return tz.FromUtc(d)
}

// localEpoch is local() on the epoch scale.
func localEpoch(t int64) (int64, error) {
	r, err := local(t + epochSecondsOrd)
	if err != nil {
		return 0, err
	}
	return r - epochSecondsOrd, nil
}

// Timetuple fields CPython hands to strftime.
func (d Datetime) Yday() int { return d.Yearday() }

// Repr is repr(timedelta) as CPython 3.12 prints it.
func (t Timedelta) String() string { return t.Repr() }

// MustOrdinal is date(year, month, day).toordinal() for a valid date.
func MustOrdinal(year, month, day int) int { return ymdToOrd(year, month, day) }
