package pendulum

import (
	"fmt"
	"sync"
)

// WeekDay is pendulum.WeekDay: MONDAY is 0 and SUNDAY 6.
type WeekDay int

// The days of the week.
const (
	MONDAY WeekDay = iota
	TUESDAY
	WEDNESDAY
	THURSDAY
	FRIDAY
	SATURDAY
	SUNDAY
)

var weekDayNames = [7]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

// capitalName is WeekDay(d).name.capitalize(), or the ValueError the enum raises.
func (d WeekDay) capitalName() (string, error) {
	if d < MONDAY || d > SUNDAY {
		return "", valueError("%d is not a valid WeekDay", int(d))
	}
	return weekDayNames[d], nil
}

var (
	weekMu    sync.Mutex
	weekStart = MONDAY
	weekEnd   = SUNDAY
)

func weekStartsAt() WeekDay { weekMu.Lock(); defer weekMu.Unlock(); return weekStart }
func weekEndsAt() WeekDay   { weekMu.Lock(); defer weekMu.Unlock(); return weekEnd }

// WeekStartsAt is pendulum.week_starts_at(day): the first day of a week for
// start_of("week").
func WeekStartsAt(day WeekDay) error {
	if err := checkWeekday(day); err != nil {
		return err
	}
	weekMu.Lock()
	defer weekMu.Unlock()
	weekStart = day
	return nil
}

// WeekEndsAt is pendulum.week_ends_at(day).
func WeekEndsAt(day WeekDay) error {
	if err := checkWeekday(day); err != nil {
		return err
	}
	weekMu.Lock()
	defer weekMu.Unlock()
	weekEnd = day
	return nil
}

// monthDays is calendar.Calendar(MONDAY).monthdayscalendar(year, month).
func monthDays(year, month int) [][7]int {
	first := weekdayOf(year, month, 1)
	n := daysIn(year, month)
	var weeks [][7]int
	var w [7]int
	col := first
	for d := 1; d <= n; d++ {
		w[col] = d
		col++
		if col == 7 {
			weeks = append(weeks, w)
			w, col = [7]int{}, 0
		}
	}
	if col > 0 {
		weeks = append(weeks, w)
	}
	return weeks
}

// pyIndex is seq[i] with Python's negative indexing and IndexError.
func pyIndex(week [7]int, i int) (int, error) {
	if i < 0 {
		i += 7
	}
	if i < 0 || i >= 7 {
		return 0, newError("IndexError", "list index out of range")
	}
	return week[i], nil
}

func firstWeekdayOfMonth(year, month, dow int) (int, error) {
	weeks := monthDays(year, month)
	v, err := pyIndex(weeks[0], dow)
	if err != nil {
		return 0, err
	}
	if v > 0 {
		return v, nil
	}
	return pyIndex(weeks[1], dow)
}

func lastWeekdayOfMonth(year, month, dow int) (int, error) {
	weeks := monthDays(year, month)
	v, err := pyIndex(weeks[len(weeks)-1], dow)
	if err != nil {
		return 0, err
	}
	if v > 0 {
		return v, nil
	}
	return pyIndex(weeks[len(weeks)-2], dow)
}

func (d WeekDay) String() string {
	if n, err := d.capitalName(); err == nil {
		return fmt.Sprintf("WeekDay.%s", upperASCII(n))
	}
	return fmt.Sprintf("WeekDay(%d)", int(d))
}

func upperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
