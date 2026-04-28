.PHONY: build clean run install

BINARY := radioplayer

build:
	pkg-config --exists gtk4 gstreamer-1.0
	go build -o $(BINARY)

clean:
	rm -f $(BINARY)

run: build
	./$(BINARY) "Favourites (Radio).m3u8"

install: build
	cp $(BINARY) ~/.local/bin/
