package dtparser

import (
	"fmt"

	"github.com/python-pendulum/pendulum/internal/clocal"
	"github.com/python-pendulum/pendulum/internal/pydt"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

var zero pydt.Timedelta

func secs(n int) pydt.Timedelta { td, _ := pydt.NewTimedelta(0, n, 0); return td }

// TzUtc is dateutil.tz.tzutc.
type TzUtc struct{}

// UTC is the tzutc() singleton.
var UTC = &TzUtc{}

func (*TzUtc) UtcOffset(pydt.Datetime) (pydt.Timedelta, bool) { return zero, true }
func (*TzUtc) Dst(pydt.Datetime) (pydt.Timedelta, bool)       { return zero, true }
func (*TzUtc) TzName(pydt.Datetime) (string, bool)            { return "UTC", true }
func (*TzUtc) FromUtc(d pydt.Datetime) (pydt.Datetime, error) { return d, nil }
func (*TzUtc) String() string                                 { return "tzutc()" }

// TzOffset is dateutil.tz.tzoffset: a fixed offset with an optional name.
type TzOffset struct {
	Name    string
	HasName bool
	Offset  pydt.Timedelta
}

// NewTzOffset is tzoffset(name, offset) with the offset in seconds; nil name is None.
func NewTzOffset(name *string, offset int) (*TzOffset, error) {
	td, err := pydt.NewTimedelta(0, offset, 0)
	if err != nil {
		return nil, err
	}
	t := &TzOffset{Offset: td}
	if name != nil {
		t.Name, t.HasName = *name, true
	}
	return t, nil
}

func (t *TzOffset) UtcOffset(pydt.Datetime) (pydt.Timedelta, bool) { return t.Offset, true }
func (t *TzOffset) Dst(pydt.Datetime) (pydt.Timedelta, bool)       { return zero, true }
func (t *TzOffset) TzName(pydt.Datetime) (string, bool)            { return t.Name, t.HasName }
func (t *TzOffset) FromUtc(d pydt.Datetime) (pydt.Datetime, error) { return d.AddDelta(t.Offset) }

func (t *TzOffset) String() string {
	name := "None"
	if t.HasName {
		name = pyrt.Repr(t.Name)
	}
	return fmt.Sprintf("tzoffset(%s, %d)", name, t.Offset.TotalMicroseconds()/1000000)
}

// TzLocal is dateutil.tz.tzlocal over the process's C-library local zone, described
// by time.timezone, time.altzone and time.tzname.
type TzLocal struct {
	StdOffset, DstOffset pydt.Timedelta
	HasDst               bool
	Names                [2]string
	stdSeconds           int
}

// LocalTzNames is time.tzname.
func LocalTzNames() [2]string { return clocal.TzName() }

// NewTzLocal is tzlocal().
func NewTzLocal() *TzLocal {
	t := &TzLocal{stdSeconds: clocal.Timezone(), Names: clocal.TzName()}
	t.StdOffset = secs(-clocal.Timezone())
	t.DstOffset = t.StdOffset
	if clocal.Daylight() != 0 {
		t.DstOffset = secs(-clocal.Altzone())
	}
	t.HasDst = t.DstOffset != t.StdOffset
	return t
}

func (t *TzLocal) naiveIsDst(dt pydt.Datetime) bool {
	ts := dt.LocalSeconds() + int64(t.stdSeconds)
	_, isdst, _ := clocal.Local(ts)
	return isdst != 0
}

func (t *TzLocal) isAmbiguous(dt pydt.Datetime) bool {
	naive := t.naiveIsDst(dt)
	saved, _ := t.DstOffset.Sub(t.StdOffset)
	earlier, err := dt.SubDelta(saved)
	if err != nil {
		return false
	}
	return !naive && naive != t.naiveIsDst(earlier)
}

func (t *TzLocal) isDst(dt pydt.Datetime) bool {
	if !t.HasDst {
		return false
	}
	dst := t.naiveIsDst(dt)
	if t.isAmbiguous(dt) {
		return dt.Fold() == 0
	}
	return dst
}

func (t *TzLocal) UtcOffset(dt pydt.Datetime) (pydt.Timedelta, bool) {
	if t.isDst(dt) {
		return t.DstOffset, true
	}
	return t.StdOffset, true
}

func (t *TzLocal) Dst(dt pydt.Datetime) (pydt.Timedelta, bool) {
	if t.isDst(dt) {
		d, _ := t.DstOffset.Sub(t.StdOffset)
		return d, true
	}
	return zero, true
}

func (t *TzLocal) TzName(dt pydt.Datetime) (string, bool) {
	if t.isDst(dt) {
		return t.Names[1], true
	}
	return t.Names[0], true
}

func (t *TzLocal) FromUtc(d pydt.Datetime) (pydt.Datetime, error) {
	off, _ := t.UtcOffset(d)
	return d.AddDelta(off)
}

func (t *TzLocal) String() string { return "tzlocal()" }

// Enfold is dateutil.tz.enfold.
func Enfold(dt pydt.Datetime, fold int) pydt.Datetime { return dt.WithFold(fold) }

// addWeekday is naive + relativedelta(weekday=wd) for a plain weekday number.
func addWeekday(naive pydt.Datetime, wd int) (pydt.Datetime, error) {
	jump := (7 - naive.Weekday() + wd) % 7
	td, err := pydt.NewTimedelta(jump, 0, 0)
	if err != nil {
		return pydt.Datetime{}, err
	}
	return naive.AddDelta(td)
}
