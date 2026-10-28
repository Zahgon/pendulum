package zoneinfo

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// tzRule is what applies after a zone's last transition: _TZStr when the
// footer has DST rules, or a single ttinfo (stdOnly) otherwise.
type tzRule struct {
	std, dst   ttinfo
	stdOnly    bool
	dstDiff    int64
	start, end transitionRule
}

// transitionRule is _DayOffset or _CalendarOffset.
type transitionRule interface {
	// yearToEpoch is the rule's moment in the given year, in seconds since
	// 1970-01-01 local time.
	yearToEpoch(year int64) int64
}

// find is _TZStr._get_trans_info.
func (r *tzRule) find(ts, year int64, fold int) *ttinfo {
	if r.stdOnly {
		return &r.std
	}
	start, end := r.start.yearToEpoch(year), r.end.yearToEpoch(year)
	// With fold=0 the period with the smaller offset starts at the end of
	// the gap and ends at the end of the fold; with fold=1 it runs from the
	// start of the gap to the beginning of the fold. Which bound moves is
	// fold XOR "DST is positive".
	if (fold == 1) == (r.dstDiff >= 0) {
		end -= r.dstDiff
	} else {
		start += r.dstDiff
	}
	if inDST(ts, start, end) {
		return &r.dst
	}
	return &r.std
}

// fromUTC is _TZStr._get_trans_info_fromutc.
func (r *tzRule) fromUTC(ts, year int64) (*ttinfo, bool) {
	if r.stdOnly {
		return &r.std, false
	}
	start := r.start.yearToEpoch(year) - r.std.utcoff
	end := r.end.yearToEpoch(year) - r.dst.utcoff
	tti := &r.std
	if inDST(ts, start, end) {
		tti = &r.dst
	}
	// The ambiguous period follows the end of positive DST, or precedes
	// the start of negative DST.
	ambigStart, ambigEnd := end, end+r.dstDiff
	if r.dstDiff <= 0 {
		ambigStart, ambigEnd = start, start-r.dstDiff
	}
	return tti, ambigStart <= ts && ts < ambigEnd
}

func inDST(ts, start, end int64) bool {
	if start < end {
		return start <= ts && ts < end
	}
	return !(end <= ts && ts < start)
}

// dayOffset is _DayOffset: "Jn" (julian, 1..365, never counting Feb 29) or
// "n" (0..365).
type dayOffset struct {
	d      int64
	julian bool
	secs   int64 // time of day, from the /time suffix
}

func (o *dayOffset) yearToEpoch(year int64) int64 {
	d := o.d
	if o.julian && d >= 59 && isLeap(year) {
		d++
	}
	return (postEpochDaysBeforeYear(year)+d)*86400 + o.secs
}

// calendarOffset is _CalendarOffset: "Mm.w.d", day d (0 = Sunday) of week w
// (5 = last) of month m.
type calendarOffset struct {
	m, w, d int64
	secs    int64 // time of day, from the /time suffix
}

var (
	daysBeforeMonth = [13]int64{-1, 0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	daysInMonth     = [13]int64{-1, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
)

func (o *calendarOffset) yearToEpoch(year int64) int64 {
	// calendar.monthrange: weekday of the 1st (0 = Monday) and month length.
	firstDay := floorMod(ymdToOrd(year, o.m, 1)+postEpochOrdinal+6, 7)
	monthLen := daysInMonth[o.m]
	if o.m == 2 && isLeap(year) {
		monthLen++
	}
	// Week 1 is the first week containing day d; week 5 means the last.
	monthDay := floorMod(o.d-(firstDay+1), 7) + 1 + (o.w-1)*7
	if monthDay > monthLen {
		monthDay -= 7
	}
	return ymdToOrd(year, o.m, monthDay)*86400 + o.secs
}

// postEpochOrdinal is date(1970, 1, 1).toordinal().
const postEpochOrdinal = 719163

// postEpochDaysBeforeYear is _post_epoch_days_before_year, which is one less
// than the number of days from 1970-01-01 to January 1st of year (so a
// zero-based "n" footer day falls on the previous day, as in CPython).
func postEpochDaysBeforeYear(year int64) int64 {
	y := year - 1
	return y*365 + floorDiv(y, 4) - floorDiv(y, 100) + floorDiv(y, 400) - postEpochOrdinal
}

// ymdToOrd is _CalendarOffset._ymd2ord: days from 1970-01-01.
func ymdToOrd(year, month, day int64) int64 {
	n := postEpochDaysBeforeYear(year) + daysBeforeMonth[month] + day
	if month > 2 && isLeap(year) {
		n++
	}
	return n
}

func isLeap(year int64) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func floorMod(a, b int64) int64 {
	return a - floorDiv(a, b)*b
}

const tzOffsetPattern = `[+-]?[0-9]{1,3}(?::[0-9]{2}(?::[0-9]{2})?)?`

var (
	// tzStrRe is _parse_tz_str's parser_re (re.ASCII, fullmatch); RE2's
	// leftmost-first submatches equal those of Python's backtracking.
	tzStrRe = regexp.MustCompile(`^([^<0-9:.+-]+|<[a-zA-Z0-9+-]+>)(?:(` + tzOffsetPattern +
		`)(?:([^0-9:.+-]+|<[a-zA-Z0-9+-]+>)(` + tzOffsetPattern + `)?)?)?$`)
	// hmsRe is the pattern of _parse_tz_delta and _parse_transition_time.
	hmsRe = regexp.MustCompile(`^([+-])?([0-9]{1,3})(?::([0-9]{2})(?::([0-9]{2}))?)?$`)
	// calendarRuleRe is _parse_dst_start_end's, unescaped second dot included.
	calendarRuleRe = regexp.MustCompile(`^M([0-9]{1,2})\.([0-9]).([0-9])$`)
)

func valueError(msg string) error {
	return &Error{"ValueError", msg}
}

// parseTZStr is _parse_tz_str: std[offset[dst[offset],start[/time],end[/time]]].
func parseTZStr(tzStr string) (*tzRule, error) {
	offsetStr, startEnd, hasRules := strings.Cut(tzStr, ",")
	m := tzStrRe.FindStringSubmatch(offsetStr)
	if m == nil {
		return nil, valueError(tzStr + " is not a valid TZ string")
	}
	// Groups that did not take part are "", and none can match "".
	stdAbbr := strings.Trim(m[1], "<>")
	var stdOffset int64
	if m[2] != "" {
		var ok bool
		if stdOffset, ok = parseTZDelta(m[2]); !ok {
			return nil, valueError("Invalid STD offset in " + tzStr)
		}
	}
	if m[3] == "" {
		if hasRules {
			return nil, valueError("Transition rule present without DST: " + tzStr)
		}
		return &tzRule{std: ttinfo{utcoff: stdOffset, tzname: stdAbbr}, stdOnly: true}, nil
	}
	dstAbbr := strings.Trim(m[3], "<>")
	dstOffset := stdOffset + 3600
	if m[4] != "" {
		var ok bool
		if dstOffset, ok = parseTZDelta(m[4]); !ok {
			return nil, valueError("Invalid DST offset in " + tzStr)
		}
	}
	if !hasRules {
		return nil, valueError("Missing transition rules: " + tzStr)
	}
	// Every failure below is a ValueError that Python rewraps, including
	// the unpacking of a lone rule into "start, end".
	startStr, endStr, two := strings.Cut(startEnd, ",")
	start, okStart := parseDSTStartEnd(startStr)
	end, okEnd := parseDSTStartEnd(endStr)
	if !two || !okStart || !okEnd {
		return nil, valueError("Invalid TZ string: " + tzStr)
	}
	diff := dstOffset - stdOffset
	return &tzRule{
		std:     ttinfo{utcoff: stdOffset, tzname: stdAbbr},
		dst:     ttinfo{utcoff: dstOffset, dstoff: diff, tzname: dstAbbr},
		dstDiff: diff,
		start:   start,
		end:     end,
	}, nil
}

// parseDSTStartEnd is _parse_dst_start_end: Mm.w.d, Jn or n, then an
// optional /time (default 02:00); false where Python raises ValueError.
func parseDSTStartEnd(s string) (transitionRule, bool) {
	date, tm, hasTime := strings.Cut(s, "/")
	secs := int64(2 * 3600)
	if hasTime {
		var ok bool
		if secs, ok = parseTransitionTime(tm); !ok {
			return nil, false
		}
	}
	if strings.HasPrefix(date, "M") {
		m := calendarRuleRe.FindStringSubmatch(date)
		if m == nil {
			return nil, false
		}
		mo, w, d := atoi(m[1]), atoi(m[2]), atoi(m[3])
		if mo < 1 || mo > 12 || w < 1 || w > 5 || d > 6 {
			return nil, false
		}
		return &calendarOffset{m: mo, w: w, d: d, secs: secs}, true
	}
	julian := strings.HasPrefix(date, "J")
	if julian {
		date = date[1:]
	}
	doy, ok := pyInt(date)
	if !ok || doy > 365 || doy < 0 || (julian && doy == 0) {
		return nil, false
	}
	return &dayOffset{d: doy, julian: julian, secs: secs}, true
}

// parseTransitionTime is _parse_transition_time: [+-]h[:mm[:ss]] with
// hours up to 167, the sign applying to every field.
func parseTransitionTime(s string) (int64, bool) {
	m := hmsRe.FindStringSubmatch(s)
	if m == nil || atoi(m[2]) > 167 {
		return 0, false
	}
	secs := atoi(m[2])*3600 + atoi(m[3])*60 + atoi(m[4])
	if m[1] == "-" {
		secs = -secs
	}
	return secs, true
}

// parseTZDelta is _parse_tz_delta: hours up to 24 and the POSIX sign
// convention, so "+5" (or "5") is five hours west of UTC.
func parseTZDelta(s string) (int64, bool) {
	m := hmsRe.FindStringSubmatch(s)
	if m == nil || atoi(m[2]) > 24 {
		return 0, false
	}
	total := atoi(m[2])*3600 + atoi(m[3])*60 + atoi(m[4])
	if m[1] != "-" {
		total = -total
	}
	return total, true
}

// atoi converts a regexp group of ASCII digits; an empty group is 0.
func atoi(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// pyInt is int(s) for the day-of-year field: surrounding whitespace, an
// optional sign and ASCII digits, single underscores allowed between digits.
func pyInt(s string) (int64, bool) {
	t := strings.TrimFunc(s, unicode.IsSpace)
	digits := strings.TrimPrefix(strings.TrimPrefix(t, "+"), "-")
	if len(t)-len(digits) > 1 || digits == "" || digits[0] == '_' ||
		strings.HasSuffix(digits, "_") || strings.Contains(digits, "__") {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(t, "_", ""), 10, 64)
	return n, err == nil
}
