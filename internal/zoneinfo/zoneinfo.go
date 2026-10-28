// Package zoneinfo reproduces CPython 3.12's zoneinfo.ZoneInfo over an
// embedded copy of the IANA time zone database.
//
// It is a port of Lib/zoneinfo (_zoneinfo.py, _common.py, _tzpath.py) and of
// the _zoneinfo C accelerator that CPython actually runs (CPython is Copyright
// (c) 2001 Python Software Foundation, under the PSF License Agreement). Where the two differ
// the accelerator wins: right after a zone's last explicit transition,
// FromUTC reports fold=1 for as long as the clock repeats wall times relative
// to the offset in force before that transition, which the pure-Python
// fromutc misses (for example Asia/Riyadh in 1947 or America/Metlakatla in
// 2019).
//
// Results are computed from the TZif files alone, never from Go's time
// package: transitions, the inferred dst() offsets, the POSIX TZ footer rules
// and the fold handling follow the Python code step by step, quirks included
// (a zero-based "n" footer day lands one day early, exactly as in CPython).
//
// # Data
//
// zoneinfo.zip holds the zoneinfo/ tree of the Python package tzdata 2026.5,
// which packages IANA tzdata release 2026e: its 598 TZif files, stored
// uncompressed, one entry per key (it is generated outside this module from
// that tree). The tzdata package is distributed under the Apache License
// 2.0; the IANA time zone database is in the public domain. Nothing is read
// from the file system or the environment at run time.
//
// Load also reproduces how CPython resolves keys that are not zones: it
// checks TZPATH (/usr/share/zoneinfo, an exact copy of the package tree) and
// then the tzdata package through importlib, so its errors mention the
// package's site-packages path, and a few odd spellings such as
// "America.Argentina/Cordoba" load successfully.
package zoneinfo

import (
	"sort"
	"sync"
)

// Zone is one loaded IANA zone, with zoneinfo.ZoneInfo semantics. It is
// immutable and safe for concurrent use.
type Zone struct {
	key string

	// transUTC holds the transition instants in UTC; transLocal[f] holds
	// the same transitions as wall-clock timestamps for fold f.
	transUTC   []int64
	transLocal [2][]int64

	// ttinfos[i] is in force from transition i to transition i+1.
	ttinfos []*ttinfo

	// before applies ahead of the first transition.
	before *ttinfo

	// after applies past the last transition: the POSIX TZ footer, or the
	// last ttinfo when the file has no footer.
	after *tzRule

	// raw is the file as read, for the C library's own reading of it.
	raw *tzifData
}

// ttinfo is one local time type: offset from UTC, DST offset, abbreviation.
type ttinfo struct {
	utcoff int64
	dstoff int64
	tzname string
}

var cache struct {
	sync.Mutex
	zones map[string]*Zone
}

// Load is zoneinfo.ZoneInfo(key): one *Zone per key for the life of the
// process (repeat calls return the identical pointer). A failure returns
// *Error carrying the Python exception class and str() of what CPython
// raises for the same key.
func Load(key string) (*Zone, error) {
	cache.Lock()
	defer cache.Unlock()
	if z, ok := cache.zones[key]; ok {
		return z, nil
	}
	data, err := findTZif(key)
	if err != nil {
		return nil, err
	}
	z, err := newZone(key, data)
	if err != nil {
		return nil, err
	}
	if cache.zones == nil {
		cache.zones = make(map[string]*Zone)
	}
	cache.zones[key] = z
	return z, nil
}

// Key is zone.key: the key the zone was loaded with.
func (z *Zone) Key() string {
	return z.key
}

// Available is zoneinfo.available_timezones(), sorted ascending. The caller
// owns the returned slice.
func Available() []string {
	db := database()
	keys := make([]string, 0, len(db.tzif))
	for k := range db.tzif {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Wall-clock and UTC instants below are "local timestamps" exactly as
// _zoneinfo.py's _get_local_timestamp computes them for a naive reading:
// (date.toordinal() - 719163)*86400 + hour*3600 + minute*60 + second
// (microseconds ignored). Years run 1..9999 (the ordinal range of Python's
// datetime); the year the footer rules need is derived from the timestamp.

// UTCOffset is utcoffset(dt) in seconds for a datetime whose wall reading is
// wall and whose fold is fold (0 or 1; any other value counts as 1).
func (z *Zone) UTCOffset(wall int64, fold int) int {
	return int(z.find(wall, fold).utcoff)
}

// DST is dst(dt) in seconds for a datetime whose wall reading is wall and
// whose fold is fold (0 or 1; any other value counts as 1).
func (z *Zone) DST(wall int64, fold int) int {
	return int(z.find(wall, fold).dstoff)
}

// TZName is tzname(dt) for a datetime whose wall reading is wall and whose
// fold is fold (0 or 1; any other value counts as 1).
func (z *Zone) TZName(wall int64, fold int) string {
	return z.find(wall, fold).tzname
}

// FromUTC is fromutc(dt) for a datetime whose UTC reading is utc: it returns
// the offset fromutc adds (tti.utcoff, in seconds) and the fold of the
// result. The caller does the addition (and its OverflowError).
func (z *Zone) FromUTC(utc int64) (offset int, fold int) {
	n := len(z.transUTC)
	var tti *ttinfo
	isFold := false
	switch {
	case n >= 1 && utc < z.transUTC[0]:
		tti = z.before
	case n == 0 || utc > z.transUTC[n-1]:
		tti, isFold = z.after.fromUTC(utc, yearOf(utc))
		// The C accelerator's correction: just after the last explicit
		// transition the fold is relative to the ttinfo in force before
		// it, not to the footer's own STD/DST pair.
		if n > 0 {
			prev := z.before
			if n > 1 {
				prev = z.ttinfos[n-2]
			}
			if diff := prev.utcoff - tti.utcoff; diff > 0 && utc < z.transUTC[n-1]+diff {
				isFold = true
			}
		}
	default:
		idx := bisectRight(z.transUTC, utc)
		prev := z.before
		tti = z.ttinfos[0]
		if idx >= 2 {
			prev, tti = z.ttinfos[idx-2], z.ttinfos[idx-1]
		}
		isFold = prev.utcoff-tti.utcoff > utc-z.transUTC[idx-1]
	}
	if isFold {
		fold = 1
	}
	return int(tti.utcoff), fold
}

// find is _find_trans: the ttinfo for a wall reading and fold.
func (z *Zone) find(wall int64, fold int) *ttinfo {
	if fold != 0 {
		fold = 1
	}
	lt := z.transLocal[fold]
	n := len(lt)
	switch {
	case n > 0 && wall < lt[0]:
		return z.before
	case n == 0 || wall > lt[n-1]:
		return z.after.find(wall, yearOf(wall), fold)
	default:
		return z.ttinfos[bisectRight(lt, wall)-1]
	}
}

// bisectRight is Python's bisect.bisect_right, step for step, so that it
// gives the same answer on the (rare) unsorted wall-time lists.
func bisectRight(a []int64, x int64) int {
	lo, hi := 0, len(a)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if x < a[mid] {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// yearOf returns the proleptic Gregorian year of a local timestamp.
func yearOf(ts int64) int64 {
	z := floorDiv(ts, 86400) + 719468 // days since 0000-03-01
	era := floorDiv(z, 146097)
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	if (5*doy+2)/153 >= 10 { // January or February
		y++
	}
	return y
}

// floorDiv is Python's // for a positive divisor.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b < 0 {
		q--
	}
	return q
}
