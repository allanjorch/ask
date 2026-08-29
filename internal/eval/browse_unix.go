//go:build !windows

package eval

import (
	"os/exec"
	"syscall"
)

func openBrowse(target string) error {
	if p := lookBrave(); p != "" {
		args := []string{}
		if target != "" {
			args = []string{target}
		}
		return startDetachedUnix(p, args...)
	}
	if target == "" {
		target = "https://search.brave.com"
	}
	return startDetachedUnix("xdg-open", target)
}

func startDetachedUnix(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
