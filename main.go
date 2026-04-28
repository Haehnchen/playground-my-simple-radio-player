package main

/*
#cgo pkg-config: libvlc
#include <stdlib.h>
#include <vlc/vlc.h>
*/
import "C"

import (
	"embed"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed icon.png
var iconFS embed.FS

const (
	appID   = "local.radioplayer"
	appName = "Radio Player"
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

	app         *gtk.Application
	window      *gtk.ApplicationWindow
	statusLabel *gtk.Label
	muteBtn     *gtk.Button
	playBtn     *gtk.Button
	volumeScale *gtk.Scale
	searchEntry *gtk.SearchEntry
	countLabel  *gtk.Label
	stationList *gtk.ListBox
	rowTracks   map[*gtk.ListBoxRow]Track
}

func main() {
	runtime.LockOSThread()
	glib.SetPrgname(appID)
	glib.SetApplicationName(appName)
	writeUserDesktopIdentity()

	noVideo := C.CString("--no-video")
	args := []*C.char{noVideo}
	defer C.free(unsafe.Pointer(noVideo))
	instance := C.libvlc_new(1, &args[0])
	if instance == nil {
		fmt.Fprintln(os.Stderr, "Failed to init VLC. Install: sudo apt install libvlc-dev vlc")
		os.Exit(1)
	}

	settings := loadSettings()
	p := &Player{
		instance:   instance,
		playingIdx: -1,
		settings:   settings,
		savedVolume: func() int {
			if settings.Volume <= 0 {
				return 75
			}
			return settings.Volume
		}(),
		rowTracks: make(map[*gtk.ListBoxRow]Track),
	}

	app := gtk.NewApplication(appID, gio.ApplicationDefaultFlags)
	p.app = app
	var initialFile string
	if len(os.Args) >= 2 {
		initialFile = os.Args[1]
	}

	app.ConnectActivate(func() {
		p.activate(initialFile)
	})

	status := app.Run([]string{os.Args[0]})
	p.cleanup()
	os.Exit(status)
}
