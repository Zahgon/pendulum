package strftime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// referenceFile is testdata/cases.json: results recorded from CPython 3.12.15
// on glibc 2.36 (see its "source" field for the layout).
type referenceFile struct {
	Source string           `json:"source"`
	Groups []referenceGroup `json:"groups"`
}

type referenceGroup struct {
	TZ    string              `json:"tz"`
	Keys  []string            `json:"keys"`
	Envs  [][2]string         `json:"envs"`
	Objs  [][]json.RawMessage `json:"objs"`
	Cases [][]json.RawMessage `json:"cases"`
}

// referenceObj is one recorded object: its Tm and glibc mktime() of its fields.
type referenceObj struct {
	tm Tm
	mk *int64
}

func decodeObj(keys []string, raw []json.RawMessage) (referenceObj, error) {
	var o referenceObj
	ints := map[string]*int{"Y": &o.tm.Year, "M": &o.tm.Month, "D": &o.tm.Day, "h": &o.tm.Hour,
		"m": &o.tm.Minute, "s": &o.tm.Second, "us": &o.tm.Microsecond, "wd": &o.tm.Weekday,
		"yd": &o.tm.Yday, "dst": &o.tm.IsDST}
	for i, k := range keys {
		var err error
		switch k {
		case "off":
			err = json.Unmarshal(raw[i], &o.tm.UTCOffset)
		case "name":
			err = json.Unmarshal(raw[i], &o.tm.TZName)
		case "mk":
			err = json.Unmarshal(raw[i], &o.mk)
		default:
			err = json.Unmarshal(raw[i], ints[k])
		}
		if err != nil {
			return o, fmt.Errorf("field %s: %w", k, err)
		}
	}
	return o, nil
}

// decodeWant turns a recorded result, a string or [[text, repeat], ...] runs, into a string.
func decodeWant(raw json.RawMessage) (string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var runs [][2]json.RawMessage
	if err := json.Unmarshal(raw, &runs); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, r := range runs {
		var text string
		var n int
		if err := json.Unmarshal(r[0], &text); err != nil {
			return "", err
		}
		if err := json.Unmarshal(r[1], &n); err != nil {
			return "", err
		}
		b.WriteString(strings.Repeat(text, n))
	}
	return b.String(), nil
}

// describe quotes s, or for a long s its length and the part around offset at.
func describe(s string, at int) string {
	if len(s) <= 160 {
		return fmt.Sprintf("%q", s)
	}
	lo, hi := max(0, at-40), min(len(s), at+40)
	return fmt.Sprintf("len %d, bytes %d..%d: %q", len(s), lo, hi, s[lo:hi])
}

// showTm prints t with its optional fields dereferenced.
func showTm(t Tm) string {
	off, name := "None", "None"
	if t.UTCOffset != nil {
		off = fmt.Sprint(*t.UTCOffset)
	}
	if t.TZName != nil {
		name = fmt.Sprintf("%q", *t.TZName)
	}
	return fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%06d wd=%d yd=%d dst=%d off=%s name=%s",
		t.Year, t.Month, t.Day, t.Hour, t.Minute, t.Second, t.Microsecond, t.Weekday, t.Yday, t.IsDST, off, name)
}

func firstDiff(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

func TestReference(t *testing.T) {
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref referenceFile
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, g := range ref.Groups {
		objs := make([]referenceObj, len(g.Objs))
		for i, r := range g.Objs {
			if objs[i], err = decodeObj(g.Keys, r); err != nil {
				t.Fatalf("%s obj %d: %v", g.TZ, i, err)
			}
		}
		for ci, c := range g.Cases {
			var oi, ei int
			var format string
			var pyErr *[2]string
			if err := errors.Join(json.Unmarshal(c[0], &oi), json.Unmarshal(c[1], &ei),
				json.Unmarshal(c[2], &format), json.Unmarshal(c[4], &pyErr)); err != nil {
				t.Fatalf("%s case %d: %v", g.TZ, ci, err)
			}
			o := objs[oi]
			var want string
			if pyErr != nil {
				want = "raises " + pyErr[0] + ": " + pyErr[1]
			} else if want, err = decodeWant(c[3]); err != nil {
				t.Fatalf("%s case %d: %v", g.TZ, ci, err)
			}

			var mkProblem string
			env := Env{TZName: g.Envs[ei], Mktime: func(a Tm) (int64, bool) {
				// glibc's %s sees the fields after checktm and the tm_isdst clamp.
				w := o.tm
				w.Month, w.Day, w.IsDST = max(w.Month, 1), max(w.Day, 1), max(-1, min(1, w.IsDST))
				if a.Year != w.Year || a.Month != w.Month || a.Day != w.Day || a.Hour != w.Hour ||
					a.Minute != w.Minute || a.Second != w.Second || a.IsDST != w.IsDST {
					mkProblem = fmt.Sprintf("; Mktime called with %+v", a)
				}
				if o.mk == nil {
					return 0, false
				}
				return *o.mk, true
			}}
			res, err := Format(format, o.tm, env)
			got := res
			if err != nil {
				var pe *Error
				if !errors.As(err, &pe) {
					t.Fatalf("%s case %d: error %v is not an *Error", g.TZ, ci, err)
				}
				got = "raises " + pe.Class + ": " + pe.Message
			}
			total++
			if got != want || mkProblem != "" {
				at := firstDiff(want, got)
				t.Errorf("TZ=%s case %d: Format(%s) with %s, tzname %q\n  want %s\n  got  %s%s",
					g.TZ, ci, describe(format, 0), showTm(o.tm), env.TZName, describe(want, at), describe(got, at), mkProblem)
			}
		}
	}
	if total < 1000 {
		t.Errorf("only %d reference cases ran", total)
	}
}

func TestErrorString(t *testing.T) {
	e := &Error{Class: "ValueError", Message: "month out of range"}
	if got, want := e.Error(), "ValueError: month out of range"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestMktimeFailurePrintsMinusOne(t *testing.T) {
	// glibc prints mktime's -1 error return as is; a nil Mktime counts as failing.
	// Observed in the reference image: TZ=XST-5:30XDT,M3.5.0,M10.5.0 and
	// time.strftime("%s|%5s|%05s", (-2**31+1900, 1, 1, 0, 0, 0, 0, 1, -1)).
	tm := Tm{Year: 2024, Month: 1, Day: 1, Weekday: 0, Yday: 1, IsDST: -1}
	for _, env := range []Env{{}, {Mktime: func(Tm) (int64, bool) { return 42, false }}} {
		got, err := Format("%s|%5s|%05s", tm, env)
		if want := "-1|   -1|000-1"; got != want || err != nil {
			t.Errorf("Format = %q, %v; want %q, nil", got, err, want)
		}
	}
}

func FuzzFormat(f *testing.F) {
	for _, s := range []string{"", "%", "%c%z%:z%Z%f%s", "%#Z%^10Ez%-_0^#5EOq", "é%5é\x00%",
		"%2147483647Y", "%99999999999999999999d", "%:%::z%%z"} {
		f.Add(s, 2024, 2, 29, 23, 0, 3, 60, 1, int64(-1), true)
	}
	f.Fuzz(func(t *testing.T, format string, year, month, day, hour, wday, yday, us, isdst int, off int64, aware bool) {
		tm := Tm{Year: year, Month: month, Day: day, Hour: hour, Weekday: wday, Yday: yday,
			Microsecond: us, IsDST: isdst}
		if aware {
			name := format
			tm.UTCOffset, tm.TZName = &off, &name
		}
		env := Env{TZName: [2]string{"STD", "DST"}, Mktime: func(Tm) (int64, bool) { return off, off != -1 }}
		got, err := Format(format, tm, env)
		if err != nil {
			var pe *Error
			if !errors.As(err, &pe) || got != "" {
				t.Fatalf("Format(%q) = %q, %v", format, got, err)
			}
		}
	})
}
