package main

/*
#cgo pkg-config: libvlc
#include <stdlib.h>
#include <vlc/vlc.h>
*/
import "C"

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unsafe"
)

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

	p.setVolume(p.settings.Volume)
	C.libvlc_media_player_play(p.mediaPlayer)
	p.statusMsg = ""
	p.settings.LastTrackURL = track.URL
	saveSettings(p.settings)
	p.refreshUI()
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
	p.refreshUI()
}

func (p *Player) setVolume(vol int) {
	if p.mediaPlayer != nil {
		C.libvlc_audio_set_volume(p.mediaPlayer, C.int(vol))
	}
}

func (p *Player) toggleMute() {
	if p.isMuted {
		p.isMuted = false
		p.setVolume(p.savedVolume)
		if p.volumeScale != nil {
			p.volumeScale.SetValue(float64(p.savedVolume))
		}
	} else {
		p.isMuted = true
		p.savedVolume = p.settings.Volume
		p.setVolume(0)
	}
	p.refreshUI()
}

func (p *Player) isPlayingTrack(track Track) bool {
	if p.playingIdx < 0 || p.playingIdx >= len(p.playlist) {
		return false
	}
	return p.playlist[p.playingIdx].URL == track.URL
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

// --- Settings ---

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

// --- Playlist parsing ---

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
	p.rebuildStationList()
	p.refreshUI()
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

// --- Desktop identity ---

func writeUserDesktopIdentity() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	desktopDir := filepath.Join(home, ".local", "share", "applications")
	iconDir := filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps")

	os.MkdirAll(desktopDir, 0755)
	os.MkdirAll(iconDir, 0755)

	iconData, err := iconFS.ReadFile("icon.png")
	if err != nil {
		return false
	}
	iconPath := filepath.Join(iconDir, appID+".png")
	if err := os.WriteFile(iconPath, iconData, 0644); err != nil {
		return false
	}

	exe, err := os.Executable()
	if err != nil {
		exe = "radioplayer"
	}
	os.Chmod(exe, 0755)

	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Comment=Simple Radio Player
Exec=%s %%u
Icon=%s
Terminal=false
Categories=AudioVideo;Audio;
StartupNotify=true
StartupWMClass=%s
`, appName, strconv.Quote(exe), iconPath, appID)

	if err := os.WriteFile(filepath.Join(desktopDir, appID+".desktop"), []byte(desktop), 0644); err != nil {
		return false
	}
	return true
}

func (p *Player) cleanup() {
	if p.mediaPlayer != nil {
		C.libvlc_media_player_release(p.mediaPlayer)
		p.mediaPlayer = nil
	}
	if p.media != nil {
		C.libvlc_media_release(p.media)
		p.media = nil
	}
	if p.instance != nil {
		C.libvlc_release(p.instance)
		p.instance = nil
	}
}
