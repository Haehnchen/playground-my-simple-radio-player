package main

/*
#cgo pkg-config: gstreamer-1.0
#include <stdlib.h>
#include <gst/gst.h>

static void radio_on_pad_added(GstElement *src, GstPad *pad, gpointer data) {
	GstElement *convert = GST_ELEMENT(data);
	GstPad *sink = gst_element_get_static_pad(convert, "sink");
	if (sink == NULL || gst_pad_is_linked(sink)) {
		if (sink != NULL) {
			gst_object_unref(sink);
		}
		return;
	}

	GstCaps *caps = gst_pad_get_current_caps(pad);
	if (caps == NULL) {
		caps = gst_pad_query_caps(pad, NULL);
	}
	if (caps != NULL) {
		GstStructure *structure = gst_caps_get_structure(caps, 0);
		const char *name = gst_structure_get_name(structure);
		if (name != NULL && g_str_has_prefix(name, "audio/")) {
			gst_pad_link(pad, sink);
		}
		gst_caps_unref(caps);
	}
	gst_object_unref(sink);
}

static GstElement* radio_new_pipeline(const char *uri) {
	GstElement *pipeline = gst_pipeline_new("radio-player");
	GstElement *source = gst_element_factory_make("uridecodebin", "source");
	GstElement *convert = gst_element_factory_make("audioconvert", "convert");
	GstElement *resample = gst_element_factory_make("audioresample", "resample");
	GstElement *volume = gst_element_factory_make("volume", "radio-volume");
	GstElement *sink = gst_element_factory_make("autoaudiosink", "sink");

	if (pipeline == NULL || source == NULL || convert == NULL || resample == NULL || volume == NULL || sink == NULL) {
		if (pipeline != NULL) {
			gst_object_unref(pipeline);
		}
		return NULL;
	}

	g_object_set(G_OBJECT(source), "uri", uri, NULL);
	gst_bin_add_many(GST_BIN(pipeline), source, convert, resample, volume, sink, NULL);
	if (!gst_element_link_many(convert, resample, volume, sink, NULL)) {
		gst_object_unref(pipeline);
		return NULL;
	}
	g_signal_connect(source, "pad-added", G_CALLBACK(radio_on_pad_added), convert);
	return pipeline;
}

static void radio_set_volume(GstElement *player, double volume) {
	GstElement *volume_element = gst_bin_get_by_name(GST_BIN(player), "radio-volume");
	if (volume_element != NULL) {
		g_object_set(G_OBJECT(volume_element), "volume", volume, NULL);
		gst_object_unref(volume_element);
	}
}

static void radio_set_mute(GstElement *player, gboolean muted) {
	GstElement *volume_element = gst_bin_get_by_name(GST_BIN(player), "radio-volume");
	if (volume_element != NULL) {
		g_object_set(G_OBJECT(volume_element), "mute", muted, NULL);
		gst_object_unref(volume_element);
	}
}

static int radio_play(GstElement *player) {
	return gst_element_set_state(player, GST_STATE_PLAYING) != GST_STATE_CHANGE_FAILURE;
}

static void radio_stop(GstElement *player) {
	gst_element_set_state(player, GST_STATE_NULL);
}

static void radio_unref(GstElement *player) {
	gst_object_unref(player);
}
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

func initAudioBackend() bool {
	C.gst_init(nil, nil)
	return true
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

	if p.gstPlayer != nil {
		player := (*C.GstElement)(p.gstPlayer)
		C.radio_stop(player)
		C.radio_unref(player)
		p.gstPlayer = nil
	}

	curl := C.CString(track.URL)
	defer C.free(unsafe.Pointer(curl))

	player := C.radio_new_pipeline(curl)
	if player == nil {
		p.statusMsg = "Error creating player"
		p.playingIdx = -1
		return
	}
	p.gstPlayer = unsafe.Pointer(player)
	p.setVolume(p.settings.Volume)
	C.radio_set_mute(player, gboolean(p.isMuted))
	if C.radio_play(player) == 0 {
		p.statusMsg = "Error playing " + track.Name
		p.playingIdx = -1
		return
	}
	p.statusMsg = ""
	p.settings.LastTrackURL = track.URL
	saveSettings(p.settings)
	p.refreshUI()
}

func (p *Player) stopPlayback() {
	if p.gstPlayer != nil {
		C.radio_stop((*C.GstElement)(p.gstPlayer))
	}
	p.playingIdx = -1
	p.settings.LastTrackURL = ""
	saveSettings(p.settings)
	p.refreshUI()
}

func (p *Player) setVolume(vol int) {
	if p.gstPlayer != nil {
		C.radio_set_volume((*C.GstElement)(p.gstPlayer), C.double(float64(vol)/100))
	}
}

func (p *Player) toggleMute() {
	if p.isMuted {
		p.isMuted = false
		if p.volumeScale != nil {
			p.volumeScale.SetValue(float64(p.savedVolume))
		}
	} else {
		p.isMuted = true
		p.savedVolume = p.settings.Volume
	}
	if p.gstPlayer != nil {
		C.radio_set_mute((*C.GstElement)(p.gstPlayer), gboolean(p.isMuted))
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
	if p.gstPlayer != nil {
		player := (*C.GstElement)(p.gstPlayer)
		C.radio_stop(player)
		C.radio_unref(player)
		p.gstPlayer = nil
	}
}

func gboolean(value bool) C.gboolean {
	if value {
		return C.gboolean(1)
	}
	return C.gboolean(0)
}
