package pendulum

var daysPerMonth = [2][13]int{
	{-1, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31},
	{-1, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31},
}

var monthsOffsets = [2][14]int{
	{-1, 0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334, 365},
	{-1, 0, 31, 60, 91, 121, 152, 182, 213, 244, 274, 305, 335, 366},
}

var dayOfWeekTable = [12]int{0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// isLeap is pendulum's is_leap for any year.
func isLeap(year int) bool {
	return pyMod(year, 4) == 0 && (pyMod(year, 100) != 0 || pyMod(year, 400) == 0)
}

func leapIndex(year int) int {
	if isLeap(year) {
		return 1
	}
	return 0
}

// isLongYear is pendulum's is_long_year: the ISO year has 53 weeks.
func isLongYear(year int) bool {
	p := func(y int) int { return y + floorDiv(y, 4) - floorDiv(y, 100) + floorDiv(y, 400) }
	return pyMod(p(year), 7) == 4 || pyMod(p(year-1), 7) == 3
}

// weekDay is pendulum's week_day: ISO weekday, Monday 1 .. Sunday 7.
func weekDay(year, month, day int) int {
	if month < 3 {
		year--
	}
	w := pyMod(year+floorDiv(year, 4)-floorDiv(year, 100)+floorDiv(year, 400)+dayOfWeekTable[month-1]+day, 7)
	if w == 0 {
		w = 7
	}
	return w
}

func daysInYear(year int) int {
	if isLeap(year) {
		return 366
	}
	return 365
}

func daysIn(year, month int) int { return daysPerMonth[leapIndex(year)][month] }

// weekdayOf is the Monday-based weekday (0..6) of a date in any year.
func weekdayOf(year, month, day int) int { return weekDay(year, month, day) - 1 }

// dayNumber is pendulum's _day_number.
func dayNumber(year, month, day int) int {
	month = (month + 9) % 12
	year = year - month/10
	return 365*year + floorDiv(year, 4) - floorDiv(year, 100) + floorDiv(year, 400) + (month*306+5)/10 + (day - 1)
}
