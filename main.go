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
	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"time"
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
	clrAccentFg  = color.NRGBA{R: 0, G: 122, B: 255, A: 18}    // accent tint for row highlight
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
	volScroll        gesture.Scroll // scroll-to-volume outside the list

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
		"--file-filter=Playlist Files (m3u, m3u8, xspf)|*.m3u *.m3u8 *.xspf",
		"--file-filter=All Files|*",
		"--filename="+startDir+"/").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	out, err = exec.Command("kdialog", "--getopenfilename", startDir, "*.m3u *.m3u8 *.xspf|Playlist Files (m3u, m3u8, xspf)").Output()
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
		p.showInstallMenu = !p.showInstallMenu
	}

	if p.installUbuntuBtn.Clicked(gtx) {
		p.showInstallMenu = false
		go func() {
			iconData, err := iconFS.ReadFile("icon.png")
			if err == nil && installDesktopEntry(iconData) {
				exec.Command("zenity", "--info",
					"--title=Radio Player",
					"--width=420",
					"--text=Desktop entry installed! Radio Player is now in your application menu.").Run()
			} else {
				exec.Command("zenity", "--error",
					"--title=Radio Player",
					"--width=420",
					"--text=Installation failed. Check permissions.").Run()
			}
		}()
	}

	for i := range p.stationBtns {
		if i < len(p.filteredList) && p.stationBtns[i].Clicked(gtx) {
			p.showInstallMenu = false
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

	// Scroll-to-volume: gesture registered on the top area in draw().
	// Positive delta = scrolled down = quieter; negative = up = louder.
	if scrollDelta := p.volScroll.Update(gtx.Metric, gtx.Source, time.Now(),
		gesture.Vertical,
		pointer.ScrollRange{},
		pointer.ScrollRange{Min: -1e6, Max: 1e6},
	); scrollDelta != 0 {
		newVol := p.settings.Volume - scrollDelta/30
		if newVol < 0 {
			newVol = 0
		} else if newVol > 100 {
			newVol = 100
		}
		if newVol != p.settings.Volume {
			p.settings.Volume = newVol
			p.volSlider.Value = float32(newVol) / 100.0
			if !p.isMuted {
				p.setVolume(newVol)
			}
			saveSettings(p.settings)
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

// withBackground draws a rounded-rect background behind the given widget.
func withBackground(gtx layout.Context, bg color.NRGBA, radius unit.Dp, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	r := gtx.Dp(radius)
	defer clip.RRect{
		Rect: image.Rectangle{Max: dims.Size},
		NW:   r, NE: r, SW: r, SE: r,
	}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, bg)
	call.Add(gtx.Ops)
	return dims
}

// iconBtn draws a rounded-rectangle icon button with properly centred label.
func iconBtn(gtx layout.Context, th *material.Theme, btn *widget.Clickable,
	label string, bg, fg color.NRGBA, size unit.Dp, textSize unit.Sp) layout.Dimensions {
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		s := gtx.Dp(size)
		gtx.Constraints = layout.Exact(image.Point{X: s, Y: s})
		r := gtx.Dp(unit.Dp(9)) // rounded rect, not full circle
		defer clip.RRect{
			Rect: image.Rectangle{Max: image.Point{X: s, Y: s}},
			NW: r, NE: r, SW: r, SE: r,
		}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, bg)
		// Centre label both horizontally and vertically.
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Body1(th, label)
				lbl.Color = fg
				lbl.TextSize = textSize
				return lbl.Layout(gtx)
			}),
		)
	})
}

// thinDivider draws a single-pixel separator line.
func thinDivider(gtx layout.Context) layout.Dimensions {
	size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, clrSeparator)
	return layout.Dimensions{Size: size}
}

// approxControlsTop is the approximate dp offset from the window top to the
// bottom of the controls bar (status ~42 + divider 1 + controls 64).
const approxControlsTop = 107

func (p *Player) draw(gtx layout.Context, th *material.Theme) layout.Dimensions {
	for len(p.stationBtns) < len(p.filteredList) {
		p.stationBtns = append(p.stationBtns, widget.Clickable{})
	}

	// Apply theme palette
	th.Palette.Bg = clrBg
	th.Palette.Fg = clrLabel
	th.Palette.ContrastBg = clrAccent
	th.Palette.ContrastFg = clrWhite

	paint.Fill(gtx.Ops, clrBg)

	// Draw main UI.
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(p.drawStatus(th)),
				layout.Rigid(thinDivider),
				layout.Rigid(p.drawControls(th)),
				layout.Rigid(p.drawSearch(th)),
				layout.Rigid(thinDivider),
			)
			scrollArea := clip.Rect{Max: d.Size}.Push(gtx.Ops)
			pass := pointer.PassOp{}.Push(gtx.Ops)
			p.volScroll.Add(gtx.Ops)
			pass.Pop()
			scrollArea.Pop()
			return d
		}),
		layout.Flexed(1, p.drawStationList(th)),
	)

	// Floating dropdown: drawn after main content so it appears on top.
	// op.Offset positions it without affecting the main layout dimensions.
	if p.showInstallMenu {
		cardMaxW := gtx.Dp(unit.Dp(260))
		dropGtx := gtx
		dropGtx.Constraints = layout.Constraints{
			Max: image.Point{X: cardMaxW, Y: gtx.Dp(unit.Dp(200))},
		}
		cardDims := p.drawInstallDropdown(th)(dropGtx)
		x := gtx.Constraints.Max.X - cardDims.Size.X - gtx.Dp(unit.Dp(8))
		y := gtx.Dp(unit.Dp(approxControlsTop))
		defer op.Offset(image.Point{X: x, Y: y}).Push(gtx.Ops).Pop()
		p.drawInstallDropdown(th)(dropGtx)
	}

	return dims
}

func (p *Player) drawStatus(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		isPlaying := p.playingIdx >= 0
		status := p.currentStatus()

		return layout.Inset{
			Top: unit.Dp(12), Bottom: unit.Dp(10),
			Left: unit.Dp(16), Right: unit.Dp(16),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if isPlaying {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Pulsing-style accent dot
						dotSize := gtx.Dp(unit.Dp(8))
						defer clip.RRect{
							Rect: image.Rectangle{Max: image.Point{X: dotSize, Y: dotSize}},
							NW: dotSize / 2, NE: dotSize / 2, SW: dotSize / 2, SE: dotSize / 2,
						}.Push(gtx.Ops).Pop()
						paint.Fill(gtx.Ops, clrAccent)
						return layout.Dimensions{Size: image.Point{X: dotSize, Y: dotSize}}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(th, status)
						lbl.Color = clrLabel
						lbl.Font.Weight = font.SemiBold
						return lbl.Layout(gtx)
					}),
				)
			}
			lbl := material.Body2(th, status)
			lbl.Color = clrSecondary
			return lbl.Layout(gtx)
		})
	}
}

func (p *Player) drawControls(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{
			Left: unit.Dp(12), Right: unit.Dp(12),
			Top: unit.Dp(10), Bottom: unit.Dp(10),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				// Mute toggle
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					icon := "♪"
					if p.isMuted {
						icon = "x"
					}
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return iconBtn(gtx, th, &p.muteBtn, icon, clrBtnBg, clrLabel, 36, unit.Sp(15))
					})
				}),
				// Volume slider
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx,
						material.Slider(th, &p.volSlider).Layout)
				}),
				// Play / Stop – primary action, accent colored.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					icon := "▶"
					if p.playingIdx >= 0 {
						icon = "■"
					}
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return iconBtn(gtx, th, &p.playBtn, icon, clrAccent, clrWhite, 36, unit.Sp(16))
					})
				}),
				// Shuffle
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return iconBtn(gtx, th, &p.randomBtn, "↻", clrBtnBg, clrLabel, 36, unit.Sp(17))
					})
				}),
				// Open file
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return iconBtn(gtx, th, &p.openBtn, "\u229e", clrBtnBg, clrLabel, 36, unit.Sp(15))
					})
				}),
				// Settings / install cogwheel – active state uses accent bg
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					bg := clrBtnBg
					fg := clrLabel
					if p.showInstallMenu {
						bg = clrAccent
						fg = clrWhite
					}
					return iconBtn(gtx, th, &p.installBtn, "⚙", bg, fg, 36, unit.Sp(16))
				}),
			)
		})
	}
}

// drawInstallDropdown renders a compact floating dropdown card.
// Avoids nested withBackground to prevent any accidental full-height fills.
func (p *Player) drawInstallDropdown(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}

		// Measure content to get the card's natural size.
		macro := op.Record(gtx.Ops)
		contentDims := p.installUbuntuBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left: unit.Dp(14), Right: unit.Dp(18),
				Top: unit.Dp(10), Bottom: unit.Dp(10),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body2(th, "⚙  ")
						lbl.Color = clrAccent
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Body1(th, "Install for Ubuntu")
						lbl.Color = clrAccent
						lbl.Font.Weight = font.Medium
						return lbl.Layout(gtx)
					}),
				)
			})
		})
		call := macro.Stop()

		r := gtx.Dp(unit.Dp(12))
		b := 1 // border thickness px

		// Border ring.
		{
			s := clip.RRect{Rect: image.Rectangle{Max: contentDims.Size}, NW: r, NE: r, SW: r, SE: r}.Push(gtx.Ops)
			paint.Fill(gtx.Ops, clrSeparator)
			s.Pop()
		}
		// White surface (1 px inset).
		{
			inner := image.Rectangle{
				Min: image.Point{X: b, Y: b},
				Max: image.Point{X: contentDims.Size.X - b, Y: contentDims.Size.Y - b},
			}
			ri := r - b
			s := clip.RRect{Rect: inner, NW: ri, NE: ri, SW: ri, SE: ri}.Push(gtx.Ops)
			paint.Fill(gtx.Ops, clrSurface)
			s.Pop()
		}
		// Replay button content on top.
		call.Add(gtx.Ops)
		return contentDims
	}
}

func (p *Player) drawSearch(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{
			Left: unit.Dp(12), Right: unit.Dp(12),
			Top: unit.Dp(8), Bottom: unit.Dp(8),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return withBackground(gtx, clrSurface, unit.Dp(10), func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{
							Left: unit.Dp(12), Right: unit.Dp(8),
							Top: unit.Dp(9), Bottom: unit.Dp(9),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							edit := material.Editor(th, &p.searchEdit, "Search stations...")
							edit.Color = clrLabel
							edit.HintColor = clrSecondary
							edit.TextSize = unit.Sp(15)
							return edit.Layout(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if len(p.filteredList) == 0 {
							return layout.Dimensions{}
						}
						return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Caption(th, fmt.Sprintf("%d", len(p.filteredList)))
							lbl.Color = clrSecondary
							return lbl.Layout(gtx)
						})
					}),
				)
			})
		})
	}
}

func (p *Player) drawStationList(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return material.List(th, &p.stationList).Layout(gtx, len(p.filteredList),
			func(gtx layout.Context, i int) layout.Dimensions {
				if i >= len(p.stationBtns) {
					return layout.Dimensions{}
				}
				track := p.filteredList[i]
				isPlaying := p.isPlayingTrack(track)

				rowBg := clrSurface
				if isPlaying {
					rowBg = clrAccentFg
				}

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.stationBtns[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return withBackground(gtx, rowBg, unit.Dp(0), func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{
									Left: unit.Dp(16), Right: unit.Dp(16),
									Top: unit.Dp(12), Bottom: unit.Dp(12),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									// Track name only – playing state shown via color + weight.
								lbl := material.Body1(th, track.Name)
								lbl.TextSize = unit.Sp(15)
								if isPlaying {
									lbl.Color = clrAccent
									lbl.Font.Weight = font.SemiBold
								} else {
									lbl.Color = clrLabel
								}
								return lbl.Layout(gtx)
								})
							})
						})
					}),
					layout.Rigid(thinDivider),
				)
			})
	}
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
