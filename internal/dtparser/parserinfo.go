package dtparser

import (
	"time"

	"github.com/python-pendulum/pendulum/internal/pydt"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// Tables are the class attributes a parserinfo subclass overrides. A nil field keeps
// the default table. Each entry of WEEKDAYS, MONTHS, HMS and AMPM lists the names of
// one value, as the tuples of the Python tables do.
type Tables struct {
	JUMP     []string
	WEEKDAYS [][]string
	MONTHS   [][]string
	HMS      [][]string
	AMPM     [][]string
	UTCZONE  []string
	PERTAIN  []string
	TZOFFSET map[string]int
}

// DefaultTables returns parserinfo's own tables.
func DefaultTables() Tables {
	return Tables{
		JUMP: []string{" ", ".", ",", ";", "-", "/", "'", "at", "on", "and", "ad", "m", "t", "of", "st", "nd", "rd", "th"},
		WEEKDAYS: [][]string{{"Mon", "Monday"}, {"Tue", "Tuesday"}, {"Wed", "Wednesday"}, {"Thu", "Thursday"},
			{"Fri", "Friday"}, {"Sat", "Saturday"}, {"Sun", "Sunday"}},
		MONTHS: [][]string{{"Jan", "January"}, {"Feb", "February"}, {"Mar", "March"}, {"Apr", "April"},
			{"May", "May"}, {"Jun", "June"}, {"Jul", "July"}, {"Aug", "August"}, {"Sep", "Sept", "September"},
			{"Oct", "October"}, {"Nov", "November"}, {"Dec", "December"}},
		HMS:      [][]string{{"h", "hour", "hours"}, {"m", "minute", "minutes"}, {"s", "second", "seconds"}},
		AMPM:     [][]string{{"am", "a"}, {"pm", "p"}},
		UTCZONE:  []string{"UTC", "GMT", "Z", "z"},
		PERTAIN:  []string{"of"},
		TZOFFSET: map[string]int{},
	}
}

// ParserInfo is dateutil.parser.parserinfo.
type ParserInfo struct {
	Tables
	Dayfirst, Yearfirst bool

	jump, weekdays, months, hms, ampm, utczone, pertain map[string]int
	year, century                                       int
}

// NewParserInfo is parserinfo(dayfirst, yearfirst).
func NewParserInfo(dayfirst, yearfirst bool) *ParserInfo {
	return NewCustomParserInfo(Tables{}, dayfirst, yearfirst)
}

// NewCustomParserInfo is a parserinfo subclass that overrides the non-nil tables of t.
func NewCustomParserInfo(t Tables, dayfirst, yearfirst bool) *ParserInfo {
	d := DefaultTables()
	if t.JUMP != nil {
		d.JUMP = t.JUMP
	}
	if t.WEEKDAYS != nil {
		d.WEEKDAYS = t.WEEKDAYS
	}
	if t.MONTHS != nil {
		d.MONTHS = t.MONTHS
	}
	if t.HMS != nil {
		d.HMS = t.HMS
	}
	if t.AMPM != nil {
		d.AMPM = t.AMPM
	}
	if t.UTCZONE != nil {
		d.UTCZONE = t.UTCZONE
	}
	if t.PERTAIN != nil {
		d.PERTAIN = t.PERTAIN
	}
	if t.TZOFFSET != nil {
		d.TZOFFSET = t.TZOFFSET
	}
	p := &ParserInfo{Tables: d, Dayfirst: dayfirst, Yearfirst: yearfirst}
	p.jump = convertFlat(d.JUMP)
	p.weekdays = convert(d.WEEKDAYS)
	p.months = convert(d.MONTHS)
	p.hms = convert(d.HMS)
	p.ampm = convert(d.AMPM)
	p.utczone = convertFlat(d.UTCZONE)
	p.pertain = convertFlat(d.PERTAIN)
	p.year = time.Now().Year()
	p.century = p.year / 100 * 100
	return p
}

func convert(entries [][]string) map[string]int {
	m := map[string]int{}
	for i, names := range entries {
		for _, n := range names {
			m[pyrt.Lower(n)] = i
		}
	}
	return m
}

func convertFlat(names []string) map[string]int {
	m := map[string]int{}
	for i, n := range names {
		m[pyrt.Lower(n)] = i
	}
	return m
}

func lookup(m map[string]int, name string) (int, bool) {
	v, ok := m[pyrt.Lower(name)]
	return v, ok
}

// Jump is parserinfo.jump(name).
func (p *ParserInfo) Jump(name string) bool { _, ok := lookup(p.jump, name); return ok }

// Weekday is parserinfo.weekday(name); ok is false for None.
func (p *ParserInfo) Weekday(name string) (int, bool) { return lookup(p.weekdays, name) }

// Month is parserinfo.month(name), counting from 1.
func (p *ParserInfo) Month(name string) (int, bool) {
	v, ok := lookup(p.months, name)
	return v + 1, ok
}

// Hms is parserinfo.hms(name).
func (p *ParserInfo) Hms(name string) (int, bool) { return lookup(p.hms, name) }

// Ampm is parserinfo.ampm(name).
func (p *ParserInfo) Ampm(name string) (int, bool) { return lookup(p.ampm, name) }

// Pertain is parserinfo.pertain(name).
func (p *ParserInfo) Pertain(name string) bool { _, ok := lookup(p.pertain, name); return ok }

// Utczone is parserinfo.utczone(name).
func (p *ParserInfo) Utczone(name string) bool { _, ok := lookup(p.utczone, name); return ok }

// Tzoffset is parserinfo.tzoffset(name). The UTC names match case-sensitively against
// their lowercased keys, as in Python.
func (p *ParserInfo) Tzoffset(name string) (int, bool) {
	if _, ok := p.utczone[name]; ok {
		return 0, true
	}
	v, ok := p.TZOFFSET[name]
	return v, ok
}

// Convertyear is parserinfo.convertyear(year, century_specified). A negative year fails the
// original's assertion: an AssertionError with an empty message.
func (p *ParserInfo) Convertyear(year int, centurySpecified bool) (int, error) {
	if year < 0 {
		return 0, pyrt.NewError("AssertionError", "")
	}
	return p.convertyear(year, centurySpecified), nil
}

// convertyear serves the parser's own calls, whose years come from digit tokens.
func (p *ParserInfo) convertyear(year int, centurySpecified bool) int {
	if year < 100 && !centurySpecified {
		year += p.century
		if year >= p.year+50 {
			year -= 100
		} else if year < p.year-50 {
			year += 100
		}
	}
	return year
}

func (p *ParserInfo) validate(res *result) bool {
	if res.year.ok {
		res.year.set(p.convertyear(res.year.v, res.centurySpecified))
	}
	if (res.tzoffset.isZero() && !res.tzname.truthy()) || (res.tzname.ok && (res.tzname.v == "Z" || res.tzname.v == "z")) {
		res.tzname.set("UTC")
		res.tzoffset = pyInt{ok: true}
	} else if !res.tzoffset.isZero() && res.tzname.truthy() && p.Utczone(res.tzname.v) {
		res.tzoffset = pyInt{ok: true}
	}
	return true
}

// ymd is _ymd: the year, month and day candidates in the order they were seen.
type ymd struct {
	vals                      []int
	centurySpecified          bool
	dstridx, mstridx, ystridx int
}

func newYMD() *ymd { return &ymd{dstridx: -1, mstridx: -1, ystridx: -1} }

func (y *ymd) hasYear() bool  { return y.ystridx >= 0 }
func (y *ymd) hasMonth() bool { return y.mstridx >= 0 }
func (y *ymd) hasDay() bool   { return y.dstridx >= 0 }

func (y *ymd) at(i int) int {
	if i < 0 {
		i += len(y.vals)
	}
	if i < 0 || i >= len(y.vals) {
		panic(pyrt.IndexError("list index out of range"))
	}
	return y.vals[i]
}

func (y *ymd) couldBeDay(value pyrt.Decimal) bool {
	in := func(hi int) bool { return value.CmpInt(1) >= 0 && value.CmpInt(hi) <= 0 }
	switch {
	case y.hasDay():
		return false
	case !y.hasMonth():
		return in(31)
	case !y.hasYear():
		_, dim := monthrange(2000, y.at(y.mstridx))
		return in(dim)
	}
	_, dim := monthrange(y.at(y.ystridx), y.at(y.mstridx))
	return in(dim)
}

func monthrange(year, month int) (int, int) {
	wd, n, err := pydt.MonthRange(year, month)
	if err != nil {
		panic(err)
	}
	return wd, n
}

func (y *ymd) century(label byte) byte {
	y.centurySpecified = true
	if label != 0 && label != 'Y' {
		panic(pyrt.NewError("ValueError", string(label)))
	}
	return 'Y'
}

func (y *ymd) appendStr(val string, label byte) {
	if pyrt.StrIsDigit(val) && pyrt.Len(val) > 2 {
		label = y.century(label)
	}
	v, err := pyrt.ParseInt(val)
	if err != nil {
		panic(err)
	}
	y.push(v, label)
}

func (y *ymd) appendDec(val pyrt.Decimal, label byte) {
	if val.CmpInt(100) > 0 {
		label = y.century(label)
	}
	v, err := val.Int()
	if err != nil {
		panic(err)
	}
	y.push(v, label)
}

func (y *ymd) appendInt(val int, label byte) {
	if val > 100 {
		label = y.century(label)
	}
	y.push(val, label)
}

func (y *ymd) push(v int, label byte) {
	y.vals = append(y.vals, v)
	idx := len(y.vals) - 1
	switch label {
	case 'M':
		if y.hasMonth() {
			panic(pyrt.NewError("ValueError", "Month is already set"))
		}
		y.mstridx = idx
	case 'D':
		if y.hasDay() {
			panic(pyrt.NewError("ValueError", "Day is already set"))
		}
		y.dstridx = idx
	case 'Y':
		if y.hasYear() {
			panic(pyrt.NewError("ValueError", "Year is already set"))
		}
		y.ystridx = idx
	}
}

func (y *ymd) resolve(yearfirst, dayfirst bool) (year, month, day pyInt) {
	type strid struct {
		key byte
		idx int
	}
	var strids []strid
	for _, s := range []strid{{'y', y.ystridx}, {'m', y.mstridx}, {'d', y.dstridx}} {
		if s.idx >= 0 {
			strids = append(strids, s)
		}
	}
	n := len(y.vals)
	if (n == len(strids) && n > 0) || (n == 3 && len(strids) == 2) {
		if n == 3 && len(strids) == 2 {
			used := map[int]bool{}
			keys := map[byte]bool{}
			for _, s := range strids {
				used[s.idx] = true
				keys[s.key] = true
			}
			missing := -1
			for x := 0; x < 3; x++ {
				if !used[x] {
					missing = x
				}
			}
			for _, k := range []byte{'y', 'm', 'd'} {
				if !keys[k] {
					strids = append(strids, strid{k, missing})
				}
			}
		}
		for _, s := range strids {
			v := pyInt{v: y.at(s.idx), ok: true}
			switch s.key {
			case 'y':
				year = v
			case 'm':
				month = v
			case 'd':
				day = v
			}
		}
		return
	}
	set := func(p *pyInt, v int) { *p = pyInt{v: v, ok: true} }
	mstridx := y.mstridx
	switch {
	case n > 3:
		panic(pyrt.NewError("ValueError", "More than three YMD values"))
	case n == 1 || (mstridx >= 0 && n == 2):
		var other int
		if mstridx >= 0 {
			set(&month, y.at(mstridx))
			other = y.at(mstridx - 1)
		} else {
			other = y.at(0)
		}
		if n > 1 || mstridx < 0 {
			if other > 31 {
				set(&year, other)
			} else {
				set(&day, other)
			}
		}
	case n == 2:
		a, b := y.vals[0], y.vals[1]
		switch {
		case a > 31:
			set(&year, a)
			set(&month, b)
		case b > 31:
			set(&month, a)
			set(&year, b)
		case dayfirst && b <= 12:
			set(&day, a)
			set(&month, b)
		default:
			set(&month, a)
			set(&day, b)
		}
	case n == 3:
		a, b, c := y.vals[0], y.vals[1], y.vals[2]
		order := func(p1, p2, p3 *pyInt) { set(p1, a); set(p2, b); set(p3, c) }
		switch {
		case mstridx == 0:
			if b > 31 {
				order(&month, &year, &day)
			} else {
				order(&month, &day, &year)
			}
		case mstridx == 1:
			if a > 31 || (yearfirst && c <= 31) {
				order(&year, &month, &day)
			} else {
				order(&day, &month, &year)
			}
		case mstridx == 2:
			if b > 31 {
				order(&day, &year, &month)
			} else {
				order(&year, &day, &month)
			}
		case a > 31 || y.ystridx == 0 || (yearfirst && b <= 12 && c <= 31):
			if dayfirst && c <= 12 {
				order(&year, &day, &month)
			} else {
				order(&year, &month, &day)
			}
		case a > 12 || (dayfirst && b <= 12):
			order(&day, &month, &year)
		default:
			order(&month, &day, &year)
		}
	}
	return
}
