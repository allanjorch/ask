package ui

import (
	"context"
	"image"
	"image/color"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/allanjorch/ask/internal/eval"
	"github.com/allanjorch/ask/internal/theme"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	winWidth = unit.Dp(560)
	inputH   = unit.Dp(44)
	rowH     = unit.Dp(52)
	pad      = unit.Dp(16)
	hintRowH = unit.Dp(22)
	maxRows  = 6
	debounce = 180 * time.Millisecond
)

type state struct {
	win    *app.Window
	engine *eval.Engine
	colors theme.Colors
	theme  *material.Theme
	editor widget.Editor
	list   widget.List

	gen             uint64
	cancel          context.CancelFunc
	results         []eval.Result
	selected        int
	query           string
	focused         bool
	lastH           unit.Dp
	closeAfterFrame bool
}

// Run opens the Ask window and blocks until it is closed.
func Run() error {
	w := new(app.Window)
	w.Option(
		app.Title("Ask"),
		app.Size(winWidth, unit.Dp(200)),
		app.Decorated(false),
	)
	s := &state{
		win:    w,
		engine: eval.New(),
		colors: theme.Load(),
		theme:  material.NewTheme(),
		list:   widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	s.theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	s.theme.Palette.Fg = s.colors.Foreground
	s.theme.Palette.Bg = s.colors.Background
	s.theme.Palette.ContrastBg = s.colors.Selection
	s.theme.Palette.ContrastFg = s.colors.Foreground
	s.editor.SingleLine = true
	s.editor.Submit = false
	s.editor.InputHint = key.HintAny

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			if s.cancel != nil {
				s.cancel()
			}
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			s.frame(gtx)
			e.Frame(gtx.Ops)
			if s.closeAfterFrame {
				s.close()
			}
		}
	}
}

func (s *state) frame(gtx layout.Context) {
	if s.colors.Refresh() {
		s.theme.Palette.Fg = s.colors.Foreground
		s.theme.Palette.Bg = s.colors.Background
		s.theme.Palette.ContrastBg = s.colors.Selection
		s.theme.Palette.ContrastFg = s.colors.Foreground
	}

	// Escape always closes, before the editor can see it.
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
			key.Filter{Name: key.NameUpArrow},
			key.Filter{Name: key.NameDownArrow},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		switch ke.Name {
		case key.NameEscape:
			s.close()
		case key.NameReturn, key.NameEnter:
			s.commit(gtx)
		case key.NameUpArrow:
			s.moveSel(-1)
		case key.NameDownArrow:
			s.moveSel(1)
		}
	}

	for {
		ev, ok := s.editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			s.onChange()
		}
	}

	if !s.focused {
		s.focused = true
		s.resize(0)
	}

	paint.Fill(gtx.Ops, s.colors.Background)
	layout.UniformInset(pad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(s.layoutInput),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Height: unit.Dp(10)}.Layout(gtx)
			}),
			layout.Flexed(1, s.layoutBody),
		)
	})
	if !gtx.Focused(&s.editor) {
		gtx.Execute(key.FocusCmd{Tag: &s.editor})
	}
}

func (s *state) layoutInput(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(inputH)
	gtx.Constraints.Max.Y = gtx.Dp(inputH)
	r := image.Rectangle{Max: gtx.Constraints.Max}
	rr := clip.RRect{Rect: r, SE: gtx.Dp(8), SW: gtx.Dp(8), NW: gtx.Dp(8), NE: gtx.Dp(8)}
	paint.FillShape(gtx.Ops, s.colors.Selection, rr.Op(gtx.Ops))

	return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(10), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		ed := material.Editor(s.theme, &s.editor, "")
		ed.Hint = ""
		ed.TextSize = unit.Sp(18)
		ed.Color = s.colors.Foreground
		ed.HintColor = s.colors.Muted
		ed.SelectionColor = s.colors.Accent
		dims := ed.Layout(gtx)
		if s.editor.Text() == "" {
			l := material.Label(s.theme, unit.Sp(18), "Ask…")
			l.Color = s.withAlpha(s.colors.Muted, 160)
			l.Layout(gtx)
		}
		return dims
	})
}

func (s *state) layoutBody(gtx layout.Context) layout.Dimensions {
	if strings.TrimSpace(s.editor.Text()) == "" {
		return s.layoutHint(gtx)
	}
	if len(s.results) == 0 {
		l := material.Label(s.theme, unit.Sp(13), "")
		l.Color = s.colors.Muted
		return l.Layout(gtx)
	}
	return s.list.Layout(gtx, len(s.results), func(gtx layout.Context, i int) layout.Dimensions {
		return s.layoutRow(gtx, i)
	})
}

func (s *state) layoutHint(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		hintLine(s, "m  math            c  convert           s  stock"),
		hintLine(s, "t  time            d  define            w  weather"),
		hintLine(s, "!  browser"),
	)
}

func hintLine(s *state, text string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		l := material.Label(s.theme, unit.Sp(13), text)
		l.Color = s.withAlpha(s.colors.DarkForeground, 180)
		l.Font.Typeface = "Go Mono"
		l.Font.Weight = font.Normal
		gtx.Constraints.Min.Y = gtx.Dp(hintRowH)
		return l.Layout(gtx)
	})
}

func (s *state) layoutRow(gtx layout.Context, i int) layout.Dimensions {
	r := s.results[i]
	gtx.Constraints.Min.Y = gtx.Dp(rowH)
	gtx.Constraints.Max.Y = gtx.Dp(rowH)
	rect := image.Rectangle{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(rowH)}}
	if i == s.selected {
		rr := clip.RRect{Rect: rect, SE: gtx.Dp(8), SW: gtx.Dp(8), NW: gtx.Dp(8), NE: gtx.Dp(8)}
		paint.FillShape(gtx.Ops, s.colors.Selection, rr.Op(gtx.Ops))
		bar := clip.Rect{Max: image.Point{X: gtx.Dp(3), Y: gtx.Dp(rowH)}}.Op()
		paint.FillShape(gtx.Ops, s.colors.Accent, bar)
	}
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(8), Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Label(s.theme, unit.Sp(16), r.Title)
				l.Color = s.colors.Foreground
				l.MaxLines = 1
				return l.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.Subtitle == "" {
					return layout.Dimensions{}
				}
				l := material.Label(s.theme, unit.Sp(12), r.Subtitle)
				l.Color = s.colors.Muted
				l.MaxLines = 1
				return l.Layout(gtx)
			}),
		)
	})
}

func (s *state) onChange() {
	q := s.editor.Text()
	if q == s.query {
		return
	}
	s.query = q
	s.gen++
	s.selected = 0
	if strings.TrimSpace(q) == "" {
		s.results = nil
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
		s.resize(0)
		return
	}
	s.kick(s.gen, q)
}

func (s *state) kick(gen uint64, q string) {
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	s.cancel = cancel
	delay := time.Duration(0)
	if eval.NeedsNetwork(q) {
		delay = debounce
	}
	go func() {
		if delay > 0 {
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
		}
		res := s.engine.Evaluate(ctx, q)
		if ctx.Err() != nil {
			return
		}
		s.win.Run(func() {
			if s.gen != gen {
				return
			}
			s.results = res
			if s.selected >= len(res) {
				s.selected = 0
			}
			s.resize(len(res))
		})
		s.win.Invalidate()
	}()
}

func (s *state) resize(n int) {
	h := pad*2 + inputH + unit.Dp(12)
	if strings.TrimSpace(s.editor.Text()) == "" {
		h += hintRowH*3 + unit.Dp(24)
	} else if n == 0 {
		h += unit.Dp(20)
	} else {
		if n > maxRows {
			n = maxRows
		}
		h += rowH * unit.Dp(n)
	}
	if h == s.lastH {
		return
	}
	s.lastH = h
	s.win.Option(app.Size(winWidth, h))
}

func (s *state) moveSel(delta int) {
	if len(s.results) == 0 {
		return
	}
	s.selected = (s.selected + delta + len(s.results)) % len(s.results)
}

func (s *state) commit(gtx layout.Context) {
	if len(s.results) == 0 {
		s.close()
		return
	}
	if s.selected < 0 || s.selected >= len(s.results) {
		s.selected = 0
	}
	r := s.results[s.selected]
	if r.Action == "browse" {
		_ = eval.OpenBrowse(r.Arg)
		s.close()
		return
	}
	text := r.CopyText()
	if text != "" {
		// Wayland clipboard is offered by the source process. Closing the
		// window in the same breath drops the offer, so hand the bytes to
		// wl-copy (which stays around) and only then dismiss.
		if !systemCopy(text) {
			gtx.Execute(clipboard.WriteCmd{
				Type: "application/text",
				Data: io.NopCloser(strings.NewReader(text)),
			})
		}
	}
	s.closeAfterFrame = true
}

func systemCopy(text string) bool {
	path, err := exec.LookPath("wl-copy")
	if err != nil {
		return false
	}
	cmd := exec.Command(path, "--trim-newline")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run() == nil
}

func (s *state) close() {
	s.win.Perform(system.ActionClose)
}

func (s *state) withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
