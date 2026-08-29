package eval

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
)

type timeQuery struct {
	when       time.Time // wall clock, ignored if now
	now        bool
	structured bool // "in X" / "9am PT" — time even if the place is unknown
	fromName   string
	toName     string
}

func parseTimeQuery(body string) (timeQuery, bool) {
	s := strings.TrimSpace(body)
	if s == "" {
		return timeQuery{}, false
	}
	low := strings.ToLower(s)

	if rest, ok := stripPrefix(low, s, "now in "); ok {
		if !looksLikePlace(rest) {
			return timeQuery{}, false
		}
		return timeQuery{now: true, structured: true, toName: rest}, true
	}
	if rest, ok := stripPrefix(low, s, "in "); ok {
		if !looksLikePlace(rest) {
			return timeQuery{}, false
		}
		return timeQuery{now: true, structured: true, toName: rest}, true
	}

	when, rest, ok := takeClock(s)
	if !ok {
		if !looksLikePlace(s) {
			return timeQuery{}, false
		}
		// Bare place: current time there. Try-all only treats this as time
		// when the nickname is already known; t/time still resolves via geocode.
		return timeQuery{now: true, toName: s}, true
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return timeQuery{}, false
	}
	fromPart, toPart := rest, ""
	if i := strings.Index(strings.ToLower(rest), " to "); i >= 0 {
		fromPart = strings.TrimSpace(rest[:i])
		toPart = strings.TrimSpace(rest[i+4:])
	}
	if !looksLikePlace(fromPart) {
		return timeQuery{}, false
	}
	if toPart != "" && !looksLikePlace(toPart) {
		return timeQuery{}, false
	}
	return timeQuery{when: when, structured: true, fromName: fromPart, toName: toPart}, true
}

func looksLikePlace(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	hasLetter := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r), r == ' ', r == '-', r == '_', r == '/', r == '\'', r == '.':
		default:
			return false
		}
	}
	return hasLetter
}

func stripPrefix(low, orig, prefix string) (string, bool) {
	if !strings.HasPrefix(low, prefix) {
		return "", false
	}
	return strings.TrimSpace(orig[len(prefix):]), true
}

func takeClock(s string) (time.Time, string, bool) {
	return takeClockManual(strings.TrimSpace(s))
}

func takeClockManual(s string) (time.Time, string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return time.Time{}, s, false
	}
	hourStr := s[:i]
	min := 0
	if i < len(s) && s[i] == ':' {
		i++
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i {
			return time.Time{}, s, false
		}
		fmt.Sscanf(s[i:j], "%d", &min)
		i = j
	}
	rest := strings.TrimSpace(s[i:])
	ampm := ""
	low := strings.ToLower(rest)
	switch {
	case strings.HasPrefix(low, "am"):
		ampm = "am"
		rest = strings.TrimSpace(rest[2:])
	case strings.HasPrefix(low, "pm"):
		ampm = "pm"
		rest = strings.TrimSpace(rest[2:])
	}
	var hour int
	fmt.Sscanf(hourStr, "%d", &hour)
	if ampm == "am" {
		if hour == 12 {
			hour = 0
		}
	} else if ampm == "pm" {
		if hour != 12 {
			hour += 12
		}
	} else if hour > 23 {
		return time.Time{}, s, false
	}
	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		return time.Time{}, s, false
	}
	t := time.Date(2000, 1, 1, hour, min, 0, 0, time.UTC)
	return t, rest, rest != "" || ampm != ""
}

func (e *Engine) evalTime(ctx context.Context, body string) []Result {
	q, ok := parseTimeQuery(body)
	if !ok {
		return nil
	}
	now := e.now()
	local := e.zone()

	if q.now {
		dest, label, ok := e.resolveZone(ctx, q.toName)
		if !ok {
			return nil
		}
		t := now.In(dest)
		if label == "" {
			label = t.Format("MST")
		}
		title := formatWall(t, true)
		return []Result{{
			Kind:     KindTime,
			Title:    title,
			Subtitle: label,
			Copy:     title,
			Rank:     85,
		}}
	}

	from, fromLabel, ok := e.resolveZone(ctx, q.fromName)
	if !ok {
		return nil
	}
	to, toLabel := local, localAbbr(now.In(local))
	if q.toName != "" {
		var ok bool
		to, toLabel, ok = e.resolveZone(ctx, q.toName)
		if !ok {
			return nil
		}
	}
	srcNow := now.In(from)
	when := time.Date(srcNow.Year(), srcNow.Month(), srcNow.Day(),
		q.when.Hour(), q.when.Minute(), 0, 0, from)
	out := when.In(to)
	title := formatWall(out, true)
	sub := fmt.Sprintf("%s %s → %s", formatWall(when, true), fromLabel, toLabel)
	return []Result{{
		Kind:     KindTime,
		Title:    title,
		Subtitle: sub,
		Copy:     title,
		Rank:     85,
	}}
}

func (e *Engine) resolveZone(ctx context.Context, name string) (*time.Location, string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", false
	}
	if loc, label, ok := lookupZone(name); ok {
		return loc, label, true
	}
	g, err := e.geocode(ctx, name)
	if err != nil || g.Timezone == "" {
		return nil, "", false
	}
	loc, err := time.LoadLocation(g.Timezone)
	if err != nil {
		return nil, "", false
	}
	label := g.Label
	if label == "" {
		label = displayCity(name)
	}
	return loc, label, true
}

func formatWall(t time.Time, twelve bool) string {
	zone := t.Format("MST")
	if twelve {
		return t.Format("3:04 PM") + " " + zone
	}
	return t.Format("15:04") + " " + zone
}

func localAbbr(t time.Time) string {
	return t.Format("MST")
}

func lookupZone(name string) (*time.Location, string, bool) {
	s := strings.TrimSpace(name)
	if s == "" {
		return nil, "", false
	}
	key := strings.ToLower(s)
	key = strings.ReplaceAll(key, "_", " ")
	key = strings.Join(strings.Fields(key), " ")

	if iana, ok := cities[key]; ok {
		loc, err := time.LoadLocation(iana)
		if err != nil {
			return nil, "", false
		}
		return loc, displayCity(s), true
	}
	if iana, label, ok := abbrevZone(strings.ToUpper(strings.ReplaceAll(key, " ", ""))); ok {
		loc, err := time.LoadLocation(iana)
		if err != nil {
			return nil, "", false
		}
		return loc, label, true
	}
	if loc, err := time.LoadLocation(s); err == nil {
		return loc, s, true
	}
	// America/New_York style with spaces
	guess := strings.ReplaceAll(s, " ", "_")
	if loc, err := time.LoadLocation(guess); err == nil {
		return loc, s, true
	}
	return nil, "", false
}

func displayCity(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	parts := strings.Fields(strings.ToLower(s))
	for i, p := range parts {
		r := []rune(p)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

func abbrevZone(code string) (iana, label string, ok bool) {
	switch code {
	case "UTC", "GMT", "Z":
		return "UTC", "UTC", true
	case "PT", "PST", "PDT":
		return "America/Los_Angeles", "PT", true
	case "MT", "MST", "MDT":
		return "America/Denver", "MT", true
	case "CT", "CDT":
		return "America/Chicago", "CT", true
	case "CST": // US Central; China is noted if someone types CST in Asia, we pick US
		return "America/Chicago", "CT", true
	case "ET", "EST", "EDT":
		return "America/New_York", "ET", true
	case "CET", "CEST":
		return "Europe/Copenhagen", "CET", true
	case "WET", "WEST":
		return "Europe/Lisbon", "WET", true
	case "EET", "EEST":
		return "Europe/Athens", "EET", true
	case "BST":
		return "Europe/London", "UK", true
	case "JST":
		return "Asia/Tokyo", "JST", true
	case "KST":
		return "Asia/Seoul", "KST", true
	case "IST":
		return "Asia/Kolkata", "IST", true
	case "AET", "AEST", "AEDT":
		return "Australia/Sydney", "AET", true
	case "NZST", "NZDT", "NZT":
		return "Pacific/Auckland", "NZ", true
	}
	return "", "", false
}

var cities = map[string]string{
	"tokyo":         "Asia/Tokyo",
	"london":        "Europe/London",
	"copenhagen":    "Europe/Copenhagen",
	"paris":         "Europe/Paris",
	"berlin":        "Europe/Berlin",
	"oslo":          "Europe/Oslo",
	"stockholm":     "Europe/Stockholm",
	"helsinki":      "Europe/Helsinki",
	"amsterdam":     "Europe/Amsterdam",
	"rome":          "Europe/Rome",
	"madrid":        "Europe/Madrid",
	"lisbon":        "Europe/Lisbon",
	"dublin":        "Europe/Dublin",
	"zurich":        "Europe/Zurich",
	"vienna":        "Europe/Vienna",
	"prague":        "Europe/Prague",
	"warsaw":        "Europe/Warsaw",
	"athens":        "Europe/Athens",
	"istanbul":      "Europe/Istanbul",
	"moscow":        "Europe/Moscow",
	"new york":      "America/New_York",
	"nyc":           "America/New_York",
	"boston":        "America/New_York",
	"washington":    "America/New_York",
	"miami":         "America/New_York",
	"chicago":       "America/Chicago",
	"dallas":        "America/Chicago",
	"denver":        "America/Denver",
	"phoenix":       "America/Phoenix",
	"los angeles":   "America/Los_Angeles",
	"la":            "America/Los_Angeles",
	"san francisco": "America/Los_Angeles",
	"sf":            "America/Los_Angeles",
	"seattle":       "America/Los_Angeles",
	"vancouver":     "America/Vancouver",
	"toronto":       "America/Toronto",
	"mexico city":   "America/Mexico_City",
	"sao paulo":     "America/Sao_Paulo",
	"buenos aires":  "America/Argentina/Buenos_Aires",
	"sydney":        "Australia/Sydney",
	"melbourne":     "Australia/Melbourne",
	"auckland":      "Pacific/Auckland",
	"honolulu":      "Pacific/Honolulu",
	"shanghai":      "Asia/Shanghai",
	"beijing":       "Asia/Shanghai",
	"hong kong":     "Asia/Hong_Kong",
	"singapore":     "Asia/Singapore",
	"seoul":         "Asia/Seoul",
	"taipei":        "Asia/Taipei",
	"mumbai":        "Asia/Kolkata",
	"delhi":         "Asia/Kolkata",
	"bangalore":     "Asia/Kolkata",
	"dubai":         "Asia/Dubai",
	"tel aviv":      "Asia/Jerusalem",
	"cairo":         "Africa/Cairo",
	"johannesburg":  "Africa/Johannesburg",
	"nairobi":       "Africa/Nairobi",
}
