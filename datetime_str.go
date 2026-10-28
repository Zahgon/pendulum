package pendulum

import (
	"fmt"
	"strings"
)

func en() *string { s := "en"; return &s }

// ToTimeString and the other To*String methods are pendulum's named formats.
func (dt DateTime) ToTimeString() (string, error)     { return dt.Format("HH:mm:ss", nil) }
func (dt DateTime) ToDatetimeString() (string, error) { return dt.Format("YYYY-MM-DD HH:mm:ss", nil) }
func (dt DateTime) ToDayDatetimeString() (string, error) {
	return dt.Format("ddd, MMM D, YYYY h:mm A", en())
}
func (dt DateTime) ToAtomString() (string, error) { return dt.Format("YYYY-MM-DDTHH:mm:ssZ", nil) }
func (dt DateTime) ToCookieString() (string, error) {
	return dt.Format("dddd, DD-MMM-YYYY HH:mm:ss zz", en())
}
func (dt DateTime) ToRfc822String() (string, error) {
	return dt.Format("ddd, DD MMM YY HH:mm:ss ZZ", nil)
}
func (dt DateTime) ToRfc850String() (string, error) {
	return dt.Format("dddd, DD-MMM-YY HH:mm:ss zz", nil)
}
func (dt DateTime) ToRfc1036String() (string, error) {
	return dt.Format("ddd, DD MMM YY HH:mm:ss ZZ", nil)
}
func (dt DateTime) ToRfc1123String() (string, error) {
	return dt.Format("ddd, DD MMM YYYY HH:mm:ss ZZ", nil)
}
func (dt DateTime) ToRfc2822String() (string, error) {
	return dt.Format("ddd, DD MMM YYYY HH:mm:ss ZZ", nil)
}
func (dt DateTime) ToRfc3339String() string { return dt.d.Isoformat("T") }
func (dt DateTime) ToRssString() (string, error) {
	return dt.Format("ddd, DD MMM YYYY HH:mm:ss ZZ", nil)
}
func (dt DateTime) ToW3cString() (string, error) { return dt.Format("YYYY-MM-DDTHH:mm:ssZ", nil) }

// ToIso8601String is dt.to_iso8601_string(): isoformat with "Z" for the UTC zone.
func (dt DateTime) ToIso8601String() string {
	s := dt.d.Isoformat("T")
	if z := dt.Timezone(); z != nil && z.Name() == "UTC" {
		s = strings.ReplaceAll(s, "+00:00", "Z")
	}
	return s
}

// Isoformat is dt.isoformat(), String str(dt) (with a space separator) and ForJSON
// dt.for_json().
func (dt DateTime) Isoformat() string { return dt.d.Isoformat("T") }
func (dt DateTime) String() string    { return dt.d.Isoformat(" ") }
func (dt DateTime) ForJSON() string   { return dt.Isoformat() }

// Repr is repr(dt): DateTime(2024, 3, 10, 1, 30, 0, tzinfo=Timezone('UTC')).
func (dt DateTime) Repr() string {
	us := ""
	if dt.Microsecond() != 0 {
		us = fmt.Sprintf(", %d", dt.Microsecond())
	}
	s := fmt.Sprintf("DateTime(%d, %d, %d, %d, %d, %d%s", dt.Year(), dt.Month(), dt.Day(), dt.Hour(),
		dt.Minute(), dt.Second(), us)
	if t := dt.d.Tzinfo(); t != nil {
		s += ", tzinfo=" + t.String()
	}
	return s + ")"
}
