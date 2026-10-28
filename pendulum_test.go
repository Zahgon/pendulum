package pendulum

import (
	"fmt"
	"strings"
	"testing"
)

const ny = "America/New_York"

// frozen stops the clock at 2024-03-10 06:30 UTC for the test.
func frozen(t *testing.T) {
	t.Helper()
	now, err := Naive(2024, 3, 10, 6, 30, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := TravelTo(now, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(TravelBack)
}

func str(v any, err error) string {
	if err != nil {
		return "E:" + err.Error()
	}
	switch x := v.(type) {
	case fmt.Stringer:
		return x.String()
	case string:
		return x
	}
	return fmt.Sprint(v)
}

func repr(v any, err error) string {
	if err != nil {
		return "E:" + err.Error()
	}
	switch x := v.(type) {
	case DateTime:
		return x.Repr()
	case Date:
		return x.Repr()
	case Duration:
		return x.Repr()
	}
	return fmt.Sprint(v)
}

func dt(t *testing.T, y, mo, d, h, mi int, tz any, fold int) DateTime {
	t.Helper()
	v, err := Datetime(y, mo, d, h, mi, 0, 0, tz, fold, false)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func check(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", name, got, want)
	}
}

func TestConstructionAcrossTransitions(t *testing.T) {
	check(t, "gap fold=1", dt(t, 2024, 3, 10, 2, 30, ny, 1).String(), "2024-03-10 03:30:00-04:00")
	check(t, "gap fold=0", dt(t, 2024, 3, 10, 2, 30, ny, 0).String(), "2024-03-10 01:30:00-05:00")
	check(t, "overlap fold=0", dt(t, 2024, 11, 3, 1, 30, ny, 0).String(), "2024-11-03 01:30:00-04:00")
	check(t, "raise", str(Datetime(2024, 3, 10, 2, 30, 0, 0, ny, 1, true)),
		"E:NonExistingTime: The datetime 2024-03-10 02:30:00 does not exist.")
	check(t, "float hours", dt(t, 2024, 1, 1, 0, 0, 5.5, 1).Repr(),
		`DateTime(2024, 1, 1, 0, 0, 0, tzinfo=FixedTimezone(19800, name="+05:30"))`)
}

func TestArithmetic(t *testing.T) {
	check(t, "add day over the gap", str(dt(t, 2024, 3, 9, 2, 30, ny, 1).Add(AddArgs{Days: 1})), "2024-03-10 03:30:00-04:00")
	check(t, "add hour over the gap", str(dt(t, 2024, 3, 10, 1, 30, ny, 1).Add(AddArgs{Hours: 1})), "2024-03-10 03:30:00-04:00")
	check(t, "month end", str(dt(t, 2024, 1, 31, 0, 0, UTC, 1).Add(AddArgs{Months: 1})), "2024-02-29 00:00:00+00:00")
	iv, err := dt(t, 2024, 1, 31, 22, 0, ny, 1).Diff(ptr(dt(t, 2024, 3, 31, 3, 0, ny, 1)), true)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(iv.Years(), iv.Months(), iv.Days(), iv.RemainingDays(), iv.Hours(), iv.InDays())
	check(t, "precise diff", got, "0 1 59 2 5 60")
}

func TestZones(t *testing.T) {
	check(t, "Kathmandu", str(dt(t, 2024, 6, 1, 12, 0, UTC, 1).InTz("Asia/Kathmandu")), "2024-06-01 17:45:00+05:45")
	check(t, "Winnipeg on tzdata 2026.5", str(dt(t, 2027, 1, 15, 12, 0, UTC, 1).InTz("America/Winnipeg")),
		"2027-01-15 07:00:00-05:00")
	check(t, "invalid zone", str(NewTimezone("Mars/Olympus")), "E:InvalidTimezone: Mars/Olympus")
	check(t, "from_timestamp", str(FromTimestamp(1700000000.5, "Europe/London")), "2023-11-14 22:13:20.500000+00:00")
}

func TestParsing(t *testing.T) {
	strict := ParseOptions{Strict: true, YearFirst: true}
	check(t, "week date", repr(Parse("2016-W07-3T10:30:00+05:30", strict)),
		`DateTime(2016, 2, 17, 10, 30, 0, tzinfo=FixedTimezone(19800, name="+05:30"))`)
	exact := strict
	exact.Exact = true
	check(t, "ordinal date", repr(Parse("2024-060", exact)), "Date(2024, 2, 29)")
	check(t, "duration", repr(Parse("P1Y2M3DT4H5M6.5S", strict)),
		"Duration(years=1, months=2, days=3, hours=4, minutes=5, seconds=6, microseconds=500000)")
	check(t, "strict", str(Parse("March 5th 2021", strict)), "E:ParserError: Unable to parse string [March 5th 2021]")
	lenient := strict
	lenient.Strict = false
	check(t, "lenient", str(Parse("March 5th 2021 10pm", lenient)), "2021-03-05 22:00:00+00:00")
	ru := "ru"
	check(t, "from_format ru", str(FromFormat("5 марта 2024", "D MMMM YYYY", nil, &ru)), "2024-03-05 00:00:00+00:00")
}

func TestFormatting(t *testing.T) {
	fr := "fr"
	d := dt(t, 2024, 3, 1, 15, 4, UTC, 1)
	check(t, "fr tokens", str(d.Format("dddd Do MMMM YYYY LT", &fr)), "vendredi 1er mars 2024 15:04")
	check(t, "unknown tokens pass through", str(d.Format("[Week] wo, Qo quarter, h:mm A z", nil)),
		"Week 9th, 1st quart5r, 3:04 PM UTC")
	check(t, "strftime", str(dt(t, 2024, 3, 1, 15, 4, ny, 1).Strftime("%a %d %b %Y %I:%M %p %Z %z %G-W%V")),
		"Fri 01 Mar 2024 03:04 PM EST -0500 2024-W09")
}

func TestHumans(t *testing.T) {
	frozen(t)
	ru, pl, de, lt := "ru", "pl", "de", "lt"
	check(t, "ru past", str(dt(t, 2024, 3, 5, 0, 0, UTC, 1).DiffForHumans(nil, false, &ru)), "5 дней назад")
	future, err := dt(t, 2024, 3, 10, 6, 30, UTC, 1).Add(AddArgs{Years: 2, Months: 7})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "pl future", str(future.DiffForHumans(nil, false, &pl)), "za 3 lata")
	other := dt(t, 2024, 1, 23, 0, 0, UTC, 1)
	check(t, "de before", str(dt(t, 2024, 1, 1, 0, 0, UTC, 1).DiffForHumans(&other, false, &de)), "3 Wochen zuvor")
	d, err := NewDuration(DurationArgs{Days: 12, Hours: 5, Seconds: 61})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "lt words", str(d.InWords(&lt, " ")), "1 savaitė 5 dienos 5 valandos 1 minutė 1 sekundė")
	check(t, "now in Tokyo", str(Now("Asia/Tokyo")), "2024-03-10 15:30:00+09:00")
}

func TestDurations(t *testing.T) {
	d, err := NewDuration(DurationArgs{Years: 1, Days: 3, Hours: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "float multiplication drops years", repr(d.MulFloat(1.5)), "Duration(days=4, hours=14, minutes=15)")
	e, err := NewDuration(DurationArgs{Days: 10, Seconds: 7})
	if err != nil {
		t.Fatal(err)
	}
	check(t, "division", repr(e.TrueDivInt(3)), "Duration(days=3, hours=8, seconds=2, microseconds=333333)")
	a, _ := NewTime(10, 0, 0, 0)
	b, _ := NewTime(9, 30, 15, 0)
	check(t, "time diff", repr(a.Diff(&b, true)), "AbsoluteDuration(minutes=29, seconds=45)")
}

func TestIntervalsAndComparisons(t *testing.T) {
	iv, err := NewInterval(mustDate(2024, 1, 30), mustDate(2024, 4, 1), false)
	if err != nil {
		t.Fatal(err)
	}
	days, err := iv.Range("months", 1)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range days {
		got = append(got, d.(Date).String())
	}
	check(t, "range", strings.Join(got, ","), "2024-01-30,2024-02-29,2024-03-30")
	naive, _ := Naive(2024, 1, 1, 12, 0, 0, 0, 0)
	check(t, "closest takes naive arguments in UTC",
		str(dt(t, 2024, 1, 1, 0, 0, UTC, 1).Closest(dt(t, 2023, 12, 31, 0, 0, UTC, 1), naive)), "2024-01-01 12:00:00+00:00")
	check(t, "PEP 495 equality", fmt.Sprint(dt(t, 2024, 11, 3, 1, 30, ny, 0).Equal(dt(t, 2024, 11, 3, 5, 30, UTC, 1))), "false")
	check(t, "nth_of", str(mustDate(2024, 2, 1).NthOf("month", 5, THURSDAY)), "2024-02-29")
	check(t, "nth_of missing", str(mustDate(2024, 2, 1).NthOf("month", 5, MONDAY)),
		"E:PendulumException: Unable to find occurrence 5 of Monday in month")
	check(t, "start of week", str(dt(t, 2024, 3, 7, 13, 0, UTC, 1).StartOf("week")), "2024-03-04 00:00:00+00:00")
}
