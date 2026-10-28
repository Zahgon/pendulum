package pydt

import (
	"fmt"
	"math"
	"math/big"

	"github.com/python-pendulum/pendulum/internal/pyrt"
)

const maxDeltaDays = 999999999

// Timedelta is a datetime.timedelta, normalised as CPython stores it:
// 0 <= seconds < 86400 and 0 <= microseconds < 1000000.
type Timedelta struct{ days, seconds, microseconds int }

func (t Timedelta) Days() int         { return t.days }
func (t Timedelta) Seconds() int      { return t.seconds }
func (t Timedelta) Microseconds() int { return t.microseconds }

func checkDeltaDays(days int) error {
	if days < -maxDeltaDays || days > maxDeltaDays {
		return pyrt.NewError("OverflowError", fmt.Sprintf("days=%d; must have magnitude <= %d", days, maxDeltaDays))
	}
	return nil
}

// newDelta is CPython's new_delta(d, s, us, normalize=1).
func newDelta(d, s, us int) (Timedelta, error) {
	if us < 0 || us >= 1000000 {
		normalizePair(&s, &us, 1000000)
	}
	if s < 0 || s >= 86400 {
		normalizePair(&d, &s, 86400)
	}
	if err := checkDeltaDays(d); err != nil {
		return Timedelta{}, err
	}
	return Timedelta{d, s, us}, nil
}

var (
	usPerSecond = big.NewInt(1000000)
	secsPerDay  = big.NewInt(86400)
)

func floorDivmod(x, y *big.Int) (*big.Int, *big.Int) {
	q, m := new(big.Int).DivMod(x, y, new(big.Int))
	return q, m
}

// microsecondsToDelta is microseconds_to_delta_ex.
func microsecondsToDelta(x *big.Int) (Timedelta, error) {
	secs, us := floorDivmod(x, usPerSecond)
	days, s := floorDivmod(secs, secsPerDay)
	if !days.IsInt64() || days.Int64() > math.MaxInt32 || days.Int64() < math.MinInt32 {
		return Timedelta{}, pyrt.NewError("OverflowError", "Python int too large to convert to C int")
	}
	d := int(days.Int64())
	if err := checkDeltaDays(d); err != nil {
		return Timedelta{}, err
	}
	return Timedelta{d, int(s.Int64()), int(us.Int64())}, nil
}

// NewTimedelta is datetime.timedelta(days=, seconds=, microseconds=) with int arguments.
func NewTimedelta(days, seconds, microseconds int) (Timedelta, error) {
	x := big.NewInt(int64(microseconds))
	x.Add(x, new(big.Int).Mul(big.NewInt(int64(seconds)), usPerSecond))
	x.Add(x, new(big.Int).Mul(big.NewInt(int64(days)), big.NewInt(86400000000)))
	return microsecondsToDelta(x)
}

func bigFromIntegral(v float64) (*big.Int, error) {
	if math.IsNaN(v) {
		return nil, pyrt.NewError("ValueError", "cannot convert float NaN to integer")
	}
	if math.IsInf(v, 0) {
		return nil, pyrt.NewError("OverflowError", "cannot convert float infinity to integer")
	}
	i, _ := big.NewFloat(v).Int(nil)
	return i, nil
}

// accum is the float branch of CPython's timedelta accum().
func accum(sum *big.Int, num float64, factor int64, leftover *float64) error {
	intpart, frac := math.Modf(num)
	if math.IsInf(num, 0) {
		intpart, frac = num, 0
	}
	x, err := bigFromIntegral(intpart)
	if err != nil {
		return err
	}
	sum.Add(sum, x.Mul(x, big.NewInt(factor)))
	if frac == 0 {
		return nil
	}
	dnum := float64(float64(factor) * frac)
	ip, fp := math.Modf(dnum)
	y, err := bigFromIntegral(ip)
	if err != nil {
		return err
	}
	sum.Add(sum, y)
	*leftover += fp
	return nil
}

// FromParts is datetime.timedelta(days=, hours=, minutes=, seconds=, microseconds=)
// with every argument passed. A whole number stands for a Python int; others are floats.
func FromParts(days, hours, minutes, seconds, microseconds float64) (Timedelta, error) {
	sum := new(big.Int)
	leftover := 0.0
	parts := []struct {
		v      float64
		factor int64
	}{
		{microseconds, 1}, {seconds, 1000000}, {minutes, 60000000},
		{hours, 3600000000}, {days, 86400000000},
	}
	for _, p := range parts {
		if err := accum(sum, p.v, p.factor, &leftover); err != nil {
			return Timedelta{}, err
		}
	}
	if leftover != 0 {
		whole := math.Round(leftover)
		if math.Abs(whole-leftover) == 0.5 {
			odd := float64(sum.Bit(0))
			whole = 2.0*math.Round(float64((leftover+odd)*0.5)) - odd
		}
		sum.Add(sum, big.NewInt(int64(whole)))
	}
	return microsecondsToDelta(sum)
}

// Add is timedelta + timedelta.
func (t Timedelta) Add(o Timedelta) (Timedelta, error) {
	return newDelta(t.days+o.days, t.seconds+o.seconds, t.microseconds+o.microseconds)
}

// Sub is timedelta - timedelta.
func (t Timedelta) Sub(o Timedelta) (Timedelta, error) {
	return newDelta(t.days-o.days, t.seconds-o.seconds, t.microseconds-o.microseconds)
}

// Neg is -timedelta.
func (t Timedelta) Neg() (Timedelta, error) { return newDelta(-t.days, -t.seconds, -t.microseconds) }

// Abs is abs(timedelta).
func (t Timedelta) Abs() (Timedelta, error) {
	if t.days < 0 {
		return t.Neg()
	}
	return t, nil
}

// Compare orders two timedeltas.
func (t Timedelta) Compare(o Timedelta) int {
	return cmpInts(t.days, o.days, t.seconds, o.seconds, t.microseconds, o.microseconds)
}

// TotalSeconds is timedelta.total_seconds().
func (t Timedelta) TotalSeconds() float64 {
	us := (int64(t.days)*86400+int64(t.seconds))*1000000 + int64(t.microseconds)
	return float64(us) / 1e6
}

// Repr is repr(timedelta).
func (t Timedelta) Repr() string {
	var parts []string
	if t.days != 0 {
		parts = append(parts, fmt.Sprintf("days=%d", t.days))
	}
	if t.seconds != 0 {
		parts = append(parts, fmt.Sprintf("seconds=%d", t.seconds))
	}
	if t.microseconds != 0 {
		parts = append(parts, fmt.Sprintf("microseconds=%d", t.microseconds))
	}
	if parts == nil {
		return "datetime.timedelta(0)"
	}
	out := "datetime.timedelta("
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out + ")"
}
