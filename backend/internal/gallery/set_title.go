package gallery

import (
	"strings"
	"time"
)

func titleSetDate(title string) (time.Time, bool) {
	if len(title) < 6 {
		return time.Time{}, false
	}
	for _, digit := range title[:6] {
		if digit < '0' || digit > '9' {
			return time.Time{}, false
		}
	}
	date, err := time.Parse("060102", title[:6])
	return date, err == nil && date.Year() >= 2000
}

// buildSetTitle keeps a date already supplied at the start of Name. Otherwise
// it prefixes the set date selected in the back office. The returned date is
// used to place the row within its group.
func buildSetTitle(setDate time.Time, name string) (string, time.Time) {
	name = strings.TrimSpace(name)
	if len(name) > 6 && name[6] == ' ' {
		if date, ok := titleSetDate(name); ok && strings.TrimSpace(name[6:]) != "" {
			return date.Format("060102") + " " + strings.TrimSpace(name[6:]), date
		}
	}
	return setDate.Format("060102") + " " + name, setDate
}
