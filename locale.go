package pendulum

import (
	"strings"
	"sync"

	"github.com/python-pendulum/pendulum/internal/locales"
	"github.com/python-pendulum/pendulum/internal/pyrt"
)

// Locale is a loaded pendulum locale.
type Locale struct {
	name string
	data *locales.Locale
}

var (
	localeMu      sync.Mutex
	localeCache   = map[string]*Locale{}
	currentLocale = "en"
)

// normalizeLocale is Locale.normalize_locale: two letters, "-" or "_", two letters
// (matched case-insensitively, as Python's re.I matches [a-z]) become "xx_yy".
func normalizeLocale(name string) string {
	rs := []rune(name)
	if len(rs) == 5 && (rs[2] == '-' || rs[2] == '_') && letters(rs[0:2]) && letters(rs[3:5]) {
		return pyrt.Lower(string(rs[0:2])) + "_" + pyrt.Lower(string(rs[3:5]))
	}
	return pyrt.Lower(name)
}

// letters reports whether every rune matches [a-z] under Python's IGNORECASE, which
// also admits U+0130, U+0131, U+017F and U+212A.
func letters(rs []rune) bool {
	for _, c := range rs {
		ascii := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !ascii && c != 0x130 && c != 0x131 && c != 0x17F && c != 0x212A {
			return false
		}
	}
	return true
}

// LoadLocale is pendulum.locale(name), Locale.load: names are matched case
// insensitively, with "-" or "_" between language and region.
func LoadLocale(name string) (*Locale, error) {
	norm := normalizeLocale(name)
	localeMu.Lock()
	defer localeMu.Unlock()
	if l, ok := localeCache[norm]; ok {
		return l, nil
	}
	data, ok := locales.All[norm]
	if !ok {
		return nil, valueError("Locale [%s] does not exist.", norm)
	}
	l := &Locale{name: norm, data: data}
	localeCache[norm] = l
	return l, nil
}

// SetLocale is pendulum.set_locale(name).
func SetLocale(name string) error {
	if _, err := LoadLocale(name); err != nil {
		return err
	}
	localeMu.Lock()
	defer localeMu.Unlock()
	currentLocale = name
	return nil
}

// GetLocale is pendulum.get_locale().
func GetLocale() string {
	localeMu.Lock()
	defer localeMu.Unlock()
	return currentLocale
}

// loadOrCurrent is Locale.load(locale or pendulum.get_locale()).
func loadOrCurrent(name *string) (*Locale, error) {
	if name == nil || *name == "" {
		return LoadLocale(GetLocale())
	}
	return LoadLocale(*name)
}

// Name is the normalised locale name.
func (l *Locale) Name() string { return l.name }

// get is Locale.get(key): the value at a dotted path, nil when absent.
func (l *Locale) get(key string) any {
	var cur any = l.data.Data
	for _, part := range strings.Split(key, ".") {
		d, ok := cur.(*locales.Dict)
		if !ok {
			return nil
		}
		v, ok := d.Get(part)
		if !ok {
			return nil
		}
		cur = v
	}
	return cur
}

func (l *Locale) getString(key string) (string, bool) {
	s, ok := l.get(key).(string)
	return s, ok
}

func (l *Locale) translation(key string) any { return l.get("translations." + key) }

// Plural and Ordinal are the locale's CLDR rules.
func (l *Locale) Plural(n int) string  { return l.data.Plural(n) }
func (l *Locale) Ordinal(n int) string { return l.data.Ordinal(n) }

// Ordinalize is locale.ordinalize(n): the number with the locale's ordinal suffix.
func (l *Locale) Ordinalize(n int) string {
	if s, ok := l.getString("custom.ordinal." + l.Ordinal(n)); ok && s != "" {
		return itoa(n) + s
	}
	return itoa(n)
}

func (l *Locale) indexed(key string, i int) (string, error) {
	d, ok := l.get(key).(*locales.Dict)
	if !ok {
		return "", newError("TypeError", "'NoneType' object is not subscriptable")
	}
	v, ok := d.Get(i)
	if !ok {
		return "", newError("KeyError", itoa(i))
	}
	s, _ := v.(string)
	return s, nil
}

// pyFormat is str.format with the positional placeholders the locale data uses
// ("{0}" and "{}").
func pyFormat(tmpl string, arg string) string {
	r := strings.ReplaceAll(tmpl, "{0}", arg)
	return strings.ReplaceAll(r, "{}", arg)
}
