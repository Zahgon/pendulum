// Package pendulum is a Go port of the Python library pendulum 3.3.0: time zone aware
// date, time and datetime values, durations and intervals, ISO 8601 and lenient
// parsing, token and strftime formatting, and human-readable differences in thirty
// locales.
//
// The port reproduces the Python original's observable behaviour, including what it
// inherits from CPython's datetime module, the zoneinfo module over the IANA time zone
// database (tzdata 2026.5, embedded), python-dateutil's parser, the C library's
// strftime and local time, and time-machine's frozen clock. Python's None is a nil
// pointer, and every exception the original raises is returned as an error whose
// Error() text is the exception's class name, ": " and its str().
//
// pendulum is Copyright (c) 2015 Sébastien Eustace and released under the MIT License
// (see LICENSE); this port keeps that license.
package pendulum
