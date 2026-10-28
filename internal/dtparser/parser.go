// Package dtparser is dateutil.parser.parse (python-dateutil 2.9.0.post0), the lenient
// parser pendulum falls back to, with its parserinfo and the tzutc, tzoffset and
// tzlocal zones it attaches.
//
// It is a Go port of python-dateutil's parser module. python-dateutil is
// Copyright (c) 2003-2011 Gustavo Niemeyer, (c) 2012-2014 Tomi Pieviläinen and
// (c) 2014-2016 Yaron de Leeuw, and (c) 2015- Paul Ganssle and dateutil
// contributors, dual licensed under the Apache License 2.0 and the BSD 3-Clause
// License; this port is offered under the same terms.
package dtparser

import (
	"math/big"
	"strings"

	"github.com/python-pendulum/pendulum/internal/pydt"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// Options are the keyword arguments of parser.parse().
type Options struct {
	// Default supplies the fields the string leaves out; nil is today at midnight.
	Default  *pydt.Datetime
	IgnoreTz bool
	// Tzinfos maps time zone names to a pydt.Tzinfo, nil (None) or an int offset
	// in seconds.
	Tzinfos map[string]any
	// TzinfosFunc is a callable tzinfos: it receives the name and offset found (nil for
	// None) and returns what a Tzinfos value may hold. It takes precedence over Tzinfos.
	TzinfosFunc     func(name *string, offset *int) any
	Dayfirst        *bool
	Yearfirst       *bool
	Fuzzy           bool
	FuzzyWithTokens bool
}

// Parser is dateutil.parser.parser.
type Parser struct{ Info *ParserInfo }

// NewParser is parser(info); a nil info is parserinfo().
func NewParser(info *ParserInfo) *Parser {
	if info == nil {
		info = NewParserInfo(false, false)
	}
	return &Parser{Info: info}
}

var defaultParser = NewParser(nil)

// Parse is dateutil.parser.parse(timestr, parserinfo, **kwargs). The skipped tokens
// are returned only when FuzzyWithTokens is set.
func Parse(timestr string, info *ParserInfo, o Options) (pydt.Datetime, []string, error) {
	if info != nil {
		return NewParser(info).Parse(timestr, o)
	}
	return defaultParser.Parse(timestr, o)
}

func recoverPy(err *error) {
	if r := recover(); r != nil {
		e, ok := r.(*pyrt.Error)
		if !ok {
			panic(r)
		}
		*err = e
	}
}

func raise(err error) {
	if err != nil {
		panic(err)
	}
}

// Parse is parser.parse(timestr, default, ignoretz, tzinfos, **kwargs).
func (p *Parser) Parse(timestr string, o Options) (ret pydt.Datetime, skipped []string, err error) {
	defer recoverPy(&err)
	var def pydt.Datetime
	if o.Default != nil {
		def = *o.Default
	} else {
		zero := 0
		def, err = pydt.Now().Replace(pydt.Fields{Hour: &zero, Minute: &zero, Second: &zero, Microsecond: &zero})
		raise(err)
	}
	res, tokens := p.parse(timestr, o)
	if res == nil {
		return pydt.Datetime{}, nil, pyrt.NewError("ParserError", "Unknown string format: "+timestr)
	}
	if res.len() == 0 {
		return pydt.Datetime{}, nil, pyrt.NewError("ParserError", "String does not contain a date: "+timestr)
	}
	ret, err = p.buildNaiveCaught(res, def)
	if err != nil {
		if pyrt.IsInstance(err, "ValueError") {
			e := err.(*pyrt.Error)
			return pydt.Datetime{}, nil, &pyrt.Error{Type: "ParserError", Msg: e.Msg + ": " + timestr, Cause: e}
		}
		return pydt.Datetime{}, nil, err
	}
	if !o.IgnoreTz {
		ret = p.buildTzaware(ret, res, o)
	}
	if o.FuzzyWithTokens {
		if tokens == nil {
			tokens = []string{}
		}
		return ret, tokens, nil
	}
	return ret, nil, nil
}

type pyInt struct {
	v        int
	ok, huge bool
}

func (p *pyInt) set(v int)    { *p = pyInt{v: v, ok: true} }
func (p pyInt) isZero() bool  { return p.ok && !p.huge && p.v == 0 }
func (p pyInt) truthy() bool  { return p.ok && (p.huge || p.v != 0) }
func (p pyInt) ptr() *int     { v := p.v; return &v }
func some(v int) pyInt        { return pyInt{v: v, ok: true} }
func (p pyInt) value() any    { return p.v }
func (p pyInt) intOrNil() any { return p.v }

type pyStr struct {
	v  string
	ok bool
}

func (p *pyStr) set(v string)      { *p = pyStr{v: v, ok: true} }
func (p pyStr) truthy() bool       { return p.ok && p.v != "" }
func (p pyStr) equal(o pyStr) bool { return p.ok == o.ok && (!p.ok || p.v == o.v) }

// result is parser._result.
type result struct {
	year, month, day, weekday, hour, minute, second, microsecond pyInt
	tzname                                                       pyStr
	tzoffset, ampm                                               pyInt
	centurySpecified                                             bool
}

func (r *result) len() int {
	n := 0
	for _, v := range []pyInt{r.year, r.month, r.day, r.weekday, r.hour, r.minute, r.second, r.microsecond, r.tzoffset, r.ampm} {
		if v.ok {
			n++
		}
	}
	if r.tzname.ok {
		n++
	}
	return n
}

func toInt(s string) int {
	v, err := pyrt.ParseInt(s)
	raise(err)
	return v
}

func decInt(d pyrt.Decimal) int {
	v, err := d.Int()
	raise(err)
	return v
}

func toDecimal(val string) pyrt.Decimal {
	d, err := pyrt.ParseDecimal(val)
	if err == nil && !d.IsFinite() {
		err = pyrt.NewError("ValueError", "Converted decimal value is infinite or NaN")
	}
	if err != nil {
		panic(&pyrt.Error{Type: "ValueError", Msg: "Could not convert " + val + " to decimal", Cause: err})
	}
	return d
}

type tokens []string

func (l tokens) at(i int) string {
	if i < 0 {
		i += len(l)
	}
	if i < 0 || i >= len(l) {
		panic(pyrt.IndexError("list index out of range"))
	}
	return l[i]
}

func (p *Parser) parse(timestr string, o Options) (*result, []string) {
	fuzzy := o.Fuzzy || o.FuzzyWithTokens
	info := p.Info
	dayfirst, yearfirst := info.Dayfirst, info.Yearfirst
	if o.Dayfirst != nil {
		dayfirst = *o.Dayfirst
	}
	if o.Yearfirst != nil {
		yearfirst = *o.Yearfirst
	}
	res := &result{}
	l := tokens(splitTokens(timestr))
	var skippedIdxs []int
	y := newYMD()
	lenL := len(l)
	failed := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				if e, ok := r.(*pyrt.Error); ok && (pyrt.IsInstance(e, "IndexError") || pyrt.IsInstance(e, "ValueError")) {
					failed = true
					return
				}
				panic(r)
			}
		}()
		for i := 0; i < lenL; i++ {
			if _, err := pyrt.ParseFloat(l[i]); err == nil {
				i = p.parseNumericToken(l, i, info, y, res, fuzzy)
			} else if v, ok := info.Weekday(l[i]); ok {
				res.weekday.set(v)
			} else if v, ok := info.Month(l[i]); ok {
				y.appendInt(v, 'M')
				if i+1 < lenL {
					if l[i+1] == "-" || l[i+1] == "/" {
						sep := l[i+1]
						y.appendStr(l.at(i+2), 0)
						if i+3 < lenL && l[i+3] == sep {
							y.appendStr(l.at(i+4), 0)
							i += 2
						}
						i += 2
					} else if i+4 < lenL && l[i+1] == " " && l[i+3] == " " && info.Pertain(l[i+2]) {
						if pyrt.StrIsDigit(l[i+4]) {
							year := info.convertyear(toInt(l[i+4]), false)
							y.appendStr(itoa(year), 'Y')
						}
						i += 4
					}
				}
			} else if v, ok := info.Ampm(l[i]); ok {
				if p.ampmValid(res.hour, res.ampm, fuzzy) {
					res.hour.set(adjustAmpm(res.hour.v, v))
					res.ampm.set(v)
				} else if fuzzy {
					skippedIdxs = append(skippedIdxs, i)
				}
			} else if p.couldBeTzname(res.hour, res.tzname, res.tzoffset, l[i]) {
				res.tzname.set(l[i])
				if off, ok := info.Tzoffset(l[i]); ok {
					res.tzoffset = some(off)
				} else {
					res.tzoffset = pyInt{}
				}
				if i+1 < lenL && (l[i+1] == "+" || l[i+1] == "-") {
					if l[i+1] == "+" {
						l[i+1] = "-"
					} else {
						l[i+1] = "+"
					}
					res.tzoffset = pyInt{}
					if info.Utczone(res.tzname.v) {
						res.tzname = pyStr{}
					}
				}
			} else if res.hour.ok && (l[i] == "+" || l[i] == "-") {
				signal := -1
				if l[i] == "+" {
					signal = 1
				}
				next := l.at(i + 1)
				lenLi := pyrt.Len(next)
				var hourOff, minOff int
				switch {
				case lenLi == 4:
					hourOff = toInt(pyrt.Slice(next, 0, 2))
					minOff = toInt(pyrt.From(next, 2))
				case i+2 < lenL && l[i+2] == ":":
					hourOff = toInt(next)
					minOff = toInt(l.at(i + 3))
					i += 2
				case lenLi <= 2:
					hourOff = toInt(pyrt.Slice(next, 0, 2))
					minOff = 0
				default:
					panic(pyrt.NewError("ValueError", timestr))
				}
				res.tzoffset = offsetSeconds(signal, hourOff, minOff)
				if i+5 < lenL && info.Jump(l[i+2]) && l[i+3] == "(" && l[i+5] == ")" &&
					3 <= pyrt.Len(l[i+4]) && p.couldBeTzname(res.hour, res.tzname, pyInt{}, l[i+4]) {
					res.tzname.set(l[i+4])
					i += 4
				}
				i++
			} else if !(info.Jump(l[i]) || fuzzy) {
				panic(pyrt.NewError("ValueError", timestr))
			} else {
				skippedIdxs = append(skippedIdxs, i)
			}
		}
		year, month, day := y.resolve(yearfirst, dayfirst)
		res.centurySpecified = y.centurySpecified
		res.year, res.month, res.day = year, month, day
	}()
	if failed {
		return nil, nil
	}
	if !info.validate(res) {
		return nil, nil
	}
	if o.FuzzyWithTokens {
		return res, recombineSkipped(l, skippedIdxs)
	}
	return res, nil
}

func itoa(v int) string { return big.NewInt(int64(v)).String() }

func offsetSeconds(signal, hourOff, minOff int) pyInt {
	if hourOff == pyrt.BigPos || hourOff == pyrt.BigNeg || minOff == pyrt.BigPos || minOff == pyrt.BigNeg {
		return pyInt{ok: true, huge: true}
	}
	v := new(big.Int).Mul(big.NewInt(int64(hourOff)), big.NewInt(3600))
	v.Add(v, new(big.Int).Mul(big.NewInt(int64(minOff)), big.NewInt(60)))
	v.Mul(v, big.NewInt(int64(signal)))
	if !v.IsInt64() {
		return pyInt{ok: true, huge: true}
	}
	return some(int(v.Int64()))
}

func (p *Parser) parseNumericToken(l tokens, idx int, info *ParserInfo, y *ymd, res *result, fuzzy bool) int {
	valueRepr := l[idx]
	value := func() (d pyrt.Decimal) {
		defer func() {
			if r := recover(); r != nil {
				e, ok := r.(*pyrt.Error)
				if !ok {
					panic(r)
				}
				panic(&pyrt.Error{Type: "ValueError", Msg: "Unknown numeric token", Cause: e})
			}
		}()
		return toDecimal(valueRepr)
	}()
	lenLi := pyrt.Len(valueRepr)
	lenL := len(l)
	hmsAt := func(i int) bool { _, ok := info.Hms(l[i]); return ok }
	switch {
	case len(y.vals) == 3 && (lenLi == 2 || lenLi == 4) && !res.hour.ok &&
		(idx+1 >= lenL || (l[idx+1] != ":" && !hmsAt(idx+1))):
		s := l[idx]
		res.hour.set(toInt(pyrt.Slice(s, 0, 2)))
		if lenLi == 4 {
			res.minute.set(toInt(pyrt.From(s, 2)))
		}
	case lenLi == 6 || (lenLi > 6 && runeIndex(l[idx], '.') == 6):
		s := l[idx]
		if len(y.vals) == 0 && !strings.Contains(s, ".") {
			y.appendStr(pyrt.Slice(s, 0, 2), 0)
			y.appendStr(pyrt.Slice(s, 2, 4), 0)
			y.appendStr(pyrt.From(s, 4), 0)
		} else {
			res.hour.set(toInt(pyrt.Slice(s, 0, 2)))
			res.minute.set(toInt(pyrt.Slice(s, 2, 4)))
			res.second, res.microsecond = parsems(pyrt.From(s, 4))
		}
	case lenLi == 8 || lenLi == 12 || lenLi == 14:
		s := l[idx]
		y.appendStr(pyrt.Slice(s, 0, 4), 'Y')
		y.appendStr(pyrt.Slice(s, 4, 6), 0)
		y.appendStr(pyrt.Slice(s, 6, 8), 0)
		if lenLi > 8 {
			res.hour.set(toInt(pyrt.Slice(s, 8, 10)))
			res.minute.set(toInt(pyrt.Slice(s, 10, 12)))
			if lenLi > 12 {
				res.second.set(toInt(pyrt.From(s, 12)))
			}
		}
	case p.findHmsIdx(idx, l, info, true) >= 0:
		hmsIdx := p.findHmsIdx(idx, l, info, true)
		var hms pyInt
		idx, hms = parseHms(idx, l, info, hmsIdx)
		if hms.ok {
			assignHms(res, valueRepr, hms.v)
		}
	case idx+2 < lenL && l[idx+1] == ":":
		res.hour.set(decInt(value))
		value = toDecimal(l[idx+2])
		res.minute, res.second = parseMinSec(value)
		if idx+4 < lenL && l[idx+3] == ":" {
			res.second, res.microsecond = parsems(l[idx+4])
			idx += 2
		}
		idx += 2
	case idx+1 < lenL && (l[idx+1] == "-" || l[idx+1] == "/" || l[idx+1] == "."):
		sep := l[idx+1]
		y.appendStr(valueRepr, 0)
		if idx+2 < lenL && !info.Jump(l[idx+2]) {
			if pyrt.StrIsDigit(l[idx+2]) {
				y.appendStr(l[idx+2], 0)
			} else if m, ok := info.Month(l[idx+2]); ok {
				y.appendInt(m, 'M')
			} else {
				panic(pyrt.NewError("ValueError", ""))
			}
			if idx+3 < lenL && l[idx+3] == sep {
				if m, ok := info.Month(l.at(idx + 4)); ok {
					y.appendInt(m, 'M')
				} else {
					y.appendStr(l.at(idx+4), 0)
				}
				idx += 2
			}
			idx++
		}
		idx++
	case idx+1 >= lenL || info.Jump(l[idx+1]):
		if idx+2 < lenL {
			if a, ok := info.Ampm(l[idx+2]); ok {
				res.hour.set(adjustAmpm(decInt(value), a))
				idx++
				idx++
				return idx
			}
		}
		y.appendDec(value, 0)
		idx++
	case ampmOK(info, l[idx+1]) && value.CmpInt(0) >= 0 && value.CmpInt(24) < 0:
		a, _ := info.Ampm(l[idx+1])
		res.hour.set(adjustAmpm(decInt(value), a))
		idx++
	case y.couldBeDay(value):
		y.appendDec(value, 0)
	case !fuzzy:
		panic(pyrt.NewError("ValueError", ""))
	}
	return idx
}

func ampmOK(info *ParserInfo, s string) bool { _, ok := info.Ampm(s); return ok }

func runeIndex(s string, c rune) int {
	for i, r := range []rune(s) {
		if r == c {
			return i
		}
	}
	return -1
}

func (p *Parser) findHmsIdx(idx int, l tokens, info *ParserInfo, allowJump bool) int {
	lenL := len(l)
	hms := func(i int) bool { _, ok := info.Hms(l.at(i)); return ok }
	switch {
	case idx+1 < lenL && hms(idx+1):
		return idx + 1
	case allowJump && idx+2 < lenL && l[idx+1] == " " && hms(idx+2):
		return idx + 2
	case idx > 0 && hms(idx-1):
		return idx - 1
	case 1 < idx && idx == lenL-1 && l[idx-1] == " " && hms(idx-2):
		return idx - 2
	}
	return -1
}

func parseHms(idx int, l tokens, info *ParserInfo, hmsIdx int) (int, pyInt) {
	if hmsIdx < 0 {
		return idx, pyInt{}
	}
	v, ok := info.Hms(l.at(hmsIdx))
	if hmsIdx > idx {
		if !ok {
			return hmsIdx, pyInt{}
		}
		return hmsIdx, some(v)
	}
	if !ok {
		panic(pyrt.TypeError("unsupported operand type(s) for +: 'NoneType' and 'int'"))
	}
	return idx, some(v + 1)
}

func assignHms(res *result, valueRepr string, hms int) {
	value := toDecimal(valueRepr)
	switch hms {
	case 0:
		res.hour.set(decInt(value))
		if rem := mod1(value); !rem.IsZero() {
			res.minute.set(decInt(mod1(value).MulInt(60)))
		}
	case 1:
		res.minute, res.second = parseMinSec(value)
	case 2:
		res.second, res.microsecond = parsems(valueRepr)
	}
}

func mod1(d pyrt.Decimal) pyrt.Decimal {
	r, err := d.Mod1()
	raise(err)
	return r
}

func parseMinSec(value pyrt.Decimal) (pyInt, pyInt) {
	minute := some(decInt(value))
	var second pyInt
	if rem := mod1(value); !rem.IsZero() {
		second = some(decInt(rem.MulInt(60)))
	}
	return minute, second
}

func parsems(value string) (pyInt, pyInt) {
	if !strings.Contains(value, ".") {
		return some(toInt(value)), some(0)
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		panic(pyrt.NewError("ValueError", "too many values to unpack (expected 2)"))
	}
	f := parts[1]
	for pyrt.Len(f) < 6 {
		f += "0"
	}
	return some(toInt(parts[0])), some(toInt(pyrt.Slice(f, 0, 6)))
}

func (p *Parser) couldBeTzname(hour pyInt, tzname pyStr, tzoffset pyInt, token string) bool {
	if !hour.ok || tzname.ok || tzoffset.ok || pyrt.Len(token) > 5 {
		return false
	}
	upper := true
	for _, r := range token {
		if r < 'A' || r > 'Z' {
			upper = false
			break
		}
	}
	if upper {
		return true
	}
	for _, z := range p.Info.UTCZONE {
		if z == token {
			return true
		}
	}
	return false
}

func (p *Parser) ampmValid(hour, ampm pyInt, fuzzy bool) bool {
	valid := true
	if fuzzy && ampm.ok {
		valid = false
	}
	if !hour.ok {
		if fuzzy {
			valid = false
		} else {
			panic(pyrt.NewError("ValueError", "No hour specified with AM or PM flag."))
		}
	} else if !(0 <= hour.v && hour.v <= 12) {
		if fuzzy {
			valid = false
		} else {
			panic(pyrt.NewError("ValueError", "Invalid hour specified for 12-hour clock."))
		}
	}
	return valid
}

func adjustAmpm(hour, ampm int) int {
	if hour < 12 && ampm == 1 {
		hour += 12
	} else if hour == 12 && ampm == 0 {
		hour = 0
	}
	return hour
}

func (p *Parser) buildNaiveCaught(res *result, def pydt.Datetime) (ret pydt.Datetime, err error) {
	defer recoverPy(&err)
	return p.buildNaive(res, def), nil
}

func (p *Parser) buildNaive(res *result, def pydt.Datetime) pydt.Datetime {
	var f pydt.Fields
	for _, a := range []struct {
		dst **int
		v   pyInt
	}{{&f.Year, res.year}, {&f.Month, res.month}, {&f.Day, res.day}, {&f.Hour, res.hour},
		{&f.Minute, res.minute}, {&f.Second, res.second}, {&f.Microsecond, res.microsecond}} {
		if a.v.ok {
			*a.dst = a.v.ptr()
		}
	}
	if f.Day == nil {
		cyear, cmonth, cday := def.Year(), def.Month(), def.Day()
		if res.year.ok {
			cyear = res.year.v
		}
		if res.month.ok {
			cmonth = res.month.v
		}
		if _, dim := monthrange(cyear, cmonth); cday > dim {
			_, dim = monthrange(cyear, cmonth)
			f.Day = &dim
		}
	}
	naive, err := def.Replace(f)
	raise(err)
	if res.weekday.ok && !res.day.truthy() {
		x, err := addWeekday(naive, res.weekday.v)
		raise(err)
		naive = x
	}
	return naive
}

func optStr(s pyStr) *string {
	if !s.ok {
		return nil
	}
	v := s.v
	return &v
}

func optInt(v pyInt) *int {
	if !v.ok {
		return nil
	}
	if v.huge {
		x := pyrt.BigPos
		return &x
	}
	return v.ptr()
}

func newTzOffset(name pyStr, offset pyInt) pydt.Tzinfo {
	if offset.huge {
		panic(pyrt.NewError("OverflowError", "Python int too large to convert to C int"))
	}
	t, err := NewTzOffset(optStr(name), offset.v)
	raise(err)
	return t
}

func (p *Parser) buildTzinfo(o Options, res *result) pydt.Tzinfo {
	var data any
	if o.TzinfosFunc != nil {
		data = o.TzinfosFunc(optStr(res.tzname), optInt(res.tzoffset))
	} else {
		data = o.Tzinfos[res.tzname.v]
	}
	switch v := data.(type) {
	case nil:
		return nil
	case pydt.Tzinfo:
		return v
	case int:
		return newTzOffset(res.tzname, some(v))
	}
	panic(pyrt.TypeError("Offset must be tzinfo subclass, tz string, or int offset."))
}

func tzname(dt pydt.Datetime) pyStr {
	name, ok := dt.TzName()
	return pyStr{v: name, ok: ok}
}

func assignTzname(dt pydt.Datetime, want pyStr) pydt.Datetime {
	if !tzname(dt).equal(want) {
		folded := Enfold(dt, 1)
		if tzname(folded).equal(want) {
			return folded
		}
	}
	return dt
}

func (p *Parser) buildTzaware(naive pydt.Datetime, res *result, o Options) pydt.Datetime {
	_, known := o.Tzinfos[res.tzname.v]
	if o.TzinfosFunc != nil || (len(o.Tzinfos) > 0 && res.tzname.ok && known) {
		aware := naive.WithTzinfo(p.buildTzinfo(o, res))
		return assignTzname(aware, res.tzname)
	}
	local := LocalTzNames()
	if res.tzname.truthy() && (res.tzname.v == local[0] || res.tzname.v == local[1]) {
		aware := assignTzname(naive.WithTzinfo(NewTzLocal()), res.tzname)
		if !tzname(aware).equal(res.tzname) {
			for _, z := range p.Info.UTCZONE {
				if z == res.tzname.v {
					return aware.WithTzinfo(UTC)
				}
			}
		}
		return aware
	}
	switch {
	case res.tzoffset.isZero():
		return naive.WithTzinfo(UTC)
	case res.tzoffset.truthy():
		return naive.WithTzinfo(newTzOffset(res.tzname, res.tzoffset))
	case !res.tzname.truthy():
		return naive
	}
	// dateutil warns (UnknownTimezoneWarning) and keeps the naive datetime.
	return naive
}

func recombineSkipped(l tokens, idxs []int) []string {
	out := []string{}
	for i, idx := range idxs {
		if i > 0 && idx-1 == idxs[i-1] {
			out[len(out)-1] += l[idx]
		} else {
			out = append(out, l[idx])
		}
	}
	return out
}
