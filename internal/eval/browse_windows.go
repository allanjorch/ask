//go:build windows

package eval

import (
	"os"
	"path/filepath"
)

func openBrowse(target string) error {
	if p := windowsBrave(); p != "" {
		if target == "" {
			return startDetached(p)
		}
		return startDetached(p, target)
	}
	if target == "" {
		target = "https://search.brave.com"
	}
	return startDetached("rundll32", "url.dll,FileProtocolHandler", target)
}

func windowsBrave() string {
	if p := lookBrave(); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(os.Getenv("PROGRAMFILES"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
		filepath.Join(home, "AppData", "Local", "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}
