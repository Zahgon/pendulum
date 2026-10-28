// Package pyrt reproduces the CPython 3.12 built-ins that pendulum and python-dateutil
// lean on: the str predicates and case mappings, int(), float() and Decimal() text
// conversion, float formatting and rounding, the repr of str and bytes, and Python
// exceptions as Go errors. Its Unicode tables are generated from CPython 3.12's
// unicodedata (Unicode 15.0.0).
package pyrt

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type runeRange struct{ lo, hi rune }

func inRanges(table []runeRange, r rune) bool {
	i := sort.Search(len(table), func(i int) bool { return table[i].hi >= r })
	return i < len(table) && table[i].lo <= r
}

// IsAlpha reports str.isalpha() for one code point.
func IsAlpha(r rune) bool { return inRanges(pyAlpha, r) }

// IsDecimal reports str.isdecimal() for one code point.
func IsDecimal(r rune) bool { return DecimalValue(r) >= 0 }

// DecimalValue is unicodedata.decimal(r), or -1 when r is not a decimal digit.
func DecimalValue(r rune) int {
	if r >= '0' && r <= '9' {
		return int(r - '0')
	}
	if r < 0x80 || !unicode.IsDigit(r) {
		return -1
	}
	i := sort.Search(len(decimalZeros), func(i int) bool { return decimalZeros[i].lo > r })
	if i == 0 {
		return -1
	}
	if d := int(r - decimalZeros[i-1].lo); d <= 9 {
		return d
	}
	return -1
}

// IsDigit reports str.isdigit() for one code point.
func IsDigit(r rune) bool { return IsDecimal(r) || inRanges(digitExtra, r) }

// IsSpace reports str.isspace() for one code point.
func IsSpace(r rune) bool { return inRanges(pySpace, r) }

// IsPrintable reports str.isprintable() for one code point.
func IsPrintable(r rune) bool { return inRanges(pyPrintable, r) }

func isCaseIgnorable(r rune) bool { return inRanges(sigmaIgnorable, r) }

func isCased(r rune) bool { return r == 0x3A3 || inRanges(sigmaCased, r) }

func all(s string, pred func(rune) bool) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !pred(r) {
			return false
		}
	}
	return true
}

// StrIsDigit is str.isdigit().
func StrIsDigit(s string) bool { return all(s, IsDigit) }

// StrIsAlpha is str.isalpha().
func StrIsAlpha(s string) bool { return all(s, IsAlpha) }

// StrIsSpace is str.isspace().
func StrIsSpace(s string) bool { return all(s, IsSpace) }

// Len is len() of a str: the number of code points.
func Len(s string) int { return utf8.RuneCountInString(s) }

// Slice is s[i:j] on code points, with Python's clamping and negative indices.
func Slice(s string, i, j int) string {
	rs := []rune(s)
	n := len(rs)
	clamp := func(k int) int {
		if k < 0 {
			k += n
			if k < 0 {
				k = 0
			}
		}
		if k > n {
			k = n
		}
		return k
	}
	i, j = clamp(i), clamp(j)
	if i >= j {
		return ""
	}
	return string(rs[i:j])
}

// From is s[i:].
func From(s string, i int) string { return Slice(s, i, 1<<62) }

// Lower is str.lower(): the full lowercase mapping with the Final_Sigma rule.
func Lower(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		switch {
		case r == 0x3A3:
			b.WriteRune(finalSigma(rs, i))
		case lowerSpecial[r] != "":
			b.WriteString(lowerSpecial[r])
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func finalSigma(rs []rune, i int) rune {
	j := i - 1
	for j >= 0 && isCaseIgnorable(rs[j]) {
		j--
	}
	final := j >= 0 && isCased(rs[j])
	if final && i+1 < len(rs) {
		j = i + 1
		for j < len(rs) && isCaseIgnorable(rs[j]) {
			j++
		}
		final = j == len(rs) || !isCased(rs[j])
	}
	if final {
		return 0x3C2
	}
	return 0x3C3
}

// Upper is str.upper(): the full uppercase mapping.
func Upper(s string) string {
	var b strings.Builder
	for _, r := range s {
		if special, ok := upperSpecial[r]; ok {
			b.WriteString(special)
		} else {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

// Split is str.split() with no separator: runs of str.isspace() characters.
func Split(s string) []string {
	out := []string{}
	start := -1
	for i, r := range s {
		if IsSpace(r) {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}

// SplitSep is str.split(sep, maxsplit); maxsplit < 0 means no limit.
func SplitSep(s, sep string, maxsplit int) []string {
	if maxsplit < 0 {
		return strings.Split(s, sep)
	}
	return strings.SplitN(s, sep, maxsplit+1)
}

// Strip is str.strip() with no argument.
func Strip(s string) string { return strings.TrimFunc(s, IsSpace) }

// RStrip is str.rstrip() with no argument.
func RStrip(s string) string { return strings.TrimRightFunc(s, IsSpace) }

// StripChars is str.strip(chars).
func StripChars(s, chars string) string { return strings.Trim(s, chars) }

// RStripChars is str.rstrip(chars).
func RStripChars(s, chars string) string { return strings.TrimRight(s, chars) }

// LStripChars is str.lstrip(chars).
func LStripChars(s, chars string) string { return strings.TrimLeft(s, chars) }

// Splitlines is str.splitlines() without keepends.
func Splitlines(s string) []string {
	out := []string{}
	rs := []rune(s)
	start := 0
	for i := 0; i < len(rs); i++ {
		if !inRanges(lineBreaks, rs[i]) {
			continue
		}
		out = append(out, string(rs[start:i]))
		if rs[i] == '\r' && i+1 < len(rs) && rs[i+1] == '\n' {
			i++
		}
		start = i + 1
	}
	if start < len(rs) {
		out = append(out, string(rs[start:]))
	}
	return out
}

// Repr is repr() of a str.
func Repr(s string) string {
	quote := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteRune(quote)
	for _, r := range s {
		switch {
		case r == quote || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7F:
			b.WriteString(hexEscape(`\x`, int(r), 2))
		case r < 0x7F:
			b.WriteRune(r)
		case IsPrintable(r):
			b.WriteRune(r)
		case r < 0x100:
			b.WriteString(hexEscape(`\x`, int(r), 2))
		case r < 0x10000:
			b.WriteString(hexEscape(`\u`, int(r), 4))
		default:
			b.WriteString(hexEscape(`\U`, int(r), 8))
		}
	}
	b.WriteRune(quote)
	return b.String()
}

// BytesRepr is repr() of a bytes object.
func BytesRepr(p []byte) string {
	quote := byte('\'')
	if strings.IndexByte(string(p), '\'') >= 0 && strings.IndexByte(string(p), '"') < 0 {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte('b')
	b.WriteByte(quote)
	for _, c := range p {
		switch {
		case c == quote || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c < 0x20 || c >= 0x7F:
			b.WriteString(hexEscape(`\x`, int(c), 2))
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

func hexEscape(prefix string, v, width int) string {
	const digits = "0123456789abcdef"
	buf := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		buf[i] = digits[v&0xF]
		v >>= 4
	}
	return prefix + string(buf)
}

// TruncateRunes keeps the first n code points of s, as the %.200R format of CPython
// error messages does.
func TruncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
