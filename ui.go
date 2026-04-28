package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

func (p *Player) activate(initialFile string) {
	p.buildUI()
	p.window.Present()

	if initialFile != "" {
		if p.loadPlaylist(initialFile) {
			p.autoPlayLastTrack()
			return
		}
	} else if p.settings.LastFile != "" {
		if _, err := os.Stat(p.settings.LastFile); err == nil && p.loadPlaylist(p.settings.LastFile) {
			p.autoPlayLastTrack()
			return
		}
	}

	glib.IdleAdd(func() {
		p.openPlaylistDialog()
	})
}

func (p *Player) buildUI() {
	installAppCSS()

	p.window = gtk.NewApplicationWindow(p.app)
	p.window.SetTitle(appName)
	p.window.SetIconName(appID)
	p.window.SetDefaultSize(420, 480)
	p.window.SetTitlebar(p.titlebar())

	root := gtk.NewBox(gtk.OrientationVertical, 6)
	setMargin(root, 8)
	p.window.SetChild(root)

	top := gtk.NewBox(gtk.OrientationVertical, 6)
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical)
	scroll.SetPropagationPhase(gtk.PhaseCapture)
	scroll.ConnectScroll(func(_ float64, dy float64) bool {
		return p.scrollVolume(dy)
	})
	top.AddController(scroll)
	root.Append(top)

	p.statusLabel = gtk.NewLabel("")
	p.statusLabel.SetXAlign(0)
	p.statusLabel.SetEllipsize(pango.EllipsizeEnd)
	top.Append(p.statusLabel)
	top.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	controls := gtk.NewBox(gtk.OrientationHorizontal, 6)
	p.muteBtn = iconButton("xsi-audio-volume-high-symbolic", "Mute")
	p.volumeScale = gtk.NewScaleWithRange(gtk.OrientationHorizontal, 0, 100, 1)
	p.volumeScale.SetDrawValue(false)
	p.volumeScale.SetValue(float64(p.settings.Volume))
	p.volumeScale.SetHExpand(true)
	p.playBtn = iconButton("media-playback-start-symbolic", "Play")
	shuffleBtn := iconButton("media-playlist-shuffle-symbolic", "Shuffle")
	openBtn := iconButton("document-open-symbolic", "Open")

	p.muteBtn.ConnectClicked(func() { p.toggleMute() })
	p.volumeScale.ConnectValueChanged(func() {
		p.updateVolume(int(p.volumeScale.Value()))
	})
	p.playBtn.ConnectClicked(func() {
		if p.playingIdx >= 0 {
			p.stopPlayback()
		} else if len(p.filteredList) > 0 {
			p.playTrack(0)
		}
	})
	shuffleBtn.ConnectClicked(func() {
		if len(p.filteredList) > 0 {
			p.playTrack(rand.Intn(len(p.filteredList)))
		}
	})
	openBtn.ConnectClicked(func() { p.openPlaylistDialog() })

	controls.Append(p.muteBtn)
	controls.Append(p.volumeScale)
	controls.Append(p.playBtn)
	controls.Append(shuffleBtn)
	controls.Append(openBtn)
	top.Append(controls)

	searchRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	p.searchEntry = gtk.NewSearchEntry()
	p.searchEntry.SetPlaceholderText("Search stations...")
	p.searchEntry.SetHExpand(true)
	p.searchEntry.ConnectSearchChanged(func() {
		p.filterPlaylist(p.searchEntry.Text())
		p.rebuildStationList()
		p.refreshUI()
	})
	p.countLabel = gtk.NewLabel("")
	p.countLabel.SetXAlign(1)
	searchRow.Append(p.searchEntry)
	searchRow.Append(p.countLabel)
	top.Append(searchRow)

	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	p.stationList = gtk.NewListBox()
	p.stationList.SetSelectionMode(gtk.SelectionNone)
	p.stationList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		track, ok := p.rowTracks[row]
		if !ok {
			return
		}
		for i, candidate := range p.filteredList {
			if candidate.URL == track.URL {
				p.playTrack(i)
				return
			}
		}
	})
	scroller.SetChild(p.stationList)
	root.Append(scroller)

	p.refreshUI()
}

func (p *Player) titlebar() *gtk.HeaderBar {
	header := gtk.NewHeaderBar()
	header.SetShowTitleButtons(true)

	infoIcon := gtk.NewImageFromIconName("dialog-information-symbolic")
	infoIcon.SetPixelSize(16)
	infoBtn := gtk.NewButton()
	infoBtn.SetChild(infoIcon)
	infoBtn.SetHasFrame(false)
	infoBtn.SetFocusOnClick(false)
	infoBtn.SetTooltipText("Info")
	infoBtn.AddCSSClass("titlebar-info-button")
	infoBtn.ConnectClicked(func() { p.showAboutDialog() })
	header.PackStart(infoBtn)

	return header
}

func (p *Player) showAboutDialog() {
	dialog := gtk.NewDialog()
	dialog.SetTransientFor(&p.window.Window)
	dialog.SetModal(true)
	dialog.SetTitle("Info")
	dialog.SetDefaultSize(336, -1)
	dialog.SetResizable(false)

	content := dialog.ContentArea()
	content.SetSpacing(12)
	setMargins(content, 16, 18, 12, 18)

	body := gtk.NewBox(gtk.OrientationHorizontal, 14)
	logo := gtk.NewImageFromPaintable(appLogo(64, 64))
	logo.SetPixelSize(64)
	logo.SetVAlign(gtk.AlignStart)
	body.Append(logo)

	text := gtk.NewLabel(buildInfoText())
	text.SetXAlign(0)
	text.SetSelectable(false)
	body.Append(text)
	content.Append(body)

	closeBtn := dialog.AddButton("Close", int(gtk.ResponseClose))
	if widget, ok := closeBtn.(interface {
		SetMarginEnd(int)
		SetMarginBottom(int)
	}); ok {
		widget.SetMarginEnd(12)
		widget.SetMarginBottom(10)
	}
	dialog.ConnectResponse(func(_ int) {
		dialog.Close()
	})
	dialog.Present()
	glib.IdleAdd(func() {
		dialog.SetFocus(nil)
		dialog.SetFocusVisible(false)
	})
}

func appLogo(canvasSize, iconSize int) gdk.Paintabler {
	iconData, err := iconFS.ReadFile("icon.png")
	if err != nil {
		return nil
	}
	stream := gio.NewMemoryInputStreamFromBytes(glib.NewBytes(iconData))
	icon, err := gdkpixbuf.NewPixbufFromStreamAtScale(context.Background(), stream, iconSize, iconSize, true)
	if err != nil {
		return nil
	}
	canvas := gdkpixbuf.NewPixbuf(gdkpixbuf.ColorspaceRGB, true, 8, canvasSize, canvasSize)
	canvas.Fill(0x00000000)
	offset := (canvasSize - iconSize) / 2
	icon.CopyArea(0, 0, iconSize, iconSize, canvas, offset, offset)
	return gdk.NewTextureForPixbuf(canvas)
}

var appCSSLoaded bool

func installAppCSS() {
	if appCSSLoaded {
		return
	}
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}
	provider := gtk.NewCSSProvider()
	provider.LoadFromString(`
button.titlebar-info-button {
  min-width: 20px;
  min-height: 20px;
  padding: 2px;
  margin-left: 8px;
  margin-right: 2px;
  border-radius: 999px;
}
button.titlebar-info-button:hover {
  background-color: rgba(127, 127, 127, 0.14);
}
button.titlebar-info-button:active {
  background-color: rgba(127, 127, 127, 0.22);
}
`)
	gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
	appCSSLoaded = true
}

type marginSetter interface {
	SetMarginTop(int)
	SetMarginBottom(int)
	SetMarginStart(int)
	SetMarginEnd(int)
}

func setMargin(widget marginSetter, margin int) {
	setMargins(widget, margin, margin, margin, margin)
}

func setMargins(widget marginSetter, top, right, bottom, left int) {
	widget.SetMarginTop(top)
	widget.SetMarginBottom(bottom)
	widget.SetMarginStart(left)
	widget.SetMarginEnd(right)
}

func iconButton(iconName, tooltip string) *gtk.Button {
	button := gtk.NewButtonFromIconName(iconName)
	button.SetTooltipText(tooltip)
	button.SetFocusOnClick(false)
	return button
}

func (p *Player) scrollVolume(dy float64) bool {
	const step = 3
	if dy < 0 {
		p.updateVolume(p.settings.Volume + step)
		return true
	}
	if dy > 0 {
		p.updateVolume(p.settings.Volume - step)
		return true
	}
	return false
}

func (p *Player) updateVolume(vol int) {
	oldVolume := p.settings.Volume
	shouldUnmute := p.isMuted && vol != oldVolume
	if vol < 0 {
		vol = 0
	} else if vol > 100 {
		vol = 100
	}
	if shouldUnmute {
		p.isMuted = false
		p.setMuted(false)
	}
	if p.settings.Volume == vol && !shouldUnmute {
		return
	}
	p.settings.Volume = vol
	p.volumeScale.SetValue(float64(vol))
	if !p.isMuted {
		p.setVolume(vol)
	}
	saveSettings(p.settings)
	p.refreshUI()
}

func (p *Player) rebuildStationList() {
	p.stationList.RemoveAll()
	p.rowTracks = make(map[*gtk.ListBoxRow]Track, len(p.filteredList))
	for _, track := range p.filteredList {
		track := track
		row := gtk.NewListBoxRow()
		row.SetActivatable(true)
		click := gtk.NewGestureClick()
		click.SetButton(1)
		click.ConnectPressed(func(_ int, _, _ float64) {
			for i, candidate := range p.filteredList {
				if candidate.URL == track.URL {
					p.playTrack(i)
					return
				}
			}
		})
		row.AddController(click)
		label := gtk.NewLabel(track.Name)
		label.SetXAlign(0)
		label.SetEllipsize(pango.EllipsizeEnd)
		setMargin(label, 4)
		row.SetChild(label)
		p.rowTracks[row] = track
		p.stationList.Append(row)
	}
}

func (p *Player) refreshUI() {
	if p.statusLabel == nil {
		return
	}
	p.statusLabel.SetMarkup(p.currentStatusMarkup())
	p.statusLabel.SetTooltipText(p.currentStatusTooltip())
	if p.playingIdx >= 0 {
		p.playBtn.SetIconName("media-playback-stop-symbolic")
	} else {
		p.playBtn.SetIconName("media-playback-start-symbolic")
	}
	if p.isMuted {
		p.muteBtn.SetIconName("xsi-audio-volume-muted-symbolic")
	} else {
		p.muteBtn.SetIconName("xsi-audio-volume-high-symbolic")
	}
	if len(p.filteredList) == 0 {
		p.countLabel.SetText("")
	} else {
		p.countLabel.SetText(fmt.Sprintf("%d", len(p.filteredList)))
	}
}

func (p *Player) openPlaylistDialog() {
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Open Playlist")

	playlistFilter := gtk.NewFileFilter()
	playlistFilter.SetName("Playlist Files (m3u, m3u8, xspf)")
	playlistFilter.AddPattern("*.m3u")
	playlistFilter.AddPattern("*.m3u8")
	playlistFilter.AddPattern("*.xspf")
	allFilter := gtk.NewFileFilter()
	allFilter.SetName("All Files")
	allFilter.AddPattern("*")

	filters := gio.NewListStore(gtk.GTypeFileFilter)
	filters.Append(playlistFilter.Object)
	filters.Append(allFilter.Object)
	dialog.SetFilters(filters)
	dialog.SetDefaultFilter(playlistFilter)

	if p.settings.LastFile != "" {
		startDir := filepath.Dir(p.settings.LastFile)
		if _, err := os.Stat(startDir); err == nil {
			dialog.SetInitialFolder(gio.NewFileForPath(startDir))
		}
	}

	dialog.Open(context.Background(), &p.window.Window, func(result gio.AsyncResulter) {
		file, err := dialog.OpenFinish(result)
		if err != nil || file == nil {
			return
		}
		if p.loadPlaylist(file.Path()) {
			p.autoPlayLastTrack()
		}
	})
}
