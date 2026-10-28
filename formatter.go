package pendulum

import (
	"fmt"
	"regexp"
	"strings"
)

// formatRE is Formatter._TOKENS: a bracketed literal, an escaped character or a token.
var formatRE = regexp.MustCompile(`\[([^\[]*)\]|\\(.)|` +
	`(Mo|MM?M?M?|Do|DDDo|DD?D?D?|ddd?d?|do?|eo?|E{1,4}|w[o|w]?|W[o|W]?|Qo?|YYYY|YY|Y|` +
	`gg(ggg?)?|GG(GGG?)?|a|A|hh?|HH?|kk?|mm?|ss?|S{1,9}|x|X|zz?|ZZ?|LTS|LT|LL?L?L?)`)

var dateFormatTokens = map[string]string{
	"LTS": "h:mm:ss A", "LT": "h:mm A", "L": "MM/DD/YYYY", "LL": "MMMM D, YYYY",
	"LLL": "MMMM D, YYYY h:mm A", "LLLL": "dddd, MMMM D, YYYY h:mm A",
}

var localizable = map[string]bool{"Qo": true, "MMMM": true, "MMM": true, "Mo": true, "DDDo": true,
	"Do": true, "dddd": true, "ddd": true, "dd": true, "do": true, "e": true, "eo": true, "Wo": true,
	"wo": true, "A": true, "a": true}

// fmtObj is the DateTime, Date or Time a format runs over, with Python's attribute
// lookups (a Date has no hour, a Time no year).
type fmtObj struct {
	class string
	dt    DateTime
	date  Date
	t     Time
}

func (o fmtObj) missing(attr string) error {
	return newError("AttributeError", fmt.Sprintf("'%s' object has no attribute '%s'", o.class, attr))
}

func (o fmtObj) hasDate() bool { return o.class != "Time" }

func (o fmtObj) dateVal() Date {
	if o.class == "DateTime" {
		return o.dt.Date()
	}
	return o.date
}

func (o fmtObj) timeVal() Time {
	if o.class == "DateTime" {
		return o.dt.Time()
	}
	return o.t
}

func (o fmtObj) dateAttr(attr string) (Date, error) {
	if !o.hasDate() {
		return Date{}, o.missing(attr)
	}
	return o.dateVal(), nil
}

func (o fmtObj) timeAttr(attr string) (Time, error) {
	if o.class == "Date" {
		return Time{}, o.missing(attr)
	}
	return o.timeVal(), nil
}

func formatWith(o fmtObj, fmtStr string, locale *string) (string, error) {
	loc, err := loadOrCurrent(locale)
	if err != nil {
		return "", err
	}
	return formatLoaded(o, fmtStr, loc)
}

func formatLoaded(o fmtObj, fmtStr string, loc *Locale) (string, error) {
	var b strings.Builder
	last := 0
	for _, m := range formatRE.FindAllStringSubmatchIndex(fmtStr, -1) {
		b.WriteString(fmtStr[last:m[0]])
		last = m[1]
		switch {
		case m[2] >= 0 && m[3] > m[2]:
			b.WriteString(fmtStr[m[2]:m[3]])
		case m[4] >= 0 && m[5] > m[4]:
			b.WriteString(fmtStr[m[4]:m[5]])
		case m[6] >= 0:
			s, err := formatToken(o, fmtStr[m[6]:m[7]], loc)
			if err != nil {
				return "", err
			}
			b.WriteString(s)
		}
	}
	b.WriteString(fmtStr[last:])
	return b.String(), nil
}

func formatToken(o fmtObj, token string, loc *Locale) (string, error) {
	if def, ok := dateFormatTokens[token]; ok {
		f, ok := loc.getString("custom.date_formats." + token)
		if !ok {
			f = def
		}
		return formatLoaded(o, f, loc)
	}
	if localizable[token] {
		return formatLocalizable(o, token, loc)
	}
	switch token {
	case "YYYY", "Y", "YY", "Q", "MM", "M", "DD", "D", "DDDD", "DDD", "d", "E":
		d, err := o.dateAttr(dateAttrOf(token))
		if err != nil {
			return "", err
		}
		switch token {
		case "YYYY", "Y":
			return itoa(d.Year()), nil
		case "YY":
			y := itoa(d.Year())
			if len(y) <= 2 {
				return "", nil
			}
			return y[2:], nil
		case "Q":
			return itoa(d.Quarter()), nil
		case "MM":
			return fmt.Sprintf("%02d", d.Month()), nil
		case "M":
			return itoa(d.Month()), nil
		case "DD":
			return fmt.Sprintf("%02d", d.Day()), nil
		case "D":
			return itoa(d.Day()), nil
		case "DDDD":
			return fmt.Sprintf("%03d", d.DayOfYear()), nil
		case "DDD":
			return itoa(d.DayOfYear()), nil
		case "d":
			return itoa((int(d.DayOfWeek()) + 1) % 7), nil
		}
		return itoa(d.Isoweekday()), nil
	case "HH", "H", "hh", "h", "mm", "m", "ss", "s", "S", "SS", "SSS", "SSSS", "SSSSS", "SSSSSS":
		t, err := o.timeAttr(timeAttrOf(token))
		if err != nil {
			return "", err
		}
		h12 := t.Hour() % 12
		if h12 == 0 {
			h12 = 12
		}
		us := t.Microsecond()
		switch token {
		case "HH":
			return fmt.Sprintf("%02d", t.Hour()), nil
		case "H":
			return itoa(t.Hour()), nil
		case "hh":
			return fmt.Sprintf("%02d", h12), nil
		case "h":
			return itoa(h12), nil
		case "mm":
			return fmt.Sprintf("%02d", t.Minute()), nil
		case "m":
			return itoa(t.Minute()), nil
		case "ss":
			return fmt.Sprintf("%02d", t.Second()), nil
		case "s":
			return itoa(t.Second()), nil
		case "S":
			return fmt.Sprintf("%01d", us/100000), nil
		case "SS":
			return fmt.Sprintf("%02d", us/10000), nil
		case "SSS":
			return fmt.Sprintf("%03d", us/1000), nil
		case "SSSS":
			return fmt.Sprintf("%04d", us/100), nil
		case "SSSSS":
			return fmt.Sprintf("%05d", us/10), nil
		}
		return fmt.Sprintf("%06d", us), nil
	case "X", "x":
		if o.class != "DateTime" {
			return "", o.missing("int_timestamp")
		}
		ts, err := o.dt.IntTimestamp()
		if err != nil {
			return "", err
		}
		if token == "X" {
			return itoa(ts), nil
		}
		return itoa(ts*1000 + o.dt.Microsecond()/1000), nil
	case "zz":
		if o.class == "Date" {
			return "", o.missing("tzinfo")
		}
		if o.class == "Time" || o.dt.d.Tzinfo() == nil {
			return "", nil
		}
		n, _ := o.dt.d.TzName()
		return n, nil
	case "z":
		if o.class != "DateTime" {
			return "", o.missing("timezone_name")
		}
		if n := o.dt.TimezoneName(); n != nil {
			return *n, nil
		}
		return "", nil
	case "ZZ", "Z":
		if o.class == "Date" {
			return "", o.missing("tzinfo")
		}
		if o.class == "Time" || o.dt.d.Tzinfo() == nil {
			return "", nil
		}
		sep := ""
		if token == "Z" {
			sep = ":"
		}
		off, _ := o.dt.d.UtcOffset()
		minutes := off.TotalSecondsExact() / 60
		sign := "+"
		if minutes < 0 {
			sign = "-"
		}
		m := int(minutes)
		if m < 0 {
			m = -m
		}
		return fmt.Sprintf("%s%02d%s%02d", sign, m/60, sep, m%60), nil
	}
	return token, nil
}

func dateAttrOf(token string) string {
	switch token {
	case "Q":
		return "quarter"
	case "MM", "M":
		return "month"
	case "DD", "D":
		return "day"
	case "DDDD", "DDD":
		return "day_of_year"
	case "d":
		return "day_of_week"
	case "E":
		return "isoweekday"
	}
	return "year"
}

func timeAttrOf(token string) string {
	switch token[0] {
	case 'H', 'h':
		return "hour"
	case 'm':
		return "minute"
	case 's':
		return "second"
	}
	return "microsecond"
}

func formatLocalizable(o fmtObj, token string, loc *Locale) (string, error) {
	switch token {
	case "MMM", "MMMM", "Mo", "Do", "Qo", "DDDo", "wo", "dd", "ddd", "dddd", "do", "e", "eo":
		attr := map[string]string{"MMM": "month", "MMMM": "month", "Mo": "month", "Do": "day",
			"Qo": "quarter", "DDDo": "day_of_year", "wo": "week_of_year"}[token]
		if attr == "" {
			attr = "day_of_week"
		}
		d, err := o.dateAttr(attr)
		if err != nil {
			return "", err
		}
		switch token {
		case "MMM":
			return loc.indexed("translations.months.abbreviated", d.Month())
		case "MMMM":
			return loc.indexed("translations.months.wide", d.Month())
		case "dd":
			return loc.indexed("translations.days.short", int(d.DayOfWeek()))
		case "ddd":
			return loc.indexed("translations.days.abbreviated", int(d.DayOfWeek()))
		case "dddd":
			return loc.indexed("translations.days.wide", int(d.DayOfWeek()))
		case "e", "eo":
			first, _ := loc.get("translations.week_data.first_day").(int)
			v := pyMod(pyMod(int(d.DayOfWeek()), 7)-first, 7)
			if token == "e" {
				return itoa(v), nil
			}
			return loc.Ordinalize(v + 1), nil
		case "Do":
			return loc.Ordinalize(d.Day()), nil
		case "do":
			return loc.Ordinalize((int(d.DayOfWeek()) + 1) % 7), nil
		case "Mo":
			return loc.Ordinalize(d.Month()), nil
		case "Qo":
			return loc.Ordinalize(d.Quarter()), nil
		case "wo":
			return loc.Ordinalize(d.WeekOfYear()), nil
		}
		return loc.Ordinalize(d.DayOfYear()), nil
	case "A":
		t, err := o.timeAttr("hour")
		if err != nil {
			return "", err
		}
		key := "translations.day_periods.am"
		if t.Hour() >= 12 {
			key = "translations.day_periods.pm"
		}
		s, _ := loc.getString(key)
		return s, nil
	}
	return token, nil
}

// Format is dt.format(fmt, locale): pendulum's tokens in the given (nil: current) locale.
func (dt DateTime) Format(fmtStr string, locale *string) (string, error) {
	return formatWith(fmtObj{class: "DateTime", dt: dt}, fmtStr, locale)
}

// Format is d.format(fmt, locale).
func (d Date) Format(fmtStr string, locale *string) (string, error) {
	return formatWith(fmtObj{class: "Date", date: d}, fmtStr, locale)
}

// Format is t.format(fmt, locale).
func (t Time) Format(fmtStr string, locale *string) (string, error) {
	return formatWith(fmtObj{class: "Time", t: t}, fmtStr, locale)
}
