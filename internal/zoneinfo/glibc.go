package zoneinfo

// The C library reads the same TZif files differently from zoneinfo: localtime()
// takes tm_isdst from the file's isdst flags (or the footer rule), and each call
// rewrites the global tzname[] pair, which strftime's own %Z consults. These
// functions reproduce glibc 2.36 (time/tzfile.c and time/tzset.c) for a zone.

// footerNames are the footer's standard and DST abbreviations; a footer without DST
// repeats the standard one.
func (z *Zone) footerNames() (string, string, bool) {
	if len(z.raw.footer) == 0 {
		return "", "", false
	}
	if z.after.stdOnly {
		return z.after.std.tzname, z.after.std.tzname, true
	}
	return z.after.std.tzname, z.after.dst.tzname, true
}

// GlibcTzsetNames is tzname[] as __tzfile_read leaves it after tzset(): the
// abbreviations of the last standard and last DST types transitioned to.
func (z *Zone) GlibcTzsetNames() [2]string {
	d := z.raw
	var names [2]*string
	for i := len(d.transIdx); i > 0; {
		i--
		t := d.transIdx[i]
		dst := boolIdx(d.isdst[t])
		if names[dst] == nil {
			n := d.abbr[t]
			names[dst] = &n
			if names[1-dst] != nil {
				break
			}
		}
	}
	if names[0] == nil && len(d.abbr) > 0 {
		n := d.abbr[0]
		names[0] = &n
	}
	if names[1] == nil {
		names[1] = names[0]
	}
	return [2]string{deref(names[0]), deref(names[1])}
}

// GlibcLocal is what localtime(t) reports from the C library for the UTC instant t:
// the tm_isdst flag and the tzname[] pair the call leaves behind.
func (z *Zone) GlibcLocal(t int64) (isdst int, names [2]string) {
	d := z.raw
	n := len(d.transUTC)
	var nm [2]*string
	if n == 0 || t < d.transUTC[0] {
		i := 0
		for i < len(d.isdst) && d.isdst[i] != 0 {
			if nm[1] == nil {
				s := d.abbr[i]
				nm[1] = &s
			}
			i++
		}
		if i == len(d.isdst) {
			i = 0
		}
		if len(d.abbr) == 0 {
			return 0, [2]string{"UTC", "UTC"}
		}
		s := d.abbr[i]
		nm[0] = &s
		if nm[1] == nil {
			for j := i; j < len(d.isdst); j++ {
				if d.isdst[j] != 0 {
					s := d.abbr[j]
					nm[1] = &s
					break
				}
			}
		}
		return int(d.isdst[i]), [2]string{deref(nm[0]), deref(nm[1])}
	}
	var i int
	if t >= d.transUTC[n-1] {
		if std, dst, ok := z.footerNames(); ok {
			isdst = 0
			wall := t + z.after.std.utcoff
			if !z.after.stdOnly {
				off, fold := z.FromUTC(t)
				wall = t + int64(off)
				if z.after.find(wall, yearOf(wall), fold) == &z.after.dst {
					isdst = 1
				}
			}
			return isdst, [2]string{std, dst}
		}
		i = n
	} else {
		lo, hi := 0, n-1
		for lo+1 < hi {
			m := (lo + hi) / 2
			if t < d.transUTC[m] {
				hi = m
			} else {
				lo = m
			}
		}
		i = hi
		for i > 0 && t < d.transUTC[i-1] {
			i--
		}
		for i < n && t >= d.transUTC[i] {
			i++
		}
	}
	cur := d.transIdx[i-1]
	s := d.abbr[cur]
	nm[boolIdx(d.isdst[cur])] = &s
	for j := i; j < n; j++ {
		tp := d.transIdx[j]
		dst := boolIdx(d.isdst[tp])
		if nm[dst] == nil {
			s := d.abbr[tp]
			nm[dst] = &s
			if nm[1-dst] != nil {
				break
			}
		}
	}
	if nm[0] == nil {
		nm[0] = nm[1]
	}
	return int(d.isdst[cur]), [2]string{deref(nm[0]), deref(nm[1])}
}

func boolIdx(v int64) int {
	if v != 0 {
		return 1
	}
	return 0
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
