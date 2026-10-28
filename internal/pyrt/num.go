package pyrt

import (
	"math"
	"strconv"
	"strings"
)

// Python ints are unbounded; the port keeps them in int64 and saturates. A saturated
// value stands for "too large for a C long", which is what CPython reports for it.
const (
	BigPos = math.MaxInt64
	BigNeg = math.MinInt64
)

func isCSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}

// toASCIIDigits is _PyUnicode_TransformDecimalAndSpaceToASCII: ASCII stays, other
// whitespace becomes ' ', other decimal digits become ASCII digits, and anything else
// makes the conversion fail.
func toASCIIDigits(s string) ([]byte, bool) {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r < 127:
			out = append(out, byte(r))
		case IsSpace(r):
			out = append(out, ' ')
		default:
			d := DecimalValue(r)
			if d < 0 {
				return nil, false
			}
			out = append(out, byte('0'+d))
		}
	}
	return out, true
}

// parseLongASCII is PyLong_FromString for base 10 over the whole buffer.
func parseLongASCII(b []byte) (int, bool) {
	i, n := 0, len(b)
	for i < n && isCSpace(b[i]) {
		i++
	}
	neg := false
	if i < n && (b[i] == '+' || b[i] == '-') {
		neg = b[i] == '-'
		i++
	}
	start := i
	digits := make([]byte, 0, n)
	for i < n {
		c := b[i]
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
			i++
			continue
		}
		if c == '_' && i > start && i+1 < n && b[i+1] >= '0' && b[i+1] <= '9' {
			i++
			continue
		}
		break
	}
	if len(digits) == 0 {
		return 0, false
	}
	for i < n && isCSpace(b[i]) {
		i++
	}
	if i != n {
		return 0, false
	}
	return saturate(string(digits), neg), true
}

func saturate(digits string, neg bool) int {
	v, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return beyondLong(neg)
	}
	return fromMagnitude(v, neg)
}

func beyondLong(neg bool) int {
	if neg {
		return BigNeg
	}
	return BigPos
}

// fromMagnitude keeps the sentinels for values beyond a C long. The C long bounds
// themselves are held one step inward: every C conversion treats a bound and its
// neighbour alike, and only printing the value could tell them apart.
func fromMagnitude(v uint64, neg bool) int {
	switch {
	case !neg && v > math.MaxInt64, neg && v > 1<<63:
		return beyondLong(neg)
	case !neg && v == math.MaxInt64:
		return BigPos - 1
	case neg && v >= math.MaxInt64:
		return BigNeg + 1
	case neg:
		return -int(v)
	}
	return int(v)
}

// ParseInt is int(s) for a str.
func ParseInt(s string) (int, error) {
	if b, ok := toASCIIDigits(s); ok {
		if v, ok := parseLongASCII(b); ok {
			return v, nil
		}
	}
	return 0, NewError("ValueError", "invalid literal for int() with base 10: "+TruncateRunes(Repr(s), 200))
}

// ParseIntBytes is int(b) for a bytes object.
func ParseIntBytes(b []byte) (int, error) {
	if v, ok := parseLongASCII(b); ok {
		return v, nil
	}
	return 0, NewError("ValueError", "invalid literal for int() with base 10: "+TruncateRunes(BytesRepr(b), 200))
}

// ParseFloat is float(s) for a str.
func ParseFloat(s string) (float64, error) {
	if b, ok := toASCIIDigits(s); ok {
		if v, ok := parseFloatASCII(b); ok {
			return v, nil
		}
	}
	return 0, NewError("ValueError", "could not convert string to float: "+Repr(s))
}

func parseFloatASCII(b []byte) (float64, bool) {
	i, j := 0, len(b)
	for i < j && isCSpace(b[i]) {
		i++
	}
	for j > i && isCSpace(b[j-1]) {
		j--
	}
	b = b[i:j]
	if strings.IndexByte(string(b), '_') >= 0 {
		var ok bool
		if b, ok = dropUnderscores(b); !ok {
			return 0, false
		}
	}
	s := string(b)
	body, neg := s, false
	if body != "" && (body[0] == '+' || body[0] == '-') {
		neg = body[0] == '-'
		body = body[1:]
	}
	switch strings.ToLower(body) {
	case "inf", "infinity":
		if neg {
			return math.Inf(-1), true
		}
		return math.Inf(1), true
	case "nan":
		if neg {
			return math.Copysign(math.NaN(), -1), true
		}
		return math.NaN(), true
	}
	if !floatSyntax(body) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
			return 0, false
		}
	}
	return v, true
}

// dropUnderscores is _Py_string_to_number_with_underscores: an underscore must sit
// between two digits.
func dropUnderscores(b []byte) ([]byte, bool) {
	out := make([]byte, 0, len(b))
	prev := byte(0)
	for _, c := range b {
		if c == '_' {
			if !(prev >= '0' && prev <= '9') {
				return nil, false
			}
		} else {
			out = append(out, c)
			if prev == '_' && !(c >= '0' && c <= '9') {
				return nil, false
			}
		}
		prev = c
	}
	if prev == '_' {
		return nil, false
	}
	return out, true
}

// floatSyntax accepts digits [. digits] | . digits, then an optional exponent.
func floatSyntax(s string) bool {
	i, n, mant := 0, len(s), 0
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
		mant++
	}
	if i < n && s[i] == '.' {
		i++
		for i < n && s[i] >= '0' && s[i] <= '9' {
			i++
			mant++
		}
	}
	if mant == 0 {
		return false
	}
	if i < n && (s[i] == 'e' || s[i] == 'E') {
		k := i + 1
		if k < n && (s[k] == '+' || s[k] == '-') {
			k++
		}
		e := k
		for k < n && s[k] >= '0' && s[k] <= '9' {
			k++
		}
		if k > e {
			i = k
		}
	}
	return i == n
}

// FormatG is format(x, 'g'), or format(x, '+g') when plus is set. CPython ignores the
// sign bit of a NaN, so a negated NaN still formats as "+nan".
func FormatG(x float64, plus bool) string {
	sign := ""
	if math.Signbit(x) && !math.IsNaN(x) {
		sign = "-"
	} else if plus {
		sign = "+"
	}
	switch {
	case math.IsNaN(x):
		return sign + "nan"
	case math.IsInf(x, 0):
		return sign + "inf"
	}
	return sign + strconv.FormatFloat(math.Abs(x), 'g', 6, 64)
}

// FloatRepr is repr(x) for a float.
func FloatRepr(x float64) string {
	switch {
	case math.IsNaN(x):
		return "nan"
	case math.IsInf(x, 1):
		return "inf"
	case math.IsInf(x, -1):
		return "-inf"
	}
	e := strconv.FormatFloat(x, 'e', -1, 64)
	mant, expText, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expText)
	sign := ""
	if mant[0] == '-' {
		sign, mant = "-", mant[1:]
	}
	digits := strings.Replace(mant, ".", "", 1)
	if exp < -4 || exp >= 16 {
		out := digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		es := "+"
		if exp < 0 {
			es, exp = "-", -exp
		}
		return sign + out + "e" + es + leftPad(strconv.Itoa(exp), 2)
	}
	if exp < 0 {
		return sign + "0." + strings.Repeat("0", -exp-1) + digits
	}
	if len(digits) <= exp+1 {
		return sign + digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
	}
	return sign + digits[:exp+1] + "." + digits[exp+1:]
}

func leftPad(s string, n int) string {
	for len(s) < n {
		s = "0" + s
	}
	return s
}

// Round is round(x, ndigits) for a float and ndigits >= 0.
func Round(x float64, ndigits int) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || ndigits > 323 {
		return x
	}
	v, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', ndigits, 64), 64)
	return v
}

// RoundInt is round(x) for a float: the nearest integer, ties to even.
func RoundInt(x float64) (float64, error) {
	if err := checkIntegral(x); err != nil {
		return 0, err
	}
	return math.RoundToEven(x), nil
}

// Trunc is int(x) for a float.
func Trunc(x float64) (float64, error) {
	if err := checkIntegral(x); err != nil {
		return 0, err
	}
	return math.Trunc(x), nil
}

func checkIntegral(x float64) error {
	if math.IsNaN(x) {
		return NewError("ValueError", "cannot convert float NaN to integer")
	}
	if math.IsInf(x, 0) {
		return NewError("OverflowError", "cannot convert float infinity to integer")
	}
	return nil
}

// FloatDivmod is divmod(x, y) for floats (CPython's _float_div_mod).
func FloatDivmod(x, y float64) (float64, float64) {
	mod := math.Mod(x, y)
	div := (x - mod) / y
	if mod != 0 {
		if (y < 0) != (mod < 0) {
			mod += y
			div -= 1.0
		}
	} else {
		mod = math.Copysign(0, y)
	}
	var floordiv float64
	if div != 0 {
		floordiv = math.Floor(div)
		if div-floordiv > 0.5 {
			floordiv += 1.0
		}
	} else {
		floordiv = math.Copysign(0, x/y)
	}
	return floordiv, mod
}

// Sign is dateutil's _sign: int(copysign(1, x)).
func Sign(x float64) float64 { return math.Copysign(1, x) }

// FloorDiv and Mod are Python's // and % on ints.
func FloorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func Mod(a, b int) int {
	m := a % b
	if m != 0 && ((m < 0) != (b < 0)) {
		m += b
	}
	return m
}

// AsCLong converts a Python int for a C long argument.
func AsCLong(v int) (int, error) {
	if v == BigPos || v == BigNeg {
		return 0, NewError("OverflowError", "Python int too large to convert to C long")
	}
	return v, nil
}

// AsCInt converts a Python int for an "i" (C int) argument.
func AsCInt(v int) (int, error) {
	if _, err := AsCLong(v); err != nil {
		return 0, err
	}
	if v > math.MaxInt32 {
		return 0, NewError("OverflowError", "signed integer is greater than maximum")
	}
	if v < math.MinInt32 {
		return 0, NewError("OverflowError", "signed integer is less than minimum")
	}
	return int(v), nil
}

// AsCIntStrict is _PyLong_AsInt, which reports any overflow the same way.
func AsCIntStrict(v int) (int, error) {
	if v > math.MaxInt32 || v < math.MinInt32 {
		return 0, NewError("OverflowError", "Python int too large to convert to C int")
	}
	return int(v), nil
}
