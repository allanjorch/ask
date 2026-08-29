package eval

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"
)

const userAgent = "ask/0.1 (desktop lookup)"

// Engine answers a prompt. Network helpers are overridable for tests.
type Engine struct {
	HTTP       *http.Client
	Now        func() time.Time
	Zone       func() *time.Location
	QuoteURL   func(symbol string) string
	FXURL      func(from, to string) string
	DictURL    func(word string) string
	GeoURL     func(place string) string
	WeatherURL func(lat, lon float64) string

	mu     sync.Mutex
	fx     map[string]fxCache
	quotes map[string]quoteCache
}

func New() *Engine {
	return &Engine{
		HTTP: &http.Client{Timeout: 8 * time.Second},
		Now:  time.Now,
		Zone: func() *time.Location { return time.Local },
	}
}

func (e *Engine) http() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return http.DefaultClient
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) zone() *time.Location {
	if e.Zone != nil {
		return e.Zone()
	}
	return time.Local
}

// Evaluate returns ranked results for a prompt. Misses stay silent.
func (e *Engine) Evaluate(ctx context.Context, raw string) []Result {
	if ctx == nil {
		ctx = context.Background()
	}
	q := Parse(raw)
	if q.Filter != KindBrowse && q.emptyBody() {
		return nil
	}

	var out []Result
	run := func(k Kind, looks bool, fn func() []Result) {
		if q.Filter != KindNone && q.Filter != k {
			return
		}
		if q.Filter == KindNone && !looks {
			return
		}
		out = append(out, fn()...)
	}

	looseConvert := q.Filter == KindConvert
	run(KindBrowse, false, func() []Result { return evalBrowse(q.Body) })
	run(KindMath, looksLikeMath(q.Body), func() []Result { return evalMath(q.Body) })
	run(KindConvert, looksLikeConvert(q.Body), func() []Result { return e.evalConvert(ctx, q.Body, looseConvert) })
	run(KindTime, looksLikeTime(q.Body), func() []Result { return e.evalTime(q.Body) })
	run(KindTicker, looksLikeTicker(q.Body), func() []Result { return e.evalTicker(ctx, q.Body) })
	run(KindDefine, looksLikeDefine(q.Body), func() []Result { return e.evalDefine(ctx, q.Body) })
	run(KindWeather, q.Filter == KindWeather, func() []Result { return e.evalWeather(ctx, q.Body) })

	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank > out[j].Rank })
	return out
}

// HintLines is the faint empty-state operator list.
func HintLines() []string {
	return []string{
		"m  math          c  convert         s  stock",
		"t  time          d  define          w  weather",
		"!  browser",
	}
}
