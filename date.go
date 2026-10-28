package pendulum

import (
	"fmt"
	"math"

	"github.com/python-pendulum/pendulum/internal/pydt"
)

// Date is pendulum.Date.
type Date struct{ d pydt.Date }

// NewDate is pendulum.date(year, month, day).
func NewDate(year, month, day int) (Date, error) {
	d, err := pydt.NewDate(year, month, day)
	return Date{d}, err
}

// DateToday is pendulum.Date.today(): the local date of the (possibly frozen) clock.
func DateToday() Date { return Date{pydt.Now().Date()} }

// Year, Month and Day are the date's fields.
func (d Date) Year() int  { return d.d.Year() }
func (d Date) Month() int { return d.d.Month() }
func (d Date) Day() int   { return d.d.Day() }

// DayOfWeek is d.day_of_week.
func (d Date) DayOfWeek() WeekDay { return WeekDay(d.d.Weekday()) }

// Weekday is d.weekday() and Isoweekday d.isoweekday().
func (d Date) Weekday() int    { return d.d.Weekday() }
func (d Date) Isoweekday() int { return d.d.Isoweekday() }

// Isocalendar is d.isocalendar().
func (d Date) Isocalendar() (int, int, int) { return d.d.Isocalendar() }

// Toordinal is d.toordinal().
func (d Date) Toordinal() int { return d.d.Toordinal() }

// DayOfYear is d.day_of_year.
func (d Date) DayOfYear() int {
	k := 2
	if d.IsLeapYear() {
		k = 1
	}
	m := d.Month()
	return 275*m/9 - k*((m+9)/12) + d.Day() - 30
}

// WeekOfYear is d.week_of_year.
func (d Date) WeekOfYear() int { _, w, _ := d.d.Isocalendar(); return w }

// DaysInMonth is d.days_in_month.
func (d Date) DaysInMonth() int { return daysIn(d.Year(), d.Month()) }

// WeekOfMonth is d.week_of_month.
func (d Date) WeekOfMonth() int {
	first, _ := d.Replace(nil, nil, ip(1))
	return int(math.Ceil(float64(d.Day()+first.Isoweekday()-1) / 7))
}

// Quarter is d.quarter.
func (d Date) Quarter() int { return int(math.Ceil(float64(d.Month()) / 3)) }

// Age is d.age: whole years from d to today.
func (d Date) Age() (int, error) {
	iv, err := d.Diff(nil, false)
	if err != nil {
		return 0, err
	}
	return iv.InYears(), nil
}

// IsLeapYear is d.is_leap_year().
func (d Date) IsLeapYear() bool { return pydt.IsLeap(d.Year()) }

// IsLongYear is d.is_long_year().
func (d Date) IsLongYear() bool {
	_, w, _ := mustDate(d.Year(), 12, 28).d.Isocalendar()
	return w == 53
}

func mustDate(y, m, d int) Date {
	v, _ := NewDate(y, m, d)
	return v
}

// IsSameDay is d.is_same_day(other).
func (d Date) IsSameDay(other Date) bool { return d.d == other.d }

// IsAnniversary is d.is_anniversary(other) (also is_birthday); nil is today.
func (d Date) IsAnniversary(other *Date) bool {
	o := DateToday()
	if other != nil {
		o = *other
	}
	return d.Month() == o.Month() && d.Day() == o.Day()
}

// IsFuture is d.is_future() and IsPast d.is_past(), against today.
func (d Date) IsFuture() bool { return d.d.Compare(DateToday().d) > 0 }
func (d Date) IsPast() bool   { return d.d.Compare(DateToday().d) < 0 }

// Equal is d == other and Compare orders two dates.
func (d Date) Equal(other Date) bool  { return d.d == other.d }
func (d Date) Compare(other Date) int { return d.d.Compare(other.d) }

// Closest is d.closest(dt1, dt2) and Farthest d.farthest(dt1, dt2).
func (d Date) Closest(dt1, dt2 Date) (Date, error)  { return d.pick(dt1, dt2, -1) }
func (d Date) Farthest(dt1, dt2 Date) (Date, error) { return d.pick(dt1, dt2, 1) }

func (d Date) pick(dt1, dt2 Date, want int) (Date, error) {
	a, err := d.Diff(&dt1, true)
	if err != nil {
		return Date{}, err
	}
	b, err := d.Diff(&dt2, true)
	if err != nil {
		return Date{}, err
	}
	if (want < 0 && a.InSeconds() < b.InSeconds()) || (want > 0 && a.InSeconds() > b.InSeconds()) {
		return dt1, nil
	}
	return dt2, nil
}

// Add is d.add(years, months, weeks, days).
func (d Date) Add(years, months, weeks, days int) (Date, error) {
	return d.add(ni(years), ni(months), ni(weeks), ni(days))
}

// Subtract is d.subtract(years, months, weeks, days).
func (d Date) Subtract(years, months, weeks, days int) (Date, error) {
	return d.add(ni(-years), ni(-months), ni(-weeks), ni(-days))
}

func (d Date) add(years, months, weeks, days num) (Date, error) {
	r, err := addDurationDate(d.d, addArgs{years: years, months: months, weeks: weeks, days: days})
	return Date{r}, err
}

// AddDuration is d + dur: years, months, weeks and remaining days.
func (d Date) AddDuration(dur Duration) (Date, error) {
	return d.add(ni(dur.Years()), ni(dur.Months()), ni(dur.Weeks()), ni(dur.RemainingDays()))
}

// SubDuration is d - dur.
func (d Date) SubDuration(dur Duration) (Date, error) {
	return d.add(ni(-dur.Years()), ni(-dur.Months()), ni(-dur.Weeks()), ni(-dur.RemainingDays()))
}

// AddInterval is d + iv and SubInterval d - iv, with the interval's precise units.
func (d Date) AddInterval(iv Interval) (Date, error) {
	return d.add(ni(iv.Years()), ni(iv.Months()), ni(iv.Weeks()), ni(iv.RemainingDays()))
}

func (d Date) SubInterval(iv Interval) (Date, error) {
	return d.add(ni(-iv.Years()), ni(-iv.Months()), ni(-iv.Weeks()), ni(-iv.RemainingDays()))
}

// Sub is d - other: the Interval from other to d.
func (d Date) Sub(other Date) (Interval, error) { return newInterval(other, d, false) }

// Diff is d.diff(other, abs): nil is today.
func (d Date) Diff(other *Date, abs bool) (Interval, error) {
	o := DateToday()
	if other != nil {
		o = *other
	}
	return newInterval(d, o, abs)
}

// DiffForHumans is d.diff_for_humans(other, absolute, locale); nil other is today.
func (d Date) DiffForHumans(other *Date, absolute bool, locale *string) (string, error) {
	isNow := other == nil
	diff, err := d.Diff(other, true)
	if err != nil {
		return "", err
	}
	return formatDiffDuration(diff.dur, diff, isNow, absolute, locale)
}

var dateUnits = []string{"day", "week", "month", "year", "decade", "century"}

// StartOf is d.start_of(unit).
func (d Date) StartOf(unit string) (Date, error) {
	if !contains(dateUnits, unit) {
		return Date{}, valueError(`Invalid unit "%s" for start_of()`, unit)
	}
	y := d.Year()
	switch unit {
	case "day":
		return d, nil
	case "month":
		return d.set(y, d.Month(), 1)
	case "year":
		return d.set(y, 1, 1)
	case "decade":
		return d.set(y-pyMod(y, 10), 1, 1)
	case "century":
		return d.set(y-1-pyMod(y-1, 100)+1, 1, 1)
	}
	r := d
	if d.DayOfWeek() != weekStartsAt() {
		var err error
		if r, err = d.Previous(ptr(weekStartsAt())); err != nil {
			return Date{}, err
		}
	}
	return r, nil
}

// EndOf is d.end_of(unit).
func (d Date) EndOf(unit string) (Date, error) {
	if !contains(dateUnits, unit) {
		return Date{}, valueError(`Invalid unit "%s" for end_of()`, unit)
	}
	y := d.Year()
	switch unit {
	case "day":
		return d, nil
	case "month":
		return d.set(y, d.Month(), d.DaysInMonth())
	case "year":
		return d.set(y, 12, 31)
	case "decade":
		return d.set(y-pyMod(y, 10)+10-1, 12, 31)
	case "century":
		return d.set(y-1-pyMod(y-1, 100)+100, 12, 31)
	}
	r := d
	if d.DayOfWeek() != weekEndsAt() {
		var err error
		if r, err = d.Next(ptr(weekEndsAt())); err != nil {
			return Date{}, err
		}
	}
	return r, nil
}

func (d Date) set(y, m, day int) (Date, error) { return NewDate(y, m, day) }

// Replace is d.replace(year, month, day); nil keeps a field.
func (d Date) Replace(year, month, day *int) (Date, error) {
	r, err := d.d.Replace(year, month, day)
	return Date{r}, err
}

// Next is d.next(day_of_week) and Previous d.previous(day_of_week); nil is d's weekday.
func (d Date) Next(dayOfWeek *WeekDay) (Date, error)     { return d.step(dayOfWeek, 1) }
func (d Date) Previous(dayOfWeek *WeekDay) (Date, error) { return d.step(dayOfWeek, -1) }

func (d Date) step(dayOfWeek *WeekDay, dir int) (Date, error) {
	dow := d.DayOfWeek()
	if dayOfWeek != nil {
		dow = *dayOfWeek
	}
	if err := checkWeekday(dow); err != nil {
		return Date{}, err
	}
	r := d
	var err error
	for {
		if r, err = r.Add(0, 0, 0, dir); err != nil {
			return Date{}, err
		}
		if r.DayOfWeek() == dow {
			return r, nil
		}
	}
}

// FirstOf is d.first_of(unit, day_of_week).
func (d Date) FirstOf(unit string, dayOfWeek *WeekDay) (Date, error) {
	if !contains(ofUnits, unit) {
		return Date{}, valueError(`Invalid unit "%s" for first_of()`, unit)
	}
	switch unit {
	case "month":
		if dayOfWeek == nil {
			return d.Replace(nil, nil, ip(1))
		}
		day, err := firstWeekdayOfMonth(d.Year(), d.Month(), int(*dayOfWeek))
		if err != nil {
			return Date{}, err
		}
		return d.Replace(nil, nil, &day)
	case "quarter":
		r, err := d.set(d.Year(), d.Quarter()*3-2, 1)
		if err != nil {
			return Date{}, err
		}
		return r.FirstOf("month", dayOfWeek)
	}
	r, err := d.Replace(nil, ip(1), nil)
	if err != nil {
		return Date{}, err
	}
	return r.FirstOf("month", dayOfWeek)
}

// LastOf is d.last_of(unit, day_of_week).
func (d Date) LastOf(unit string, dayOfWeek *WeekDay) (Date, error) {
	if !contains(ofUnits, unit) {
		return Date{}, valueError(`Invalid unit "%s" for first_of()`, unit)
	}
	switch unit {
	case "month":
		if dayOfWeek == nil {
			return d.Replace(nil, nil, ip(d.DaysInMonth()))
		}
		day, err := lastWeekdayOfMonth(d.Year(), d.Month(), int(*dayOfWeek))
		if err != nil {
			return Date{}, err
		}
		return d.Replace(nil, nil, &day)
	case "quarter":
		r, err := d.set(d.Year(), d.Quarter()*3, 1)
		if err != nil {
			return Date{}, err
		}
		return r.LastOf("month", dayOfWeek)
	}
	r, err := d.Replace(nil, ip(12), nil)
	if err != nil {
		return Date{}, err
	}
	return r.LastOf("month", dayOfWeek)
}

// NthOf is d.nth_of(unit, nth, day_of_week).
func (d Date) NthOf(unit string, nth int, dayOfWeek WeekDay) (Date, error) {
	if !contains(ofUnits, unit) {
		return Date{}, valueError(`Invalid unit "%s" for first_of()`, unit)
	}
	var (
		r     Date
		found bool
		err   error
	)
	switch unit {
	case "month":
		r, found, err = d.nthOfMonth(nth, dayOfWeek)
	case "quarter":
		r, found, err = d.nthOfQuarter(nth, dayOfWeek)
	default:
		r, found, err = d.nthOfYear(nth, dayOfWeek)
	}
	if err != nil {
		return Date{}, err
	}
	if !found {
		return Date{}, notFound(nth, dayOfWeek, unit)
	}
	return r, nil
}

func (d Date) advance(r Date, nth int, dow WeekDay) (Date, error) {
	n := nth
	if r.DayOfWeek() == dow {
		n--
	}
	var err error
	for i := 0; i < n; i++ {
		if r, err = r.Next(&dow); err != nil {
			return Date{}, err
		}
	}
	return r, nil
}

func (d Date) nthOfMonth(nth int, dow WeekDay) (Date, bool, error) {
	if nth == 1 {
		r, err := d.FirstOf("month", &dow)
		return r, err == nil, err
	}
	r, err := d.FirstOf("month", nil)
	if err != nil {
		return Date{}, false, err
	}
	check := fmt.Sprintf("%d-%02d", r.Year(), r.Month())
	if r, err = d.advance(r, nth, dow); err != nil {
		return Date{}, false, err
	}
	if fmt.Sprintf("%d-%02d", r.Year(), r.Month()) != check {
		return Date{}, false, nil
	}
	out, err := d.Replace(nil, nil, ip(r.Day()))
	return out, err == nil, err
}

func (d Date) nthOfQuarter(nth int, dow WeekDay) (Date, bool, error) {
	if nth == 1 {
		r, err := d.FirstOf("quarter", &dow)
		return r, err == nil, err
	}
	r, err := d.set(d.Year(), d.Quarter()*3, 1)
	if err != nil {
		return Date{}, false, err
	}
	lastMonth, year := r.Month(), r.Year()
	if r, err = r.FirstOf("quarter", nil); err != nil {
		return Date{}, false, err
	}
	if r, err = d.advance(r, nth, dow); err != nil {
		return Date{}, false, err
	}
	if lastMonth < r.Month() || year != r.Year() {
		return Date{}, false, nil
	}
	out, err := d.set(d.Year(), r.Month(), r.Day())
	return out, err == nil, err
}

func (d Date) nthOfYear(nth int, dow WeekDay) (Date, bool, error) {
	if nth == 1 {
		r, err := d.FirstOf("year", &dow)
		return r, err == nil, err
	}
	r, err := d.FirstOf("year", nil)
	if err != nil {
		return Date{}, false, err
	}
	year := r.Year()
	if r, err = d.advance(r, nth, dow); err != nil {
		return Date{}, false, err
	}
	if year != r.Year() {
		return Date{}, false, nil
	}
	out, err := d.set(d.Year(), r.Month(), r.Day())
	return out, err == nil, err
}

// Average is d.average(other): nil is today.
func (d Date) Average(other *Date) (Date, error) {
	o := DateToday()
	if other != nil {
		o = *other
	}
	iv, err := d.Diff(&o, false)
	if err != nil {
		return Date{}, err
	}
	return d.Add(0, 0, 0, int(float64(iv.InDays())/2))
}

// Isoformat is d.isoformat() and String str(d).
func (d Date) Isoformat() string { return d.d.Isoformat() }
func (d Date) String() string    { return d.d.Isoformat() }

// Repr is repr(d): Date(2024, 3, 10).
func (d Date) Repr() string { return fmt.Sprintf("Date(%d, %d, %d)", d.Year(), d.Month(), d.Day()) }

// ToDateString is d.to_date_string() and ToFormattedDateString
// d.to_formatted_date_string().
func (d Date) ToDateString() (string, error)          { return d.Strftime("%Y-%m-%d") }
func (d Date) ToFormattedDateString() (string, error) { return d.Strftime("%b %d, %Y") }

// ForJSON is d.for_json().
func (d Date) ForJSON() string { return d.Isoformat() }
