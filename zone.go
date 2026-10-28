package pendulum

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/python-pendulum/pendulum/internal/clocal"
	"github.com/python-pendulum/pendulum/internal/pydt"
	"github.com/python-pendulum/pendulum/internal/pyrt"
	"github.com/python-pendulum/pendulum/internal/zoneinfo"
)

// Zone is a pendulum time zone: a *Timezone (a named IANA zone) or a *FixedTimezone.
type Zone interface {
	// Name is tz.name: the IANA key, or the fixed zone's name.
	Name() string
	// String is repr(tz).
	String() string
	// Convert is tz.convert(dt, raise_on_unknown_times).
	Convert(dt DateTime, raiseOnUnknownTimes bool) (DateTime, error)
	tzinfo() pydt.Tzinfo
}

// Timezone is pendulum.Timezone, a zoneinfo.ZoneInfo over the embedded tzdata 2026.5.
// There is one *Timezone per key, as zoneinfo caches its instances, so two datetimes
// in the same named zone share their tzinfo the way Python's do.
type Timezone struct {
	key string
	zi  *zoneinfo.Zone
}

// FixedTimezone is pendulum.FixedTimezone, a fixed UTC offset with a name.
type FixedTimezone struct {
	offset int
	name   string
}

var (
	tzMu      sync.Mutex
	tzCache   = map[string]*Timezone{}
	fixedMemo = map[int]*FixedTimezone{}
)

// UTC is pendulum.UTC, Timezone("UTC").
var UTC = mustTimezone("UTC")

func mustTimezone(key string) *Timezone {
	tz, err := NewTimezone(key)
	if err != nil {
		panic(err)
	}
	return tz
}

// NewTimezone is Timezone(key): InvalidTimezone when the database has no such zone.
func NewTimezone(key string) (*Timezone, error) {
	tzMu.Lock()
	defer tzMu.Unlock()
	if tz, ok := tzCache[key]; ok {
		return tz, nil
	}
	zi, err := zoneinfo.Load(key)
	if err != nil {
		if e, ok := err.(*zoneinfo.Error); ok {
			if e.Class == "ZoneInfoNotFoundError" {
				return nil, newError("InvalidTimezone", key)
			}
			return nil, newError(e.Class, e.Message)
		}
		return nil, err
	}
	tz := &Timezone{key: key, zi: zi}
	tzCache[key] = tz
	return tz, nil
}

// Name is tz.name, the zone's key.
func (tz *Timezone) Name() string { return tz.key }

// String is repr(tz): Timezone('Europe/Paris').
func (tz *Timezone) String() string { return "Timezone('" + tz.key + "')" }

func (tz *Timezone) tzinfo() pydt.Tzinfo { return namedTz{tz} }

// namedTz is a *Timezone as a datetime.tzinfo; equal values are the same zone.
type namedTz struct{ tz *Timezone }

func seconds(n int) pydt.Timedelta {
	td, _ := pydt.NewTimedelta(0, n, 0)
	return td
}

func (n namedTz) UtcOffset(d pydt.Datetime) (pydt.Timedelta, bool) {
	return seconds(n.tz.zi.UTCOffset(d.LocalSeconds(), d.Fold())), true
}

func (n namedTz) Dst(d pydt.Datetime) (pydt.Timedelta, bool) {
	return seconds(n.tz.zi.DST(d.LocalSeconds(), d.Fold())), true
}

func (n namedTz) TzName(d pydt.Datetime) (string, bool) {
	return n.tz.zi.TZName(d.LocalSeconds(), d.Fold()), true
}

// FromUtc is ZoneInfo.fromutc.
func (n namedTz) FromUtc(d pydt.Datetime) (pydt.Datetime, error) {
	off, fold := n.tz.zi.FromUTC(d.LocalSeconds())
	r, err := d.AddDelta(seconds(off))
	if err != nil {
		return pydt.Datetime{}, err
	}
	return r.WithFold(fold), nil
}

func (n namedTz) String() string { return n.tz.String() }

// convertPlain is Timezone.convert for a plain datetime.
func (tz *Timezone) convertPlain(d pydt.Datetime, raise bool) (pydt.Datetime, error) {
	if d.Tzinfo() != nil {
		return d.Astimezone(tz.tzinfo())
	}
	before, after := tz.foldOffsets(d)
	if after > before {
		if raise {
			return pydt.Datetime{}, newError("NonExistingTime", fmt.Sprintf("The datetime %s does not exist.", d.Isoformat(" ")))
		}
		shift := before - after
		if d.Fold() == 1 {
			shift = after - before
		}
		var err error
		if d, err = d.AddDelta(seconds(shift)); err != nil {
			return pydt.Datetime{}, err
		}
	} else if before > after && raise {
		return pydt.Datetime{}, newError("AmbiguousTime", fmt.Sprintf("The datetime %s is ambiguous.", d.Isoformat(" ")))
	}
	return d.WithTzinfo(tz.tzinfo()), nil
}

func (tz *Timezone) foldOffsets(d pydt.Datetime) (int, int) {
	wall := d.LocalSeconds()
	return tz.zi.UTCOffset(wall, 0), tz.zi.UTCOffset(wall, 1)
}

// Convert is Timezone.convert(dt): a naive dt is placed in the zone (a skipped wall time
// is moved by the gap, in the direction its fold selects), an aware one converted.
func (tz *Timezone) Convert(dt DateTime, raiseOnUnknownTimes bool) (DateTime, error) {
	d := dt.d
	if d.Tzinfo() != nil {
		return dt.Astimezone(tz)
	}
	before, after := tz.foldOffsets(d)
	if after > before {
		if raiseOnUnknownTimes {
			return DateTime{}, newError("NonExistingTime", fmt.Sprintf("The datetime %s does not exist.", dt.String()))
		}
		shift := before - after
		if d.Fold() == 1 {
			shift = after - before
		}
		var err error
		// dt + timedelta goes through DateTime.add(seconds=...).
		if dt, err = dt.add(addArgs{seconds: nf(float64(shift))}); err != nil {
			return DateTime{}, err
		}
	} else if before > after && raiseOnUnknownTimes {
		return DateTime{}, newError("AmbiguousTime", fmt.Sprintf("The datetime %s is ambiguous.", dt.String()))
	}
	return dt.replace(replaceArgs{tz: tz, setTz: true})
}

// NewFixedTimezone is FixedTimezone(offset, name); a nil or empty name becomes the
// signed hours and minutes of the offset.
func NewFixedTimezone(offset int, name *string) *FixedTimezone {
	n := ""
	if name != nil {
		n = *name
	}
	if n == "" {
		sign := "+"
		if offset < 0 {
			sign = "-"
		}
		minutes := int(math.Trunc(float64(offset) / 60))
		if minutes < 0 {
			minutes = -minutes
		}
		n = fmt.Sprintf("%s%02d:%02d", sign, minutes/60, minutes%60)
	}
	return &FixedTimezone{offset: offset, name: n}
}

// FixedTimezoneOf is pendulum.fixed_timezone(offset): one shared instance per offset.
func FixedTimezoneOf(offset int) *FixedTimezone {
	tzMu.Lock()
	defer tzMu.Unlock()
	if tz, ok := fixedMemo[offset]; ok {
		return tz
	}
	tz := NewFixedTimezone(offset, nil)
	fixedMemo[offset] = tz
	return tz
}

// Name is tz.name.
func (tz *FixedTimezone) Name() string { return tz.name }

// Offset is tz.offset in seconds.
func (tz *FixedTimezone) Offset() int { return tz.offset }

// String is repr(tz): FixedTimezone(3600, name="+01:00").
func (tz *FixedTimezone) String() string {
	name := ""
	if tz.name != "" {
		name = `, name="` + tz.name + `"`
	}
	return fmt.Sprintf("FixedTimezone(%d%s)", tz.offset, name)
}

func (tz *FixedTimezone) tzinfo() pydt.Tzinfo { return fixedTz{tz} }

type fixedTz struct{ tz *FixedTimezone }

func (f fixedTz) UtcOffset(pydt.Datetime) (pydt.Timedelta, bool) { return seconds(f.tz.offset), true }
func (f fixedTz) Dst(pydt.Datetime) (pydt.Timedelta, bool)       { return pydt.Timedelta{}, true }
func (f fixedTz) TzName(pydt.Datetime) (string, bool)            { return f.tz.name, true }
func (f fixedTz) String() string                                 { return f.tz.String() }

// FromUtc is FixedTimezone.fromutc: the stdlib addition, then the zone attached.
func (f fixedTz) FromUtc(d pydt.Datetime) (pydt.Datetime, error) {
	r, err := d.AddDelta(seconds(f.tz.offset))
	if err != nil {
		return pydt.Datetime{}, err
	}
	return r.WithFold(0).WithTzinfo(f), nil
}

func (tz *FixedTimezone) convertPlain(d pydt.Datetime) (pydt.Datetime, error) {
	if d.Tzinfo() != nil {
		return d.Astimezone(tz.tzinfo())
	}
	return d.WithTzinfo(tz.tzinfo()).WithFold(0), nil
}

// Convert is FixedTimezone.convert(dt): a naive dt gets the zone with fold 0, an aware
// one is converted.
func (tz *FixedTimezone) Convert(dt DateTime, _ bool) (DateTime, error) {
	if dt.d.Tzinfo() != nil {
		return dt.Astimezone(tz)
	}
	return DateTime{dt.d.WithTzinfo(tz.tzinfo()).WithFold(0)}, nil
}

func zoneOf(t pydt.Tzinfo) Zone {
	switch v := t.(type) {
	case namedTz:
		return v.tz
	case fixedTz:
		return v.tz
	}
	return nil
}

func convertPlain(z Zone, d pydt.Datetime, raise bool) (pydt.Datetime, error) {
	switch v := z.(type) {
	case *Timezone:
		return v.convertPlain(d, raise)
	case *FixedTimezone:
		return v.convertPlain(d)
	}
	return d, nil
}

var (
	localMu   sync.Mutex
	localMock Zone
	localZone Zone
)

// LocalTimezone is pendulum.local_timezone(): the zone set with SetLocalTimezone, or
// the zone the TZ environment variable names when it is first asked for.
func LocalTimezone() Zone {
	localMu.Lock()
	defer localMu.Unlock()
	if localMock != nil {
		return localMock
	}
	if localZone == nil {
		localZone = UTC
		if name, ok := clocal.TZ(); ok && name != "" {
			if name[0] == ':' {
				name = name[1:]
			}
			if tz, err := NewTimezone(name); err == nil {
				localZone = tz
			}
		}
	}
	return localZone
}

// SetLocalTimezone is pendulum.set_local_timezone(mock); nil removes the mock.
func SetLocalTimezone(mock Zone) {
	localMu.Lock()
	defer localMu.Unlock()
	localMock = mock
}

// Timezones is pendulum.timezones(), sorted.
func Timezones() []string {
	names := zoneinfo.Available()
	out := make([]string, len(names))
	copy(out, names)
	sort.Strings(out)
	return out
}

// TimezoneOf is pendulum.timezone(name): name is a string (an IANA key; any casing of
// "utc" is UTC) or an int offset in seconds, which gives a shared FixedTimezone.
func TimezoneOf(name any) (Zone, error) {
	switch v := name.(type) {
	case int:
		return FixedTimezoneOf(v), nil
	case string:
		if pyrt.Lower(v) == "utc" {
			return UTC, nil
		}
		return NewTimezone(v)
	}
	return nil, typeError(fmt.Sprintf("unsupported timezone %T", name))
}

// safeTimezone is pendulum._safe_timezone(obj): a zone as is, nil or "local" for the
// local zone, a number of hours for a fixed offset, and a string for a named zone.
func safeTimezone(obj any) (Zone, error) {
	switch v := obj.(type) {
	case *Timezone:
		if v == nil {
			return LocalTimezone(), nil
		}
		return v, nil
	case *FixedTimezone:
		if v == nil {
			return LocalTimezone(), nil
		}
		return v, nil
	case Zone:
		return v, nil
	case nil:
		return LocalTimezone(), nil
	case string:
		if v == "local" {
			return LocalTimezone(), nil
		}
		return TimezoneOf(v)
	case int:
		return TimezoneOf(v * 60 * 60)
	case float64:
		secs, err := pyrt.Trunc(v * 60 * 60)
		if err != nil {
			return nil, err
		}
		return TimezoneOf(int(secs))
	}
	return nil, typeError(fmt.Sprintf("unsupported timezone argument %T", obj))
}
