package eval

import (
	"strings"
	"unicode"
)

// Query is a typed prompt: an optional filter plus the remaining body.
type Query struct {
	Raw    string
	Filter Kind
	Body   string
}

var wordOperators = map[string]Kind{
	"m": KindMath, "calc": KindMath, "math": KindMath,
	"c": KindConvert, "conv": KindConvert, "convert": KindConvert,
	"s": KindTicker, "stock": KindTicker, "quote": KindTicker,
	"ticker": KindTicker, "crypto": KindTicker,
	"t": KindTime, "time": KindTime, "tz": KindTime,
	"d": KindDefine, "define": KindDefine, "dict": KindDefine,
	"w": KindWeather, "weather": KindWeather, "wx": KindWeather,
	"forecast": KindWeather,
}

// Parse splits a prompt into an optional operator and a body.
func Parse(raw string) Query {
	q := Query{Raw: raw, Body: strings.TrimSpace(raw)}
	s := q.Body
	if s == "" {
		return q
	}

	if s[0] == '!' {
		q.Filter = KindBrowse
		q.Body = strings.TrimSpace(s[1:])
		return q
	}

	if k, ok := symbolOperator(s[0]); ok {
		q.Filter = k
		q.Body = strings.TrimSpace(s[1:])
		return q
	}

	first, rest, ok := splitFirst(s)
	if !ok {
		return q
	}
	if k, ok := wordOperators[strings.ToLower(first)]; ok {
		q.Filter = k
		q.Body = rest
	}
	return q
}

func symbolOperator(b byte) (Kind, bool) {
	switch b {
	case '=':
		return KindMath, true
	case '~':
		return KindConvert, true
	case '$':
		return KindTicker, true
	case '@':
		return KindTime, true
	case '?':
		return KindDefine, true
	case '*':
		return KindWeather, true
	}
	return KindNone, false
}

func splitFirst(s string) (first, rest string, ok bool) {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return "", "", false
	}
	return s[:i], strings.TrimSpace(s[i:]), true
}

func (q Query) emptyBody() bool {
	return strings.TrimSpace(q.Body) == ""
}

// NeedsNetwork reports whether answering this query may hit the network.
func NeedsNetwork(raw string) bool {
	q := Parse(raw)
	if q.Filter == KindBrowse || q.emptyBody() {
		return false
	}
	if q.Filter != KindNone {
		switch q.Filter {
		case KindTicker, KindDefine, KindWeather:
			return true
		case KindConvert:
			return looksLikeCurrency(q.Body)
		case KindTime:
			return timeNeedsNetwork(q.Body)
		default:
			return false
		}
	}
	return looksLikeTicker(q.Body) || looksLikeDefine(q.Body) || looksLikeCurrency(q.Body) || timeNeedsNetwork(q.Body)
}

func looksLikeMath(body string) bool {
	s := strings.TrimSpace(body)
	if s == "" {
		return false
	}
	hasDigit := false
	hasOp := false
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			hasDigit = true
		case strings.ContainsRune("+-*/^%()×÷−", r):
			hasOp = true
		}
	}
	if !hasDigit {
		return false
	}
	low := strings.ToLower(s)
	return hasOp || strings.Contains(low, "%") || strings.Contains(low, " of ")
}

func looksLikeConvert(body string) bool {
	_, _, _, ok := parseConvert(body, false)
	return ok
}

func looksLikeCurrency(body string) bool {
	from, to, _, ok := parseConvert(body, true)
	if !ok {
		from, to, _, ok = parseConvert(body, false)
	}
	return ok && isCurrency(from) && isCurrency(to)
}

func looksLikeTicker(body string) bool {
	_, _, ok := parseTickerQuery(body)
	return ok
}

func looksLikeTime(body string) bool {
	q, ok := parseTimeQuery(body)
	if !ok {
		return false
	}
	if q.structured {
		return true
	}
	_, _, known := lookupZone(q.toName)
	return known
}

func timeNeedsNetwork(body string) bool {
	q, ok := parseTimeQuery(body)
	if !ok {
		return false
	}
	return zoneNeedsNetwork(q.fromName) || zoneNeedsNetwork(q.toName)
}

func zoneNeedsNetwork(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	_, _, ok := lookupZone(name)
	return !ok
}

func looksLikeDefine(body string) bool {
	s := strings.TrimSpace(body)
	if s == "" || strings.ContainsAny(s, " \t") {
		return false
	}
	letters := 0
	for _, r := range s {
		if !unicode.IsLetter(r) && r != '-' && r != '\'' {
			return false
		}
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 6
}
