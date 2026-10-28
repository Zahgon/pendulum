// Package clocal is the C library's local time as CPython 3.12 on glibc sees it: the
// zone the TZ environment variable names, which time-machine may replace while
// travelling, the values of time.tzname, time.timezone, time.altzone and
// time.daylight, localtime() and mktime() in that zone, and the tzname[] pair that
// each localtime() call leaves behind for strftime's own %Z.
package clocal

import (
	"os"
	"sync"
	"time"

	"github.com/python-pendulum/pendulum/internal/zoneinfo"
)

// state is what tzset() leaves behind.
type state struct {
	zone     *zoneinfo.Zone // nil: UTC
	tzname   [2]string
	timezone int
	altzone  int
	daylight int
}

var (
	mu   sync.Mutex
	cur  *state
	tz   *string // the TZ value tzset last read; nil while unset
	live [2]string
)

func init() {
	if v, ok := os.LookupEnv("TZ"); ok {
		tz = &v
	}
}

// SetTZ is os.environ["TZ"] = name (or its deletion for nil) followed by time.tzset().
func SetTZ(name *string) {
	mu.Lock()
	tz = name
	cur = nil
	mu.Unlock()
	load()
}

// TZ is the TZ value in effect, and whether it is set.
func TZ() (string, bool) {
	mu.Lock()
	defer mu.Unlock()
	if tz == nil {
		return "", false
	}
	return *tz, true
}

func load() *state {
	mu.Lock()
	if cur != nil {
		s := cur
		mu.Unlock()
		return s
	}
	s := &state{}
	if tz != nil {
		name := *tz
		if len(name) > 0 && name[0] == ':' {
			name = name[1:]
		}
		if z, err := zoneinfo.Load(name); err == nil {
			s.zone = z
		}
	}
	live = [2]string{"UTC", "UTC"}
	if s.zone != nil {
		live = s.zone.GlibcTzsetNames()
	}
	cur = s
	mu.Unlock()
	// CPython's init_timezone reads localtime() near January 1 and July 1 of the
	// current year (by the real clock, which time-machine does not reach), and takes
	// the zone with the larger offset as the daylight one.
	const year = (365*24 + 6) * 3600
	t := time.Now().Unix() / year * year
	janOff, _, janName := Local(t)
	julOff, _, julName := Local(t + year/2)
	janzone, julzone := -janOff, -julOff
	mu.Lock()
	defer mu.Unlock()
	if janzone < julzone {
		s.timezone, s.altzone, s.tzname = julzone, janzone, [2]string{julName, janName}
	} else {
		s.timezone, s.altzone, s.tzname = janzone, julzone, [2]string{janName, julName}
	}
	if janzone != julzone {
		s.daylight = 1
	}
	return s
}

func zone() *zoneinfo.Zone {
	mu.Lock()
	s := cur
	mu.Unlock()
	if s == nil {
		s = load()
	}
	return s.zone
}

// TzName is time.tzname.
func TzName() [2]string { return load().tzname }

// LiveTzName is the C library's tzname[] as the last localtime() or mktime() left it.
func LiveTzName() [2]string {
	load()
	mu.Lock()
	defer mu.Unlock()
	return live
}

// Timezone is time.timezone: seconds west of UTC of the standard zone.
func Timezone() int { return load().timezone }

// Altzone is time.altzone.
func Altzone() int { return load().altzone }

// Daylight is time.daylight.
func Daylight() int { return load().daylight }

// Local is localtime(utc): the UTC offset, the tm_isdst flag and the abbreviation in
// force at the UTC instant utc (seconds since the epoch). Like glibc's, the call
// rewrites the live tzname[] pair.
func Local(utc int64) (offset int, isdst int, abbr string) {
	z := zone()
	if z == nil {
		return 0, 0, "UTC"
	}
	off, fold := z.FromUTC(utc)
	wall := utc + int64(off)
	dst, names := z.GlibcLocal(utc)
	mu.Lock()
	live = names
	mu.Unlock()
	return off, dst, z.TZName(wall, fold)
}

// Mktime is glibc's mktime() for a broken-down local time given as wall (seconds since
// the epoch of the naive reading) and the tm_isdst hint (-1, 0 or 1).
func Mktime(wall int64, isdst int) (int64, bool) {
	t := wall
	for i := 0; i < 6; i++ {
		off, _, _ := Local(t)
		next := wall - int64(off)
		if next == t {
			break
		}
		t = next
	}
	off, dst, _ := Local(t)
	if t+int64(off) != wall {
		// The reading is in a gap; glibc keeps the offset in force before it.
		before, _, _ := Local(t - 86400)
		t = wall - int64(before)
		off, dst, _ = Local(t)
	}
	if isdst >= 0 && (dst != 0) != (isdst != 0) {
		// tm_isdst disagrees: use the offset of the nearest instant with the requested
		// flag, probing outwards as glibc does.
		const stride, bound = 601200, 536454000/2 + 601200
		for delta := int64(stride); delta < bound; delta += stride {
			for _, dir := range []int64{-1, 1} {
				ot := t + delta*dir
				ooff, odst, _ := Local(ot)
				if (odst != 0) == (isdst != 0) {
					gt := t + int64(off) - int64(ooff)
					Local(gt)
					return gt, true
				}
			}
		}
		// No instant has the requested flag: shift by an hour per flag of difference.
		gt := t + int64(dst-isdst)*3600
		Local(gt)
		return gt, true
	}
	Local(t)
	return t, true
}
