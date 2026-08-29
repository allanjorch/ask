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

type quote struct {
	Symbol    string
	Name      string
	Price     float64
	PrevClose float64
	Currency  string
}

type quoteCache struct {
	q       quote
	ok      bool
	fetched time.Time
}

func parseTickerQuery(body string) (symbol string, shares float64, ok bool) {
	s := strings.TrimSpace(body)
	if s == "" {
		return "", 0, false
	}
	fields := strings.Fields(s)
	switch len(fields) {
	case 1:
		if isTickerSymbol(fields[0]) {
			return normalizeSymbol(fields[0]), 0, true
		}
	case 2:
		if isTickerSymbol(fields[0]) && isShareCount(fields[1]) {
			n, _ := strconv.ParseFloat(strings.ReplaceAll(fields[1], ",", ""), 64)
			return normalizeSymbol(fields[0]), n, true
		}
		if isShareCount(fields[0]) && isTickerSymbol(fields[1]) {
			n, _ := strconv.ParseFloat(strings.ReplaceAll(fields[0], ",", ""), 64)
			return normalizeSymbol(fields[1]), n, true
		}
	}
	return "", 0, false
}

func isShareCount(s string) bool {
	dots := 0
	digits := 0
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			digits++
		case r == ',':
		case r == '.':
			dots++
			if dots > 1 {
				return false
			}
		default:
			return false
		}
	}
	return digits > 0
}

func isTickerSymbol(s string) bool {
	if s == "" || len(s) > 16 {
		return false
	}
	low := strings.ToLower(s)
	switch low {
	case "to", "in", "of", "at", "and", "the", "for", "or", "a":
		return false
	}
	letters := 0
	for i, r := range s {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			if i == 0 {
				return false
			}
		case r == '.' || r == '-' || r == '=':
			if i == 0 || i == len(s)-1 {
				return false
			}
		default:
			return false
		}
	}
	if letters < 1 {
		return false
	}
	// Bare words longer than a typical ticker are definitions, not quotes.
	if !strings.ContainsAny(s, ".-") && letters > 5 {
		return false
	}
	return true
}

func normalizeSymbol(s string) string {
	return strings.ToUpper(s)
}

var cryptoUSD = map[string]bool{
	"BTC": true, "ETH": true, "SOL": true, "DOGE": true, "XRP": true,
	"ADA": true, "DOT": true, "AVAX": true, "LINK": true, "UNI": true,
	"SHIB": true, "BNB": true, "LTC": true, "MATIC": true, "ATOM": true,
}

func (e *Engine) evalTicker(ctx context.Context, body string) []Result {
	sym, shares, ok := parseTickerQuery(body)
	if !ok {
		return nil
	}
	q, err := e.quote(ctx, sym)
	if err != nil {
		if cryptoUSD[sym] && !strings.Contains(sym, "-") {
			q, err = e.quote(ctx, sym+"-USD")
		}
	}
	if err != nil {
		return nil
	}
	chg := q.Price - q.PrevClose
	pct := 0.0
	if q.PrevClose != 0 {
		pct = chg / q.PrevClose * 100
	}
	sign := "+"
	if chg < 0 {
		sign = ""
	}
	price := formatMoney(q.Price, q.Currency)
	chgS := sign + formatMoney(chg, q.Currency)
	pctS := fmt.Sprintf("%s%.2f%%", sign, pct)
	name := q.Name
	if name == "" {
		name = q.Symbol
	}

	if shares > 0 {
		total := q.Price * shares
		title := formatMoney(total, q.Currency)
		sub := fmt.Sprintf("%s · %s × %s  %s %s", name, formatNumber(shares), price, chgS, pctS)
		return []Result{{
			Kind:     KindTicker,
			Title:    title,
			Subtitle: sub,
			Copy:     title,
			Rank:     80,
		}}
	}
	title := price
	sub := fmt.Sprintf("%s · %s %s", name, chgS, pctS)
	return []Result{{
		Kind:     KindTicker,
		Title:    title,
		Subtitle: sub,
		Copy:     title,
		Rank:     70,
	}}
}

func (e *Engine) quote(ctx context.Context, symbol string) (quote, error) {
	e.mu.Lock()
	if e.quotes == nil {
		e.quotes = map[string]quoteCache{}
	}
	if c, ok := e.quotes[symbol]; ok && time.Since(c.fetched) < 30*time.Second {
		e.mu.Unlock()
		if !c.ok {
			return quote{}, fmt.Errorf("no quote")
		}
		return c.q, nil
	}
	e.mu.Unlock()

	url := e.quoteURL(symbol)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return quote{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	res, err := e.http().Do(req)
	if err != nil {
		return quote{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return quote{}, err
	}
	q, err := parseYahooChart(body)
	if err != nil {
		e.mu.Lock()
		e.quotes[symbol] = quoteCache{fetched: time.Now()}
		e.mu.Unlock()
		return quote{}, err
	}
	if q.Symbol == "" {
		q.Symbol = symbol
	}
	e.mu.Lock()
	e.quotes[symbol] = quoteCache{q: q, ok: true, fetched: time.Now()}
	e.mu.Unlock()
	return q, nil
}

func (e *Engine) quoteURL(symbol string) string {
	if e.QuoteURL != nil {
		return e.QuoteURL(symbol)
	}
	return "https://query1.finance.yahoo.com/v8/finance/chart/" + symbol + "?interval=1d&range=1d"
}

func parseYahooChart(body []byte) (quote, error) {
	var parsed struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol             string  `json:"symbol"`
					ShortName          string  `json:"shortName"`
					LongName           string  `json:"longName"`
					Currency           string  `json:"currency"`
					RegularMarketPrice float64 `json:"regularMarketPrice"`
					ChartPreviousClose float64 `json:"chartPreviousClose"`
					PreviousClose      float64 `json:"previousClose"`
				} `json:"meta"`
			} `json:"result"`
			Error any `json:"error"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return quote{}, err
	}
	if len(parsed.Chart.Result) == 0 {
		return quote{}, fmt.Errorf("no result")
	}
	m := parsed.Chart.Result[0].Meta
	if m.RegularMarketPrice == 0 {
		return quote{}, fmt.Errorf("no price")
	}
	prev := m.ChartPreviousClose
	if prev == 0 {
		prev = m.PreviousClose
	}
	name := m.ShortName
	if name == "" {
		name = m.LongName
	}
	return quote{
		Symbol:    m.Symbol,
		Name:      name,
		Price:     m.RegularMarketPrice,
		PrevClose: prev,
		Currency:  m.Currency,
	}, nil
}
