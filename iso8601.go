package pendulum

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/python-pendulum/pendulum/internal/pydt"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// pyDigit is the character class of Python's \d on str patterns: every character with
// a decimal digit value (Unicode category Nd as CPython 3.12 knows it).
var pyDigit = func() string {
	var b strings.Builder
	b.WriteString("[")
	start := rune(-1)
	for r := rune(0); r <= utf8.MaxRune+1; r++ {
		in := r <= utf8.MaxRune && pyrt.DecimalValue(r) >= 0
		if in && start < 0 {
			start = r
		}
		if !in && start >= 0 {
			if start == r-1 {
				fmt.Fprintf(&b, `\x{%x}`, start)
			} else {
				fmt.Fprintf(&b, `\x{%x}-\x{%x}`, start, r-1)
			}
			start = -1
		}
	}
	b.WriteString("]")
	return b.String()
}()

func pyRegexp(pattern string) *regexp.Regexp {
	return regexp.MustCompile(strings.ReplaceAll(pattern, `\d`, pyDigit))
}

var iso8601DT = pyRegexp(`^(?P<date>(?P<classic>(?P<year>\d{4})(?P<monthday>(?P<monthsep>-)?(?P<month>\d{2})((?P<daysep>-)?(?P<day>\d{1,2}))?)?)` +
	`|(?P<isocalendar>(?P<isoyear>\d{4})(?P<weeksep>-)?W(?P<isoweek>\d{2})(?P<weekdaysep>-)?(?P<isoweekday>\d)?))?` +
	`(?P<time>(?P<timesep>[T ])?(?P<hour>\d{1,2})(?P<minsep>:)?(?P<minute>\d{1,2})?(?P<secsep>:)?(?P<second>\d{1,2})?` +
	`(?P<subsecondsection>(?:[.,])(?P<subsecond>\d{1,9}))?(?P<tz>(?:[-+])\d{2}:?(?:\d{2})?|Z)?)?$`)

var iso8601Duration = pyRegexp(`^P(?P<w>(?P<weeks>\d+(?:[.,]\d+)?W))?` +
	`(?P<ymd>(?P<years>\d+(?:[.,]\d+)?Y)?(?P<months>\d+(?:[.,]\d+)?M)?(?P<days>\d+(?:[.,]\d+)?D)?)?` +
	`(?P<hms>(?P<timesep>T)(?P<hours>\d+(?:[.,]\d+)?H)?(?P<minutes>\d+(?:[.,]\d+)?M)?(?P<seconds>\d+(?:[.,]\d+)?S)?)?$`)

// match is a regexp match with Python's group() accessors.
type match struct {
	re  *regexp.Regexp
	s   string
	idx []int
}

func fullmatch(re *regexp.Regexp, s string) *match {
	idx := re.FindStringSubmatchIndex(s)
	if idx == nil {
		return nil
	}
	return &match{re, s, idx}
}

// group returns the group's text and whether it participated.
func (m *match) group(name string) (string, bool) {
	i := m.re.SubexpIndex(name)
	if m.idx[2*i] < 0 {
		return "", false
	}
	return m.s[m.idx[2*i]:m.idx[2*i+1]], true
}

// g is group(name) or "" (Python's falsy None or empty match).
func (m *match) g(name string) string { s, _ := m.group(name); return s }

func (m *match) start(name string) int { return m.idx[2*m.re.SubexpIndex(name)] }

func parserError(format string, args ...any) error {
	return pyrt.NewError("ParserError", fmt.Sprintf(format, args...))
}

func pyInt(s string) int {
	v, _ := pyrt.ParseInt(s)
	return v
}

// isoResult is what parse_iso8601 returns: a datetime, date, time or Duration.
type isoResult struct {
	kind string // "datetime", "date", "time", "duration"
	dt   pydt.Datetime
	date pydt.Date
	t    pydt.Time
	tz   Zone // the time's zone, for a time with an offset
	dur  Duration
}

// parseISO8601 is the pure-Python parse_iso8601.
func parseISO8601(text string) (isoResult, error) {
	if d, ok, err := parseISO8601Duration(text); ok || err != nil {
		return isoResult{kind: "duration", dur: d}, err
	}
	m := fullmatch(iso8601DT, text)
	if m == nil {
		return isoResult{}, parserError("Invalid ISO 8601 string")
	}
	ambiguous, isDate, isTime := false, false, false
	year, month, day := 0, 1, 1
	minute, second, microsecond := 0, 0, 0
	var tz Zone
	if m.g("date") != "" {
		isDate = true
		if m.g("isocalendar") != "" {
			if m.g("weeksep") != "" && m.g("weekdaysep") == "" && m.g("isoweekday") != "" {
				return isoResult{}, parserError("Invalid date string: %s", text)
			}
			if m.g("weeksep") == "" && m.g("weekdaysep") != "" {
				return isoResult{}, parserError("Invalid date string: %s", text)
			}
			y, mo, d, err := isoWeekDate(m.g("isoyear"), m.g("isoweek"), m.g("isoweekday"))
			if err != nil {
				if pe, ok := err.(*pyrt.Error); ok && pe.Type == "ParserError" {
					return isoResult{}, err
				}
				return isoResult{}, parserError("Invalid date string: %s", text)
			}
			year, month, day = y, mo, d
		} else {
			year = pyInt(m.g("year"))
			if m.g("monthday") != "" {
				if m.g("month") != "" && m.g("day") != "" {
					if m.g("daysep") == "" && utf8.RuneCountInString(m.g("day")) == 1 {
						ordinal := pyInt(m.g("month") + m.g("day"))
						offsets := monthsOffsets[leapIndex(year)]
						if ordinal > offsets[13] {
							return isoResult{}, parserError("Ordinal day is out of range")
						}
						for i := 1; i < 14; i++ {
							if ordinal <= offsets[i] {
								day = ordinal - offsets[i-1]
								month = i - 1
								break
							}
						}
					} else {
						month = pyInt(m.g("month"))
						day = pyInt(m.g("day"))
					}
				} else {
					if m.g("monthsep") == "" {
						ambiguous = true
					}
					month = pyInt(m.g("month"))
					day = 1
				}
			}
		}
	}
	if m.g("time") == "" {
		if ambiguous {
			rs := []rune(fmt.Sprintf("%d%02d", year, month))
			var hms [3]int
			for i, part := range []string{pySlice(rs, 0, 2), pySlice(rs, 2, 4), pySlice(rs, 4, len(rs))} {
				v, err := pyrt.ParseInt(part)
				if err != nil {
					return isoResult{}, err
				}
				hms[i] = v
			}
			t, err := pydt.NewTime(hms[0], hms[1], hms[2], 0, nil)
			return isoResult{kind: "time", t: t}, err
		}
		d, err := pydt.NewDate(year, month, day)
		return isoResult{kind: "date", date: d}, err
	}
	if ambiguous {
		return isoResult{}, parserError("Invalid date string: %s", text)
	}
	if isDate && m.g("timesep") == "" {
		return isoResult{}, parserError("Invalid date string: %s", text)
	}
	if !isDate {
		isTime = true
	}
	hour := pyInt(m.g("hour"))
	minsep := m.g("minsep")
	if m.g("minute") != "" {
		minute = pyInt(m.g("minute"))
	} else if minsep != "" {
		return isoResult{}, parserError("Invalid ISO 8601 time part")
	}
	secsep := m.g("secsep")
	if secsep != "" && minsep == "" && m.g("minute") != "" {
		return isoResult{}, parserError("Invalid ISO 8601 time part")
	}
	if m.g("second") != "" {
		if secsep == "" && minsep != "" {
			return isoResult{}, parserError("Invalid ISO 8601 time part")
		}
		second = pyInt(m.g("second"))
	} else if secsep != "" {
		return isoResult{}, parserError("Invalid ISO 8601 time part")
	}
	if m.g("subsecondsection") != "" {
		microsecond = subsecondMicros(m.g("subsecond"))
	}
	if t := m.g("tz"); t != "" {
		if t == "Z" {
			tz = UTC
		} else {
			negative := strings.HasPrefix(t, "-")
			t = t[1:]
			var offH, offM string
			if !strings.Contains(t, ":") {
				if utf8.RuneCountInString(t) == 2 {
					t += "00"
				}
				rs := []rune(t)
				offH, offM = string(rs[0:2]), string(rs[2:4])
			} else {
				offH, offM, _ = strings.Cut(t, ":")
			}
			offset := (pyInt(offH)*60 + pyInt(offM)) * 60
			if negative {
				offset = -offset
			}
			tz = NewFixedTimezone(offset, nil)
		}
	}
	var ti pydt.Tzinfo
	if tz != nil {
		ti = tz.tzinfo()
	}
	if isTime {
		t, err := pydt.NewTime(hour, minute, second, microsecond, ti)
		return isoResult{kind: "time", t: t, tz: tz}, err
	}
	d, err := pydt.NewDatetime(year, month, day, hour, minute, second, microsecond, ti, 0)
	return isoResult{kind: "datetime", dt: d}, err
}

// pySlice is s[i:j] on code points, clamped as Python clamps.
func pySlice(rs []rune, i, j int) string {
	if i > len(rs) {
		i = len(rs)
	}
	if j > len(rs) {
		j = len(rs)
	}
	return string(rs[i:j])
}

// subsecondMicros is int(f"{subsecond[:6]:0<6}").
func subsecondMicros(s string) int {
	rs := []rune(s)
	if len(rs) > 6 {
		rs = rs[:6]
	}
	for len(rs) < 6 {
		rs = append(rs, '0')
	}
	return pyInt(string(rs))
}

// isoWeekDate is _get_iso_8601_week; its strptime("%Y-%j") only accepts four-digit years.
func isoWeekDate(yearS, weekS, weekdayS string) (int, int, int, error) {
	weekday := 1
	if weekdayS != "" {
		weekday = pyInt(weekdayS)
	}
	year, week := pyInt(yearS), pyInt(weekS)
	if week > 53 || (week > 52 && !isLongYear(year)) {
		return 0, 0, 0, parserError("Invalid week for week date")
	}
	if weekday > 7 {
		return 0, 0, 0, parserError("Invalid weekday for week date")
	}
	ordinal := week*7 + weekday - (weekDay(year, 1, 4) + 3)
	if ordinal < 1 {
		ordinal += daysInYear(year - 1)
		year--
	}
	if ordinal > daysInYear(year) {
		ordinal -= daysInYear(year)
		year++
	}
	if year < 1000 || year > 9999 || ordinal < 1 || ordinal > daysInYear(year) {
		return 0, 0, 0, valueError("time data '%d-%d' does not match format '%%Y-%%j'", year, ordinal)
	}
	d, err := pydt.DateFromOrdinal(pydt.MustOrdinal(year, 1, 1) + ordinal - 1)
	if err != nil {
		return 0, 0, 0, err
	}
	return d.Year(), d.Month(), d.Day(), nil
}

// parseISO8601Duration is _parse_iso8601_duration: ok is false when text is not a
// duration at all.
func parseISO8601Duration(text string) (Duration, bool, error) {
	m := fullmatch(iso8601Duration, text)
	if m == nil || (m.g("w") == "" && m.g("ymd") == "" && m.g("hms") == "") {
		return Duration{}, false, nil
	}
	var (
		years, months, weeks                  = 0, 0, 0
		days, hours, minutes, seconds, micros = ni(0), ni(0), ni(0), ni(0), ni(0)
		fractional                            = false
	)
	invalid := func() (Duration, bool, error) { return Duration{}, true, parserError("Invalid duration") }
	if m.g("w") != "" {
		if m.g("ymd") != "" || m.g("hms") != "" {
			return Duration{}, true, parserError("Invalid duration string")
		}
		w := strings.ReplaceAll(strings.ReplaceAll(m.g("weeks"), ",", "."), "W", "")
		if strings.Contains(w, ".") {
			whole, portion, _ := strings.Cut(w, ".")
			weeks = pyInt(whole)
			d := float64(pyInt(portion)) / 10 * 7
			q, r := pyrt.FloatDivmod(d, 1)
			days = ni(int(q))
			hours = ni(int(r * 24))
		} else {
			weeks = pyInt(w)
		}
	}
	if m.g("ymd") != "" {
		ys, ms, ds := m.g("years"), m.g("months"), m.g("days")
		ystart := -3
		if ys != "" {
			ystart = m.start("years")
		}
		mstart := ystart + 1
		if ms != "" {
			mstart = m.start("months")
		}
		dstart := mstart + 1
		if ds != "" {
			dstart = m.start("days")
		}
		if !(ystart < mstart && mstart < dstart) {
			return invalid()
		}
		if ys != "" {
			ys = strings.ReplaceAll(strings.ReplaceAll(ys, ",", "."), "Y", "")
			if strings.Contains(ys, ".") {
				return Duration{}, true, parserError("Float years in duration are not supported")
			}
			years = pyInt(ys)
		}
		if ms != "" {
			if fractional {
				return invalid()
			}
			ms = strings.ReplaceAll(strings.ReplaceAll(ms, ",", "."), "M", "")
			if strings.Contains(ms, ".") {
				return Duration{}, true, parserError("Float months in duration are not supported")
			}
			months = pyInt(ms)
		}
		if ds != "" {
			if fractional {
				return invalid()
			}
			ds = strings.ReplaceAll(strings.ReplaceAll(ds, ",", "."), "D", "")
			if strings.Contains(ds, ".") {
				fractional = true
				whole, frac, _ := strings.Cut(ds, ".")
				days = ni(pyInt(whole))
				hours = nf(float64(pyInt(frac)) / 10 * 24)
			} else {
				days = ni(pyInt(ds))
			}
		}
	}
	if m.g("hms") != "" {
		hs, mis, ss := m.g("hours"), m.g("minutes"), m.g("seconds")
		hstart := -3
		if hs != "" {
			hstart = m.start("hours")
		}
		mstart := hstart + 1
		if mis != "" {
			mstart = m.start("minutes")
		}
		sstart := mstart + 1
		if ss != "" {
			sstart = m.start("seconds")
		}
		if !(hstart < mstart && mstart < sstart) {
			return invalid()
		}
		if hs != "" {
			if fractional {
				return invalid()
			}
			hs = strings.ReplaceAll(strings.ReplaceAll(hs, ",", "."), "H", "")
			if strings.Contains(hs, ".") {
				fractional = true
				whole, frac, _ := strings.Cut(hs, ".")
				hours = hours.add(ni(pyInt(whole)))
				minutes = minutes.add(nf(float64(pyInt(frac)) / 10 * 60))
			} else {
				hours = hours.add(ni(pyInt(hs)))
			}
		}
		if mis != "" {
			if fractional {
				return invalid()
			}
			mis = strings.ReplaceAll(strings.ReplaceAll(mis, ",", "."), "M", "")
			if strings.Contains(mis, ".") {
				fractional = true
				whole, frac, _ := strings.Cut(mis, ".")
				minutes = minutes.add(ni(pyInt(whole)))
				seconds = seconds.add(nf(float64(pyInt(frac)) / 10 * 60))
			} else {
				minutes = minutes.add(ni(pyInt(mis)))
			}
		}
		if ss != "" {
			if fractional {
				return invalid()
			}
			ss = strings.ReplaceAll(strings.ReplaceAll(ss, ",", "."), "S", "")
			if strings.Contains(ss, ".") {
				whole, frac, _ := strings.Cut(ss, ".")
				seconds = seconds.add(ni(pyInt(whole)))
				micros = micros.add(ni(subsecondMicros(frac)))
			} else {
				seconds = seconds.add(ni(pyInt(ss)))
			}
		}
	}
	d, err := newDuration(durationArgs{years: ni(years), months: ni(months), weeks: ni(weeks), days: days,
		hours: hours, minutes: minutes, seconds: seconds, microseconds: micros})
	return d, true, err
}
