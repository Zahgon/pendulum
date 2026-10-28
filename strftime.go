package pendulum

import (
	"github.com/python-pendulum/pendulum/internal/clocal"
	"github.com/python-pendulum/pendulum/internal/strftime"
)

func strftimeEnv() strftime.Env {
	return strftime.Env{
		TZName: clocal.LiveTzName(),
		Mktime: func(t strftime.Tm) (int64, bool) {
			wall := int64(daysFromCivil(t.Year, t.Month, t.Day))*86400 + int64(t.Hour*3600+t.Minute*60+t.Second)
			return clocal.Mktime(wall, t.IsDST)
		},
	}
}

// daysFromCivil is the day count from 1970-01-01 of a proleptic Gregorian date.
func daysFromCivil(y, m, d int) int {
	if m <= 2 {
		y--
	}
	era := floorDiv(y, 400)
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// Strftime is dt.strftime(fmt), as CPython and the C library render it.
func (dt DateTime) Strftime(format string) (string, error) {
	t := strftime.Tm{Year: dt.Year(), Month: dt.Month(), Day: dt.Day(), Hour: dt.Hour(),
		Minute: dt.Minute(), Second: dt.Second(), Microsecond: dt.Microsecond(),
		Weekday: dt.Weekday(), Yday: dt.Date().d.Yearday(), IsDST: -1}
	if dst, ok := dt.d.Dst(); ok {
		t.IsDST = 0
		if dst.TotalMicroseconds() != 0 {
			t.IsDST = 1
		}
	}
	if off, ok := dt.d.UtcOffset(); ok {
		us := off.TotalMicroseconds()
		t.UTCOffset = &us
	}
	if n, ok := dt.d.TzName(); ok {
		t.TZName = &n
	}
	return strftime.Format(format, t, strftimeEnv())
}

// Strftime is d.strftime(fmt).
func (d Date) Strftime(format string) (string, error) {
	t := strftime.Tm{Year: d.Year(), Month: d.Month(), Day: d.Day(), Weekday: d.Weekday(),
		Yday: d.d.Yearday(), IsDST: -1}
	return strftime.Format(format, t, strftimeEnv())
}

// Strftime is t.strftime(fmt): the date part reads 1900-01-01.
func (t Time) Strftime(format string) (string, error) {
	tm := strftime.Tm{Year: 1900, Month: 1, Day: 1, Hour: t.Hour(), Minute: t.Minute(), Second: t.Second(),
		Microsecond: t.Microsecond(), Weekday: 0, Yday: 1, IsDST: -1}
	return strftime.Format(format, tm, strftimeEnv())
}

// ToDateString is dt.to_date_string() and ToFormattedDateString
// dt.to_formatted_date_string(), both through strftime.
func (dt DateTime) ToDateString() (string, error)          { return dt.Strftime("%Y-%m-%d") }
func (dt DateTime) ToFormattedDateString() (string, error) { return dt.Strftime("%b %d, %Y") }
