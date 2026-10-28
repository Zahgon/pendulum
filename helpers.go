package pendulum

import (
	"github.com/python-pendulum/pendulum/internal/pydt"
)

// normalizeUnits is the normalising half of pendulum's add_duration.
func normalizeUnits(a addArgs) addArgs {
	a.days = a.days.add(a.weeks.mul(ni(7)))
	carry := func(lo, hi *num, limit, factor int) {
		if lo.abs().gt(ni(limit)) {
			s := lo.sign()
			div, mod := lo.mul(s).divmod(ni(factor))
			*lo = mod.mul(s)
			*hi = hi.add(div.mul(s))
		}
	}
	carry(&a.microseconds, &a.seconds, 999999, 1000000)
	carry(&a.seconds, &a.minutes, 59, 60)
	carry(&a.minutes, &a.hours, 59, 60)
	carry(&a.hours, &a.days, 23, 24)
	carry(&a.months, &a.years, 11, 12)
	return a
}

func calendarTarget(year, month, day int, a addArgs) (int, int, int) {
	y := year + int(a.years.i)
	m := month
	if a.months.truthy() {
		m += int(a.months.i)
		if m > 12 {
			y++
			m -= 12
		} else if m < 1 {
			y--
			m += 12
		}
	}
	return y, m, min(daysPerMonth[leapIndex(y)][m], day)
}

// addDuration is pendulum's add_duration for a plain naive datetime.
func addDuration(dt pydt.Datetime, a addArgs) (pydt.Datetime, error) {
	a = normalizeUnits(a)
	y, m, d := calendarTarget(dt.Year(), dt.Month(), dt.Day(), a)
	r, err := dt.Replace(pydt.Fields{Year: &y, Month: &m, Day: &d})
	if err != nil {
		return pydt.Datetime{}, err
	}
	td, err := timedelta(a.days, a.hours, a.minutes, a.seconds, a.microseconds)
	if err != nil {
		return pydt.Datetime{}, err
	}
	return r.AddDelta(td)
}

// addDurationDate is add_duration for a date: only years, months, weeks and days.
func addDurationDate(dt pydt.Date, a addArgs) (pydt.Date, error) {
	if a.hours.truthy() || a.minutes.truthy() || a.seconds.truthy() || a.microseconds.truthy() {
		return pydt.Date{}, newError("RuntimeError", "Time elements cannot be added to a date instance.")
	}
	a = normalizeUnits(a)
	y, m, d := calendarTarget(dt.Year(), dt.Month(), dt.Day(), a)
	r, err := pydt.NewDate(y, m, d)
	if err != nil {
		return pydt.Date{}, err
	}
	td, err := timedelta(a.days, a.hours, a.minutes, a.seconds, a.microseconds)
	if err != nil {
		return pydt.Date{}, err
	}
	return r.Add(td)
}

// preciseDiff is pendulum's precise_diff between two plain datetimes or two dates.
type preciseDiff struct {
	years, months, days, hours, minutes, seconds, microseconds, totalDays int
}

// point is a plain datetime or a date, as precise_diff and Interval see them.
type point struct {
	isDate bool
	dt     pydt.Datetime
	date   pydt.Date
}

func (p point) ymd() (int, int, int) {
	if p.isDate {
		return p.date.Year(), p.date.Month(), p.date.Day()
	}
	return p.dt.Year(), p.dt.Month(), p.dt.Day()
}

func (p point) equal(o point) bool {
	if p.isDate && o.isDate {
		return p.date == o.date
	}
	if !p.isDate && !o.isDate {
		return p.dt.Equal(o.dt)
	}
	return false
}

func (p point) compare(o point) (int, error) {
	if p.isDate && o.isDate {
		return p.date.Compare(o.date), nil
	}
	if !p.isDate && !o.isDate {
		return p.dt.Compare(o.dt)
	}
	return 0, typeError("can't compare datetime.datetime to datetime.date")
}

func tzName(t pydt.Tzinfo) (string, bool) {
	if z := zoneOf(t); z != nil {
		return z.Name(), true
	}
	return "", false
}

func computePreciseDiff(d1, d2 point) (preciseDiff, error) {
	sign := 1
	if d1.equal(d2) {
		return preciseDiff{}, nil
	}
	var tz1, tz2 pydt.Tzinfo
	if !d1.isDate {
		tz1 = d1.dt.Tzinfo()
	}
	if !d2.isDate {
		tz2 = d2.dt.Tzinfo()
	}
	if (tz1 == nil) != (tz2 == nil) {
		return preciseDiff{}, valueError("Comparison between naive and aware datetimes is not supported")
	}
	c, err := d1.compare(d2)
	if err != nil {
		return preciseDiff{}, err
	}
	if c > 0 {
		d1, d2 = d2, d1
		sign = -1
	}
	var dDiff, hourDiff, minDiff, secDiff, micDiff int
	y1, m1, dd1 := d1.ymd()
	y2, m2, dd2 := d2.ymd()
	totalDays := dayNumber(y2, m2, dd2) - dayNumber(y1, m1, dd1)
	inSameTz := false
	if tz1 != nil && tz2 != nil {
		n1, ok1 := tzName(tz1)
		n2, ok2 := tzName(tz2)
		inSameTz = ok1 && ok2 && n1 == n2
	}
	if !d2.isDate {
		if !d1.isDate {
			a, b := d1.dt, d2.dt
			if !inSameTz || totalDays == 0 {
				if off, ok := a.UtcOffset(); ok && off != (pydt.Timedelta{}) {
					if a, err = a.SubDelta(off); err != nil {
						return preciseDiff{}, err
					}
				}
				if off, ok := b.UtcOffset(); ok && off != (pydt.Timedelta{}) {
					if b, err = b.SubDelta(off); err != nil {
						return preciseDiff{}, err
					}
				}
			}
			hourDiff = b.Hour() - a.Hour()
			minDiff = b.Minute() - a.Minute()
			secDiff = b.Second() - a.Second()
			micDiff = b.Microsecond() - a.Microsecond()
			d1 = point{dt: a}
			d2 = point{dt: b}
		} else {
			hourDiff = d2.dt.Hour()
			minDiff = d2.dt.Minute()
			secDiff = d2.dt.Second()
			micDiff = d2.dt.Microsecond()
		}
		if micDiff < 0 {
			micDiff += 1000000
			secDiff--
		}
		if secDiff < 0 {
			secDiff += 60
			minDiff--
		}
		if minDiff < 0 {
			minDiff += 60
			hourDiff--
		}
		if hourDiff < 0 {
			hourDiff += 24
			dDiff--
		}
	}
	y1, m1, dd1 = d1.ymd()
	y2, m2, dd2 = d2.ymd()
	yDiff := y2 - y1
	mDiff := m2 - m1
	dDiff += dd2 - dd1
	if dDiff < 0 {
		year, month := y2, m2
		if month == 1 {
			month = 12
			year--
		} else {
			month--
		}
		daysInLastMonth := daysPerMonth[leapIndex(year)][month]
		daysInMonth := daysPerMonth[leapIndex(y2)][m2]
		switch {
		case dDiff < daysInMonth-daysInLastMonth:
			if daysInLastMonth < dd1 {
				dDiff += dd1
			} else {
				dDiff += daysInLastMonth
			}
		case dDiff == daysInMonth-daysInLastMonth:
			dDiff = 0
			mDiff++
		default:
			dDiff += daysInLastMonth
		}
		mDiff--
	}
	if mDiff < 0 {
		mDiff += 12
		yDiff--
	}
	return preciseDiff{sign * yDiff, sign * mDiff, sign * dDiff, sign * hourDiff, sign * minDiff,
		sign * secDiff, sign * micDiff, sign * totalDays}, nil
}
