.PHONY: build clean run install

BINARY := radioplayer
BUILD_VERSION ?= dev
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.buildVersion=$(BUILD_VERSION) -X main.buildDate=$(BUILD_DATE)

build:
	pkg-config --exists gtk4 gstreamer-1.0
	go build -ldflags "$(LDFLAGS)" -o $(BINARY)

clean:
	rm -f $(BINARY)

run: build
	./$(BINARY) "Favourites (Radio).m3u8"

install: build
	cp $(BINARY) ~/.local/bin/
