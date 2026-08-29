package eval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseOperators(t *testing.T) {
	cases := []struct {
		in, body string
		filter   Kind
	}{
		{"2+2", "2+2", KindNone},
		{"= 2+2", "2+2", KindMath},
		{"=2+2", "2+2", KindMath},
		{"m 2+2", "2+2", KindMath},
		{"calc 2^10", "2^10", KindMath},
		{"c 100 usd dkk", "100 usd dkk", KindConvert},
		{"~ 10 km miles", "10 km miles", KindConvert},
		{"s NVDA 200", "NVDA 200", KindTicker},
		{"$NVDA", "NVDA", KindTicker},
		{"stock NVDA 200", "NVDA 200", KindTicker},
		{"t 9am PT", "9am PT", KindTime},
		{"time in Tokyo", "in Tokyo", KindTime},
		{"@ 9am PT", "9am PT", KindTime},
		{"d ephemeral", "ephemeral", KindDefine},
		{"define ephemeral", "ephemeral", KindDefine},
		{"?ephemeral", "ephemeral", KindDefine},
		{"w Tokyo", "Tokyo", KindWeather},
		{"weather in Tokyo", "in Tokyo", KindWeather},
		{"!", "", KindBrowse},
		{"!dr.dk", "dr.dk", KindBrowse},
		{"! dr.dk", "dr.dk", KindBrowse},
	}
	for _, c := range cases {
		q := Parse(c.in)
		if q.Filter != c.filter || q.Body != c.body {
			t.Errorf("Parse(%q) = {%q %q}, want {%q %q}", c.in, q.Filter, q.Body, c.filter, c.body)
		}
	}
}

func TestMath(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"2+2", "4"},
		{"2 * 3 + 4", "10"},
		{"2^10", "1,024"},
		{"15% of 80", "12"},
		{"200 + 10%", "220"},
		{"200 - 10%", "180"},
		{"(1+2)*3", "9"},
		{"10 / 4", "2.5"},
	}
	for _, c := range cases {
		res := evalMath(c.in)
		if len(res) != 1 || res[0].Title != c.out {
			t.Errorf("math %q → %#v, want %s", c.in, res, c.out)
		}
	}
}

func TestConvertUnits(t *testing.T) {
	e := New()
	res := e.evalConvert(context.Background(), "10 km to miles", false)
	if len(res) != 1 {
		t.Fatalf("km to miles: %+v", res)
	}
	if !strings.HasPrefix(res[0].Title, "6.213") {
		t.Errorf("10 km to miles = %q", res[0].Title)
	}
	res = e.evalConvert(context.Background(), "72f to c", false)
	if len(res) != 1 {
		t.Fatalf("72f to c: %+v", res)
	}
	if !strings.HasPrefix(res[0].Title, "22.22") {
		t.Errorf("72f to c = %q", res[0].Title)
	}
	from, to, amt, ok := parseConvert("100 usd dkk", true)
	if !ok || from == "" || to == "" || amt != 100 {
		t.Fatalf("loose parse 100 usd dkk: %q %q %v %v", from, to, amt, ok)
	}
}

func TestConvertCurrencyCached(t *testing.T) {
	e := New()
	e.SetFX("USD", "DKK", 6.5)
	res := e.Evaluate(context.Background(), "100 usd to dkk")
	if len(res) != 1 {
		t.Fatalf("got %+v", res)
	}
	if res[0].Kind != KindConvert {
		t.Errorf("kind %s", res[0].Kind)
	}
	if !strings.Contains(res[0].Title, "650") {
		t.Errorf("title %q", res[0].Title)
	}
}

func TestTickerParse(t *testing.T) {
	cases := []struct {
		in     string
		sym    string
		shares float64
		ok     bool
	}{
		{"NVDA", "NVDA", 0, true},
		{"NVDA 200", "NVDA", 200, true},
		{"200 NVDA", "NVDA", 200, true},
		{"btc", "BTC", 0, true},
		{"NOVO-B.CO", "NOVO-B.CO", 0, true},
		{"ephemeral", "", 0, false},
		{"100 usd to dkk", "", 0, false},
		{"2+2", "", 0, false},
	}
	for _, c := range cases {
		sym, sh, ok := parseTickerQuery(c.in)
		if ok != c.ok || sym != c.sym || sh != c.shares {
			t.Errorf("parseTickerQuery(%q) = %q %v %v, want %q %v %v", c.in, sym, sh, ok, c.sym, c.shares, c.ok)
		}
	}
}

func TestTickerHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"chart": map[string]any{
				"result": []any{
					map[string]any{
						"meta": map[string]any{
							"symbol":             "NVDA",
							"shortName":          "NVIDIA Corporation",
							"currency":           "USD",
							"regularMarketPrice": 100.0,
							"chartPreviousClose": 90.0,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	e := New()
	e.QuoteURL = func(symbol string) string { return srv.URL }
	res := e.Evaluate(context.Background(), "NVDA 200")
	if len(res) != 1 {
		t.Fatalf("got %+v", res)
	}
	if res[0].Kind != KindTicker {
		t.Errorf("kind %s", res[0].Kind)
	}
	if !strings.Contains(res[0].Title, "20,000") && !strings.Contains(res[0].Title, "20000") {
		t.Errorf("position title %q", res[0].Title)
	}
}

func TestCryptoPrefersUSD(t *testing.T) {
	if got := quoteSymbols("BTC"); len(got) < 1 || got[0] != "BTC-USD" {
		t.Fatalf("BTC → %v, want BTC-USD first", got)
	}
	if got := quoteSymbols("ETH"); got[0] != "ETH-USD" {
		t.Fatalf("ETH → %v", got)
	}
	if got := quoteSymbols("NVDA"); len(got) != 1 || got[0] != "NVDA" {
		t.Fatalf("NVDA → %v", got)
	}

	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"chart": map[string]any{
				"result": []any{
					map[string]any{
						"meta": map[string]any{
							"symbol":             "BTC-USD",
							"shortName":          "Bitcoin USD",
							"currency":           "USD",
							"regularMarketPrice": 80000.0,
							"chartPreviousClose": 79000.0,
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	e := New()
	e.QuoteURL = func(symbol string) string {
		requested = append(requested, symbol)
		return srv.URL
	}
	res := e.Evaluate(context.Background(), "BTC")
	if len(requested) == 0 || requested[0] != "BTC-USD" {
		t.Fatalf("requested %v, want BTC-USD first", requested)
	}
	if len(res) != 1 || !strings.Contains(res[0].Subtitle, "Bitcoin") {
		t.Fatalf("result %+v", res)
	}
}

func TestTimeTokyo(t *testing.T) {
	e := New()
	fixed := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	e.Now = func() time.Time { return fixed }
	e.Zone = func() *time.Location { return time.FixedZone("CEST", 2*3600) }
	res := e.Evaluate(context.Background(), "time in Tokyo")
	if len(res) != 1 {
		t.Fatalf("got %+v", res)
	}
	if res[0].Kind != KindTime {
		t.Errorf("kind %s", res[0].Kind)
	}
	if !strings.Contains(res[0].Title, "9:00 PM") && !strings.Contains(res[0].Title, "21:00") {
		t.Errorf("tokyo time %q (UTC 12:00 is 21:00 JST)", res[0].Title)
	}
}

func TestTimePTToLocal(t *testing.T) {
	e := New()
	loc, err := time.LoadLocation("Europe/Copenhagen")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	e.Now = func() time.Time { return fixed }
	e.Zone = func() *time.Location { return loc }
	q, ok := parseTimeQuery("9am PT")
	if !ok {
		t.Fatal("parse 9am PT")
	}
	if q.fromName == "" {
		t.Fatal("missing from zone")
	}
	res := e.Evaluate(context.Background(), "9am PT")
	if len(res) != 1 {
		t.Fatalf("got %+v", res)
	}
	if !strings.Contains(res[0].Title, "6:00 PM") {
		t.Errorf("9am PT → %q, want 6:00 PM CEST", res[0].Title)
	}
}

func TestBrowse(t *testing.T) {
	res := evalBrowse("")
	if len(res) != 1 || res[0].Action != "browse" || res[0].Arg != "" {
		t.Fatalf("empty: %+v", res)
	}
	res = evalBrowse("dr.dk")
	if len(res) != 1 || res[0].Arg != "https://dr.dk" {
		t.Fatalf("dr.dk: %+v", res)
	}
	res = evalBrowse("https://example.com/x")
	if res[0].Arg != "https://example.com/x" {
		t.Fatalf("url: %+v", res)
	}
	res = evalBrowse("omarchy")
	if len(res) != 1 {
		t.Fatalf("word: %+v", res)
	}
	if res[0].Arg != "https://search.brave.com/search?q=omarchy" {
		t.Fatalf("bare word should search, got Arg %q Title %q", res[0].Arg, res[0].Title)
	}
	if strings.HasSuffix(res[0].Arg, "/") || strings.Contains(res[0].Arg, "omarchy/") {
		t.Fatalf("must not treat a word as a host with a slash: %q", res[0].Arg)
	}
	if res[0].Title != "Search omarchy" {
		t.Fatalf("title %q", res[0].Title)
	}
	res = evalBrowse("hello world")
	if !strings.Contains(res[0].Arg, "search.brave.com/search?q=hello") {
		t.Fatalf("phrase search: %q", res[0].Arg)
	}
}

func TestRankingTryAll(t *testing.T) {
	e := New()
	e.SetFX("USD", "DKK", 6.5)
	res := e.Evaluate(context.Background(), "2+2")
	if len(res) != 1 || res[0].Kind != KindMath {
		t.Fatalf("2+2 → %+v", res)
	}
	res = e.Evaluate(context.Background(), "100 usd to dkk")
	if len(res) != 1 || res[0].Kind != KindConvert {
		t.Fatalf("fx → %+v", res)
	}
	if looksLikeTicker("ephemeral") {
		t.Fatal("ephemeral should not look like a ticker")
	}
	if !looksLikeDefine("ephemeral") {
		t.Fatal("ephemeral should look like a definition")
	}
	res = e.Evaluate(context.Background(), "!")
	if len(res) != 1 || res[0].Kind != KindBrowse {
		t.Fatalf("! → %+v", res)
	}
}

func TestLooksLike(t *testing.T) {
	if !looksLikeMath("2+2") || looksLikeMath("NVDA") {
		t.Fatal("math")
	}
	if !looksLikeConvert("10 km to miles") || looksLikeConvert("NVDA 200") {
		t.Fatal("convert")
	}
	if !looksLikeTime("9am PT") || !looksLikeTime("in Tokyo") {
		t.Fatal("time")
	}
	if looksLikeTime("ephemeral") || looksLikeTime("2+2") {
		t.Fatal("time should not claim definitions or math")
	}
	if !looksLikeTime("in Reykjavik") {
		t.Fatal("unknown city with in-prefix is still a time query")
	}
}

func TestTimeGeocodeFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []any{
				map[string]any{
					"name":      "Reykjavik",
					"country":   "Iceland",
					"latitude":  64.13,
					"longitude": -21.9,
					"timezone":  "Atlantic/Reykjavik",
				},
			},
		})
	}))
	defer srv.Close()
	e := New()
	fixed := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	e.Now = func() time.Time { return fixed }
	e.GeoURL = func(place string) string { return srv.URL }
	if NeedsNetwork("time in Reykjavik") != true {
		t.Fatal("unknown city should debounce for geocode")
	}
	if NeedsNetwork("time in Tokyo") {
		t.Fatal("known city should stay offline")
	}
	res := e.Evaluate(context.Background(), "time in Reykjavik")
	if len(res) != 1 {
		t.Fatalf("got %+v", res)
	}
	if res[0].Kind != KindTime {
		t.Errorf("kind %s", res[0].Kind)
	}
	// UTC 12:00 is 12:00 in Iceland (no DST).
	if !strings.Contains(res[0].Title, "12:00 PM") && !strings.Contains(res[0].Title, "12:00") {
		t.Errorf("reykjavik time %q", res[0].Title)
	}
	if !strings.Contains(res[0].Subtitle, "Reykjavik") {
		t.Errorf("subtitle %q", res[0].Subtitle)
	}
}
