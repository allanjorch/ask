package eval

// Kind is a result type, and also the filter an operator selects.
type Kind string

const (
	KindNone    Kind = ""
	KindMath    Kind = "math"
	KindConvert Kind = "convert"
	KindTicker  Kind = "ticker"
	KindTime    Kind = "time"
	KindDefine  Kind = "define"
	KindWeather Kind = "weather"
	KindBrowse  Kind = "browse"
)

// Result is one row in the overlay.
type Result struct {
	Kind     Kind
	Title    string
	Subtitle string
	Copy     string // text copied on Enter; defaults to Title if empty
	Action   string // "browse" launches a browser instead of copying
	Arg      string
	Rank     int
}

func (r Result) CopyText() string {
	if r.Copy != "" {
		return r.Copy
	}
	return r.Title
}
