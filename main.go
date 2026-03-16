package main

/*
#cgo pkg-config: libvlc
#include <stdlib.h>
#include <vlc/vlc.h>
*/
import "C"

import (
	"bufio"
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

//go:embed icon.png
var iconFS embed.FS

type Track struct {
	Name string
	URL  string
}

type Settings struct {
	LastFile     string `json:"last_file"`
	LastTrackURL string `json:"last_track_url"`
	Volume       int    `json:"volume"`
}

func getSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "radioplayer", "settings.json")
}

func loadSettings() Settings {
	data, err := os.ReadFile(getSettingsPath())
	if err != nil {
		return Settings{Volume: 75}
	}
	var s Settings
	if json.Unmarshal(data, &s) != nil {
		return Settings{Volume: 75}
	}
	return s
}

func saveSettings(s Settings) {
	path := getSettingsPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(path, data, 0644)
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

	stationList widget.List
	searchEdit  widget.Editor
	volSlider   widget.Float
	playBtn     widget.Clickable
	muteBtn     widget.Clickable
	randomBtn   widget.Clickable
	openBtn     widget.Clickable
	installBtn  widget.Clickable
	stationBtns []widget.Clickable

	window *app.Window
}

func installDesktopEntry(iconData []byte) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	desktopDir := filepath.Join(home, ".local", "share", "applications")
	iconDir := filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps")
	pixmapsDir := filepath.Join(home, ".local", "share", "pixmaps")

	os.MkdirAll(desktopDir, 0755)
	os.MkdirAll(iconDir, 0755)
	os.MkdirAll(pixmapsDir, 0755)

	os.WriteFile(filepath.Join(iconDir, "radioplayer.png"), iconData, 0644)
	os.WriteFile(filepath.Join(pixmapsDir, "radioplayer.png"), iconData, 0644)

	exe, err := os.Executable()
	if err != nil {
		exe = "radioplayer"
	}
	os.Chmod(exe, 0755)

	desktop := fmt.Sprintf(`[Desktop Entry]
Name=Radio Player
Comment=Simple Radio Player
Exec=%s
Icon=radioplayer
Terminal=false
Type=Application
Categories=AudioVideo;Audio;
StartupNotify=true
StartupWMClass=radioplayer
`, exe)

	if err := os.WriteFile(filepath.Join(desktopDir, "radioplayer.desktop"), []byte(desktop), 0644); err != nil {
		return false
	}

	exec.Command("update-desktop-database", desktopDir).Run()
	exec.Command("gtk-update-icon-cache", "-f", filepath.Join(home, ".local", "share", "icons", "hicolor")).Run()
	return true
}

func main() {
	noVideo := C.CString("--no-video")
	args := []*C.char{noVideo}
	defer C.free(unsafe.Pointer(noVideo))
	instance := C.libvlc_new(1, &args[0])
	if instance == nil {
		fmt.Println("Failed to init VLC. Install: sudo apt install libvlc-dev vlc")
		os.Exit(1)
	}

	settings := loadSettings()
	p := &Player{
		instance:    instance,
		playingIdx:  -1,
		settings:    settings,
		pendingFile: make(chan string, 1),
	}
	p.volSlider.Value = float32(settings.Volume) / 100.0
	p.stationList.List.Axis = layout.Vertical
	p.searchEdit.SingleLine = true

	go func() {
		w := new(app.Window)
		w.Option(
			app.Title("Radio Player"),
			app.Size(unit.Dp(400), unit.Dp(600)),
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
			fmt.Println(err)
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

func (p *Player) pickFile() {
	startDir := ""
	if p.settings.LastFile != "" {
		startDir = filepath.Dir(p.settings.LastFile)
	}
	path := openFileDialog(startDir)
	if path != "" {
		p.pendingFile <- path
		p.window.Invalidate()
	}
}

func openFileDialog(startDir string) string {
	if startDir == "" {
		startDir, _ = os.UserHomeDir()
	}
	if _, err := os.Stat(startDir); err != nil {
		startDir, _ = os.UserHomeDir()
	}
	out, err := exec.Command("zenity", "--file-selection",
		"--title=Open Playlist",
		"--file-filter=Playlist|*.m3u *.m3u8 *.xspf",
		"--filename="+startDir+"/").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	out, err = exec.Command("kdialog", "--getopenfilename", startDir, "*.m3u *.m3u8 *.xspf").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func (p *Player) run(w *app.Window) error {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.FingerSize = 24
	var ops op.Ops

	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			select {
			case path := <-p.pendingFile:
				p.loadPlaylist(path)
			default:
			}
			gtx := app.NewContext(&ops, e)
			p.handleEvents(gtx)
			p.draw(gtx, th)
			e.Frame(gtx.Ops)
		}
	}
}

func (p *Player) handleEvents(gtx layout.Context) {
	if p.playBtn.Clicked(gtx) {
		if p.playingIdx >= 0 {
			p.stopPlayback()
		} else if len(p.filteredList) > 0 {
			p.playTrack(0)
		}
	}

	if p.muteBtn.Clicked(gtx) {
		p.toggleMute()
	}

	if p.randomBtn.Clicked(gtx) && len(p.filteredList) > 0 {
		p.playTrack(rand.Intn(len(p.filteredList)))
	}

	if p.openBtn.Clicked(gtx) {
		go p.pickFile()
	}

	if p.installBtn.Clicked(gtx) {
		go func() {
			iconData, err := iconFS.ReadFile("icon.png")
			if err == nil && installDesktopEntry(iconData) {
				exec.Command("zenity", "--info",
					"--title=Radio Player",
					"--text=Desktop entry installed!\nRadio Player is now in your application menu.").Run()
			} else {
				exec.Command("zenity", "--error",
					"--title=Radio Player",
					"--text=Installation failed. Check permissions.").Run()
			}
		}()
	}

	for i := range p.stationBtns {
		if i < len(p.filteredList) && p.stationBtns[i].Clicked(gtx) {
			p.playTrack(i)
		}
	}

	for {
		ev, ok := p.searchEdit.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			p.filterPlaylist(p.searchEdit.Text())
		}
	}

	newVol := int(p.volSlider.Value * 100)
	if newVol != p.settings.Volume {
		p.settings.Volume = newVol
		if !p.isMuted {
			p.setVolume(newVol)
		}
		saveSettings(p.settings)
	}
}

var (
	colorDivider = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
	colorBtnGrey = color.NRGBA{R: 180, G: 180, B: 180, A: 255}
)

func (p *Player) draw(gtx layout.Context, th *material.Theme) layout.Dimensions {
	for len(p.stationBtns) < len(p.filteredList) {
		p.stationBtns = append(p.stationBtns, widget.Clickable{})
	}

	paint.Fill(gtx.Ops, th.Palette.Bg)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Status
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body2(th, p.currentStatus())
				lbl.Alignment = text.Middle
				lbl.Font.Style = font.Italic
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(divider),
		// Controls
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						muteLabel := "♪"
						if p.isMuted {
							muteLabel = "✕"
						}
						btn := material.Button(th, &p.muteBtn, muteLabel)
						btn.Background = colorBtnGrey
						btn.TextSize = unit.Sp(20)
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx,
							material.Slider(th, &p.volSlider).Layout,
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						label := "▶"
						if p.playingIdx >= 0 {
							label = "■"
						}
						btn := material.Button(th, &p.playBtn, label)
						btn.TextSize = unit.Sp(20)
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(th, &p.randomBtn, "↻")
						btn.Background = colorBtnGrey
						btn.TextSize = unit.Sp(20)
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(th, &p.openBtn, "📁")
						btn.Background = colorBtnGrey
						btn.TextSize = unit.Sp(20)
						return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, btn.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(th, &p.installBtn, "⚙")
						btn.Background = colorBtnGrey
						btn.TextSize = unit.Sp(20)
						return btn.Layout(gtx)
					}),
				)
			})
		}),
		layout.Rigid(divider),
		// Search + count
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(6)).Layout(gtx,
						material.Editor(th, &p.searchEdit, "Search...").Layout,
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx,
						material.Caption(th, fmt.Sprintf("%d", len(p.filteredList))).Layout,
					)
				}),
			)
		}),
		layout.Rigid(divider),
		// Station list
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.List(th, &p.stationList).Layout(gtx, len(p.filteredList),
				func(gtx layout.Context, i int) layout.Dimensions {
					if i >= len(p.stationBtns) {
						return layout.Dimensions{}
					}
					track := p.filteredList[i]
					dims := p.stationBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Body1(th, track.Name)
							if p.isPlayingTrack(track) {
								lbl.Font.Weight = font.Bold
							}
							return lbl.Layout(gtx)
						})
					})
					divider(gtx)
					return dims
				})
		}),
	)
}

func divider(gtx layout.Context) layout.Dimensions {
	size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(1)}
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, colorDivider)
	return layout.Dimensions{Size: size}
}

func (p *Player) currentStatus() string {
	if p.statusMsg != "" {
		return p.statusMsg
	}
	if p.playingIdx >= 0 && p.playingIdx < len(p.playlist) {
		return "Playing: " + p.playlist[p.playingIdx].Name
	}
	if len(p.playlist) == 0 {
		return "No playlist loaded"
	}
	return "Stopped"
}

func (p *Player) autoPlayLastTrack() {
	if p.settings.LastTrackURL == "" {
		return
	}
	for i, track := range p.playlist {
		if track.URL == p.settings.LastTrackURL {
			p.playTrack(i)
			return
		}
	}
}

func (p *Player) toggleMute() {
	if p.isMuted {
		p.isMuted = false
		p.setVolume(p.savedVolume)
		p.volSlider.Value = float32(p.savedVolume) / 100.0
	} else {
		p.isMuted = true
		p.savedVolume = int(p.volSlider.Value * 100)
		p.setVolume(0)
	}
}

func (p *Player) isPlayingTrack(track Track) bool {
	if p.playingIdx < 0 || p.playingIdx >= len(p.playlist) {
		return false
	}
	return p.playlist[p.playingIdx].URL == track.URL
}

func (p *Player) filterPlaylist(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		p.filteredList = p.playlist
	} else {
		var filtered []Track
		for _, t := range p.playlist {
			if strings.Contains(strings.ToLower(t.Name), query) {
				filtered = append(filtered, t)
			}
		}
		p.filteredList = filtered
	}
}

func (p *Player) playTrack(id int) {
	if id < 0 || id >= len(p.filteredList) {
		return
	}
	track := p.filteredList[id]

	for i, t := range p.playlist {
		if t.URL == track.URL {
			p.playingIdx = i
			break
		}
	}

	if p.mediaPlayer != nil {
		C.libvlc_media_player_stop(p.mediaPlayer)
		C.libvlc_media_player_release(p.mediaPlayer)
		p.mediaPlayer = nil
	}
	if p.media != nil {
		C.libvlc_media_release(p.media)
		p.media = nil
	}

	curl := C.CString(track.URL)
	defer C.free(unsafe.Pointer(curl))

	p.media = C.libvlc_media_new_location(p.instance, curl)
	if p.media == nil {
		p.statusMsg = "Error loading " + track.Name
		p.playingIdx = -1
		return
	}

	p.mediaPlayer = C.libvlc_media_player_new_from_media(p.media)
	if p.mediaPlayer == nil {
		p.statusMsg = "Error creating player"
		p.playingIdx = -1
		return
	}

	p.setVolume(int(p.volSlider.Value * 100))
	C.libvlc_media_player_play(p.mediaPlayer)
	p.statusMsg = ""
	p.settings.LastTrackURL = track.URL
	saveSettings(p.settings)
}

func (p *Player) stopPlayback() {
	if p.mediaPlayer != nil {
		C.libvlc_media_player_stop(p.mediaPlayer)
		C.libvlc_media_player_release(p.mediaPlayer)
		p.mediaPlayer = nil
	}
	if p.media != nil {
		C.libvlc_media_release(p.media)
		p.media = nil
	}
	p.playingIdx = -1
	p.settings.LastTrackURL = ""
	saveSettings(p.settings)
}

func (p *Player) setVolume(vol int) {
	if p.mediaPlayer != nil {
		C.libvlc_audio_set_volume(p.mediaPlayer, C.int(vol))
	}
}

func (p *Player) loadPlaylist(filename string) bool {
	var tracks []Track
	var err error

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".xspf" {
		tracks, err = parseXSPF(filename)
	} else {
		tracks, err = parseM3U8(filename)
	}

	if err != nil || len(tracks) == 0 {
		return false
	}
	p.playlist = tracks
	p.filteredList = tracks
	absPath, err := filepath.Abs(filename)
	if err != nil {
		absPath = filename
	}
	p.settings.LastFile = absPath
	saveSettings(p.settings)
	return true
}

func parseM3U8(filename string) ([]Track, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var tracks []Track
	var currentName string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXTINF:") {
			parts := strings.SplitN(line, ",", 2)
			if len(parts) == 2 {
				currentName = strings.TrimSpace(parts[1])
			}
		} else if !strings.HasPrefix(line, "#") {
			name := currentName
			if name == "" {
				base := filepath.Base(line)
				name = strings.TrimSuffix(base, filepath.Ext(base))
			}
			tracks = append(tracks, Track{Name: name, URL: line})
			currentName = ""
		}
	}
	return tracks, scanner.Err()
}

type xspfPlaylist struct {
	XMLName   xml.Name   `xml:"playlist"`
	TrackList xspfTracks `xml:"trackList"`
}

type xspfTracks struct {
	Tracks []xspfTrack `xml:"track"`
}

type xspfTrack struct {
	Location string `xml:"location"`
	Title    string `xml:"title"`
}

func parseXSPF(filename string) ([]Track, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var xspf xspfPlaylist
	if err := xml.Unmarshal(data, &xspf); err != nil {
		return nil, err
	}

	var tracks []Track
	for _, t := range xspf.TrackList.Tracks {
		name := t.Title
		if name == "" {
			name = filepath.Base(t.Location)
		}
		tracks = append(tracks, Track{Name: name, URL: t.Location})
	}
	return tracks, nil
}
