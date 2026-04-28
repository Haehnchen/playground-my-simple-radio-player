package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

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
	p.window = gtk.NewApplicationWindow(p.app)
	p.window.SetTitle(appName)
	p.window.SetIconName(appID)
	p.window.SetDefaultSize(420, 640)

	root := gtk.NewBox(gtk.OrientationVertical, 6)
	setMargin(root, 8)
	p.window.SetChild(root)

	top := gtk.NewBox(gtk.OrientationVertical, 6)
	scroll := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical)
	scroll.SetPropagationPhase(gtk.PhaseCapture)
	scroll.ConnectScroll(func(_ float64, dy float64) bool {
		if dy < 0 {
			p.updateVolume(p.settings.Volume + 1)
		} else if dy > 0 {
			p.updateVolume(p.settings.Volume - 1)
		}
		return false
	})
	top.AddController(scroll)
	root.Append(top)

	p.statusLabel = gtk.NewLabel("")
	p.statusLabel.SetXAlign(0)
	p.statusLabel.SetEllipsize(pango.EllipsizeEnd)
	top.Append(p.statusLabel)
	top.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	controls := gtk.NewBox(gtk.OrientationHorizontal, 6)
	p.muteBtn = iconButton("audio-volume-high-symbolic", "Mute")
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

type marginSetter interface {
	SetMarginTop(int)
	SetMarginBottom(int)
	SetMarginStart(int)
	SetMarginEnd(int)
}

func setMargin(widget marginSetter, margin int) {
	widget.SetMarginTop(margin)
	widget.SetMarginBottom(margin)
	widget.SetMarginStart(margin)
	widget.SetMarginEnd(margin)
}

func iconButton(iconName, tooltip string) *gtk.Button {
	button := gtk.NewButtonFromIconName(iconName)
	button.SetTooltipText(tooltip)
	return button
}

func (p *Player) updateVolume(vol int) {
	if vol < 0 {
		vol = 0
	} else if vol > 100 {
		vol = 100
	}
	if p.settings.Volume == vol {
		return
	}
	p.settings.Volume = vol
	p.volumeScale.SetValue(float64(vol))
	if !p.isMuted {
		p.setVolume(vol)
	}
	saveSettings(p.settings)
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
		setMargin(label, 8)
		row.SetChild(label)
		p.rowTracks[row] = track
		p.stationList.Append(row)
	}
}

func (p *Player) refreshUI() {
	if p.statusLabel == nil {
		return
	}
	p.statusLabel.SetText(p.currentStatus())
	if p.playingIdx >= 0 {
		p.playBtn.SetIconName("media-playback-stop-symbolic")
	} else {
		p.playBtn.SetIconName("media-playback-start-symbolic")
	}
	if p.isMuted {
		p.muteBtn.SetIconName("audio-volume-muted-symbolic")
	} else {
		p.muteBtn.SetIconName("audio-volume-high-symbolic")
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
