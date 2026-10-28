package zoneinfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

// tzifData is what _common.load_data returns.
type tzifData struct {
	transIdx []int
	transUTC []int64
	utcoff   []int64
	isdst    []int64
	abbr     []string
	footer   []byte // nil for a version 1 file
}

// tzifHeader is _common._TZifHeader.
type tzifHeader struct {
	version                                               int
	isutcnt, isstdcnt, leapcnt, timecnt, typecnt, charcnt int
}

// stream mimics the binary file object load_data reads: read returns at
// most n bytes, and skip is fobj.seek(n, 1).
type stream struct {
	b   []byte
	off int
}

func (s *stream) read(n int) []byte {
	if n < 0 || s.off >= len(s.b) {
		return nil
	}
	end := s.off + n
	if end > len(s.b) {
		end = len(s.b)
	}
	p := s.b[s.off:end]
	s.off = end
	return p
}

func (s *stream) skip(n int) error {
	if s.off+n < 0 {
		return &Error{"OSError", "[Errno 22] Invalid argument"}
	}
	s.off += n
	return nil
}

// unpack reads exactly n bytes, failing like struct.unpack on a short read.
func (s *stream) unpack(n int) ([]byte, error) {
	p := s.read(n)
	if len(p) != n {
		return nil, &Error{"error", fmt.Sprintf("unpack requires a buffer of %d bytes", n)}
	}
	return p, nil
}

func readHeader(s *stream) (*tzifHeader, error) {
	if !bytes.Equal(s.read(4), []byte("TZif")) {
		return nil, &Error{"ValueError", "Invalid TZif file: magic not found"}
	}
	h := &tzifHeader{version: 1}
	v := s.read(1)
	if !bytes.Equal(v, []byte{0}) {
		// int(_version): a single ASCII digit, anything else is an error.
		if len(v) != 1 || v[0] < '0' || v[0] > '9' {
			return nil, &Error{"ValueError", "invalid literal for int() with base 10: b" + pyRepr(string(v))}
		}
		h.version = int(v[0] - '0')
	}
	s.read(15)
	p, err := s.unpack(24)
	if err != nil {
		return nil, err
	}
	for i, f := range []*int{&h.isutcnt, &h.isstdcnt, &h.leapcnt, &h.timecnt, &h.typecnt, &h.charcnt} {
		*f = int(int32(binary.BigEndian.Uint32(p[4*i:])))
		if *f < 0 {
			return nil, &Error{"error", "bad char in struct format"}
		}
	}
	return h, nil
}

// loadData is _common.load_data.
func loadData(b []byte) (*tzifData, error) {
	s := &stream{b: b}
	h, err := readHeader(s)
	if err != nil {
		return nil, err
	}
	timeSize := 4
	if h.version != 1 {
		// Version 2+ repeats the data with 64-bit times after the v1 block.
		timeSize = 8
		if err := s.skip(h.timecnt*5 + h.typecnt*6 + h.charcnt + h.leapcnt*8 + h.isstdcnt + h.isutcnt); err != nil {
			return nil, err
		}
		if h, err = readHeader(s); err != nil {
			return nil, err
		}
	}

	d := &tzifData{}
	p, err := s.unpack(h.timecnt * timeSize)
	if err != nil {
		return nil, err
	}
	d.transUTC = make([]int64, h.timecnt)
	for i := range d.transUTC {
		if timeSize == 8 {
			d.transUTC[i] = int64(binary.BigEndian.Uint64(p[8*i:]))
		} else {
			d.transUTC[i] = int64(int32(binary.BigEndian.Uint32(p[4*i:])))
		}
	}
	if p, err = s.unpack(h.timecnt); err != nil {
		return nil, err
	}
	d.transIdx = make([]int, h.timecnt)
	for i, c := range p {
		d.transIdx[i] = int(c)
	}

	abbrind := make([]int, h.typecnt)
	d.utcoff = make([]int64, h.typecnt)
	d.isdst = make([]int64, h.typecnt)
	for i := 0; i < h.typecnt; i++ {
		if p, err = s.unpack(6); err != nil {
			return nil, err
		}
		d.utcoff[i] = int64(int32(binary.BigEndian.Uint32(p)))
		d.isdst[i] = int64(int8(p[4]))
		abbrind[i] = int(int8(p[5]))
	}

	chars := s.read(h.charcnt)
	d.abbr = make([]string, h.typecnt)
	for i, idx := range abbrind {
		if d.abbr[i], err = getAbbr(chars, idx); err != nil {
			return nil, err
		}
	}

	if h.version >= 2 {
		if err := s.skip(h.isutcnt + h.isstdcnt + h.leapcnt*12); err != nil {
			return nil, err
		}
		if c := s.read(1); !bytes.Equal(c, []byte("\n")) {
			return nil, &Error{"AssertionError", "b" + pyRepr(string(c))}
		}
		rest := s.b[min(s.off, len(s.b)):]
		end := bytes.IndexByte(rest, '\n')
		if end < 0 {
			return nil, &Error{"ValueError", "Invalid TZif file: unterminated TZ string"}
		}
		d.footer = rest[:end:end]
	}
	return d, nil
}

// getAbbr is load_data's get_abbr: the bytes from idx up to the next NUL,
// with Python's find and slice semantics (a negative idx counts from the
// end; a missing NUL drops the last byte), decoded as UTF-8.
func getAbbr(chars []byte, idx int) (string, error) {
	n := len(chars)
	clamp := func(i int) int {
		if i < 0 {
			i += n
		}
		return max(0, min(i, n))
	}
	end := -1
	if start := clamp(idx); idx <= n {
		if k := bytes.IndexByte(chars[start:], 0); k >= 0 {
			end = start + k
		}
	}
	lo, hi := clamp(idx), clamp(end)
	if hi < lo {
		hi = lo
	}
	s := chars[lo:hi]
	if !utf8.Valid(s) {
		return "", &Error{"UnicodeDecodeError", "'utf-8' codec can't decode TZif abbreviation"}
	}
	return string(s), nil
}

// newZone is ZoneInfo._load_file applied to the bytes of a TZif file.
func newZone(key string, b []byte) (*Zone, error) {
	d, err := loadData(b)
	if err != nil {
		return nil, err
	}
	for _, idx := range d.transIdx {
		if idx >= len(d.utcoff) {
			return nil, &Error{"IndexError", "tuple index out of range"}
		}
	}
	dstoff, err := utcoffToDstoff(d.transIdx, d.utcoff, d.isdst)
	if err != nil {
		return nil, err
	}
	z := &Zone{key: key, transUTC: d.transUTC, raw: d}
	z.transLocal = tsToLocal(d.transIdx, d.transUTC, d.utcoff)

	list := make([]*ttinfo, len(d.utcoff))
	for i := range list {
		list[i] = &ttinfo{utcoff: d.utcoff[i], dstoff: dstoff[i], tzname: d.abbr[i]}
	}
	z.ttinfos = make([]*ttinfo, len(d.transIdx))
	for i, idx := range d.transIdx {
		z.ttinfos[i] = list[idx]
	}

	// The first standard-time ttinfo, else (as the C accelerator does) the
	// first ttinfo of all.
	for i, dst := range d.isdst {
		if dst == 0 {
			z.before = list[i]
			break
		}
	}
	if z.before == nil && len(list) > 0 {
		z.before = list[0]
	}

	switch {
	case len(d.footer) > 0:
		if !utf8.Valid(d.footer) {
			return nil, &Error{"UnicodeDecodeError", "'utf-8' codec can't decode TZ string"}
		}
		if z.after, err = parseTZStr(string(d.footer)); err != nil {
			return nil, err
		}
	case len(list) == 0:
		return nil, &Error{"ValueError", "No time zone information found."}
	default:
		last := list[len(list)-1]
		if n := len(z.ttinfos); n > 0 {
			last = z.ttinfos[n-1]
		}
		z.after = &tzRule{std: *last, stdOnly: true}
	}
	return z, nil
}

// utcoffToDstoff is ZoneInfo._utcoff_to_dstoff: dst() is not stored in TZif
// files, so it is inferred from the neighbouring standard-time offsets, with
// one hour as the fallback for DST types it cannot pin down.
func utcoffToDstoff(transIdx []int, utcoffs, isdsts []int64) ([]int64, error) {
	typecnt := len(isdsts)
	dstoffs := make([]int64, typecnt)
	var dstCnt int64
	for _, v := range isdsts {
		dstCnt += v
	}
	var dstFound int64
	for i := 1; i < len(transIdx); i++ {
		if dstCnt == dstFound {
			// Every DST type is assigned; Python's for/else skips the
			// one-hour fallback, which would change nothing anyway.
			return dstoffs, nil
		}
		idx := transIdx[i]
		if isdsts[idx] == 0 || dstoffs[idx] != 0 {
			continue
		}
		var dstoff int64
		utcoff := utcoffs[idx]
		comp := transIdx[i-1]
		if isdsts[comp] == 0 {
			dstoff = utcoff - utcoffs[comp]
		}
		if dstoff == 0 && idx < typecnt-1 {
			if i+1 >= len(transIdx) {
				return nil, &Error{"IndexError", "tuple index out of range"}
			}
			comp = transIdx[i+1]
			// A DST neighbour gives nothing to measure against; hope a
			// later transition to this type does.
			if isdsts[comp] != 0 {
				continue
			}
			dstoff = utcoff - utcoffs[comp]
		}
		if dstoff != 0 {
			dstFound++
			dstoffs[idx] = dstoff
		}
	}
	for idx := range dstoffs {
		if dstoffs[idx] == 0 && isdsts[idx] != 0 {
			dstoffs[idx] = 3600
		}
	}
	return dstoffs, nil
}

// tsToLocal is ZoneInfo._ts_to_local: each UTC transition as a wall-clock
// timestamp, using the larger of the offsets on either side for fold 0 and
// the smaller for fold 1.
func tsToLocal(transIdx []int, transUTC, utcoffs []int64) [2][]int64 {
	var wall [2][]int64
	if len(transUTC) == 0 {
		return wall
	}
	wall[0] = append([]int64(nil), transUTC...)
	wall[1] = append([]int64(nil), transUTC...)
	for i := range transIdx {
		var off0, off1 int64
		if i == 0 {
			off0, off1 = utcoffs[0], utcoffs[0]
			if len(utcoffs) > 1 {
				off1 = utcoffs[transIdx[0]]
			}
		} else {
			off0, off1 = utcoffs[transIdx[i-1]], utcoffs[transIdx[i]]
		}
		if off1 > off0 {
			off0, off1 = off1, off0
		}
		wall[0][i] += off0
		wall[1][i] += off1
	}
	return wall
}
