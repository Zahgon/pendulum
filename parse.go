package pendulum

import (
	"strings"

	"github.com/python-pendulum/pendulum/internal/dtparser"
	"github.com/python-pendulum/pendulum/internal/pydt"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// ParseOptions are the keyword arguments of pendulum.parse(). Every field is passed;
// the original's defaults are Strict true, DayFirst false, YearFirst true and Exact
// false. Tz is the zone given to results that carry none (nil: the option is absent,
// which means UTC).
type ParseOptions struct {
	Tz        any
	Strict    bool
	DayFirst  bool
	YearFirst bool
	Exact     bool
}

// Parse is pendulum.parse(text, ...): a DateTime, Date, Time, Duration or Interval.
func Parse(text string, o ParseOptions) (any, error) {
	tz := o.Tz
	if tz == nil {
		tz = UTC
	}
	if text == "now" {
		return Now(tz)
	}
	parsed, err := baseParse(text, o)
	if err != nil {
		return nil, err
	}
	switch v := parsed.(type) {
	case pydt.Datetime:
		var t any = tz
		if z := v.Tzinfo(); z != nil {
			zone, err := zoneFromTzinfo(z)
			if err != nil {
				return nil, err
			}
			t = zone
		}
		return create(v.Year(), v.Month(), v.Day(), v.Hour(), v.Minute(), v.Second(), v.Microsecond(), t, 1, false)
	case pydt.Date:
		return NewDate(v.Year(), v.Month(), v.Day())
	case pydt.Time:
		return NewTime(v.Hour(), v.Minute(), v.Second(), v.Microsecond())
	case Duration:
		return v, nil
	case isoInterval:
		return v.build(tz)
	}
	return nil, newError("NotImplementedError", "")
}

// zoneFromTzinfo is _safe_timezone for the tzinfo a parser attached.
func zoneFromTzinfo(t pydt.Tzinfo) (Zone, error) {
	if z := zoneOf(t); z != nil {
		return z, nil
	}
	switch v := t.(type) {
	case *dtparser.TzUtc:
		return UTC, nil
	case *dtparser.TzOffset:
		if v.HasName && v.Name == "UTC" {
			return UTC, nil
		}
		return TimezoneOf(int(v.Offset.TotalMicroseconds() / 1000000))
	case *dtparser.TzLocal:
		if v.HasDst {
			// tzlocal().tzname(None) consults dst rules for a None datetime.
			return nil, newError("AttributeError", "'NoneType' object has no attribute 'replace'")
		}
		if v.Names[0] == "UTC" {
			return UTC, nil
		}
		return TimezoneOf(int(v.StdOffset.TotalMicroseconds() / 1000000))
	}
	return nil, typeError("unsupported tzinfo")
}

// baseParse is pendulum.parsing.parse: the parse, then _normalize.
func baseParse(text string, o ParseOptions) (any, error) {
	parsed, err := parseAny(text, o)
	if err != nil {
		return nil, err
	}
	if o.Exact {
		return parsed, nil
	}
	switch v := parsed.(type) {
	case pydt.Time:
		now := pydt.Now()
		return pydt.NewDatetime(now.Year(), now.Month(), now.Day(), v.Hour(), v.Minute(), v.Second(), v.Microsecond(), nil, 0)
	case pydt.Date:
		return pydt.NewDatetime(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, nil, 0)
	}
	return parsed, nil
}

func isValueError(err error) bool { return pyrt.IsInstance(err, "ValueError") }

func isoValue(r isoResult) any {
	switch r.kind {
	case "datetime":
		return r.dt
	case "date":
		return r.date
	case "time":
		return r.t
	}
	return r.dur
}

func parseAny(text string, o ParseOptions) (any, error) {
	r, err := parseISO8601(text)
	if err == nil {
		return isoValue(r), nil
	}
	if !isValueError(err) {
		return nil, err
	}
	iv, err := parseISO8601Interval(text)
	if err == nil {
		return iv, nil
	}
	if !isValueError(err) {
		return nil, err
	}
	c, err := parseCommon(text, o)
	if err == nil {
		return c, nil
	}
	if !pyrt.IsInstance(err, "ParserError") {
		return nil, err
	}
	if o.Strict {
		return nil, parserError("Unable to parse string [%s]", text)
	}
	df, yf := o.DayFirst, o.YearFirst
	dt, _, err := dtparser.Parse(text, nil, dtparser.Options{Dayfirst: &df, Yearfirst: &yf})
	if err != nil {
		if isValueError(err) {
			return nil, parserError("Invalid date string: %s", text)
		}
		return nil, err
	}
	return dt, nil
}

var commonRE = pyRegexp(`^(?P<date>(?P<classic>(?P<year>\d{4})(?P<monthday>(?P<monthsep>[/:])?(?P<month>\d{2})((?P<daysep>[/:])?(?P<day>\d{2})))?))?` +
	`(?P<time>(?P<timesep> )?(?P<hour>\d{1,2}):(?P<minute>\d{1,2})?(?::(?P<second>\d{1,2}))?(?P<subsecondsection>(?:[.|,])(?P<subsecond>\d{1,9}))?)?$`)

// parseCommon is _parse_common.
func parseCommon(text string, o ParseOptions) (any, error) {
	m := fullmatch(commonRE, text)
	if m == nil || (m.g("date") == "" && m.g("time") == "") {
		return nil, parserError("Invalid datetime string")
	}
	hasDate := false
	year, month, day := 0, 1, 1
	if m.g("date") != "" {
		hasDate = true
		year = pyInt(m.g("year"))
		if m.g("monthday") != "" {
			if o.DayFirst {
				month, day = pyInt(m.g("day")), pyInt(m.g("month"))
			} else {
				month, day = pyInt(m.g("month")), pyInt(m.g("day"))
			}
		}
	}
	if m.g("time") == "" {
		return pydt.NewDate(year, month, day)
	}
	hour := pyInt(m.g("hour"))
	if _, ok := m.group("minute"); !ok {
		return nil, typeError("int() argument must be a string, a bytes-like object or a real number, not 'NoneType'")
	}
	minute := pyInt(m.g("minute"))
	second := 0
	if m.g("second") != "" {
		second = pyInt(m.g("second"))
	}
	microsecond := 0
	if m.g("subsecondsection") != "" {
		microsecond = subsecondMicros(m.g("subsecond"))
	}
	if hasDate {
		return pydt.NewDatetime(year, month, day, hour, minute, second, microsecond, nil, 0)
	}
	return pydt.NewTime(hour, minute, second, microsecond, nil)
}

// isoInterval is pendulum.parsing._Interval.
type isoInterval struct {
	start, end *isoResult
	duration   *Duration
}

func parseISO8601Interval(text string) (isoInterval, error) {
	if !strings.Contains(text, "/") {
		return isoInterval{}, parserError("Invalid interval")
	}
	parts := strings.Split(text, "/")
	if len(parts) != 2 {
		return isoInterval{}, valueError("too many values to unpack (expected 2)")
	}
	first, last := parts[0], parts[1]
	if first == "" || last == "" {
		return isoInterval{}, parserError("Invalid interval.")
	}
	var iv isoInterval
	parse := func(s string) (*isoResult, error) {
		r, err := parseISO8601(s)
		if err != nil {
			return nil, err
		}
		return &r, nil
	}
	var err error
	switch {
	case strings.HasPrefix(first, "P"):
		d, e := parse(first)
		if e != nil {
			return isoInterval{}, e
		}
		iv.duration = asDuration(d)
		iv.end, err = parse(last)
	case strings.HasPrefix(last, "P"):
		if iv.start, err = parse(first); err != nil {
			return isoInterval{}, err
		}
		d, e := parse(last)
		if e != nil {
			return isoInterval{}, e
		}
		iv.duration = asDuration(d)
	default:
		if iv.start, err = parse(first); err != nil {
			return isoInterval{}, err
		}
		iv.end, err = parse(last)
	}
	if err != nil {
		return isoInterval{}, err
	}
	if iv.duration == nil {
		iv.duration = nil
	}
	return iv, nil
}

// asDuration keeps a parsed duration; any other value (a datetime parsed where the
// original expects a duration) is carried as is and fails where the original fails.
func asDuration(r *isoResult) *Duration {
	if r.kind == "duration" {
		d := r.dur
		return &d
	}
	return &Duration{years: -1 << 62}
}

func (r *isoResult) instance(tz any) (any, error) {
	switch r.kind {
	case "datetime":
		return instance(r.dt, tz)
	case "date":
		return NewDate(r.date.Year(), r.date.Month(), r.date.Day())
	case "time":
		return NewTime(r.t.Hour(), r.t.Minute(), r.t.Second(), r.t.Microsecond())
	}
	return nil, newError("AttributeError", "'Duration' object has no attribute 'tzinfo'")
}

// build is the _Interval branch of pendulum.parser._parse.
func (iv isoInterval) build(tz any) (any, error) {
	if iv.duration != nil {
		d := *iv.duration
		if d.years == -1<<62 {
			return nil, newError("AttributeError", "'DateTime' object has no attribute 'years'")
		}
		if iv.start != nil {
			v, err := iv.start.instance(tz)
			if err != nil {
				return nil, err
			}
			end, err := shiftBy(v, d, 1)
			if err != nil {
				return nil, err
			}
			return newIntervalAny(v, end)
		}
		v, err := iv.end.instance(tz)
		if err != nil {
			return nil, err
		}
		start, err := shiftBy(v, d, -1)
		if err != nil {
			return nil, err
		}
		return newIntervalAny(start, v)
	}
	s, err := iv.start.instance(tz)
	if err != nil {
		return nil, err
	}
	e, err := iv.end.instance(tz)
	if err != nil {
		return nil, err
	}
	return newIntervalAny(s, e)
}

// newIntervalAny is pendulum.interval over parsed ends. A Time end carries the
// parse's zone, and Interval's own subtraction of its ends then fails as the
// original's does.
func newIntervalAny(start, end any) (any, error) {
	_, st := start.(Time)
	_, et := end.(Time)
	_, sd := start.(Date)
	_, ed := end.(Date)
	switch {
	case st && et:
		return nil, typeError("Cannot subtract aware times to or from Time.")
	case sd && et:
		return nil, typeError("unsupported operand type(s) for -: 'Time' and 'datetime.date'")
	case st && ed:
		return nil, typeError("unsupported operand type(s) for -: 'datetime.date' and 'Time'")
	case st || et:
		return nil, valueError("Both start and end of an Interval must have the same type")
	}
	return newInterval(start, end, false)
}

// shiftBy is dt.add(...) or dt.subtract(...) with every unit of d, as
// pendulum.parser does for an interval given by one end and a duration.
func shiftBy(v any, d Duration, dir int) (any, error) {
	a := addArgs{years: ni(d.Years()), months: ni(d.Months()), weeks: ni(d.Weeks()), days: ni(d.RemainingDays()),
		hours: ni(d.Hours()), minutes: ni(d.Minutes()), seconds: ni(d.RemainingSeconds()), microseconds: ni(d.Microseconds())}
	if dir < 0 {
		a = a.neg()
	}
	switch x := v.(type) {
	case DateTime:
		return x.add(a)
	case Date:
		method := "add"
		if dir < 0 {
			method = "subtract"
		}
		return nil, typeError("Date." + method + "() got an unexpected keyword argument 'hours'")
	case Time:
		method := "add"
		if dir < 0 {
			method = "subtract"
		}
		return nil, typeError("Time." + method + "() got an unexpected keyword argument 'years'")
	}
	return nil, typeError("unsupported interval end")
}
