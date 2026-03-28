package main

/*
#cgo pkg-config: libvlc
#include <stdlib.h>
#include <vlc/vlc.h>
*/
import "C"

import (
	"embed"
	"image/color"
	"os"
	"unsafe"

	"gioui.org/app"
	"gioui.org/gesture"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

//go:embed icon.png
var iconFS embed.FS

// SwiftUI / iOS-inspired color palette
var (
	clrBg        = color.NRGBA{R: 242, G: 242, B: 247, A: 255} // iOS system background
	clrSurface   = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // card / row background
	clrAccent    = color.NRGBA{R: 0, G: 122, B: 255, A: 255}   // iOS blue
	clrLabel     = color.NRGBA{R: 28, G: 28, B: 30, A: 255}    // primary text
	clrSecondary = color.NRGBA{R: 142, G: 142, B: 147, A: 255} // secondary text
	clrSeparator = color.NRGBA{R: 198, G: 198, B: 200, A: 255} // divider
	clrBtnBg     = color.NRGBA{R: 229, G: 229, B: 234, A: 255} // secondary button
	clrWhite     = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	clrAccentFg  = color.NRGBA{R: 0, G: 122, B: 255, A: 18} // accent tint for row highlight
)

type Track struct {
	Name string
	URL  string
}

type Settings struct {
	LastFile     string `json:"last_file"`
	LastTrackURL string `json:"last_track_url"`
	Volume       int    `json:"volume"`
}

type Player struct {
	instance    *C.libvlc_instance_t
	mediaPlayer *C.libvlc_media_player_t
	media       *C.libvlc_media_t

	playlist     []Track
	filteredList []Track
	playingIdx   int
	settings     Settings
	isMuted      bool
	savedVolume  int
	statusMsg    string

	pendingFile chan string

	stationList      widget.List
	searchEdit       widget.Editor
	volSlider        widget.Float
	playBtn          widget.Clickable
	muteBtn          widget.Clickable
	randomBtn        widget.Clickable
	openBtn          widget.Clickable
	installBtn       widget.Clickable
	installUbuntuBtn widget.Clickable
	showInstallMenu  bool
	stationBtns      []widget.Clickable
	volScroll        gesture.Scroll

	// Vector icons loaded from Material Design icon set.
	icoPlay     *widget.Icon
	icoStop     *widget.Icon
	icoVolumeUp *widget.Icon
	icoVolumeOff *widget.Icon
	icoShuffle  *widget.Icon
	icoFolder   *widget.Icon
	icoSettings *widget.Icon

	window *app.Window
}

func main() {
	noVideo := C.CString("--no-video")
	args := []*C.char{noVideo}
	defer C.free(unsafe.Pointer(noVideo))
	instance := C.libvlc_new(1, &args[0])
	if instance == nil {
		println("Failed to init VLC. Install: sudo apt install libvlc-dev vlc")
		os.Exit(1)
	}

	settings := loadSettings()
	p := &Player{
		instance:    instance,
		playingIdx:  -1,
		settings:    settings,
		pendingFile: make(chan string, 1),
	}
	p.icoPlay, _ = widget.NewIcon(icons.AVPlayArrow)
	p.icoStop, _ = widget.NewIcon(icons.AVStop)
	p.icoVolumeUp, _ = widget.NewIcon(icons.AVVolumeUp)
	p.icoVolumeOff, _ = widget.NewIcon(icons.AVVolumeOff)
	p.icoShuffle, _ = widget.NewIcon(icons.AVShuffle)
	p.icoFolder, _ = widget.NewIcon(icons.FileFolderOpen)
	p.icoSettings, _ = widget.NewIcon(icons.ActionSettings)
	p.volSlider.Value = float32(settings.Volume) / 100.0
	p.stationList.List.Axis = layout.Vertical
	p.searchEdit.SingleLine = true

	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("Radio Player"),
			app.Size(unit.Dp(420), unit.Dp(640)),
		)
		p.window = w

		if len(os.Args) >= 2 {
			p.loadPlaylist(os.Args[1])
			p.autoPlayLastTrack()
		} else if settings.LastFile != "" {
			if _, err := os.Stat(settings.LastFile); err == nil {
				p.loadPlaylist(settings.LastFile)
				p.autoPlayLastTrack()
			} else {
				go p.pickFile()
			}
		} else {
			go p.pickFile()
		}

		if err := p.run(w); err != nil {
			println(err.Error())
		}
		if p.mediaPlayer != nil {
			C.libvlc_media_player_release(p.mediaPlayer)
		}
		if p.instance != nil {
			C.libvlc_release(p.instance)
		}
		os.Exit(0)
	}()

	app.Main()
}
