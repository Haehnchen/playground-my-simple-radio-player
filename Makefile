.PHONY: build clean run install

BINARY := radioplayer

build:
	go build -o $(BINARY)

clean:
	rm -f $(BINARY)

run: build
	./$(BINARY) "Favourites (Radio).m3u8"

install: build
	cp $(BINARY) ~/.local/bin/
