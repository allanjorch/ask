package eval

import (
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

func evalBrowse(body string) []Result {
	target := strings.TrimSpace(body)
	if target == "" {
		return []Result{{
			Kind:     KindBrowse,
			Title:    "Open Brave",
			Subtitle: "Launch the browser",
			Action:   "browse",
			Arg:      "",
			Rank:     100,
		}}
	}
	if looksLikeURL(target) {
		addr := normalizeURL(target)
		return []Result{{
			Kind:     KindBrowse,
			Title:    "Open " + displayHost(addr),
			Subtitle: addr,
			Copy:     addr,
			Action:   "browse",
			Arg:      addr,
			Rank:     100,
		}}
	}
	// Bare words must go through search. Passing "omarchy" to Brave's CLI
	// is treated as the hostname https://omarchy/ — Chromium always
	// appends a slash to a one-label "URL".
	addr := braveSearchURL(target)
	return []Result{{
		Kind:     KindBrowse,
		Title:    "Search " + target,
		Subtitle: "Brave",
		Copy:     target,
		Action:   "browse",
		Arg:      addr,
		Rank:     100,
	}}
}

func displayHost(addr string) string {
	u, err := url.Parse(addr)
	if err == nil && u.Host != "" {
		return u.Host
	}
	return addr
}

func normalizeURL(s string) string {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return s
	}
	return "https://" + s
}

func braveSearchURL(q string) string {
	return "https://search.brave.com/search?q=" + url.QueryEscape(q)
}

func looksLikeURL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t") {
		return false
	}
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return true
	}
	if strings.HasPrefix(low, "localhost") || strings.HasPrefix(low, "127.0.0.1") {
		return true
	}
	host := s
	if i := strings.IndexAny(s, "/:?#"); i >= 0 {
		host = s[:i]
	}
	if !strings.Contains(host, ".") {
		return false
	}
	// Require a TLD-looking last label so "v1.2" and "john.doe" stay searches.
	last := host[strings.LastIndex(host, ".")+1:]
	if len(last) < 2 {
		return false
	}
	for _, r := range last {
		if r < 'A' || r > 'z' || (r > 'Z' && r < 'a') {
			return false
		}
	}
	return true
}

// OpenBrowse launches Brave (or the platform default browser) with target.
// An empty target opens a new browser window.
func OpenBrowse(target string) error {
	return openBrowse(target)
}

func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

// silence unused on unix; windows reuses this helper.
var _ = startDetached

func lookBrave() string {
	names := []string{"brave-browser", "brave", "brave-browser-stable"}
	if runtime.GOOS == "windows" {
		names = []string{"brave.exe", "brave"}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}
