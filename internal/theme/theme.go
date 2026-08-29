package theme

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Colors follows Omarchy's colors.toml, with readable defaults.
type Colors struct {
	Background     color.NRGBA
	Foreground     color.NRGBA
	Muted          color.NRGBA
	Selection      color.NRGBA
	Accent         color.NRGBA
	DarkForeground color.NRGBA
	path           string
	mtime          time.Time
}

func Default() Colors {
	return Colors{
		Background:     hex("#1e1e1e"),
		Foreground:     hex("#eeeeee"),
		Muted:          hex("#7c6f64"),
		Selection:      hex("#3c3836"),
		Accent:         hex("#7daea3"),
		DarkForeground: hex("#7c6f64"),
	}
}

func Load() Colors {
	c := Default()
	path := colorsPath()
	if path == "" {
		return c
	}
	c.path = path
	c.reload()
	return c
}

func (c *Colors) Refresh() bool {
	if c.path == "" {
		return false
	}
	st, err := os.Stat(c.path)
	if err != nil {
		return false
	}
	if st.ModTime().Equal(c.mtime) {
		return false
	}
	return c.reload()
}

func (c *Colors) reload() bool {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return false
	}
	if st, err := os.Stat(c.path); err == nil {
		c.mtime = st.ModTime()
	}
	vals := parseTOML(string(data))
	get := func(keys ...string) (color.NRGBA, bool) {
		for _, k := range keys {
			if v, ok := vals[k]; ok {
				return hex(v), true
			}
		}
		return color.NRGBA{}, false
	}
	if v, ok := get("background", "dark_background"); ok {
		c.Background = v
	}
	if v, ok := get("foreground", "bright_foreground"); ok {
		c.Foreground = v
	}
	if v, ok := get("muted", "dark_foreground"); ok {
		c.Muted = v
	}
	if v, ok := get("dark_foreground"); ok {
		c.DarkForeground = v
	}
	if v, ok := get("selection", "lighter_background"); ok {
		c.Selection = v
	}
	if v, ok := get("accent", "cyan", "blue"); ok {
		c.Accent = v
	}
	return true
}

func colorsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".local", "state", "omarchy", "current", "theme", "colors.toml")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func parseTOML(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		out[k] = v
	}
	return out
}

func hex(s string) color.NRGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.NRGBA{A: 255}
	}
	n := func(i int) uint8 {
		a := unhex(s[i])
		b := unhex(s[i+1])
		return a<<4 | b
	}
	return color.NRGBA{R: n(0), G: n(2), B: n(4), A: 255}
}

func unhex(b byte) uint8 {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	}
	return 0
}

func (c Colors) WithAlpha(col color.NRGBA, a uint8) color.NRGBA {
	col.A = a
	return col
}
