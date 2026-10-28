package pydt

import (
	"fmt"

	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// MinYear and MaxYear bound the years a date can hold.
const (
	MinYear    = 1
	MaxYear    = 9999
	maxOrdinal = 3652059
)

var (
	daysInMonthTable     = [13]int{0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	daysBeforeMonthTable = [13]int{0, 0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
)

// IsLeap is calendar.isleap.
func IsLeap(year int) bool { return year%4 == 0 && (year%100 != 0 || year%400 == 0) }

func daysInMonth(year, month int) int {
	if month == 2 && IsLeap(year) {
		return 29
	}
	return daysInMonthTable[month]
}

func daysBeforeMonth(year, month int) int {
	n := daysBeforeMonthTable[month]
	if month > 2 && IsLeap(year) {
		n++
	}
	return n
}

func daysBeforeYear(year int) int {
	y := year - 1
	return y*365 + y/4 - y/100 + y/400
}

func ymdToOrd(year, month, day int) int {
	return daysBeforeYear(year) + daysBeforeMonth(year, month) + day
}

func ordToYMD(ordinal int) (year, month, day int) {
	const di400y, di100y, di4y = 146097, 36524, 1461
	ordinal--
	n400, n := ordinal/di400y, ordinal%di400y
	year = n400*400 + 1
	n100, n := n/di100y, n%di100y
	n4, n := n/di4y, n%di4y
	n1, n := n/365, n%365
	year += n100*100 + n4*4 + n1
	if n1 == 4 || n100 == 4 {
		return year - 1, 12, 31
	}
	leap := n1 == 3 && (n4 != 24 || n100 == 3)
	month = (n + 50) >> 5
	preceding := daysBeforeMonthTable[month]
	if month > 2 && leap {
		preceding++
	}
	if preceding > n {
		month--
		preceding -= daysInMonth(year, month)
	}
	return year, month, n - preceding + 1
}

// MonthRange is calendar.monthrange: the weekday of the first day and the number of
// days in the month. Any year is accepted, as in Python.
func MonthRange(year, month int) (int, int, error) {
	if month < 1 || month > 12 {
		return 0, 0, pyrt.NewError("IllegalMonthError", fmt.Sprintf("bad month number %d; must be 1-12", month))
	}
	y := year
	if y < MinYear || y > MaxYear {
		y = 2000 + pyrt.Mod(year, 400)
	}
	weekday := (ymdToOrd(y, month, 1) + 6) % 7
	return weekday, daysInMonth(year, month), nil
}

func checkDateArgs(year, month, day int) error {
	if year < MinYear || year > MaxYear {
		return pyrt.NewError("ValueError", fmt.Sprintf("year %d is out of range", year))
	}
	if month < 1 || month > 12 {
		return pyrt.NewError("ValueError", "month must be in 1..12")
	}
	if day < 1 || day > daysInMonth(year, month) {
		return pyrt.NewError("ValueError", "day is out of range for month")
	}
	return nil
}

func checkTimeArgs(hour, minute, second, microsecond, fold int) error {
	switch {
	case hour < 0 || hour > 23:
		return pyrt.NewError("ValueError", "hour must be in 0..23")
	case minute < 0 || minute > 59:
		return pyrt.NewError("ValueError", "minute must be in 0..59")
	case second < 0 || second > 59:
		return pyrt.NewError("ValueError", "second must be in 0..59")
	case microsecond < 0 || microsecond > 999999:
		return pyrt.NewError("ValueError", "microsecond must be in 0..999999")
	case fold != 0 && fold != 1:
		return pyrt.NewError("ValueError", "fold must be either 0 or 1")
	}
	return nil
}

// cDivmod is the C helper divmod(x, y) with a floored quotient and y > 0.
func cDivmod(x, y int) (int, int) {
	q := x / y
	r := x - q*y
	if r < 0 {
		q--
		r += y
	}
	return q, r
}

func normalizePair(hi, lo *int, factor int) {
	if *lo < 0 || *lo >= factor {
		q, r := cDivmod(*lo, factor)
		*lo = r
		*hi += q
	}
}

var errDateRange = pyrt.NewError("OverflowError", "date value out of range")

func normalizeDate(y, m, d *int) error {
	dim := daysInMonth(*y, *m)
	if *d < 1 || *d > dim {
		switch {
		case *d == 0:
			*m--
			if *m > 0 {
				*d = daysInMonth(*y, *m)
			} else {
				*y--
				*m = 12
				*d = 31
			}
		case *d == dim+1:
			*m++
			*d = 1
			if *m > 12 {
				*m = 1
				*y++
			}
		default:
			ordinal := ymdToOrd(*y, *m, 1) + *d - 1
			if ordinal < 1 || ordinal > maxOrdinal {
				return errDateRange
			}
			*y, *m, *d = ordToYMD(ordinal)
			return nil
		}
	}
	if *y >= MinYear && *y <= MaxYear {
		return nil
	}
	return errDateRange
}

func normalizeDatetime(y, mo, d, h, mi, s, us *int) error {
	normalizePair(s, us, 1000000)
	normalizePair(mi, s, 60)
	normalizePair(h, mi, 60)
	normalizePair(d, h, 24)
	return normalizeDate(y, mo, d)
}
