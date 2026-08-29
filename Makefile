.PHONY: ask install windows test

ask:
	go build -tags novulkan -o ask ./cmd/ask

install: ask
	mkdir -p $(HOME)/.local/bin
	cp ask $(HOME)/.local/bin/ask

windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o ask.exe ./cmd/ask

test:
	go test ./internal/eval/
