// Package locales is the data of pendulum's thirty locales: each locale's CLDR plural
// and ordinal rules and its translation tree, generated from the Python locale modules
// by gen_locales.py.
package locales

// Locale is one locale's data.
type Locale struct {
	// Plural and Ordinal are the CLDR rules: "zero", "one", "two", "few", "many" or
	// "other" for a count.
	Plural  func(n int) string
	Ordinal func(n int) string
	// Data is the locale dict without its rules: "translations" and "custom".
	Data *Dict
}

// Dict is a Python dict in insertion order. Keys are strings or ints; values are
// strings, ints or *Dict.
type Dict struct {
	Keys   []any
	Values []any
}

// Get is d[key].
func (d *Dict) Get(key any) (any, bool) {
	if d == nil {
		return nil, false
	}
	for i, k := range d.Keys {
		if k == key {
			return d.Values[i], true
		}
	}
	return nil, false
}

// mod is Python's % for the rules.
func mod(a, b int) int {
	m := a % b
	if m != 0 && (m < 0) != (b < 0) {
		m += b
	}
	return m
}
