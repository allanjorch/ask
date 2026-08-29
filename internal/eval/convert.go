package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type dim string

const (
	dimLength dim = "length"
	dimMass   dim = "mass"
	dimTemp   dim = "temp"
	dimVolume dim = "volume"
	dimData   dim = "data"
	dimSpeed  dim = "speed"
	dimMoney  dim = "money"
)

type unitDef struct {
	dim    dim
	toBase float64 // multiply to get the dimension's base unit
	name   string
}

var units = map[string]unitDef{}

func init() {
	add := func(d dim, toBase float64, name string, aliases ...string) {
		u := unitDef{dim: d, toBase: toBase, name: name}
		units[name] = u
		for _, a := range aliases {
			units[a] = u
		}
	}
	// length, base metre
	add(dimLength, 1000, "km", "kilometer", "kilometers", "kilometre", "kilometres")
	add(dimLength, 1, "m", "meter", "meters", "metre", "metres")
	add(dimLength, 0.01, "cm", "centimeter", "centimeters")
	add(dimLength, 0.001, "mm", "millimeter", "millimeters")
	add(dimLength, 1609.344, "mi", "mile", "miles")
	add(dimLength, 0.9144, "yd", "yard", "yards")
	add(dimLength, 0.3048, "ft", "foot", "feet")
	add(dimLength, 0.0254, "in", "inch", "inches")
	add(dimLength, 1852, "nmi", "nauticalmile")
	// mass, base kilogram
	add(dimMass, 1, "kg", "kilogram", "kilograms")
	add(dimMass, 0.001, "g", "gram", "grams")
	add(dimMass, 1e-6, "mg", "milligram", "milligrams")
	add(dimMass, 0.45359237, "lb", "lbs", "pound", "pounds")
	add(dimMass, 0.028349523125, "oz", "ounce", "ounces")
	add(dimMass, 1000, "t", "tonne", "tonnes")
	// volume, base litre
	add(dimVolume, 1, "l", "liter", "liters", "litre", "litres")
	add(dimVolume, 0.001, "ml", "milliliter", "milliliters")
	add(dimVolume, 3.785411784, "gal", "gallon", "gallons")
	add(dimVolume, 0.946352946, "qt", "quart", "quarts")
	add(dimVolume, 0.2365882365, "cup", "cups")
	// data, base byte (decimal vs binary distinguished by unit)
	add(dimData, 1, "b", "byte", "bytes")
	add(dimData, 1000, "kb", "kilobyte", "kilobytes")
	add(dimData, 1e6, "mb", "megabyte", "megabytes")
	add(dimData, 1e9, "gb", "gigabyte", "gigabytes")
	add(dimData, 1e12, "tb", "terabyte", "terabytes")
	add(dimData, 1024, "kib", "kibibyte")
	add(dimData, 1024*1024, "mib", "mebibyte")
	add(dimData, 1024*1024*1024, "gib", "gibibyte")
	add(dimData, 1024*1024*1024*1024, "tib", "tebibyte")
	// speed, base m/s
	add(dimSpeed, 1000.0/3600.0, "kph", "kmh", "km/h")
	add(dimSpeed, 1609.344/3600.0, "mph")
	add(dimSpeed, 1, "m/s", "mps")
	add(dimSpeed, 1852.0/3600.0, "kn", "kt", "knot", "knots")
	// temperature, special-cased; toBase unused
	add(dimTemp, 1, "c", "celsius", "°c", "degc")
	add(dimTemp, 1, "f", "fahrenheit", "°f", "degf")
	add(dimTemp, 1, "k", "kelvin")
	for _, code := range currencyCodes {
		add(dimMoney, 1, strings.ToLower(code), code)
	}
}

var currencyCodes = []string{
	"EUR", "USD", "JPY", "BGN", "CZK", "DKK", "GBP", "HUF", "PLN", "RON",
	"SEK", "CHF", "ISK", "NOK", "TRY", "AUD", "BRL", "CAD", "CNY", "HKD",
	"IDR", "ILS", "INR", "KRW", "MXN", "MYR", "NZD", "PHP", "SGD", "THB", "ZAR",
}

func isCurrency(name string) bool {
	u, ok := lookupUnit(name)
	return ok && u.dim == dimMoney
}

func lookupUnit(s string) (unitDef, bool) {
	key := strings.ToLower(strings.TrimSpace(s))
	key = strings.ReplaceAll(key, "°", "")
	key = strings.TrimPrefix(key, "degrees")
	key = strings.TrimSpace(key)
	u, ok := units[key]
	return u, ok
}

// parseConvert returns fromUnit, toUnit, amount.
// loose allows "100 usd dkk" without to/in.
func parseConvert(body string, loose bool) (from, to string, amount float64, ok bool) {
	s := strings.TrimSpace(body)
	if s == "" {
		return "", "", 0, false
	}
	s = strings.ReplaceAll(s, "°", "")
	// NUMBER UNIT (to|in) UNIT   or  NUMBERUNIT (to|in) UNIT
	num, rest, ok := takeNumber(s)
	if !ok {
		return "", "", 0, false
	}
	rest = strings.TrimSpace(rest)
	low := strings.ToLower(rest)
	var fromPart, toPart string
	if i := strings.Index(low, " to "); i >= 0 {
		fromPart = strings.TrimSpace(rest[:i])
		toPart = strings.TrimSpace(rest[i+4:])
	} else if i := strings.Index(low, " in "); i >= 0 {
		fromPart = strings.TrimSpace(rest[:i])
		toPart = strings.TrimSpace(rest[i+4:])
	} else if loose {
		fromPart, toPart, ok = splitTwoUnits(rest)
		if !ok {
			return "", "", 0, false
		}
	} else {
		return "", "", 0, false
	}
	if fromPart == "" || toPart == "" {
		return "", "", 0, false
	}
	if _, ok := lookupUnit(fromPart); !ok {
		return "", "", 0, false
	}
	if _, ok := lookupUnit(toPart); !ok {
		return "", "", 0, false
	}
	return fromPart, toPart, num, true
}

func splitTwoUnits(s string) (from, to string, ok bool) {
	fields := strings.Fields(s)
	if len(fields) == 2 {
		return fields[0], fields[1], true
	}
	return "", "", false
}

func takeNumber(s string) (float64, string, bool) {
	i := 0
	rs := []rune(s)
	if i < len(rs) && (rs[i] == '+' || rs[i] == '-') {
		i++
	}
	start := i
	dots := 0
	for i < len(rs) {
		if unicode.IsDigit(rs[i]) || rs[i] == ',' {
			i++
			continue
		}
		if rs[i] == '.' {
			dots++
			if dots > 1 {
				break
			}
			i++
			continue
		}
		break
	}
	if i == start {
		return 0, s, false
	}
	raw := strings.ReplaceAll(string(rs[:i]), ",", "")
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, s, false
	}
	return n, string(rs[i:]), true
}

func (e *Engine) evalConvert(ctx context.Context, body string, loose bool) []Result {
	from, to, amount, ok := parseConvert(body, loose)
	if !ok {
		return nil
	}
	fu, _ := lookupUnit(from)
	tu, _ := lookupUnit(to)
	if fu.dim != tu.dim {
		return nil
	}
	var out float64
	var err error
	switch fu.dim {
	case dimTemp:
		out = convertTemp(amount, fu.name, tu.name)
	case dimMoney:
		out, err = e.convertMoney(ctx, amount, fu.name, tu.name)
		if err != nil {
			return nil
		}
	default:
		out = amount * fu.toBase / tu.toBase
	}
	title := formatConverted(out, tu)
	sub := fmt.Sprintf("%s %s → %s", formatNumber(amount), fu.name, tu.name)
	if fu.dim == dimMoney {
		title = formatMoney(out, tu.name)
		sub = fmt.Sprintf("%s %s → %s", formatNumber(amount), strings.ToUpper(fu.name), strings.ToUpper(tu.name))
	}
	return []Result{{
		Kind:     KindConvert,
		Title:    title,
		Subtitle: sub,
		Copy:     title,
		Rank:     90,
	}}
}

func formatConverted(v float64, u unitDef) string {
	n := formatNumber(v)
	return n + " " + u.name
}

func convertTemp(v float64, from, to string) float64 {
	from, to = strings.ToLower(from), strings.ToLower(to)
	// to C
	var c float64
	switch from {
	case "c", "celsius", "degc":
		c = v
	case "f", "fahrenheit", "degf":
		c = (v - 32) * 5 / 9
	case "k", "kelvin":
		c = v - 273.15
	}
	switch to {
	case "c", "celsius", "degc":
		return c
	case "f", "fahrenheit", "degf":
		return c*9/5 + 32
	case "k", "kelvin":
		return c + 273.15
	}
	return v
}

type fxCache struct {
	rate    float64
	fetched time.Time
}

func (e *Engine) convertMoney(ctx context.Context, amount float64, from, to string) (float64, error) {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)
	if from == to {
		return amount, nil
	}
	rate, err := e.fxRate(ctx, from, to)
	if err != nil {
		return 0, err
	}
	return amount * rate, nil
}

func (e *Engine) fxRate(ctx context.Context, from, to string) (float64, error) {
	key := from + ":" + to
	e.mu.Lock()
	if e.fx == nil {
		e.fx = map[string]fxCache{}
	}
	if c, ok := e.fx[key]; ok && time.Since(c.fetched) < time.Hour {
		e.mu.Unlock()
		return c.rate, nil
	}
	e.mu.Unlock()

	url := e.fxURL(from, to)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := e.http().Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return 0, fmt.Errorf("fx %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Rates map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	rate, ok := parsed.Rates[to]
	if !ok {
		return 0, fmt.Errorf("no rate %s", to)
	}
	e.mu.Lock()
	e.fx[key] = fxCache{rate: rate, fetched: time.Now()}
	e.mu.Unlock()
	return rate, nil
}

func (e *Engine) fxURL(from, to string) string {
	if e.FXURL != nil {
		return e.FXURL(from, to)
	}
	return "https://api.frankfurter.app/latest?from=" + from + "&to=" + to
}

// SetFX injects a rate without HTTP.
func (e *Engine) SetFX(from, to string, rate float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fx == nil {
		e.fx = map[string]fxCache{}
	}
	e.fx[from+":"+to] = fxCache{rate: rate, fetched: time.Now()}
}
