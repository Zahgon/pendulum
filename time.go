package pendulum

import (
	"fmt"

	"github.com/python-pendulum/pendulum/internal/pydt"
)

// Time is pendulum.Time, a time of day without a zone.
type Time struct{ t pydt.Time }

// NewTime is pendulum.time(hour, minute, second, microsecond).
func NewTime(hour, minute, second, microsecond int) (Time, error) {
	t, err := pydt.NewTime(hour, minute, second, microsecond, nil)
	return Time{t}, err
}

// Hour, Minute, Second and Microsecond are the time's fields.
func (t Time) Hour() int        { return t.t.Hour() }
func (t Time) Minute() int      { return t.t.Minute() }
func (t Time) Second() int      { return t.t.Second() }
func (t Time) Microsecond() int { return t.t.Microsecond() }

// Equal is t == other and Compare orders two times.
func (t Time) Equal(other Time) bool  { return t.t.Compare(other.t) == 0 }
func (t Time) Compare(other Time) int { return t.t.Compare(other.t) }

// Closest is t.closest(t1, t2) and Farthest t.farthest(t1, t2).
func (t Time) Closest(t1, t2 Time) (Time, error)  { return t.pick(t1, t2, -1) }
func (t Time) Farthest(t1, t2 Time) (Time, error) { return t.pick(t1, t2, 1) }

func (t Time) pick(t1, t2 Time, want int) (Time, error) {
	a, err := t.Diff(&t1, true)
	if err != nil {
		return Time{}, err
	}
	b, err := t.Diff(&t2, true)
	if err != nil {
		return Time{}, err
	}
	if (want < 0 && a.InSeconds() < b.InSeconds()) || (want > 0 && a.InSeconds() > b.InSeconds()) {
		return t1, nil
	}
	return t2, nil
}

var epoch = func() DateTime {
	d, _ := newDT(1970, 1, 1, 0, 0, 0, 0, UTC, 0)
	return d
}()

// Add is t.add(hours, minutes, seconds, microseconds), wrapping around midnight.
func (t Time) Add(hours, minutes, seconds, microseconds int) (Time, error) {
	return t.shift(addArgs{hours: ni(hours), minutes: ni(minutes), seconds: ni(seconds), microseconds: ni(microseconds)})
}

// Subtract is t.subtract(hours, minutes, seconds, microseconds).
func (t Time) Subtract(hours, minutes, seconds, microseconds int) (Time, error) {
	return t.shift(addArgs{hours: ni(hours), minutes: ni(minutes), seconds: ni(seconds), microseconds: ni(microseconds)}.neg())
}

func (t Time) shift(a addArgs) (Time, error) {
	d, err := epoch.At(t.Hour(), t.Minute(), t.Second(), t.Microsecond())
	if err != nil {
		return Time{}, err
	}
	if d, err = d.add(a); err != nil {
		return Time{}, err
	}
	return d.Time(), nil
}

// AddDuration is t + d (t.add_timedelta(d)): a delta with days is a TypeError.
func (t Time) AddDuration(d Duration) (Time, error) {
	if d.Days() != 0 {
		return Time{}, typeError("Cannot add timedelta with days to Time.")
	}
	return t.Add(0, 0, d.Seconds(), d.Microseconds())
}

// SubDuration is t - d (t.subtract_timedelta(d)).
func (t Time) SubDuration(d Duration) (Time, error) {
	if d.Days() != 0 {
		return Time{}, typeError("Cannot subtract timedelta with days to Time.")
	}
	return t.Subtract(0, 0, d.Seconds(), d.Microseconds())
}

// Sub is t - other: other.diff(t, False).
func (t Time) Sub(other Time) (Duration, error) { return other.Diff(&t, false) }

// Diff is t.diff(other, abs): the whole seconds between the two (microseconds are not
// counted); nil is the current time.
func (t Time) Diff(other *Time, abs bool) (Duration, error) {
	var o Time
	if other == nil {
		n, err := Now(nil)
		if err != nil {
			return Duration{}, err
		}
		o = n.Time()
	} else {
		o = *other
	}
	us1 := int64(t.Hour()*3600+t.Minute()*60+t.Second()) * 1000000
	us2 := int64(o.Hour()*3600+o.Minute()*60+o.Second()) * 1000000
	if abs {
		return newAbsoluteDuration(durationArgs{microseconds: num{i: us2 - us1}})
	}
	return newDuration(durationArgs{microseconds: num{i: us2 - us1}})
}

// DiffForHumans is t.diff_for_humans(other, absolute, locale).
func (t Time) DiffForHumans(other *Time, absolute bool, locale *string) (string, error) {
	isNow := other == nil
	diff, err := t.Diff(other, true)
	if err != nil {
		return "", err
	}
	return formatDiffDuration(diff, nil, isNow, absolute, locale)
}

// Replace is t.replace(hour, minute, second, microsecond); nil keeps a field.
func (t Time) Replace(hour, minute, second, microsecond *int) (Time, error) {
	f := [4]int{t.Hour(), t.Minute(), t.Second(), t.Microsecond()}
	for i, p := range []*int{hour, minute, second, microsecond} {
		if p != nil {
			f[i] = *p
		}
	}
	return NewTime(f[0], f[1], f[2], f[3])
}

// Isoformat is t.isoformat() and String str(t).
func (t Time) Isoformat() string { return t.t.Isoformat() }
func (t Time) String() string    { return t.t.Isoformat() }

// Repr is repr(t): Time(10, 20, 0) or Time(10, 20, 0, 500).
func (t Time) Repr() string {
	us := ""
	if t.Microsecond() != 0 {
		us = fmt.Sprintf(", %d", t.Microsecond())
	}
	return fmt.Sprintf("Time(%d, %d, %d%s)", t.Hour(), t.Minute(), t.Second(), us)
}

// ForJSON is t.for_json().
func (t Time) ForJSON() string { return t.Isoformat() }
