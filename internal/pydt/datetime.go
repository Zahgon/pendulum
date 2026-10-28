// Package pydt is the part of CPython 3.12's datetime module that pendulum and
// python-dateutil build on: date, time, datetime and timedelta values with CPython's
// range checks, arithmetic, fold and tzinfo-identity comparison rules, timestamps in
// the C library's local time, and error texts.
package pydt

import (
	"github.com/python-pendulum/pendulum/internal/clock"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// Tzinfo is a datetime.tzinfo. A nil Tzinfo is None.
type Tzinfo interface {
	// UtcOffset is tzinfo.utcoffset(dt); ok is false for None.
	UtcOffset(dt Datetime) (offset Timedelta, ok bool)
	// Dst is tzinfo.dst(dt); ok is false for None.
	Dst(dt Datetime) (offset Timedelta, ok bool)
	// TzName is tzinfo.tzname(dt); ok is false for None.
	TzName(dt Datetime) (name string, ok bool)
	// FromUtc is tzinfo.fromutc(dt) for a dt whose tzinfo is this zone and whose
	// fields read UTC.
	FromUtc(dt Datetime) (Datetime, error)
	// String is repr(tzinfo).
	String() string
}

// DateOrDatetime is a Date or a Datetime. Python's datetime is a subclass of date, and
// dateutil accepts either wherever it accepts a date.
type DateOrDatetime interface{ dateOrDatetime() }

func (Date) dateOrDatetime()     {}
func (Datetime) dateOrDatetime() {}

// Date is a datetime.date.
type Date struct{ year, month, day int }

// NewDate is datetime.date(year, month, day).
func NewDate(year, month, day int) (Date, error) {
	for _, v := range []int{year, month, day} {
		if _, err := pyrt.AsCInt(v); err != nil {
			return Date{}, err
		}
	}
	if err := checkDateArgs(year, month, day); err != nil {
		return Date{}, err
	}
	return Date{year, month, day}, nil
}

// DateFromOrdinal is datetime.date.fromordinal(n).
func DateFromOrdinal(n int) (Date, error) {
	if _, err := pyrt.AsCInt(n); err != nil {
		return Date{}, err
	}
	if n < 1 {
		return Date{}, pyrt.NewError("ValueError", "ordinal must be >= 1")
	}
	y, m, d := ordToYMD(n)
	if err := checkDateArgs(y, m, d); err != nil {
		return Date{}, err
	}
	return Date{y, m, d}, nil
}

func (d Date) Year() int  { return d.year }
func (d Date) Month() int { return d.month }
func (d Date) Day() int   { return d.day }

// Toordinal is date.toordinal().
func (d Date) Toordinal() int { return ymdToOrd(d.year, d.month, d.day) }

// Weekday is date.weekday(): Monday is 0.
func (d Date) Weekday() int { return (d.Toordinal() + 6) % 7 }

// Isoweekday is date.isoweekday(): Monday is 1.
func (d Date) Isoweekday() int { return d.Weekday() + 1 }

// Yearday is timetuple().tm_yday.
func (d Date) Yearday() int { return daysBeforeMonth(d.year, d.month) + d.day }

// Add is date + timedelta; only the days of the delta count.
func (d Date) Add(td Timedelta) (Date, error) { return d.addDays(td.days) }

// Sub is date - timedelta.
func (d Date) Sub(td Timedelta) (Date, error) { return d.addDays(-td.days) }

func (d Date) addDays(days int) (Date, error) {
	y, m, day := d.year, d.month, d.day+days
	if err := normalizeDate(&y, &m, &day); err != nil {
		return Date{}, err
	}
	return Date{y, m, day}, nil
}

// Diff is date - date.
func (d Date) Diff(o Date) Timedelta {
	td, _ := newDelta(d.Toordinal()-o.Toordinal(), 0, 0)
	return td
}

// Compare orders two dates.
func (d Date) Compare(o Date) int { return cmpInts(d.year, o.year, d.month, o.month, d.day, o.day) }

// Replace is date.replace(year=, month=, day=); nil keeps a field.
func (d Date) Replace(year, month, day *int) (Date, error) {
	y, m, dd := d.year, d.month, d.day
	if year != nil {
		y = *year
	}
	if month != nil {
		m = *month
	}
	if day != nil {
		dd = *day
	}
	return NewDate(y, m, dd)
}

func cmpInts(pairs ...int) int {
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i] != pairs[i+1] {
			if pairs[i] < pairs[i+1] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Time is a datetime.time.
type Time struct {
	hour, minute, second, microsecond int
	tz                                Tzinfo
}

// NewTime is datetime.time(hour, minute, second, microsecond, tzinfo).
func NewTime(hour, minute, second, microsecond int, tz Tzinfo) (Time, error) {
	for _, v := range []int{hour, minute, second, microsecond} {
		if _, err := pyrt.AsCInt(v); err != nil {
			return Time{}, err
		}
	}
	if err := checkTimeArgs(hour, minute, second, microsecond, 0); err != nil {
		return Time{}, err
	}
	return Time{hour, minute, second, microsecond, tz}, nil
}

func (t Time) Hour() int        { return t.hour }
func (t Time) Minute() int      { return t.minute }
func (t Time) Second() int      { return t.second }
func (t Time) Microsecond() int { return t.microsecond }
func (t Time) Tzinfo() Tzinfo   { return t.tz }

// Compare orders two times that share a tzinfo, as rrule's time sets do.
func (t Time) Compare(o Time) int {
	return cmpInts(t.hour, o.hour, t.minute, o.minute, t.second, o.second, t.microsecond, o.microsecond)
}

// Datetime is a datetime.datetime.
type Datetime struct {
	year, month, day, hour, minute, second, microsecond int
	fold                                                int
	tz                                                  Tzinfo
}

// New is datetime.datetime(year, month, day, hour, minute, second, microsecond, tzinfo).
func New(year, month, day, hour, minute, second, microsecond int, tz Tzinfo) (Datetime, error) {
	return newDatetime([7]int{year, month, day, hour, minute, second, microsecond}, tz, 0)
}

func newDatetime(f [7]int, tz Tzinfo, fold int) (Datetime, error) {
	for _, v := range f {
		if _, err := pyrt.AsCInt(v); err != nil {
			return Datetime{}, err
		}
	}
	if err := checkDateArgs(f[0], f[1], f[2]); err != nil {
		return Datetime{}, err
	}
	if err := checkTimeArgs(f[3], f[4], f[5], f[6], fold); err != nil {
		return Datetime{}, err
	}
	return Datetime{f[0], f[1], f[2], f[3], f[4], f[5], f[6], fold, tz}, nil
}

// FromOrdinal is datetime.datetime.fromordinal(n).
func FromOrdinal(n int) (Datetime, error) {
	d, err := DateFromOrdinal(n)
	if err != nil {
		return Datetime{}, err
	}
	return FromDate(d), nil
}

// FromDate is datetime.datetime.fromordinal(d.toordinal()): midnight of d.
func FromDate(d Date) Datetime { return Datetime{year: d.year, month: d.month, day: d.day} }

// Combine is datetime.datetime.combine(date, time).
func Combine(d Date, t Time) Datetime {
	return Datetime{d.year, d.month, d.day, t.hour, t.minute, t.second, t.microsecond, 0, t.tz}
}

// Now is datetime.datetime.now(): the naive local time of the (possibly frozen) clock.
func Now() Datetime {
	d, err := FromTimestamp(clock.Now(), nil)
	if err != nil {
		return Datetime{year: 1970, month: 1, day: 1}
	}
	return d
}

func (d Datetime) Year() int        { return d.year }
func (d Datetime) Month() int       { return d.month }
func (d Datetime) Day() int         { return d.day }
func (d Datetime) Hour() int        { return d.hour }
func (d Datetime) Minute() int      { return d.minute }
func (d Datetime) Second() int      { return d.second }
func (d Datetime) Microsecond() int { return d.microsecond }
func (d Datetime) Fold() int        { return d.fold }
func (d Datetime) Tzinfo() Tzinfo   { return d.tz }

// Date is datetime.date().
func (d Datetime) Date() Date { return Date{d.year, d.month, d.day} }

// Time is datetime.timetz().
func (d Datetime) Time() Time { return Time{d.hour, d.minute, d.second, d.microsecond, d.tz} }

func (d Datetime) Toordinal() int { return ymdToOrd(d.year, d.month, d.day) }
func (d Datetime) Weekday() int   { return (d.Toordinal() + 6) % 7 }
func (d Datetime) Yearday() int   { return daysBeforeMonth(d.year, d.month) + d.day }

// Fields selects the datetime.replace() arguments to change; nil keeps a field.
type Fields struct {
	Year, Month, Day, Hour, Minute, Second, Microsecond *int
}

// Replace is datetime.replace() for the integer fields.
func (d Datetime) Replace(f Fields) (Datetime, error) {
	v := [7]int{d.year, d.month, d.day, d.hour, d.minute, d.second, d.microsecond}
	for i, p := range []*int{f.Year, f.Month, f.Day, f.Hour, f.Minute, f.Second, f.Microsecond} {
		if p != nil {
			v[i] = *p
		}
	}
	nd, err := newDatetime(v, d.tz, 0)
	if err != nil {
		return Datetime{}, err
	}
	nd.fold = d.fold
	return nd, nil
}

// WithTzinfo is datetime.replace(tzinfo=tz).
func (d Datetime) WithTzinfo(tz Tzinfo) Datetime { d.tz = tz; return d }

// WithFold is datetime.replace(fold=fold) for fold 0 or 1.
func (d Datetime) WithFold(fold int) Datetime { d.fold = fold; return d }

// UtcOffset is datetime.utcoffset().
func (d Datetime) UtcOffset() (Timedelta, bool) {
	if d.tz == nil {
		return Timedelta{}, false
	}
	return d.tz.UtcOffset(d)
}

// TzName is datetime.tzname().
func (d Datetime) TzName() (string, bool) {
	if d.tz == nil {
		return "", false
	}
	return d.tz.TzName(d)
}

// Add is datetime + timedelta.
func (d Datetime) Add(td Timedelta) (Datetime, error) { return d.addDelta(td, 1) }

// Sub is datetime - timedelta.
func (d Datetime) Sub(td Timedelta) (Datetime, error) { return d.addDelta(td, -1) }

func (d Datetime) addDelta(td Timedelta, factor int) (Datetime, error) {
	y, mo, day := d.year, d.month, d.day+td.days*factor
	h, mi, s, us := d.hour, d.minute, d.second+td.seconds*factor, d.microsecond+td.microseconds*factor
	if err := normalizeDatetime(&y, &mo, &day, &h, &mi, &s, &us); err != nil {
		return Datetime{}, err
	}
	return Datetime{y, mo, day, h, mi, s, us, 0, d.tz}, nil
}

func sameTzinfo(a, b Tzinfo) bool { return a == b }

// Diff is datetime - datetime.
func (d Datetime) Diff(o Datetime) (Timedelta, error) {
	var off1, off2 Timedelta
	var has1, has2 bool
	if !sameTzinfo(d.tz, o.tz) {
		off1, has1 = d.UtcOffset()
		off2, has2 = o.UtcOffset()
		if has1 != has2 {
			return Timedelta{}, pyrt.NewError("TypeError", "can't subtract offset-naive and offset-aware datetimes")
		}
	}
	days := d.Toordinal() - o.Toordinal()
	secs := (d.hour-o.hour)*3600 + (d.minute-o.minute)*60 + (d.second - o.second)
	result, err := newDelta(days, secs, d.microsecond-o.microsecond)
	if err != nil {
		return Timedelta{}, err
	}
	if has1 && off1 != off2 {
		offdiff, err := off1.Sub(off2)
		if err != nil {
			return Timedelta{}, err
		}
		return result.Sub(offdiff)
	}
	return result, nil
}

func (d Datetime) cmpFields(o Datetime) int {
	return cmpInts(d.year, o.year, d.month, o.month, d.day, o.day, d.hour, o.hour,
		d.minute, o.minute, d.second, o.second, d.microsecond, o.microsecond)
}

// Compare orders two datetimes, which must both be naive or both aware.
func (d Datetime) Compare(o Datetime) (int, error) {
	if sameTzinfo(d.tz, o.tz) {
		return d.cmpFields(o), nil
	}
	off1, has1 := d.UtcOffset()
	off2, has2 := o.UtcOffset()
	switch {
	case has1 == has2 && (!has1 || off1 == off2):
		return d.cmpFields(o), nil
	case has1 && has2:
		delta, err := d.Diff(o)
		if err != nil {
			return 0, err
		}
		switch {
		case delta.days < 0:
			return -1, nil
		case delta.days > 0 || delta.seconds != 0 || delta.microseconds != 0:
			return 1, nil
		}
		return 0, nil
	}
	return 0, pyrt.NewError("TypeError", "can't compare offset-naive and offset-aware datetimes")
}

// Equal is datetime == datetime: a naive and an aware datetime are never equal, and
// PEP 495 makes an aware datetime whose offset depends on its fold unequal to any
// datetime in another zone.
func (d Datetime) Equal(o Datetime) bool {
	if sameTzinfo(d.tz, o.tz) {
		return d.cmpFields(o) == 0
	}
	off1, has1 := d.UtcOffset()
	off2, has2 := o.UtcOffset()
	if has1 != has2 {
		return false
	}
	c, err := d.Compare(o)
	if err != nil || c != 0 {
		return false
	}
	return !pep495Exception(d, off1, has1) && !pep495Exception(o, off2, has2)
}

func pep495Exception(d Datetime, off Timedelta, has bool) bool {
	flip, fhas := d.WithFold(1 - d.fold).UtcOffset()
	return fhas != has || (has && flip != off)
}
