.PHONY: ask install windows test

LDFLAGS = -X gioui.org/app.ID=com.github.allanjorch.ask

ask:
	go build -tags novulkan -ldflags "$(LDFLAGS)" -o ask ./cmd/ask

install: ask
	mkdir -p $(HOME)/.local/bin
	cp ask $(HOME)/.local/bin/ask

windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ask.exe ./cmd/ask

test:
	go test ./internal/eval/
