# Radio Player

A simple radio player with a native GTK 4 UI, built in Go. Playback uses GStreamer inline via CGO.

## Features

- Play internet radio streams from M3U8 playlists
- Search/filter stations
- Volume control
- Mute, shuffle, and play/stop controls
- Linux desktop identity for the dock icon

## Requirements

- Go 1.21+
- GTK 4 development files
- GObject Introspection development files
- GStreamer development files and playback plugins

### Linux (Debian/Ubuntu)

```bash
sudo apt install libgtk-4-dev gobject-introspection libgirepository1.0-dev libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev gstreamer1.0-plugins-good
```

## Build

```bash
make build
```

## Usage

```bash
./radioplayer
```

```bash
./radioplayer <m3u8-file>
```

## Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the binary |
| `make clean` | Remove the binary |
| `make run` | Build and run with default playlist |
| `make install` | Install to `~/.local/bin/` |

## M3U8 Format

The player supports standard M3U8 playlists with `#EXTINF` tags:

```m3u8
#EXTM3U
#EXTINF:-1,BBC Radio 1
https://stream.live.vc.bbcmedia.co.uk/bbc_radio_one
#EXTINF:-1,Jazz FM
https://edge-bauerall-01-gos2.sharp-stream.com/jazz.mp3
```
