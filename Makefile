VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build install clean

build:
	go build $(LDFLAGS) -o openclaw-tui .

install:
	go install $(LDFLAGS) .

clean:
	rm -f openclaw-tui

run: build
	./openclaw-tui
