package pendulum

import (
	"fmt"
	"math"

	"github.com/python-pendulum/pendulum/internal/locales"
)

// diffUnits are what DifferenceFormatter reads from a Duration or an Interval.
type diffUnits struct {
	years, months, weeks, remDays, hours, minutes, remSeconds int
	invert                                                    bool
}

func unitsOf(d Duration, iv *Interval) diffUnits {
	if iv != nil {
		return diffUnits{iv.Years(), iv.Months(), iv.Weeks(), iv.RemainingDays(), iv.Hours(),
			iv.Minutes(), iv.RemainingSeconds(), iv.Invert()}
	}
	return diffUnits{d.Years(), d.Months(), d.Weeks(), d.RemainingDays(), d.Hours(), d.Minutes(),
		d.RemainingSeconds(), d.Invert()}
}

// FormatDiff is pendulum.format_diff(diff, is_now, absolute, locale).
func FormatDiff(diff Duration, isNow, absolute bool, locale *string) (string, error) {
	return formatDiffDuration(diff, nil, isNow, absolute, locale)
}

// FormatIntervalDiff is pendulum.format_diff for an Interval.
func FormatIntervalDiff(diff Interval, isNow, absolute bool, locale *string) (string, error) {
	return formatDiffDuration(diff.dur, &diff, isNow, absolute, locale)
}

func formatDiffDuration(d Duration, iv any, isNow, absolute bool, locale *string) (string, error) {
	var ivp *Interval
	switch v := iv.(type) {
	case Interval:
		ivp = &v
	case *Interval:
		ivp = v
	}
	name := GetLocale()
	if locale != nil {
		name = *locale
	}
	loc, err := LoadLocale(name)
	if err != nil {
		return "", err
	}
	return differenceFormat(unitsOf(d, ivp), isNow, absolute, loc)
}

func differenceFormat(diff diffUnits, isNow, absolute bool, loc *Locale) (string, error) {
	var unit string
	var count int
	switch {
	case diff.years > 0:
		unit, count = "year", diff.years
		if diff.months > 6 {
			count++
		}
	case diff.months == 11 && diff.weeks*7+diff.remDays > 15:
		unit, count = "year", 1
	case diff.months > 0:
		unit, count = "month", diff.months
		if diff.weeks*7+diff.remDays >= 27 {
			count++
		}
	case diff.weeks > 0:
		unit, count = "week", diff.weeks
		if diff.remDays > 3 {
			count++
		}
	case diff.remDays > 0:
		unit, count = "day", diff.remDays
		if diff.hours >= 22 {
			count++
		}
	case diff.hours > 0:
		unit, count = "hour", diff.hours
	case diff.minutes > 0:
		unit, count = "minute", diff.minutes
	case 10 < diff.remSeconds && diff.remSeconds < 60:
		unit, count = "second", diff.remSeconds
	default:
		if few, ok := loc.getString("custom.units.few_second"); ok {
			if absolute {
				return few, nil
			}
			key := "custom"
			switch {
			case isNow && diff.invert:
				key += ".from_now"
			case isNow:
				key += ".ago"
			case diff.invert:
				key += ".after"
			default:
				key += ".before"
			}
			return localeFormat(loc, key, few)
		}
		unit, count = "second", diff.remSeconds
	}
	if count == 0 {
		count = 1
	}
	var key string
	if absolute {
		key = "translations.units." + unit
	} else if isNow {
		key = "translations.relative." + unit
		if diff.invert {
			key += ".future"
		} else {
			key += ".past"
		}
	} else {
		rel := "custom.units_relative." + unit
		if diff.invert {
			rel += ".future"
		} else {
			rel += ".past"
		}
		var t string
		trans, _ := loc.get(rel).(*locales.Dict)
		if trans == nil || len(trans.Keys) == 0 {
			s, err := localeFormat(loc, "translations.units."+unit+"."+loc.Plural(count), itoa(count))
			if err != nil {
				return "", err
			}
			t = s
		} else {
			v, ok := trans.Get(loc.Plural(count))
			if !ok {
				return "", newError("KeyError", "'"+loc.Plural(count)+"'")
			}
			s, _ := v.(string)
			t = pyFormat(s, itoa(count))
		}
		k := "custom.before"
		if diff.invert {
			k = "custom.after"
		}
		return localeFormat(loc, k, t)
	}
	key += "." + loc.Plural(count)
	return localeFormat(loc, key, itoa(count))
}

// localeFormat is locale.get(key).format(arg); a missing key is the AttributeError
// None.format raises.
func localeFormat(loc *Locale, key, arg string) (string, error) {
	s, ok := loc.getString(key)
	if !ok {
		return "", newError("AttributeError", "'NoneType' object has no attribute 'format'")
	}
	return pyFormat(s, arg), nil
}

// inWords is Duration.in_words / Interval.in_words: secondPlural is the count whose
// plural form names the fractional seconds (0 for a Duration, 1 for an Interval).
func inWords(locale *string, separator string, counts [7]int, micro int, secondPlural int) (string, error) {
	loc, err := loadOrCurrent(locale)
	if err != nil {
		return "", err
	}
	units := [7]string{"year", "month", "week", "day", "hour", "minute", "second"}
	var parts []string
	for i, c := range counts {
		if c == 0 {
			continue
		}
		s, err := localeFormat(loc, "translations.units."+units[i]+"."+loc.Plural(iabs(c)), itoa(c))
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		if micro != 0 {
			count := fmt.Sprintf("%.2f", math.Abs(float64(micro))/1e6)
			s, err := localeFormat(loc, "translations.units.second."+loc.Plural(secondPlural), count)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		} else {
			s, err := localeFormat(loc, "translations.units.microsecond."+loc.Plural(0), "0")
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += separator
		}
		out += p
	}
	return out, nil
}
