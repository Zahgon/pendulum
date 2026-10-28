package strftime

import (
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// C locale LC_TIME data, as glibc 2.36 defines it.
var (
	abDay  = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	day    = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	abMon  = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	mon    = [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	amPM   = [2]string{"AM", "PM"}
	dtFmt  = "%a %b %e %H:%M:%S %Y" // D_T_FMT; ERA_D_T_FMT is empty
	dFmt   = "%m/%d/%y"             // D_FMT; ERA_D_FMT is empty
	tFmt   = "%H:%M:%S"             // T_FMT; ERA_T_FMT is empty
	tFmtAP = "%I:%M:%S %p"          // T_FMT_AMPM
)

// glibc is one wcsftime call of glibc's __strftime_internal in the C locale,
// which has no eras and no alternative digits. The C locale's alternative
// month names equal the ordinary ones.
type glibc struct {
	tm  *cTm
	t   Tm
	env *Env

	mktimeDone bool
	mktimeSecs int64

	out []byte
	n   int64 // characters written so far (glibc's i)
	max int64 // glibc's maxsize: the output and its NUL must fit
}

// directive is the per-conversion state of __strftime_internal.
type directive struct {
	pad      byte // 0, '_', '-' or '0'
	width    int  // -1 when absent
	upper    bool // to_uppcase
	lower    bool // to_lowcase
	modifier byte // 0, 'E' or 'O'
}

// run formats one ASCII run with room for maxsize-1 characters. It reports
// false where glibc returns 0 because the output does not fit.
func (g *glibc) run(format string, maxsize int64) (string, int64, bool) {
	g.out, g.n, g.max = g.out[:0], 0, maxsize
	if !g.format(format) {
		return "", 0, false
	}
	return string(g.out), g.n, true
}

// format is the main loop of __strftime_internal.
func (g *glibc) format(format string) bool {
	at := func(i int) byte {
		if i < len(format) {
			return format[i]
		}
		return 0
	}
	for f := 0; f < len(format); f++ {
		if format[f] != '%' {
			if !g.add(1, -1, 0, format[f:f+1]) {
				return false
			}
			continue
		}
		d := directive{width: -1}
		changeCase := false
	flags:
		for {
			f++
			switch c := at(f); c {
			case '_', '-', '0':
				d.pad = c
			case '^':
				d.upper = true
			case '#':
				changeCase = true
			default:
				break flags
			}
		}
		if isDigit(at(f)) {
			d.width = 0
			for ; isDigit(at(f)); f++ {
				digit := int(at(f) - '0')
				if d.width > math.MaxInt32/10 || (d.width == math.MaxInt32/10 && digit > math.MaxInt32%10) {
					d.width = math.MaxInt32
				} else {
					d.width = d.width*10 + digit
				}
			}
		}
		if c := at(f); c == 'E' || c == 'O' {
			d.modifier = c
			f++
		}
		var ok bool
		if f == len(format) {
			// GNU extension: '%' at the end of the format is copied.
			f--
			ok = g.badFormat(&d, format, f)
		} else {
			ok = g.conversion(&d, changeCase, format, f)
		}
		if !ok {
			return false
		}
	}
	return true
}

// conversion expands the conversion character format[f].
func (g *glibc) conversion(d *directive, changeCase bool, format string, f int) bool {
	tm := g.tm
	hour12 := tm.hour
	if hour12 > 12 {
		hour12 -= 12
	} else if hour12 == 0 {
		hour12 = 12
	}
	textUpper := func() {
		if changeCase {
			d.upper, d.lower = true, false
		}
	}
	switch c := format[f]; c {
	case '%':
		if d.modifier != 0 {
			break
		}
		return g.add(1, d.width, d.pad, "%")
	case 'a':
		if d.modifier != 0 {
			break
		}
		textUpper()
		return g.cpy(d, abDay[tm.wday])
	case 'A':
		if d.modifier != 0 {
			break
		}
		textUpper()
		return g.cpy(d, day[tm.wday])
	case 'b', 'h':
		textUpper()
		if d.modifier == 'E' {
			break
		}
		return g.cpy(d, abMon[tm.mon])
	case 'B':
		if d.modifier == 'E' {
			break
		}
		textUpper()
		return g.cpy(d, mon[tm.mon])
	case 'c':
		if d.modifier == 'O' {
			break
		}
		return g.subformat(d, dtFmt)
	case 'C':
		year := tm.year + 1900
		century := year / 100
		if year%100 < 0 {
			century--
		}
		return g.number(d, 1, century)
	case 'x':
		if d.modifier == 'O' {
			break
		}
		return g.subformat(d, dFmt)
	case 'D':
		if d.modifier != 0 {
			break
		}
		return g.subformat(d, "%m/%d/%y")
	case 'd':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 2, tm.mday)
	case 'e':
		if d.modifier == 'E' {
			break
		}
		return g.numberSpacePad(d, 2, tm.mday)
	case 'F':
		if d.modifier != 0 {
			break
		}
		return g.subformat(d, "%Y-%m-%d")
	case 'H':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 2, tm.hour)
	case 'I':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 2, hour12)
	case 'k':
		if d.modifier == 'E' {
			break
		}
		return g.numberSpacePad(d, 2, tm.hour)
	case 'l':
		if d.modifier == 'E' {
			break
		}
		return g.numberSpacePad(d, 2, hour12)
	case 'j':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 3, 1+tm.yday)
	case 'M':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 2, tm.min)
	case 'm':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 2, tm.mon+1)
	case 'n':
		return g.add(1, d.width, d.pad, "\n")
	case 'P', 'p':
		// %P and %#p lower-case; to_lowcase beats a '^' in cpy.
		if c == 'P' || changeCase {
			d.lower = true
		}
		return g.cpy(d, amPM[b2i(tm.hour > 11)])
	case 'R':
		return g.subformat(d, "%H:%M")
	case 'r':
		return g.subformat(d, tFmtAP)
	case 'S':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 2, tm.sec)
	case 's':
		secs := g.mktime()
		mag := uint64(secs)
		if secs < 0 {
			mag = -mag
		}
		return g.signAndPad(d, 1, secs < 0, strconv.FormatUint(mag, 10))
	case 'X':
		if d.modifier == 'O' {
			break
		}
		return g.subformat(d, tFmt)
	case 'T':
		return g.subformat(d, "%H:%M:%S")
	case 't':
		return g.add(1, d.width, d.pad, "\t")
	case 'u':
		return g.number(d, 1, (tm.wday-1+7)%7+1)
	case 'U':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 2, (tm.yday-tm.wday+7)/7)
	case 'V', 'g', 'G':
		if d.modifier == 'E' {
			break
		}
		year, days := isoYearDays(tm)
		switch c {
		case 'g':
			return g.number(d, 2, (year%100+100)%100)
		case 'G':
			return g.number(d, 1, year)
		}
		return g.number(d, 2, days/7+1)
	case 'W':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 2, (tm.yday-(tm.wday-1+7)%7+7)/7)
	case 'w':
		if d.modifier == 'E' {
			break
		}
		return g.number(d, 1, tm.wday)
	case 'Y':
		// %EY has no era in the C locale and falls back to the plain year.
		if d.modifier == 'O' {
			break
		}
		return g.number(d, 1, tm.year+1900)
	case 'y':
		return g.number(d, 2, (tm.year%100+100)%100)
	case 'Z':
		if changeCase {
			d.upper, d.lower = false, true
		}
		// tm_zone is NULL, so glibc falls back to the process's tzname[].
		zone := ""
		if tm.isdst >= 0 {
			zone = g.env.TZName[tm.isdst]
		}
		return g.cpy(d, zone)
	case 'z':
		if tm.isdst < 0 {
			return true
		}
		// tm_gmtoff is 0: a sign padded to the width, then 0 as a number.
		if !g.add(1, d.width, d.pad, "+") {
			return false
		}
		return g.number(d, 4, 0)
	}
	return g.badFormat(d, format, f)
}

// mktime returns what glibc's %s prints: mktime() of the fields, or -1 when it fails.
func (g *glibc) mktime() int64 {
	if !g.mktimeDone {
		g.mktimeDone = true
		g.mktimeSecs = -1
		if g.env.Mktime != nil {
			if secs, ok := g.env.Mktime(g.t); ok {
				g.mktimeSecs = secs
			}
		}
	}
	return g.mktimeSecs
}

// badFormat copies an unknown or malformed directive, from its '%' through
// format[f], subject to the width and case flags.
func (g *glibc) badFormat(d *directive, format string, f int) bool {
	start := strings.LastIndexByte(format[:f+1], '%')
	return g.cpy(d, format[start:f+1])
}

// subformat expands a composite conversion such as %c, padding the whole
// result to the width and upper-casing it for '^'.
func (g *glibc) subformat(d *directive, sub string) bool {
	inner := &glibc{tm: g.tm, t: g.t, env: g.env, max: math.MaxInt64}
	inner.format(sub)
	s := string(inner.out)
	if d.upper {
		s = strings.Map(unicode.ToUpper, s)
	}
	return g.add(int(inner.n), d.width, d.pad, s)
}

// numberSpacePad is DO_NUMBER_SPACEPAD: '_' padding unless '0' or '-' was given.
func (g *glibc) numberSpacePad(d *directive, digits int, v int32) bool {
	if d.pad != '0' && d.pad != '-' {
		d.pad = '_'
	}
	return g.number(d, digits, v)
}

// number is DO_NUMBER: at least max(digits, width) digits. The O modifier
// asks for alternative digits, which the C locale does not have.
func (g *glibc) number(d *directive, digits int, v int32) bool {
	mag := uint32(v)
	if v < 0 {
		mag = -mag
	}
	return g.signAndPad(d, max(digits, d.width), v < 0, strconv.FormatUint(uint64(mag), 10))
}

// signAndPad is do_number_sign_and_padding.
func (g *glibc) signAndPad(d *directive, digits int, negative bool, mag string) bool {
	s := mag
	if negative {
		s = "-" + mag
	}
	if d.pad != '-' {
		if padding := digits - len(s); padding > 0 {
			if d.pad == '_' {
				if int64(padding) >= g.max-g.n {
					return false
				}
				g.fill(' ', padding)
				g.n += int64(padding)
				if d.width > padding {
					d.width -= padding
				} else {
					d.width = 0
				}
			} else {
				if int64(digits) >= g.max-g.n {
					return false
				}
				if negative {
					s = mag
					g.out = append(g.out, '-')
					g.n++
				}
				g.fill('0', padding)
				g.n += int64(padding)
				d.width = 0
			}
		}
	}
	return g.cpy(d, s)
}

// cpy is glibc's cpy macro: add with case conversion.
func (g *glibc) cpy(d *directive, s string) bool {
	n := utf8.RuneCountInString(s)
	if d.lower {
		s = strings.Map(unicode.ToLower, s)
	} else if d.upper {
		s = strings.Map(unicode.ToUpper, s)
	}
	return g.add(n, d.width, d.pad, s)
}

// add is glibc's add macro: s, n characters long, right-aligned in width,
// padded with zeros for the '0' flag and with spaces otherwise.
func (g *glibc) add(n, width int, pad byte, s string) bool {
	delta := width - n
	incr := int64(n)
	if delta > 0 {
		incr += int64(delta)
	}
	if incr >= g.max-g.n {
		return false
	}
	if delta > 0 {
		fill := byte(' ')
		if pad == '0' {
			fill = '0'
		}
		g.fill(fill, delta)
	}
	g.out = append(g.out, s...)
	g.n += incr
	return true
}

// fill appends count copies of c; the caller accounts for them in g.n.
func (g *glibc) fill(c byte, count int) {
	for range count {
		g.out = append(g.out, c)
	}
}

// isoYearDays returns the ISO 8601 week-based year of tm and the number of
// days from the start of its first week, as glibc computes them for %G, %g and %V.
func isoYearDays(tm *cTm) (int32, int32) {
	year := tm.year + 1900
	days := isoWeekDays(tm.yday, tm.wday)
	if days < 0 {
		year--
		days = isoWeekDays(tm.yday+365+b2i(isLeap(year)), tm.wday)
	} else if d := isoWeekDays(tm.yday-(365+b2i(isLeap(year))), tm.wday); d >= 0 {
		year++
		days = d
	}
	return year, days
}

// isoWeekDays is glibc's iso_week_days.
func isoWeekDays(yday, wday int32) int32 {
	const bigEnoughMultipleOf7 = (366/7 + 2) * 7
	return yday - (yday-wday+4+bigEnoughMultipleOf7)%7 + 4 - 1
}

// isLeap is glibc's __isleap.
func isLeap(y int32) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

// isDigit is glibc's ISDIGIT: ASCII digits only.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// b2i converts a C truth value to 0 or 1.
func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
