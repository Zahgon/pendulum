package pendulum

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/python-pendulum/pendulum/internal/locales"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

const (
	match1         = `\d`
	match2         = `\d\d`
	match3         = `\d{3}`
	match4         = `\d{4}`
	match1to2      = `\d\d?`
	match1to2Left  = `[0-9 ]\d?`
	match1to3      = `\d{1,3}`
	match1to4      = `\d{1,4}`
	matchUnsigned  = `\d+`
	matchSigned    = `[+-]?\d+`
	matchOffset    = `[Zz]|[+-]\d\d:?\d\d`
	matchShortOff  = `[Zz]|[+-]\d\d(?::?\d\d)?`
	matchTimestamp = `[+-]?\d+(\.\d{1,6})?`
	matchTimezone  = `[A-Za-z0-9-+]+(/[A-Za-z0-9-+_]+)?`
)

var regexTokens = map[string][]string{
	"Y": {matchSigned}, "YY": {match1to2, match2}, "YYYY": {match1to4, match4}, "Q": {match1},
	"M": {match1to2}, "MM": {match1to2, match2}, "D": {match1to2}, "DD": {match1to2Left, match2},
	"DDD": {match1to3}, "DDDD": {match3}, "d": {match1}, "e": {match1}, "E": {match1},
	"H": {match1to2}, "HH": {match1to2, match2}, "h": {match1to2}, "hh": {match1to2, match2},
	"m": {match1to2}, "mm": {match1to2, match2}, "s": {match1to2}, "ss": {match1to2, match2},
	"S": {match1to3, match1}, "SS": {match1to3, match2}, "SSS": {match1to3, match3},
	"SSSS": {matchUnsigned}, "SSSSS": {matchUnsigned}, "SSSSSS": {matchUnsigned},
	"x": {matchSigned}, "X": {matchTimestamp}, "ZZ": {matchShortOff}, "Z": {matchOffset},
	"z": {matchTimezone},
}

// regexNone are the _REGEX_TOKENS entries whose value is None.
var regexNone = map[string]bool{"Qo": true, "Do": true}

var parseTokens = map[string]bool{"YYYY": true, "YY": true, "Q": true, "MMMM": true, "MMM": true,
	"MM": true, "M": true, "DDDD": true, "DDD": true, "DD": true, "D": true, "dddd": true, "ddd": true,
	"dd": true, "d": true, "E": true, "HH": true, "H": true, "hh": true, "h": true, "mm": true, "m": true,
	"ss": true, "s": true, "S": true, "SS": true, "SSS": true, "SSSS": true, "SSSSS": true,
	"SSSSSS": true, "a": true, "X": true, "x": true, "ZZ": true, "Z": true, "z": true}

// pyEscape is re.escape.
func pyEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("()[]{}?*+-|^$\\.&~# \t\n\r\v\f", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// tokenAlternatives lists Formatter._TOKENS' third alternative in order, each with its
// possible lengths longest first, as a backtracking engine tries them.
var tokenAlternatives = func() [][]string {
	alts := []string{"Mo", "MM?M?M?", "Do", "DDDo", "DD?D?D?", "ddd?d?", "do?", "eo?", "E{1,4}",
		"w[o|w]?", "W[o|W]?", "Qo?", "YYYY", "YY", "Y", "gg(ggg?)?", "GG(GGG?)?", "a", "A", "hh?",
		"HH?", "kk?", "mm?", "ss?", "S{1,9}", "x", "X", "zz?", "ZZ?", "LTS", "LT", "LL?L?L?"}
	var out [][]string
	for _, a := range alts {
		re := regexp.MustCompile(`^(?:` + a + `)$`)
		var forms []string
		for n := 9; n >= 1; n-- {
			for _, cand := range candidatesOfLen(a, n) {
				if re.MatchString(cand) {
					forms = append(forms, cand)
				}
			}
		}
		out = append(out, forms)
	}
	return out
}()

func candidatesOfLen(alt string, n int) []string {
	letters := map[rune]bool{}
	for _, r := range alt {
		if r >= 'A' && r <= 'z' && r != '[' && r != ']' || r == '|' {
			letters[r] = true
		}
	}
	var out []string
	var rec func(prefix string)
	rec = func(prefix string) {
		if len(prefix) == n {
			out = append(out, prefix)
			return
		}
		for r := range letters {
			rec(prefix + string(r))
		}
	}
	if n <= 5 || alt == "S{1,9}" {
		if alt == "S{1,9}" {
			return []string{strings.Repeat("S", n)}
		}
		rec("")
	}
	return out
}

// fromFormatScan is _FROM_FORMAT_RE.sub over the escaped format: each match handed to
// replace, the text between kept.
func fromFormatScan(s string, replace func(string) (string, error)) (string, bool, error) {
	var b strings.Builder
	found := false
	i := 0
	for i < len(s) {
		tok := scanTokenAt(s, i)
		if tok == "" {
			_, w := utf8.DecodeRuneInString(s[i:])
			b.WriteString(s[i : i+w])
			i += w
			continue
		}
		found = true
		r, err := replace(tok)
		if err != nil {
			return "", true, err
		}
		b.WriteString(r)
		i += len(tok)
	}
	return b.String(), found, nil
}

// scanTokenAt is the leftmost-first match of
// (?<!\\\[)\[([^\[]*)\]|\\(.)|(tokens)(?!\\\]) at position i.
func scanTokenAt(s string, i int) string {
	if s[i] == '[' && !(i >= 2 && s[i-2:i] == `\[`) {
		j := i + 1
		for j < len(s) && s[j] != '[' {
			j++
		}
		if k := strings.LastIndexByte(s[i+1:j], ']'); k >= 0 {
			return s[i : i+1+k+1]
		}
	}
	if s[i] == '\\' && i+1 < len(s) {
		r, w := utf8.DecodeRuneInString(s[i+1:])
		if r != '\n' {
			return s[i : i+1+w]
		}
	}
	for _, forms := range tokenAlternatives {
		for _, f := range forms {
			if strings.HasPrefix(s[i:], f) && !strings.HasPrefix(s[i+len(f):], `\]`) {
				return f
			}
		}
	}
	return ""
}

// fromFormatPattern builds Formatter.parse's pattern, its named groups in order, and
// the Python regex group number and name position of each.
type fmtPattern struct {
	pattern string
	tokens  []string
}

func replaceToken(tok string, loc *Locale) (string, error) {
	if strings.HasPrefix(tok, "[") && strings.HasSuffix(tok, "]") {
		return tok[1 : len(tok)-1], nil
	}
	if strings.HasPrefix(tok, "\\") {
		if tok == `\[` || tok == `\]` {
			return "", nil
		}
		return tok, nil
	}
	_, isRegex := regexTokens[tok]
	if !isRegex && !regexNone[tok] && !localizable[tok] {
		return "", valueError("Unsupported token: %s", tok)
	}
	var cands []string
	if localizable[tok] {
		switch tok {
		case "Do":
			d, _ := loc.get("custom.ordinal").(*locales.Dict)
			if d == nil {
				return "", newError("AttributeError", "'NoneType' object has no attribute 'values'")
			}
			for _, v := range d.Values {
				cands = append(cands, `\d+`+v.(string))
			}
		case "A", "a":
			am, _ := loc.getString("translations.day_periods.am")
			pm, _ := loc.getString("translations.day_periods.pm")
			if tok == "a" {
				am, pm = pyrt.Lower(am), pyrt.Lower(pm)
			}
			cands = []string{am, pm}
		case "MMMM", "MMM", "dddd", "ddd", "dd":
			key := map[string]string{"MMMM": "months.wide", "MMM": "months.abbreviated",
				"dddd": "days.wide", "ddd": "days.abbreviated", "dd": "days.short"}[tok]
			d, _ := loc.translation(key).(*locales.Dict)
			for _, v := range d.Values {
				cands = append(cands, v.(string))
			}
		default:
			return "", newError("AttributeError", "'NoneType' object has no attribute 'values'")
		}
	} else {
		cands = regexTokens[tok]
	}
	if len(cands) == 0 {
		return "", valueError("Unsupported token: %s", tok)
	}
	return "(?P<" + tok + ">" + strings.Join(cands, "|") + ")", nil
}

// checkGroups finds the named groups of a Python pattern and raises re.error for a
// repeated name, as sre_parse does.
func checkGroups(pattern string) ([]string, error) {
	rs := []rune(pattern)
	var names []string
	seen := map[string]int{}
	group := 0
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\\':
			i++
		case '[':
			// skip a character class
			j := i + 1
			if j < len(rs) && rs[j] == '^' {
				j++
			}
			if j < len(rs) && rs[j] == ']' {
				j++
			}
			for j < len(rs) && rs[j] != ']' {
				if rs[j] == '\\' {
					j++
				}
				j++
			}
			i = j
		case '(':
			if i+1 < len(rs) && rs[i+1] == '?' {
				if i+3 < len(rs) && rs[i+2] == 'P' && rs[i+3] == '<' {
					group++
					start := i + 4
					end := start
					for end < len(rs) && rs[end] != '>' {
						end++
					}
					name := string(rs[start:end])
					if prev, ok := seen[name]; ok {
						return nil, newError("error", fmt.Sprintf("redefinition of group name %s as group %d; was group %d at position %d",
							pyrt.Repr(name), group, prev, start))
					}
					seen[name] = group
					names = append(names, name)
				}
				continue
			}
			group++
		}
	}
	return names, nil
}

// goPattern translates the Python pattern for Go's regexp: \d is Python's digit class
// and an escaped non-alphanumeric character is quoted for RE2.
func goPattern(pattern string) string {
	var b strings.Builder
	rs := []rune(pattern)
	inClass := false
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\\' && i+1 < len(rs) {
			n := rs[i+1]
			i++
			switch {
			case n == 'd':
				if inClass {
					b.WriteString(pyDigit[1 : len(pyDigit)-1])
				} else {
					b.WriteString(pyDigit)
				}
			case n >= 'a' && n <= 'z' || n >= 'A' && n <= 'Z' || n >= '0' && n <= '9':
				b.WriteRune('\\')
				b.WriteRune(n)
			default:
				if inClass {
					fmt.Fprintf(&b, `\x{%x}`, n)
				} else {
					b.WriteString(regexp.QuoteMeta(string(n)))
				}
			}
			continue
		}
		if r == '[' && !inClass {
			inClass = true
		} else if r == ']' && inClass {
			inClass = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

type parsedParts struct {
	year, month, day, hour, minute, second, microsecond *int
	tz                                                  Zone
	quarter, dayOfWeek, dayOfYear                       *int
	meridiem                                            string
	timestamp                                           *float64
}

// FromFormat is pendulum.from_format(string, fmt, tz, locale): string parsed with
// pendulum's tokens; tz is the zone for results the string gives none (any tz value
// Datetime accepts), nil locale the current locale.
func FromFormat(str, fmtStr string, tz any, locale *string) (DateTime, error) {
	if tz == nil {
		tz = UTC
	}
	now, err := Now(tz)
	if err != nil {
		return DateTime{}, err
	}
	p, err := formatterParse(str, fmtStr, now, locale)
	if err != nil {
		return DateTime{}, err
	}
	var zone any = tz
	if p.tz != nil {
		zone = p.tz
	}
	return create(*p.year, *p.month, *p.day, *p.hour, *p.minute, *p.second, *p.microsecond, zone, 1, false)
}

func formatterParse(str, fmtStr string, now DateTime, locale *string) (*parsedParts, error) {
	escaped := pyEscape(fmtStr)
	if _, found, _ := fromFormatScan(escaped, func(string) (string, error) { return "", nil }); !found {
		return nil, valueError("The given time string does not match the given format")
	}
	name := GetLocale()
	if locale != nil && *locale != "" {
		name = *locale
	}
	loc, err := LoadLocale(name)
	if err != nil {
		return nil, err
	}
	pattern, _, err := fromFormatScan(escaped, func(tok string) (string, error) { return replaceToken(tok, loc) })
	if err != nil {
		return nil, err
	}
	names, err := checkGroups(pattern)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(goPattern(pattern))
	if err != nil {
		return nil, newError("error", err.Error())
	}
	full, err := regexp.Compile(`^(?:` + goPattern(pattern) + `)$`)
	if err != nil {
		return nil, newError("error", err.Error())
	}
	if !full.MatchString(str) {
		return nil, valueError("String does not match format %s", fmtStr)
	}
	p := &parsedParts{}
	pos := 0
	for pos <= len(str) {
		loc2 := re.FindStringSubmatchIndex(str[pos:])
		if loc2 == nil {
			break
		}
		for k, n := range names {
			idx := re.SubexpIndex(n)
			_ = k
			if loc2[2*idx] < 0 {
				return nil, typeError("expected string or bytes-like object, got 'NoneType'")
			}
			value := str[pos+loc2[2*idx] : pos+loc2[2*idx+1]]
			if localizable[n] {
				if err := parsedLocaleValue(n, value, p, loc); err != nil {
					return nil, err
				}
			} else if err := parsedValue(n, value, p); err != nil {
				return nil, err
			}
		}
		if loc2[1] == loc2[0] {
			_, w := utf8.DecodeRuneInString(str[pos+loc2[1]:])
			if w == 0 {
				break
			}
			pos += loc2[1] + w
		} else {
			pos += loc2[1]
		}
	}
	return checkParsed(p, now)
}

func intOf(s string) (int, error) { return pyrt.ParseInt(s) }

func parsedValue(tok, value string, p *parsedParts) error {
	if !parseTokens[tok] {
		return newError("KeyError", pyrt.Repr(tok))
	}
	switch tok {
	case "X", "x":
		f, err := pyrt.ParseFloat(value)
		if err != nil {
			return err
		}
		if tok == "x" {
			f /= 1e3
		}
		p.timestamp = &f
		return nil
	case "ZZ", "Z":
		negative := strings.HasPrefix(value, "-")
		rs := []rune(value)
		t := string(rs[1:])
		var offH, offM string
		if !strings.Contains(t, ":") {
			if utf8.RuneCountInString(t) == 2 {
				t += "00"
			}
			trs := []rune(t)
			offH, offM = pySlice(trs, 0, 2), pySlice(trs, 2, 4)
		} else {
			offH, offM, _ = strings.Cut(t, ":")
		}
		h, err := intOf(offH)
		if err != nil {
			return err
		}
		m, err := intOf(offM)
		if err != nil {
			return err
		}
		offset := (h*60 + m) * 60
		if negative {
			offset = -offset
		}
		p.tz = FixedTimezoneOf(offset)
		return nil
	case "z":
		found := false
		for _, n := range Timezones() {
			if n == value {
				found = true
				break
			}
		}
		if !found {
			return valueError("Invalid date")
		}
		z, err := TimezoneOf(value)
		if err != nil {
			return err
		}
		p.tz = z
		return nil
	}
	v, err := intOf(value)
	if err != nil {
		return err
	}
	switch {
	case strings.Contains(tok, "Y"):
		if tok == "YY" {
			if v <= 68 {
				v += 2000
			} else {
				v += 1900
			}
		}
		p.year = &v
	case tok == "Q":
		p.quarter = &v
	case tok == "MM" || tok == "M":
		p.month = &v
	case tok == "DDDD" || tok == "DDD":
		p.dayOfYear = &v
	case strings.Contains(tok, "D"):
		p.day = &v
	case strings.Contains(tok, "H"):
		p.hour = &v
	case tok == "hh" || tok == "h":
		if v > 12 {
			return valueError("Invalid date")
		}
		p.hour = &v
	case strings.Contains(tok, "m"):
		p.minute = &v
	case strings.Contains(tok, "s"):
		p.second = &v
	case strings.Contains(tok, "S"):
		mult := map[string]int{"S": 100000, "SS": 10000, "SSS": 1000, "SSSS": 100, "SSSSS": 10, "SSSSSS": 1}[tok]
		us := v * mult
		p.microsecond = &us
	case tok == "d" || tok == "E":
		if tok == "E" {
			v--
		}
		p.dayOfWeek = &v
	}
	return nil
}

func parsedLocaleValue(tok, value string, p *parsedParts, loc *Locale) error {
	var unit, key string
	switch tok {
	case "MMMM":
		unit, key = "month", "months.wide"
	case "MMM":
		unit, key = "month", "months.abbreviated"
	case "Do":
		digits := ""
		for _, r := range value {
			if pyrt.DecimalValue(r) < 0 {
				break
			}
			digits += string(r)
		}
		v, err := intOf(digits)
		if err != nil {
			return err
		}
		p.day = &v
		return nil
	case "dddd":
		unit, key = "day_of_week", "days.wide"
	case "ddd":
		unit, key = "day_of_week", "days.abbreviated"
	case "dd":
		unit, key = "day_of_week", "days.short"
	case "a", "A":
		am, _ := loc.getString("translations.day_periods.am")
		pm, _ := loc.getString("translations.day_periods.pm")
		if tok == "a" {
			value, am, pm = pyrt.Lower(value), pyrt.Lower(am), pyrt.Lower(pm)
		}
		switch value {
		case am:
			p.meridiem = "am"
		case pm:
			p.meridiem = "pm"
		default:
			return valueError("Invalid date")
		}
		return nil
	default:
		return valueError(`Invalid token "%s"`, tok)
	}
	d, _ := loc.translation(key).(*locales.Dict)
	var found *int
	for i, v := range d.Values {
		if v == value {
			k := d.Keys[i].(int)
			found = &k
		}
	}
	if unit == "month" {
		p.month = found
	} else {
		p.dayOfWeek = found
	}
	return nil
}

func checkParsed(p *parsedParts, now DateTime) (*parsedParts, error) {
	v := &parsedParts{year: p.year, month: p.month, day: p.day, hour: p.hour, minute: p.minute,
		second: p.second, microsecond: p.microsecond}
	if p.timestamp != nil {
		s := pyrt.FloatRepr(*p.timestamp)
		us := 0
		if _, frac, ok := strings.Cut(s, "."); ok {
			for utf8.RuneCountInString(frac) < 6 {
				frac += "0"
			}
			n, err := intOf(frac)
			if err != nil {
				return nil, err
			}
			us = n
		}
		f := localTime(*p.timestamp, us)
		v.year, v.month, v.day, v.hour, v.minute, v.second, v.microsecond = &f[0], &f[1], &f[2], &f[3], &f[4], &f[5], &f[6]
		return v, nil
	}
	if p.quarter != nil {
		var dt DateTime
		var err error
		if v.year != nil {
			if dt, err = Datetime(*v.year, 1, 1, 0, 0, 0, 0, UTC, 1, false); err != nil {
				return nil, err
			}
		} else {
			dt = now
		}
		if dt, err = dt.StartOf("year"); err != nil {
			return nil, err
		}
		if *p.quarter < 1 || *p.quarter > 4 {
			return nil, valueError("quarter %d never reached", *p.quarter)
		}
		for dt.Quarter() != *p.quarter {
			if dt, err = dt.Add(AddArgs{Months: 3}); err != nil {
				return nil, err
			}
		}
		v.year, v.month, v.day = ip(dt.Year()), ip(dt.Month()), ip(dt.Day())
	}
	if v.year == nil {
		v.year = ip(now.Year())
	}
	if p.dayOfYear != nil {
		r, err := Parse(fmt.Sprintf("%d-%03d", *v.year, *p.dayOfYear), ParseOptions{Strict: true, YearFirst: true})
		if err != nil {
			return nil, err
		}
		dt := r.(DateTime)
		v.month, v.day = ip(dt.Month()), ip(dt.Day())
	}
	if p.dayOfWeek != nil {
		m, d := now.Month(), now.Day()
		if v.month != nil && *v.month != 0 {
			m = *v.month
		}
		if v.day != nil && *v.day != 0 {
			d = *v.day
		}
		dt, err := Datetime(*v.year, m, d, 0, 0, 0, 0, UTC, 1, false)
		if err != nil {
			return nil, err
		}
		if dt, err = dt.StartOf("week"); err != nil {
			return nil, err
		}
		if dt, err = dt.Subtract(AddArgs{Days: 1}); err != nil {
			return nil, err
		}
		dow := WeekDay(*p.dayOfWeek)
		if dt, err = dt.Next(&dow, false); err != nil {
			return nil, err
		}
		v.year, v.month, v.day = ip(dt.Year()), ip(dt.Month()), ip(dt.Day())
	}
	if p.meridiem != "" {
		if v.hour == nil {
			return nil, valueError("Invalid Date")
		}
		if err := checkMeridiem(v); err != nil {
			return nil, err
		}
		h := *v.hour % 12
		if p.meridiem == "pm" {
			h += 12
		}
		v.hour = &h
	}
	if v.month == nil {
		switch {
		case p.month != nil && *p.month != 0:
			v.month = p.month
		case p.year != nil:
			v.month = ip(1)
		default:
			v.month = ip(now.Month())
		}
	}
	if v.day == nil {
		switch {
		case p.day != nil && *p.day != 0:
			v.day = p.day
		case p.year != nil || p.month != nil:
			v.day = ip(1)
		default:
			v.day = ip(now.Day())
		}
	}
	for _, f := range []**int{&v.hour, &v.minute, &v.second, &v.microsecond} {
		if *f == nil {
			*f = ip(0)
		}
	}
	v.tz = p.tz
	return v, nil
}

// checkMeridiem is the (hour, minute, second, microsecond) >= (13, 0, 0, 0) test.
func checkMeridiem(v *parsedParts) error {
	parts := []*int{v.hour, v.minute, v.second, v.microsecond}
	limit := []int{13, 0, 0, 0}
	for i, p := range parts {
		if p == nil {
			return typeError("'>=' not supported between instances of 'NoneType' and 'int'")
		}
		if *p != limit[i] {
			if *p > limit[i] {
				return valueError("Invalid date")
			}
			return nil
		}
	}
	return valueError("Invalid date")
}

// localTime is pendulum's local_time for a timestamp and offset 0.
func localTime(unix float64, us int) [7]int {
	const secsPerDay = 86400
	year := 1970
	seconds := int64(math.Floor(unix))
	if seconds >= 0 {
		seconds -= 10957 * secsPerDay
		year += 30
	} else {
		seconds += (146097 - 10957) * secsPerDay
		year -= 370
	}
	const s400 = 146097 * secsPerDay
	q := seconds / s400
	if seconds%s400 < 0 {
		q--
	}
	year += 400 * int(q)
	seconds -= q * s400
	s100 := [2]int64{(76*365 + 24*366) * secsPerDay, (75*365 + 25*366) * secsPerDay}
	s4 := [2]int64{(4 * 365) * secsPerDay, (3*365 + 366) * secsPerDay}
	s1 := [2]int64{365 * secsPerDay, 366 * secsPerDay}
	leap := 1
	for seconds >= s100[leap] {
		seconds -= s100[leap]
		year += 100
		leap = 0
	}
	for seconds >= s4[leap] {
		seconds -= s4[leap]
		year += 4
		leap = 1
	}
	for seconds >= s1[leap] {
		seconds -= s1[leap]
		year++
		leap = 0
	}
	month := 12
	day := int(seconds/secsPerDay) + 1
	seconds %= secsPerDay
	for month != 1 {
		off := monthsOffsets[leap][month]
		if day > off {
			day -= off
			break
		}
		month--
	}
	return [7]int{year, month, day, int(seconds / 3600), int(seconds % 3600 / 60), int(seconds % 60), us}
}
