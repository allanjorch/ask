package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gioui.org/app"
	"github.com/allanjorch/ask/internal/eval"
	"github.com/allanjorch/ask/internal/ui"
)

// Wayland app_id / X11 class. The binary stays `ask`; the window identity is
// namespaced so it cannot collide with another short-named app.
func init() {
	app.ID = "com.github.allanjorch.ask"
}

func main() {
	if len(os.Args) > 1 {
		q := strings.Join(os.Args[1:], " ")
		res := eval.New().Evaluate(context.Background(), q)
		if len(res) == 0 {
			os.Exit(1)
		}
		for i, r := range res {
			if i > 0 {
				fmt.Println()
			}
			fmt.Println(r.Title)
			if r.Subtitle != "" {
				fmt.Println(r.Subtitle)
			}
		}
		return
	}
	go func() {
		if err := ui.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}
