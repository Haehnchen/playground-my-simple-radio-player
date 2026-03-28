package main

import (
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"os/exec"
	"time"

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
)

// approxControlsTop is the approximate dp offset from the window top to the
// bottom of the controls bar (status ~42 + divider 1 + controls 64).
const approxControlsTop = 107

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

// vectorIconBtn draws a rounded-rectangle icon button with a vector IconVG icon.
func vectorIconBtn(gtx layout.Context, btn *widget.Clickable,
	ico *widget.Icon, bg, fg color.NRGBA, size unit.Dp, iconSize unit.Dp) layout.Dimensions {
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		s := gtx.Dp(size)
		gtx.Constraints = layout.Exact(image.Point{X: s, Y: s})
		r := gtx.Dp(unit.Dp(9))
		defer clip.RRect{
			Rect: image.Rectangle{Max: image.Point{X: s, Y: s}},
			NW: r, NE: r, SW: r, SE: r,
		}.Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, bg)
		return layout.Stack{Alignment: layout.Center}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				isz := gtx.Dp(iconSize)
				gtx.Constraints.Min = image.Point{X: isz, Y: isz}
				gtx.Constraints.Max = gtx.Constraints.Min
				ico.Layout(gtx, fg)
				return layout.Dimensions{Size: gtx.Constraints.Min}
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
						singleLine := gtx
						singleLine.Constraints.Max.Y = gtx.Sp(unit.Sp(20))
						return lbl.Layout(singleLine)
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
					ico := p.icoVolumeUp
					if p.isMuted {
						ico = p.icoVolumeOff
					}
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return vectorIconBtn(gtx, &p.muteBtn, ico, clrBtnBg, clrLabel, 36, 20)
					})
				}),
				// Volume slider
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx,
						material.Slider(th, &p.volSlider).Layout)
				}),
				// Play / Stop
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					ico := p.icoPlay
					if p.playingIdx >= 0 {
						ico = p.icoStop
					}
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return vectorIconBtn(gtx, &p.playBtn, ico, clrAccent, clrWhite, 36, 20)
					})
				}),
				// Shuffle
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return vectorIconBtn(gtx, &p.randomBtn, p.icoShuffle, clrBtnBg, clrLabel, 36, 20)
					})
				}),
				// Open file
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return vectorIconBtn(gtx, &p.openBtn, p.icoFolder, clrBtnBg, clrLabel, 36, 20)
					})
				}),
				// Settings / install cogwheel
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					bg := clrBtnBg
					fg := clrLabel
					if p.showInstallMenu {
						bg = clrAccent
						fg = clrWhite
					}
					return vectorIconBtn(gtx, &p.installBtn, p.icoSettings, bg, fg, 36, 20)
				}),
			)
		})
	}
}

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
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						isz := image.Point{X: gtx.Dp(unit.Dp(18)), Y: gtx.Dp(unit.Dp(18))}
						gtx.Constraints.Min = isz
						gtx.Constraints.Max = isz
						p.icoSettings.Layout(gtx, clrAccent)
						return layout.Dimensions{Size: isz}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
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
									lbl := material.Body1(th, track.Name)
									lbl.TextSize = unit.Sp(15)
									if isPlaying {
										lbl.Color = clrAccent
										lbl.Font.Weight = font.SemiBold
									} else {
										lbl.Color = clrLabel
									}
									singleLine := gtx
									singleLine.Constraints.Max.Y = gtx.Sp(unit.Sp(22))
									return lbl.Layout(singleLine)
								})
							})
						})
					}),
					layout.Rigid(thinDivider),
				)
			})
	}
}
