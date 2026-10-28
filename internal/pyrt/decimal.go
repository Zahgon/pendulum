package pyrt

import (
	"math/big"
	"strings"
)

// DecimalPrec is the precision of decimal's default context.
const DecimalPrec = 28

// Decimal is the part of decimal.Decimal the parser uses, under the default context
// (28 digits, ROUND_HALF_EVEN, InvalidOperation trapped).
type Decimal struct {
	neg     bool
	coef    *big.Int
	exp     int
	special byte // 0 finite, 'i' infinity, 'n' quiet NaN, 's' signalling NaN
}

var (
	bigTen = big.NewInt(10)
	bigOne = big.NewInt(1)
)

func conversionSyntax() error {
	return NewError("InvalidOperation", "[<class 'decimal.ConversionSyntax'>]")
}

// ParseDecimal is Decimal(s) for a str.
func ParseDecimal(s string) (Decimal, error) {
	rs := []rune(s)
	i, j := 0, len(rs)
	for i < j && IsSpace(rs[i]) {
		i++
	}
	for j > i && IsSpace(rs[j-1]) {
		j--
	}
	var b strings.Builder
	for _, r := range rs[i:j] {
		switch {
		case r == '_':
		case r > 0 && r <= 127:
			b.WriteRune(r)
		case IsSpace(r):
			b.WriteByte(' ')
		default:
			d := DecimalValue(r)
			if d < 0 {
				return Decimal{}, conversionSyntax()
			}
			b.WriteByte(byte('0' + d))
		}
	}
	return parseDecimalASCII(b.String())
}

func parseDecimalASCII(s string) (Decimal, error) {
	d := Decimal{coef: new(big.Int)}
	if s != "" && (s[0] == '+' || s[0] == '-') {
		d.neg = s[0] == '-'
		s = s[1:]
	}
	low := strings.ToLower(s)
	switch {
	case low == "inf" || low == "infinity":
		d.special = 'i'
		return d, nil
	case strings.HasPrefix(low, "nan") && allDigits(low[3:]):
		d.special = 'n'
		return d, nil
	case strings.HasPrefix(low, "snan") && allDigits(low[4:]):
		d.special = 's'
		return d, nil
	}
	mant, expPart := s, ""
	if k := strings.IndexAny(s, "eE"); k >= 0 {
		mant, expPart = s[:k], s[k+1:]
		if expPart == "" {
			return Decimal{}, conversionSyntax()
		}
	}
	intPart, frac, dot := strings.Cut(mant, ".")
	if !allDigits(intPart) || !allDigits(frac) || (intPart == "" && frac == "") {
		return Decimal{}, conversionSyntax()
	}
	_ = dot
	digits := intPart + frac
	d.coef.SetString(digits, 10)
	d.exp = -len(frac)
	if expPart != "" {
		sign := 1
		if expPart[0] == '+' || expPart[0] == '-' {
			if expPart[0] == '-' {
				sign = -1
			}
			expPart = expPart[1:]
		}
		if expPart == "" || !allDigits(expPart) {
			return Decimal{}, conversionSyntax()
		}
		e := 0
		for _, c := range expPart {
			if e < 1<<40 {
				e = e*10 + int(c-'0')
			}
		}
		d.exp += sign * e
	}
	return d, nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IsFinite is Decimal.is_finite().
func (d Decimal) IsFinite() bool { return d.special == 0 }

// IsZero reports a zero coefficient; NaN and infinity are not zero.
func (d Decimal) IsZero() bool { return d.special == 0 && d.coef.Sign() == 0 }

func pow10(n int) *big.Int { return new(big.Int).Exp(bigTen, big.NewInt(int64(n)), nil) }

func numDigits(v *big.Int) int {
	if v.Sign() == 0 {
		return 1
	}
	return len(v.Text(10))
}

// adjExp is the exponent of the most significant digit.
func (d Decimal) adjExp() int { return d.exp + numDigits(d.coef) - 1 }

// CmpInt compares a finite Decimal with an int: -1, 0 or +1.
func (d Decimal) CmpInt(v int) int {
	lhs := new(big.Int).Set(d.coef)
	if d.neg {
		lhs.Neg(lhs)
	}
	rhs := big.NewInt(int64(v))
	if d.exp >= 0 {
		if d.exp > 4000 {
			if d.coef.Sign() == 0 {
				return -rhs.Sign()
			}
			return lhs.Sign()
		}
		lhs.Mul(lhs, pow10(d.exp))
	} else {
		rhs.Mul(rhs, pow10(-d.exp))
	}
	return lhs.Cmp(rhs)
}

// Mod1 is d % 1.
func (d Decimal) Mod1() (Decimal, error) {
	if !d.IsFinite() {
		return Decimal{}, NewError("InvalidOperation", "[<class 'decimal.InvalidOperation'>]")
	}
	if d.coef.Sign() != 0 && d.adjExp() >= DecimalPrec {
		return Decimal{}, NewError("InvalidOperation", "[<class 'decimal.DivisionImpossible'>]")
	}
	r := Decimal{neg: d.neg, coef: new(big.Int)}
	if d.exp >= 0 {
		r.exp = 0
		return r, nil
	}
	r.exp = d.exp
	r.coef.Mod(d.coef, pow10(-d.exp))
	return r.round(), nil
}

// MulInt is d * v, rounded to the context precision.
func (d Decimal) MulInt(v int) Decimal {
	r := Decimal{neg: d.neg != (v < 0), coef: new(big.Int), exp: d.exp, special: d.special}
	if d.special != 0 {
		return r
	}
	m := big.NewInt(int64(v))
	r.coef.Mul(d.coef, m.Abs(m))
	return r.round()
}

func (d Decimal) round() Decimal {
	n := numDigits(d.coef)
	if d.coef.Sign() == 0 || n <= DecimalPrec {
		return d
	}
	drop := n - DecimalPrec
	q, rem := new(big.Int).QuoRem(d.coef, pow10(drop), new(big.Int))
	half := new(big.Int).Mul(big.NewInt(5), pow10(drop-1))
	switch c := rem.Cmp(half); {
	case c > 0 || (c == 0 && q.Bit(0) == 1):
		q.Add(q, bigOne)
	}
	exp := d.exp + drop
	if numDigits(q) > DecimalPrec {
		q.Quo(q, bigTen)
		exp++
	}
	return Decimal{neg: d.neg, coef: q, exp: exp}
}

// Int is int(d) for a finite Decimal: truncation toward zero, saturated.
func (d Decimal) Int() (int, error) {
	switch d.special {
	case 'i':
		return 0, NewError("OverflowError", "cannot convert Infinity to integer")
	case 'n', 's':
		return 0, NewError("ValueError", "cannot convert NaN to integer")
	}
	v := new(big.Int).Set(d.coef)
	if d.exp >= 0 {
		if d.exp > 40 && v.Sign() != 0 {
			return beyondLong(d.neg), nil
		}
		v.Mul(v, pow10(d.exp))
	} else {
		v.Quo(v, pow10(-d.exp))
	}
	return saturateBig(v, d.neg), nil
}

func saturateBig(v *big.Int, neg bool) int {
	if !v.IsUint64() {
		return beyondLong(neg)
	}
	return fromMagnitude(v.Uint64(), neg)
}
