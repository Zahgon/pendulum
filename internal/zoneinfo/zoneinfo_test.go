package zoneinfo

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"testing"
)

// testdata/cases.json was recorded with CPython 3.12.15 (C accelerator) and
// tzdata 2026.5 in the reference image. Instants are local timestamps:
// seconds since 1970-01-01 of the naive wall (or UTC) reading.
//
//	available    zoneinfo.available_timezones(), sorted
//	wall         [key, wall, utcoffset0, dst0, tzname0, utcoffset1, dst1, tzname1] for fold 0 and 1
//	fromutc      [key, utc, offset added, fold of the result]
//	errors       [key, exception class, str(exception)], or [key, null, zone.key] when it loads
//	synthetic    TZif files built from {footer, types, trans} with wall and fromutc rows
//	synthErrors  [footer, class, message] raised by _parse_tz_str, or [footer, null, null]
type testCases struct {
	Available   []string      `json:"available"`
	Wall        [][]any       `json:"wall"`
	FromUTC     [][]any       `json:"fromutc"`
	Errors      [][]*string   `json:"errors"`
	Synthetic   []syntheticTZ `json:"synthetic"`
	SynthErrors [][]*string   `json:"synthErrors"`
}

type syntheticTZ struct {
	Footer string    `json:"footer"`
	Types  [][]any   `json:"types"` // [utcoff, isdst, abbr]
	Trans  [][]int64 `json:"trans"` // [utc, type index]
	W      [][]any   `json:"w"`     // [wall, utcoffset0, dst0, tzname0, utcoffset1, dst1, tzname1]
	U      [][]int64 `json:"u"`     // [utc, offset, fold]
}

func loadCases(t *testing.T) *testCases {
	t.Helper()
	b, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c testCases
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func num(v any) int64 { return int64(v.(float64)) }

type wallResult struct {
	Offset, DST int
	Name        string
}

func checkWall(t *testing.T, label string, z *Zone, row []any) {
	t.Helper()
	wall := num(row[0])
	for fold := 0; fold < 2; fold++ {
		want := wallResult{int(num(row[1+3*fold])), int(num(row[2+3*fold])), row[3+3*fold].(string)}
		got := wallResult{z.UTCOffset(wall, fold), z.DST(wall, fold), z.TZName(wall, fold)}
		if got != want {
			t.Errorf("%s wall=%d fold=%d: got %+v, want %+v", label, wall, fold, got, want)
		}
	}
}

func checkFromUTC(t *testing.T, label string, z *Zone, utc, offset, fold int64) {
	t.Helper()
	gotOff, gotFold := z.FromUTC(utc)
	if int64(gotOff) != offset || int64(gotFold) != fold {
		t.Errorf("%s FromUTC(%d): got (offset=%d, fold=%d), want (offset=%d, fold=%d)",
			label, utc, gotOff, gotFold, offset, fold)
	}
}

func TestAvailable(t *testing.T) {
	c := loadCases(t)
	got := Available()
	if !sort.StringsAreSorted(got) {
		t.Error("Available() is not sorted")
	}
	if !reflect.DeepEqual(got, c.Available) {
		t.Fatalf("Available(): got %d keys, want %d (first %q..., want %q...)", len(got), len(c.Available), got[:min(3, len(got))], c.Available[:3])
	}
	got[0] = "mutated"
	if Available()[0] != c.Available[0] {
		t.Error("Available() shares its slice between calls")
	}
	for _, k := range c.Available {
		z, err := Load(k)
		if err != nil {
			t.Errorf("Load(%q): %v", k, err)
			continue
		}
		if z.Key() != k {
			t.Errorf("Load(%q).Key() = %q", k, z.Key())
		}
	}
}

func TestLoadIdentity(t *testing.T) {
	a, err := Load("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Load("Europe/London")
	if a != b {
		t.Error("Load returned different pointers for the same key")
	}
	// The same file under another key is another zone, as in Python.
	c, err := Load("America.Argentina/Cordoba")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Load("America/Argentina/Cordoba")
	if err != nil {
		t.Fatal(err)
	}
	if c == d || c.Key() != "America.Argentina/Cordoba" || c.UTCOffset(0, 0) != d.UTCOffset(0, 0) {
		t.Errorf("Load(America.Argentina/Cordoba) = %p %q, Load(America/Argentina/Cordoba) = %p %q", c, c.Key(), d, d.Key())
	}
}

func TestWall(t *testing.T) {
	for _, row := range loadCases(t).Wall {
		key := row[0].(string)
		z, err := Load(key)
		if err != nil {
			t.Errorf("Load(%q): %v", key, err)
			continue
		}
		checkWall(t, key, z, row[1:])
	}
}

func TestFoldOtherThanZeroOrOne(t *testing.T) {
	z, err := Load("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 2024-11-03 01:30 (repeated) and 2024-03-10 02:30 (skipped).
	for _, wall := range []int64{1730597400, 1710037800} {
		if z.UTCOffset(wall, 0) == z.UTCOffset(wall, 1) {
			t.Errorf("wall=%d: folds 0 and 1 agree (%d)", wall, z.UTCOffset(wall, 0))
		}
		if got, want := z.UTCOffset(wall, 7), z.UTCOffset(wall, 1); got != want {
			t.Errorf("UTCOffset(%d, 7) = %d, want %d (fold 1)", wall, got, want)
		}
	}
}

func TestFromUTC(t *testing.T) {
	for _, row := range loadCases(t).FromUTC {
		key := row[0].(string)
		z, err := Load(key)
		if err != nil {
			t.Errorf("Load(%q): %v", key, err)
			continue
		}
		checkFromUTC(t, key, z, num(row[1]), num(row[2]), num(row[3]))
	}
}

func TestLoadErrors(t *testing.T) {
	for _, row := range loadCases(t).Errors {
		key := *row[0]
		z, err := Load(key)
		if row[1] == nil {
			if err != nil || z.Key() != *row[2] {
				t.Errorf("Load(%q): got (%v, %v), want a zone with key %q", key, z, err, *row[2])
			}
			continue
		}
		want := &Error{Class: *row[1], Message: *row[2]}
		var got *Error
		if !errors.As(err, &got) || *got != *want {
			t.Errorf("Load(%q):\n got  %#v\n want %#v", key, err, want)
			continue
		}
		if z != nil {
			t.Errorf("Load(%q) returned a zone along with %v", key, err)
		}
		if s := err.Error(); s != want.Class+": "+want.Message {
			t.Errorf("Load(%q).Error() = %q", key, s)
		}
	}
}

// buildTZif writes a version 2 TZif file: an empty v1 block, then the given
// types ([utcoff, isdst, abbr]) and transitions ([utc, type index]), then
// the footer.
func buildTZif(types [][]any, trans [][]int64, footer string) []byte {
	var chars []byte
	var abbrIdx []byte
	for _, ty := range types {
		abbrIdx = append(abbrIdx, byte(len(chars)))
		chars = append(append(chars, ty[2].(string)...), 0)
	}
	var b bytes.Buffer
	put := func(v any) { _ = binary.Write(&b, binary.BigEndian, v) }
	header := func(timecnt, typecnt, charcnt int) {
		b.WriteString("TZif2")
		b.Write(make([]byte, 15))
		put([6]int32{0, 0, 0, int32(timecnt), int32(typecnt), int32(charcnt)})
	}
	header(0, 0, 0)
	header(len(trans), len(types), len(chars))
	for _, tr := range trans {
		put(tr[0])
	}
	for _, tr := range trans {
		b.WriteByte(byte(tr[1]))
	}
	for i, ty := range types {
		put(int32(num(ty[0])))
		put(int8(num(ty[1])))
		b.WriteByte(abbrIdx[i])
	}
	b.Write(chars)
	b.WriteString("\n" + footer + "\n")
	return b.Bytes()
}

// TestSyntheticFooters covers footer forms tzdata 2026.5 does not use:
// Julian Jn and zero-based n days, negative and >24h times, negative DST,
// seconds in offsets, files without a footer.
func TestSyntheticFooters(t *testing.T) {
	for _, s := range loadCases(t).Synthetic {
		z, err := newZone("synthetic", buildTZif(s.Types, s.Trans, s.Footer))
		if err != nil {
			t.Errorf("footer %q: %v", s.Footer, err)
			continue
		}
		label := "footer " + s.Footer
		for _, row := range s.W {
			checkWall(t, label, z, row)
		}
		for _, u := range s.U {
			checkFromUTC(t, label, z, u[0], u[1], u[2])
		}
	}
}

func TestSyntheticFooterErrors(t *testing.T) {
	for _, row := range loadCases(t).SynthErrors {
		footer := *row[0]
		_, err := newZone("synthetic", buildTZif(nil, nil, footer))
		if row[1] == nil {
			if err != nil {
				t.Errorf("footer %q: got %v, want success", footer, err)
			}
			continue
		}
		want := &Error{Class: *row[1], Message: *row[2]}
		var got *Error
		if !errors.As(err, &got) || *got != *want {
			t.Errorf("footer %q:\n got  %#v\n want %#v", footer, err, want)
		}
	}
}

func TestNonTZifData(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("TZi"), []byte("TZif"), []byte("TZif2\x00"), []byte("TZifx" + string(make([]byte, 40)))} {
		if _, err := newZone("x", b); err == nil {
			t.Errorf("newZone(%q) succeeded", b)
		}
	}
}
