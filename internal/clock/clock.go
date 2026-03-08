package clock

import "time"

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func BusinessDateCT(c Clock, tz *time.Location, cutoffHour, cutoffMinute int) string {
	now := c.Now().In(tz)
	cutoff := time.Date(now.Year(), now.Month(), now.Day(), cutoffHour, cutoffMinute, 0, 0, tz)

	date := now
	if !now.Before(cutoff) {
		date = now.AddDate(0, 0, 1)
		for date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			date = date.AddDate(0, 0, 1)
		}
	} else if date.Weekday() == time.Sunday {
		date = date.AddDate(0, 0, 1)
	}

	return date.Format("2006-01-02")
}
