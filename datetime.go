package pendulum

import (
	"github.com/python-pendulum/pendulum/internal/pydt"
)

// DateTime is pendulum.DateTime: a datetime that may carry a Zone (aware) or not
// (naive). It is a value; the zero DateTime is not a valid datetime.
type DateTime struct{ d pydt.Datetime }

func newDT(y, mo, d, h, mi, s, us int, tz Zone, fold int) (DateTime, error) {
	var ti pydt.Tzinfo
	if tz != nil {
		ti = tz.tzinfo()
	}
	v, err := pydt.NewDatetime(y, mo, d, h, mi, s, us, ti, fold)
	return DateTime{v}, err
}

func fromPlain(d pydt.Datetime) DateTime { return DateTime{d} }

// plain is datetime.datetime(self.year, ..., tzinfo=self.tzinfo, fold=self.fold).
func (dt DateTime) plain() pydt.Datetime { return dt.d }

// create is DateTime.create(...).
func create(y, mo, d, h, mi, s, us int, tz any, fold int, raise bool) (DateTime, error) {
	var zone Zone
	if tz != nil {
		z, err := safeTimezone(tz)
		if err != nil {
			return DateTime{}, err
		}
		zone = z
	}
	p, err := pydt.NewDatetime(y, mo, d, h, mi, s, us, nil, fold)
	if err != nil {
		return DateTime{}, err
	}
	if zone != nil {
		if p, err = convertPlain(zone, p, raise); err != nil {
			return DateTime{}, err
		}
	}
	return DateTime{p}, nil
}

// createIn is create with a zone that is already resolved (nil for naive) and fold 1.
func createIn(f [7]int, zone Zone) (DateTime, error) {
	var tz any
	if zone != nil {
		tz = zone
	}
	return create(f[0], f[1], f[2], f[3], f[4], f[5], f[6], tz, 1, false)
}

// Datetime is pendulum.datetime(year, month, day, hour, minute, second, microsecond,
// tz=tz, fold=fold, raise_on_unknown_times=raiseOnUnknownTimes). tz is nil (a naive
// datetime), a zone name or "local", an int or float64 number of hours, a *Timezone
// or a *FixedTimezone.
func Datetime(year, month, day, hour, minute, second, microsecond int, tz any, fold int, raiseOnUnknownTimes bool) (DateTime, error) {
	return create(year, month, day, hour, minute, second, microsecond, tz, fold, raiseOnUnknownTimes)
}

// Local is pendulum.local(...): a datetime in the local zone.
func Local(year, month, day, hour, minute, second, microsecond int) (DateTime, error) {
	return create(year, month, day, hour, minute, second, microsecond, LocalTimezone(), 1, false)
}

// Naive is pendulum.naive(...): a datetime without a zone.
func Naive(year, month, day, hour, minute, second, microsecond, fold int) (DateTime, error) {
	return newDT(year, month, day, hour, minute, second, microsecond, nil, fold)
}

// instance is DateTime.instance(dt, tz=UTC) for a pendulum or plain datetime.
func instance(d pydt.Datetime, tz any) (DateTime, error) {
	var t any = tz
	if z := zoneOf(d.Tzinfo()); z != nil {
		t = z
	}
	return create(d.Year(), d.Month(), d.Day(), d.Hour(), d.Minute(), d.Second(), d.Microsecond(), t, d.Fold(), false)
}

// Year and the other field getters return the datetime's fields.
func (dt DateTime) Year() int        { return dt.d.Year() }
func (dt DateTime) Month() int       { return dt.d.Month() }
func (dt DateTime) Day() int         { return dt.d.Day() }
func (dt DateTime) Hour() int        { return dt.d.Hour() }
func (dt DateTime) Minute() int      { return dt.d.Minute() }
func (dt DateTime) Second() int      { return dt.d.Second() }
func (dt DateTime) Microsecond() int { return dt.d.Microsecond() }
func (dt DateTime) Fold() int        { return dt.d.Fold() }

// Timezone is dt.timezone (also dt.tz): the zone, or nil for a naive datetime.
func (dt DateTime) Timezone() Zone { return zoneOf(dt.d.Tzinfo()) }

// TimezoneName is dt.timezone_name: the zone's name, or nil when naive.
func (dt DateTime) TimezoneName() *string {
	z := dt.Timezone()
	if z == nil {
		return nil
	}
	n := z.Name()
	return &n
}

// Offset is dt.offset: the UTC offset in seconds, or nil when naive.
func (dt DateTime) Offset() *int {
	off, ok := dt.d.UtcOffset()
	if !ok {
		return nil
	}
	s := int(off.TotalMicroseconds() / 1000000)
	return &s
}

// OffsetHours is dt.offset_hours.
func (dt DateTime) OffsetHours() *float64 {
	off := dt.Offset()
	if off == nil {
		return nil
	}
	h := float64(*off) / 60 / 60
	return &h
}

// UtcOffset is dt.utcoffset() in seconds, nil when naive.
func (dt DateTime) UtcOffset() *int { return dt.Offset() }

// Dst is dt.dst() in seconds, nil when naive.
func (dt DateTime) Dst() *int {
	off, ok := dt.d.Dst()
	if !ok {
		return nil
	}
	s := int(off.TotalMicroseconds() / 1000000)
	return &s
}

// TzName is dt.tzname(), nil when naive.
func (dt DateTime) TzName() *string {
	n, ok := dt.d.TzName()
	if !ok {
		return nil
	}
	return &n
}

// IsDst is dt.is_dst(): dst() != timedelta(0), which holds for a naive datetime too.
func (dt DateTime) IsDst() bool {
	off, ok := dt.d.Dst()
	return !ok || off != pydt.Timedelta{}
}

// IsUtc is dt.is_utc(): the offset is zero.
func (dt DateTime) IsUtc() bool {
	off := dt.Offset()
	return off != nil && *off == 0
}

// IsLocal is dt.is_local(): the offset equals that of the same instant in the local zone.
func (dt DateTime) IsLocal() (bool, error) {
	l, err := dt.InTimezone(LocalTimezone())
	if err != nil {
		return false, err
	}
	a, b := dt.Offset(), l.Offset()
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b), nil
}

// Date is dt.date().
func (dt DateTime) Date() Date { return Date{dt.d.Date()} }

// Time is dt.time().
func (dt DateTime) Time() Time {
	t, _ := pydt.NewTime(dt.Hour(), dt.Minute(), dt.Second(), dt.Microsecond(), nil)
	return Time{t}
}

// Naive is dt.naive(): the same fields without a zone (fold 0).
func (dt DateTime) Naive() DateTime { return DateTime{dt.d.Naive().WithFold(0)} }

// Weekday is dt.weekday() (Monday 0) and Isoweekday dt.isoweekday() (Monday 1).
func (dt DateTime) Weekday() int    { return dt.d.Weekday() }
func (dt DateTime) Isoweekday() int { return dt.d.Isoweekday() }

// Isocalendar is dt.isocalendar(): ISO year, week and weekday.
func (dt DateTime) Isocalendar() (int, int, int) { return dt.d.Isocalendar() }

// Toordinal is dt.toordinal().
func (dt DateTime) Toordinal() int { return dt.d.Toordinal() }

// DayOfWeek is dt.day_of_week.
func (dt DateTime) DayOfWeek() WeekDay { return WeekDay(dt.d.Weekday()) }

// DayOfYear is dt.day_of_year.
func (dt DateTime) DayOfYear() int { return dt.Date().DayOfYear() }

// WeekOfYear is dt.week_of_year (the ISO week).
func (dt DateTime) WeekOfYear() int { _, w, _ := dt.d.Isocalendar(); return w }

// WeekOfMonth is dt.week_of_month.
func (dt DateTime) WeekOfMonth() int { return dt.Date().WeekOfMonth() }

// DaysInMonth is dt.days_in_month.
func (dt DateTime) DaysInMonth() int { return dt.Date().DaysInMonth() }

// Quarter is dt.quarter.
func (dt DateTime) Quarter() int { return (dt.Month() + 2) / 3 }

// IsLeapYear is dt.is_leap_year().
func (dt DateTime) IsLeapYear() bool { return pydt.IsLeap(dt.Year()) }

// IsLongYear is dt.is_long_year(): December 28 of the year, in the same zone, falls in
// ISO week 53.
func (dt DateTime) IsLongYear() (bool, error) {
	var tz any
	if z := dt.Timezone(); z != nil {
		tz = z
	}
	d, err := create(dt.Year(), 12, 28, 0, 0, 0, 0, tz, 1, false)
	if err != nil {
		return false, err
	}
	_, w, _ := d.d.Isocalendar()
	return w == 53, nil
}

// IntTimestamp is dt.int_timestamp: whole seconds since the epoch.
func (dt DateTime) IntTimestamp() (int, error) {
	epoch, _ := pydt.NewDatetime(1970, 1, 1, 0, 0, 0, 0, UTC.tzinfo(), 0)
	delta, err := dt.d.Diff(epoch)
	if err != nil {
		return 0, err
	}
	return delta.Days()*86400 + delta.Seconds(), nil
}

// FloatTimestamp is dt.float_timestamp, which is dt.timestamp().
func (dt DateTime) FloatTimestamp() (float64, error) { return dt.d.Timestamp() }

// Timestamp is dt.timestamp(): for a naive datetime, the local zone's reading.
func (dt DateTime) Timestamp() (float64, error) { return dt.d.Timestamp() }

// Age is dt.age: whole years from the date to today in the same zone.
func (dt DateTime) Age() (int, error) {
	now, err := Now(zoneArg(dt.Timezone()))
	if err != nil {
		return 0, err
	}
	iv, err := dt.Date().Diff(ptr(now.Date()), false)
	if err != nil {
		return 0, err
	}
	return iv.InYears(), nil
}

func zoneArg(z Zone) any {
	if z == nil {
		return nil
	}
	return z
}

func ptr[T any](v T) *T { return &v }
