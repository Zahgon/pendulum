package pendulum

import (
	"math"

	"github.com/python-pendulum/pendulum/internal/pydt"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// num is a Python number: an int or a float, with Python's arithmetic on the pair.
type num struct {
	i       int64
	f       float64
	isFloat bool
}

func ni(v int) num     { return num{i: int64(v)} }
func nf(v float64) num { return num{f: v, isFloat: true} }
func (a num) float() float64 {
	if a.isFloat {
		return a.f
	}
	return float64(a.i)
}

func (a num) truthy() bool { return a.isFloat && a.f != 0 || !a.isFloat && a.i != 0 }

func (a num) neg() num {
	if a.isFloat {
		return nf(-a.f)
	}
	return num{i: -a.i}
}

func (a num) add(b num) num {
	if a.isFloat || b.isFloat {
		return nf(a.float() + b.float())
	}
	return num{i: a.i + b.i}
}

func (a num) mul(b num) num {
	if a.isFloat || b.isFloat {
		return nf(a.float() * b.float())
	}
	return num{i: a.i * b.i}
}

func (a num) abs() num {
	if a.isFloat {
		return nf(math.Abs(a.f))
	}
	if a.i < 0 {
		return num{i: -a.i}
	}
	return a
}

// gt is a > b.
func (a num) gt(b num) bool {
	if a.isFloat || b.isFloat {
		return a.float() > b.float()
	}
	return a.i > b.i
}

// sign is pendulum's _sign: int(copysign(1, x)).
func (a num) sign() num {
	if math.Signbit(a.float()) {
		return num{i: -1}
	}
	return num{i: 1}
}

// divmod is Python's divmod on numbers.
func (a num) divmod(b num) (num, num) {
	if a.isFloat || b.isFloat {
		q, r := pyrt.FloatDivmod(a.float(), b.float())
		return nf(q), nf(r)
	}
	q := a.i / b.i
	r := a.i % b.i
	if r != 0 && (r < 0) != (b.i < 0) {
		q--
		r += b.i
	}
	return num{i: q}, num{i: r}
}

func (a num) arg() pydt.DeltaArg {
	if a.isFloat {
		return pydt.F(a.f)
	}
	return pydt.I(a.i)
}

var zeroNum = num{}

// timedelta is datetime.timedelta(days=, hours=, minutes=, seconds=, microseconds=).
func timedelta(days, hours, minutes, seconds, microseconds num) (pydt.Timedelta, error) {
	z := zeroNum.arg()
	return pydt.NewDelta(days.arg(), seconds.arg(), microseconds.arg(), z, minutes.arg(), hours.arg(), z)
}
