VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build install clean windows linux

build:
	go build $(LDFLAGS) -o openclaw-tui .

windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o openclaw-tui.exe .

linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o openclaw-tui-linux .

all: build windows linux

install:
	go install $(LDFLAGS) .

clean:
	rm -f openclaw-tui openclaw-tui.exe openclaw-tui-linux

run: build
	./openclaw-tui
