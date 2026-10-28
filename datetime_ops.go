package pendulum

import (
	"fmt"

	"github.com/python-pendulum/pendulum/internal/clock"
	"github.com/python-pendulum/pendulum/internal/pydt"
)

// Now is pendulum.now(tz): the current (or frozen) time in tz; nil and "local" are the
// local zone.
func Now(tz any) (DateTime, error) {
	ts := clock.Now()
	if tz == nil || tz == "local" {
		d, err := pydt.FromTimestamp(ts, LocalTimezone().tzinfo())
		return DateTime{d}, err
	}
	d, err := pydt.FromTimestamp(ts, UTC.tzinfo())
	if err != nil {
		return DateTime{}, err
	}
	if z, ok := tz.(*Timezone); (ok && z == UTC) || tz == "UTC" {
		return DateTime{d}, nil
	}
	zone, err := safeTimezone(tz)
	if err != nil {
		return DateTime{}, err
	}
	d, err = d.Astimezone(zone.tzinfo())
	return DateTime{d}, err
}

// Today is pendulum.today(tz): the start of the current day in tz.
func Today(tz any) (DateTime, error) {
	n, err := Now(tz)
	if err != nil {
		return DateTime{}, err
	}
	return n.StartOf("day")
}

// Tomorrow is pendulum.tomorrow(tz).
func Tomorrow(tz any) (DateTime, error) {
	t, err := Today(tz)
	if err != nil {
		return DateTime{}, err
	}
	return t.Add(AddArgs{Days: 1})
}

// Yesterday is pendulum.yesterday(tz).
func Yesterday(tz any) (DateTime, error) {
	t, err := Today(tz)
	if err != nil {
		return DateTime{}, err
	}
	return t.Subtract(AddArgs{Days: 1})
}

// SetArgs are the keyword arguments of dt.set(): a nil field keeps the datetime's
// value, and a nil Tz keeps its zone.
type SetArgs struct {
	Year, Month, Day, Hour, Minute, Second, Microsecond *int
	Tz                                                  any
}

// Set is dt.set(...): the fields replaced, then normalised in the zone (fold kept).
func (dt DateTime) Set(a SetArgs) (DateTime, error) {
	f := dt.d.Parts()
	for i, p := range []*int{a.Year, a.Month, a.Day, a.Hour, a.Minute, a.Second, a.Microsecond} {
		if p != nil {
			f[i] = *p
		}
	}
	tz := a.Tz
	if tz == nil {
		tz = zoneArg(dt.Timezone())
	}
	return create(f[0], f[1], f[2], f[3], f[4], f[5], f[6], tz, dt.Fold(), false)
}

// On is dt.on(year, month, day).
func (dt DateTime) On(year, month, day int) (DateTime, error) {
	return dt.Set(SetArgs{Year: &year, Month: &month, Day: &day})
}

// At is dt.at(hour, minute, second, microsecond).
func (dt DateTime) At(hour, minute, second, microsecond int) (DateTime, error) {
	return dt.Set(SetArgs{Hour: &hour, Minute: &minute, Second: &second, Microsecond: &microsecond})
}

// ReplaceArgs are the keyword arguments of dt.replace(): a nil field keeps the value;
// Tzinfo replaces the zone only when TzinfoSet is true, and nil then means naive.
type ReplaceArgs struct {
	Year, Month, Day, Hour, Minute, Second, Microsecond *int
	Tzinfo                                              any
	TzinfoSet                                           bool
	Fold                                                *int
}

type replaceArgs struct {
	f     [7]*int
	tz    Zone
	setTz bool
	fold  *int
}

// Replace is dt.replace(...), which pendulum normalises through DateTime.create.
func (dt DateTime) Replace(a ReplaceArgs) (DateTime, error) {
	r := replaceArgs{f: [7]*int{a.Year, a.Month, a.Day, a.Hour, a.Minute, a.Second, a.Microsecond}, fold: a.Fold}
	if a.TzinfoSet {
		r.setTz = true
		if a.Tzinfo != nil {
			z, err := safeTimezone(a.Tzinfo)
			if err != nil {
				return DateTime{}, err
			}
			r.tz = z
		}
	}
	return dt.replace(r)
}

func (dt DateTime) replace(a replaceArgs) (DateTime, error) {
	f := dt.d.Parts()
	for i, p := range a.f {
		if p != nil {
			f[i] = *p
		}
	}
	zone := zoneOf(dt.d.Tzinfo())
	if a.setTz {
		zone = a.tz
	}
	fold := dt.Fold()
	if a.fold != nil {
		fold = *a.fold
	}
	return create(f[0], f[1], f[2], f[3], f[4], f[5], f[6], zoneArg(zone), fold, false)
}

// InTimezone is dt.in_timezone(tz) (also in_tz): the same instant in tz; a naive
// datetime is read as a wall time in tz, taking the later offset where ambiguous.
func (dt DateTime) InTimezone(tz any) (DateTime, error) {
	zone, err := safeTimezone(tz)
	if err != nil {
		return DateTime{}, err
	}
	d := dt
	if dt.Timezone() == nil {
		one := 1
		if d, err = d.replace(replaceArgs{fold: &one}); err != nil {
			return DateTime{}, err
		}
	}
	return zone.Convert(d, false)
}

// InTz is dt.in_tz(tz), the same as InTimezone.
func (dt DateTime) InTz(tz any) (DateTime, error) { return dt.InTimezone(tz) }

// Astimezone is dt.astimezone(tz) for a zone: the standard library's conversion (a
// naive datetime is read in the C library's local zone).
func (dt DateTime) Astimezone(tz Zone) (DateTime, error) {
	d, err := dt.d.Astimezone(tz.tzinfo())
	return DateTime{d}, err
}

// AddArgs are the units of dt.add() and dt.subtract().
type AddArgs struct {
	Years, Months, Weeks, Days, Hours, Minutes int
	Seconds                                    float64
	Microseconds                               int
}

type addArgs struct {
	years, months, weeks, days, hours, minutes, seconds, microseconds num
}

func (a AddArgs) internal() addArgs {
	s := nf(a.Seconds)
	if a.Seconds == float64(int64(a.Seconds)) && a.Seconds < 1e15 && a.Seconds > -1e15 {
		s = ni(int(a.Seconds))
	}
	return addArgs{ni(a.Years), ni(a.Months), ni(a.Weeks), ni(a.Days), ni(a.Hours), ni(a.Minutes), s, ni(a.Microseconds)}
}

func (a addArgs) neg() addArgs {
	return addArgs{a.years.neg(), a.months.neg(), a.weeks.neg(), a.days.neg(), a.hours.neg(),
		a.minutes.neg(), a.seconds.neg(), a.microseconds.neg()}
}

// Add is dt.add(...): calendar units move the wall time and keep the zone; time units
// move the instant.
func (dt DateTime) Add(a AddArgs) (DateTime, error) { return dt.add(a.internal()) }

// Subtract is dt.subtract(...).
func (dt DateTime) Subtract(a AddArgs) (DateTime, error) { return dt.add(a.internal().neg()) }

func (dt DateTime) add(a addArgs) (DateTime, error) {
	variable := a.years.truthy() || a.months.truthy() || a.weeks.truthy() || a.days.truthy()
	f := dt.d.Parts()
	current, err := pydt.NewDatetime(f[0], f[1], f[2], f[3], f[4], f[5], f[6], nil, 0)
	if err != nil {
		return DateTime{}, err
	}
	if !variable {
		if off, ok := dt.d.UtcOffset(); ok && off != (pydt.Timedelta{}) {
			if current, err = current.SubDelta(off); err != nil {
				return DateTime{}, err
			}
		}
	}
	res, err := addDuration(current, a)
	if err != nil {
		return DateTime{}, err
	}
	zone := dt.Timezone()
	if variable || zone == nil {
		return createIn(res.Parts(), zone)
	}
	utc := res.WithTzinfo(UTC.tzinfo())
	conv, err := convertPlain(zone, utc, false)
	if err != nil {
		return DateTime{}, err
	}
	g := conv.Parts()
	return newDT(g[0], g[1], g[2], g[3], g[4], g[5], g[6], zone, conv.Fold())
}

// AddDuration is dt + d: the units d was built from.
func (dt DateTime) AddDuration(d Duration) (DateTime, error) {
	if !d.hasSig {
		return DateTime{}, newError("AttributeError", "'"+d.className()+"' object has no attribute '_signature'")
	}
	return dt.add(d.signature())
}

// SubDuration is dt - d: years and months, then the duration's own seconds.
func (dt DateTime) SubDuration(d Duration) (DateTime, error) {
	return dt.add(addArgs{years: ni(d.years), months: ni(d.months), seconds: nf(d.total)}.neg())
}

// AddInterval is dt + iv: the interval's precise units.
func (dt DateTime) AddInterval(iv Interval) (DateTime, error) {
	return dt.add(addArgs{years: ni(iv.Years()), months: ni(iv.Months()), weeks: ni(iv.Weeks()),
		days: ni(iv.RemainingDays()), hours: ni(iv.Hours()), minutes: ni(iv.Minutes()),
		seconds: ni(iv.RemainingSeconds()), microseconds: ni(iv.Microseconds())})
}

// SubInterval is dt - iv, which subtracts as a Duration does.
func (dt DateTime) SubInterval(iv Interval) (DateTime, error) {
	return dt.add(addArgs{years: ni(iv.Years()), months: ni(iv.Months()), seconds: nf(iv.dur.total)}.neg())
}

// Sub is dt - other: the Interval from other to dt.
func (dt DateTime) Sub(other DateTime) (Interval, error) { return newInterval(other, dt, false) }

// Diff is dt.diff(other, abs): the Interval from dt to other (nil is now in dt's zone).
func (dt DateTime) Diff(other *DateTime, abs bool) (Interval, error) {
	var o DateTime
	if other == nil {
		n, err := Now(zoneArg(dt.Timezone()))
		if err != nil {
			return Interval{}, err
		}
		o = n
	} else {
		o = *other
	}
	return newInterval(dt, o, abs)
}

// DiffForHumans is dt.diff_for_humans(other, absolute, locale); nil other is now (in
// the local zone) and nil locale the current locale.
func (dt DateTime) DiffForHumans(other *DateTime, absolute bool, locale *string) (string, error) {
	isNow := other == nil
	if isNow {
		n, err := Now(nil)
		if err != nil {
			return "", err
		}
		other = &n
	}
	diff, err := dt.Diff(other, true)
	if err != nil {
		return "", err
	}
	return formatDiffDuration(diff.dur, diff, isNow, absolute, locale)
}

var dtUnits = []string{"second", "minute", "hour", "day", "week", "month", "year", "decade", "century"}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func ip(v int) *int { return &v }

// StartOf is dt.start_of(unit).
func (dt DateTime) StartOf(unit string) (DateTime, error) {
	if !contains(dtUnits, unit) {
		return DateTime{}, valueError(`Invalid unit "%s" for start_of()`, unit)
	}
	y := dt.Year()
	switch unit {
	case "second":
		return dt.Set(SetArgs{Microsecond: ip(0)})
	case "minute":
		return dt.Set(SetArgs{Second: ip(0), Microsecond: ip(0)})
	case "hour":
		return dt.Set(SetArgs{Minute: ip(0), Second: ip(0), Microsecond: ip(0)})
	case "day":
		return dt.At(0, 0, 0, 0)
	case "month":
		return dt.setAll(y, dt.Month(), 1, 0, 0, 0, 0)
	case "year":
		return dt.setAll(y, 1, 1, 0, 0, 0, 0)
	case "decade":
		return dt.setAll(y-pyMod(y, 10), 1, 1, 0, 0, 0, 0)
	case "century":
		return dt.setAll(y-1-pyMod(y-1, 100)+1, 1, 1, 0, 0, 0, 0)
	}
	d := dt
	if dt.DayOfWeek() != weekStartsAt() {
		var err error
		if d, err = dt.Previous(ptr(weekStartsAt()), false); err != nil {
			return DateTime{}, err
		}
	}
	return d.StartOf("day")
}

// EndOf is dt.end_of(unit).
func (dt DateTime) EndOf(unit string) (DateTime, error) {
	if !contains(dtUnits, unit) {
		return DateTime{}, valueError(`Invalid unit "%s" for end_of()`, unit)
	}
	y := dt.Year()
	switch unit {
	case "second":
		return dt.Set(SetArgs{Microsecond: ip(999999)})
	case "minute":
		return dt.Set(SetArgs{Second: ip(59), Microsecond: ip(999999)})
	case "hour":
		return dt.Set(SetArgs{Minute: ip(59), Second: ip(59), Microsecond: ip(999999)})
	case "day":
		return dt.At(23, 59, 59, 999999)
	case "month":
		return dt.setAll(y, dt.Month(), dt.DaysInMonth(), 23, 59, 59, 999999)
	case "year":
		return dt.setAll(y, 12, 31, 23, 59, 59, 999999)
	case "decade":
		return dt.setAll(y-pyMod(y, 10)+10-1, 12, 31, 23, 59, 59, 999999)
	case "century":
		return dt.setAll(y-1-pyMod(y-1, 100)+100, 12, 31, 23, 59, 59, 999999)
	}
	d := dt
	if dt.DayOfWeek() != weekEndsAt() {
		var err error
		if d, err = dt.Next(ptr(weekEndsAt()), false); err != nil {
			return DateTime{}, err
		}
	}
	return d.EndOf("day")
}

func (dt DateTime) setAll(y, mo, d, h, mi, s, us int) (DateTime, error) {
	return dt.Set(SetArgs{Year: &y, Month: &mo, Day: &d, Hour: &h, Minute: &mi, Second: &s, Microsecond: &us})
}

func pyMod(a, b int) int {
	m := a % b
	if m != 0 && (m < 0) != (b < 0) {
		m += b
	}
	return m
}

func checkWeekday(d WeekDay) error {
	if d < MONDAY || d > SUNDAY {
		return valueError("Invalid day of week")
	}
	return nil
}

// Next is dt.next(day_of_week, keep_time): nil is the datetime's own weekday.
func (dt DateTime) Next(dayOfWeek *WeekDay, keepTime bool) (DateTime, error) {
	return dt.step(dayOfWeek, keepTime, 1)
}

// Previous is dt.previous(day_of_week, keep_time).
func (dt DateTime) Previous(dayOfWeek *WeekDay, keepTime bool) (DateTime, error) {
	return dt.step(dayOfWeek, keepTime, -1)
}

func (dt DateTime) step(dayOfWeek *WeekDay, keepTime bool, dir int) (DateTime, error) {
	dow := dt.DayOfWeek()
	if dayOfWeek != nil {
		dow = *dayOfWeek
	}
	if err := checkWeekday(dow); err != nil {
		return DateTime{}, err
	}
	d := dt
	var err error
	if !keepTime {
		if d, err = dt.StartOf("day"); err != nil {
			return DateTime{}, err
		}
	}
	for {
		if d, err = d.Add(AddArgs{Days: dir}); err != nil {
			return DateTime{}, err
		}
		if d.DayOfWeek() == dow {
			return d, nil
		}
	}
}

var ofUnits = []string{"month", "quarter", "year"}

// FirstOf is dt.first_of(unit, day_of_week) for unit month, quarter or year.
func (dt DateTime) FirstOf(unit string, dayOfWeek *WeekDay) (DateTime, error) {
	if !contains(ofUnits, unit) {
		return DateTime{}, valueError(`Invalid unit "%s" for first_of()`, unit)
	}
	switch unit {
	case "month":
		d, err := dt.StartOf("day")
		if err != nil {
			return DateTime{}, err
		}
		if dayOfWeek == nil {
			return d.Set(SetArgs{Day: ip(1)})
		}
		day, err := firstWeekdayOfMonth(d.Year(), d.Month(), int(*dayOfWeek))
		if err != nil {
			return DateTime{}, err
		}
		return d.Set(SetArgs{Day: &day})
	case "quarter":
		d, err := dt.On(dt.Year(), dt.Quarter()*3-2, 1)
		if err != nil {
			return DateTime{}, err
		}
		return d.FirstOf("month", dayOfWeek)
	}
	d, err := dt.Set(SetArgs{Month: ip(1)})
	if err != nil {
		return DateTime{}, err
	}
	return d.FirstOf("month", dayOfWeek)
}

// LastOf is dt.last_of(unit, day_of_week).
func (dt DateTime) LastOf(unit string, dayOfWeek *WeekDay) (DateTime, error) {
	if !contains(ofUnits, unit) {
		return DateTime{}, valueError(`Invalid unit "%s" for first_of()`, unit)
	}
	switch unit {
	case "month":
		d, err := dt.StartOf("day")
		if err != nil {
			return DateTime{}, err
		}
		if dayOfWeek == nil {
			return d.Set(SetArgs{Day: ip(dt.DaysInMonth())})
		}
		day, err := lastWeekdayOfMonth(d.Year(), d.Month(), int(*dayOfWeek))
		if err != nil {
			return DateTime{}, err
		}
		return d.Set(SetArgs{Day: &day})
	case "quarter":
		d, err := dt.On(dt.Year(), dt.Quarter()*3, 1)
		if err != nil {
			return DateTime{}, err
		}
		return d.LastOf("month", dayOfWeek)
	}
	d, err := dt.Set(SetArgs{Month: ip(12)})
	if err != nil {
		return DateTime{}, err
	}
	return d.LastOf("month", dayOfWeek)
}

// NthOf is dt.nth_of(unit, nth, day_of_week).
func (dt DateTime) NthOf(unit string, nth int, dayOfWeek WeekDay) (DateTime, error) {
	if !contains(ofUnits, unit) {
		return DateTime{}, valueError(`Invalid unit "%s" for first_of()`, unit)
	}
	var (
		res   DateTime
		found bool
		err   error
	)
	switch unit {
	case "month":
		res, found, err = dt.nthOfMonth(nth, dayOfWeek)
	case "quarter":
		res, found, err = dt.nthOfQuarter(nth, dayOfWeek)
	default:
		res, found, err = dt.nthOfYear(nth, dayOfWeek)
	}
	if err != nil {
		return DateTime{}, err
	}
	if !found {
		return DateTime{}, notFound(nth, dayOfWeek, unit)
	}
	return res, nil
}

func notFound(nth int, dow WeekDay, unit string) error {
	name, err := dow.capitalName()
	if err != nil {
		return err
	}
	return newError("PendulumException", fmt.Sprintf("Unable to find occurrence %d of %s in %s", nth, name, unit))
}

func (dt DateTime) advance(d DateTime, nth int, dow WeekDay) (DateTime, error) {
	n := nth
	if d.DayOfWeek() == dow {
		n--
	}
	var err error
	for i := 0; i < n; i++ {
		if d, err = d.Next(&dow, false); err != nil {
			return DateTime{}, err
		}
	}
	return d, nil
}

func (dt DateTime) nthOfMonth(nth int, dow WeekDay) (DateTime, bool, error) {
	if nth == 1 {
		d, err := dt.FirstOf("month", &dow)
		return d, err == nil, err
	}
	d, err := dt.FirstOf("month", nil)
	if err != nil {
		return DateTime{}, false, err
	}
	check, err := d.Format("%Y-%M", nil)
	if err != nil {
		return DateTime{}, false, err
	}
	if d, err = dt.advance(d, nth, dow); err != nil {
		return DateTime{}, false, err
	}
	got, err := d.Format("%Y-%M", nil)
	if err != nil {
		return DateTime{}, false, err
	}
	if got != check {
		return DateTime{}, false, nil
	}
	r, err := dt.Set(SetArgs{Day: ip(d.Day())})
	if err != nil {
		return DateTime{}, false, err
	}
	r, err = r.StartOf("day")
	return r, err == nil, err
}

func (dt DateTime) nthOfQuarter(nth int, dow WeekDay) (DateTime, bool, error) {
	if nth == 1 {
		d, err := dt.FirstOf("quarter", &dow)
		return d, err == nil, err
	}
	d, err := dt.Set(SetArgs{Day: ip(1), Month: ip(dt.Quarter() * 3)})
	if err != nil {
		return DateTime{}, false, err
	}
	lastMonth, year := d.Month(), d.Year()
	if d, err = d.FirstOf("quarter", nil); err != nil {
		return DateTime{}, false, err
	}
	if d, err = dt.advance(d, nth, dow); err != nil {
		return DateTime{}, false, err
	}
	if lastMonth < d.Month() || year != d.Year() {
		return DateTime{}, false, nil
	}
	r, err := dt.On(dt.Year(), d.Month(), d.Day())
	if err != nil {
		return DateTime{}, false, err
	}
	r, err = r.StartOf("day")
	return r, err == nil, err
}

func (dt DateTime) nthOfYear(nth int, dow WeekDay) (DateTime, bool, error) {
	if nth == 1 {
		d, err := dt.FirstOf("year", &dow)
		return d, err == nil, err
	}
	d, err := dt.FirstOf("year", nil)
	if err != nil {
		return DateTime{}, false, err
	}
	year := d.Year()
	if d, err = dt.advance(d, nth, dow); err != nil {
		return DateTime{}, false, err
	}
	if year != d.Year() {
		return DateTime{}, false, nil
	}
	r, err := dt.On(dt.Year(), d.Month(), d.Day())
	if err != nil {
		return DateTime{}, false, err
	}
	r, err = r.StartOf("day")
	return r, err == nil, err
}

// Average is dt.average(other): halfway between dt and other (nil is now in dt's zone).
func (dt DateTime) Average(other *DateTime) (DateTime, error) {
	diff, err := dt.Diff(other, false)
	if err != nil {
		return DateTime{}, err
	}
	us := (diff.InSeconds()*1000000 + diff.Microseconds())
	half := us / 2
	if us%2 != 0 && us < 0 {
		half--
	}
	return dt.add(addArgs{microseconds: ni(half)})
}

// Equal is dt == other (Python's datetime rules: a naive and an aware datetime are
// unequal; datetimes in one zone compare their wall readings).
func (dt DateTime) Equal(other DateTime) bool { return dt.d.Equal(other.d) }

// Compare orders dt and other as Python's < and > do: -1, 0 or 1, or the TypeError
// a naive/aware comparison raises.
func (dt DateTime) Compare(other DateTime) (int, error) { return dt.d.Compare(other.d) }

// Closest is dt.closest(*dts): each argument is taken in UTC when naive.
func (dt DateTime) Closest(dts ...DateTime) (DateTime, error) { return dt.pick(dts, -1) }

// Farthest is dt.farthest(*dts).
func (dt DateTime) Farthest(dts ...DateTime) (DateTime, error) { return dt.pick(dts, 1) }

func (dt DateTime) pick(dts []DateTime, want int) (DateTime, error) {
	if len(dts) == 0 {
		op := "min"
		if want > 0 {
			op = "max"
		}
		return DateTime{}, valueError("%s() iterable argument is empty", op)
	}
	type cand struct {
		iv Interval
		dt DateTime
	}
	var best cand
	for i, x := range dts {
		p, err := instance(x.d, UTC)
		if err != nil {
			return DateTime{}, err
		}
		sub, err := dt.Sub(p)
		if err != nil {
			return DateTime{}, err
		}
		iv, err := sub.Abs()
		if err != nil {
			return DateTime{}, err
		}
		c := cand{iv, p}
		if i == 0 {
			best = c
			continue
		}
		// Tuple comparison: the first unequal element decides.
		var cmp int
		if c.iv.Equal(best.iv) {
			if c.dt.Equal(best.dt) {
				continue
			}
			if cmp, err = c.dt.Compare(best.dt); err != nil {
				return DateTime{}, err
			}
		} else {
			cmp = c.iv.dur.td.Compare(best.iv.dur.td)
		}
		if cmp == want {
			best = c
		}
	}
	return best.dt, nil
}

// IsFuture is dt.is_future(): later than now in dt's zone.
func (dt DateTime) IsFuture() (bool, error) {
	n, err := Now(zoneArg(dt.Timezone()))
	if err != nil {
		return false, err
	}
	c, err := dt.Compare(n)
	return c > 0, err
}

// IsPast is dt.is_past().
func (dt DateTime) IsPast() (bool, error) {
	n, err := Now(zoneArg(dt.Timezone()))
	if err != nil {
		return false, err
	}
	c, err := dt.Compare(n)
	return c < 0, err
}

// IsSameDay is dt.is_same_day(other): the dates match (a naive other is taken in UTC).
func (dt DateTime) IsSameDay(other DateTime) (bool, error) {
	o, err := instance(other.d, UTC)
	if err != nil {
		return false, err
	}
	return dt.Date().d == o.Date().d, nil
}

// IsAnniversary is dt.is_anniversary(other) (also is_birthday): month and day match;
// nil is now in dt's zone.
func (dt DateTime) IsAnniversary(other *DateTime) (bool, error) {
	var o DateTime
	if other == nil {
		n, err := Now(zoneArg(dt.Timezone()))
		if err != nil {
			return false, err
		}
		o = n
	} else {
		o = *other
	}
	inst, err := instance(o.d, UTC)
	if err != nil {
		return false, err
	}
	return dt.Month() == inst.Month() && dt.Day() == inst.Day(), nil
}

// FromTimestamp is pendulum.from_timestamp(timestamp, tz): ts is an int or a float64
// number of seconds since the epoch; nil tz is UTC.
func FromTimestamp(ts any, tz any) (DateTime, error) {
	var arg pydt.Timestamp
	switch v := ts.(type) {
	case int:
		arg = pydt.Timestamp{Int: int64(v)}
	case float64:
		arg = pydt.Timestamp{Float: v, IsFloat: true}
	default:
		return DateTime{}, typeError(fmt.Sprintf("'%T' object cannot be interpreted as a timestamp", ts))
	}
	d, err := pydt.FromTimestampArg(arg, UTC.tzinfo())
	if err != nil {
		return DateTime{}, err
	}
	dt, err := create(d.Year(), d.Month(), d.Day(), d.Hour(), d.Minute(), d.Second(), d.Microsecond(), UTC, 1, false)
	if err != nil {
		return DateTime{}, err
	}
	if tz == nil {
		tz = UTC
	}
	return dt.InTimezone(tz)
}
