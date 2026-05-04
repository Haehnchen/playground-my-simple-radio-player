package main

/*
#cgo pkg-config: libvlc
#include <ctype.h>
#include <stdio.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <vlc/vlc.h>

#define RADIO_NETWORK_CACHING_MS "1000"

static libvlc_instance_t* radio_new_vlc_instance(void) {
	const char *args[] = {
		"--no-video",
		"--network-caching=" RADIO_NETWORK_CACHING_MS,
		"--file-caching=" RADIO_NETWORK_CACHING_MS,
		"--live-caching=" RADIO_NETWORK_CACHING_MS,
		"--http-reconnect",
		"--ipv4-timeout=5000",
		"--no-metadata-network-access",
		"--no-xlib"
	};
	return libvlc_new(sizeof(args) / sizeof(args[0]), args);
}

static void radio_media_add_playback_options(libvlc_media_t *media) {
	const char *options[] = {
		":network-caching=" RADIO_NETWORK_CACHING_MS,
		":http-reconnect"
	};

	if (media == NULL) {
		return;
	}

	for (size_t i = 0; i < sizeof(options) / sizeof(options[0]); i++) {
		libvlc_media_add_option(media, options[i]);
	}
}

static char* radio_media_meta(libvlc_media_t *media, libvlc_meta_t meta) {
	if (media == NULL) {
		return NULL;
	}
	return libvlc_media_get_meta(media, meta);
}

static void radio_append_info_part(char *target, size_t target_size, const char *part) {
	size_t current_len;
	size_t remaining;

	if (part == NULL || part[0] == '\0') {
		return;
	}

	current_len = strlen(target);
	if (current_len >= target_size - 1) {
		return;
	}

	if (current_len > 0) {
		remaining = target_size - current_len - 1;
		strncat(target, ", ", remaining);
		current_len = strlen(target);
	}

	remaining = target_size - current_len - 1;
	strncat(target, part, remaining);
}

static void radio_codec_name(uint32_t codec, char *target, size_t target_size) {
	const char *description = libvlc_media_get_codec_description(libvlc_track_audio, codec);
	char fourcc[5] = {
		(char)(codec & 0xff),
		(char)((codec >> 8) & 0xff),
		(char)((codec >> 16) & 0xff),
		(char)((codec >> 24) & 0xff),
		'\0'
	};
	size_t len = 4;

	if (description != NULL && description[0] != '\0') {
		snprintf(target, target_size, "%s", description);
		return;
	}

	for (int i = 0; i < 4; i++) {
		if (fourcc[i] == '\0' || !isprint((unsigned char)fourcc[i])) {
			fourcc[i] = ' ';
		}
	}
	while (len > 0 && fourcc[len - 1] == ' ') {
		fourcc[len - 1] = '\0';
		len--;
	}

	if (fourcc[0] != '\0') {
		snprintf(target, target_size, "%s", fourcc);
	} else {
		snprintf(target, target_size, "Audio");
	}
}

static char* radio_stream_info(libvlc_media_t *media) {
	libvlc_media_track_t **tracks = NULL;
	unsigned int count;
	char info[256] = "";

	if (media == NULL) {
		return NULL;
	}

	count = libvlc_media_tracks_get(media, &tracks);
	if (tracks != NULL) {
		for (unsigned int i = 0; i < count; i++) {
			libvlc_media_track_t *track = tracks[i];
			char part[64];

			if (track == NULL || track->i_type != libvlc_track_audio) {
				continue;
			}

			radio_codec_name(track->i_codec, part, sizeof(part));
			radio_append_info_part(info, sizeof(info), part);

			if (track->i_bitrate > 0) {
				snprintf(part, sizeof(part), "%u kbps", (track->i_bitrate + 500) / 1000);
				radio_append_info_part(info, sizeof(info), part);
			}

			if (track->audio != NULL) {
				if (track->audio->i_rate > 0) {
					snprintf(part, sizeof(part), "%.1f kHz", track->audio->i_rate / 1000.0);
					radio_append_info_part(info, sizeof(info), part);
				}
				if (track->audio->i_channels == 1) {
					radio_append_info_part(info, sizeof(info), "mono");
				} else if (track->audio->i_channels == 2) {
					radio_append_info_part(info, sizeof(info), "stereo");
				} else if (track->audio->i_channels > 2) {
					snprintf(part, sizeof(part), "%u ch", track->audio->i_channels);
					radio_append_info_part(info, sizeof(info), part);
				}
			}
			break;
		}
	}

	if (tracks != NULL) {
		libvlc_media_tracks_release(tracks, count);
	}

	if (info[0] == '\0') {
		char *description = libvlc_media_get_meta(media, libvlc_meta_Description);
		if (description != NULL && description[0] != '\0') {
			char *copy = strdup(description);
			libvlc_free(description);
			return copy;
		}
		if (description != NULL) {
			libvlc_free(description);
		}
		return NULL;
	}

	return strdup(info);
}

static void radio_free_string(char *value) {
	free(value);
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

	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
)

var vlcInstance unsafe.Pointer

func initAudioBackend() bool {
	instance := C.radio_new_vlc_instance()
	if instance == nil {
		return false
	}
	vlcInstance = unsafe.Pointer(instance)
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

	p.releaseCurrentMedia()

	curl := C.CString(track.URL)
	defer C.free(unsafe.Pointer(curl))

	media := C.libvlc_media_new_location((*C.libvlc_instance_t)(vlcInstance), curl)
	if media == nil {
		p.statusMsg = "Error loading " + track.Name
		p.playingIdx = -1
		return
	}

	C.radio_media_add_playback_options(media)

	player := C.libvlc_media_player_new_from_media(media)
	if player == nil {
		C.libvlc_media_release(media)
		p.statusMsg = "Error creating player"
		p.playingIdx = -1
		return
	}
	p.media = unsafe.Pointer(media)
	p.mediaPlayer = unsafe.Pointer(player)
	p.setVolume(p.settings.Volume)
	p.setMuted(p.isMuted)
	if C.libvlc_media_player_play(player) != 0 {
		p.statusMsg = "Error playing " + track.Name
		p.playingIdx = -1
		p.releaseCurrentMedia()
		return
	}
	p.statusMsg = ""
	p.streamInfo = ""
	p.streamTitle = ""
	p.streamVersion = 0
	p.settings.LastTrackURL = track.URL
	saveSettings(p.settings)
	p.refreshUI()
	p.startStreamInfoPolling()
}

func (p *Player) releaseCurrentMedia() {
	p.stopStreamInfoPolling()
	if p.mediaPlayer != nil {
		player := (*C.libvlc_media_player_t)(p.mediaPlayer)
		C.libvlc_media_player_stop(player)
		C.libvlc_media_player_release(player)
		p.mediaPlayer = nil
	}
	if p.media != nil {
		C.libvlc_media_release((*C.libvlc_media_t)(p.media))
		p.media = nil
	}
}

func (p *Player) stopPlayback() {
	p.releaseCurrentMedia()
	p.playingIdx = -1
	p.streamInfo = ""
	p.streamTitle = ""
	p.streamVersion = 0
	p.settings.LastTrackURL = ""
	saveSettings(p.settings)
	p.refreshUI()
}

func (p *Player) setVolume(vol int) {
	if p.mediaPlayer != nil {
		C.libvlc_audio_set_volume((*C.libvlc_media_player_t)(p.mediaPlayer), C.int(vol))
	}
}

func (p *Player) toggleMute() {
	if p.isMuted {
		p.isMuted = false
		if p.settings.Volume == 0 {
			p.updateVolume(p.savedVolume)
		} else {
			p.setVolumeScaleValue(p.settings.Volume)
			p.setVolume(p.settings.Volume)
		}
	} else {
		p.isMuted = true
		if p.settings.Volume > 0 {
			p.savedVolume = p.settings.Volume
		}
	}
	p.setMuted(p.isMuted)
	p.refreshUI()
}

func (p *Player) setMuted(muted bool) {
	if p.mediaPlayer != nil {
		C.libvlc_audio_set_mute((*C.libvlc_media_player_t)(p.mediaPlayer), C.int(boolToInt(muted)))
	}
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
		return p.playlist[p.playingIdx].Name
	}
	if len(p.playlist) == 0 {
		return "No playlist loaded"
	}
	return "Stopped"
}

func (p *Player) currentStatusMarkup() string {
	if p.statusMsg != "" || p.playingIdx < 0 || p.playingIdx >= len(p.playlist) {
		return glib.MarkupEscapeText(p.currentStatus())
	}
	markup := glib.MarkupEscapeText(p.playlist[p.playingIdx].Name)
	if p.streamTitle != "" {
		markup += ` <span size="smaller" foreground="#6f747a"> ` + glib.MarkupEscapeText(p.streamTitle) + `</span>`
	}
	return markup
}

func (p *Player) currentStatusTooltip() string {
	if p.playingIdx < 0 || p.streamInfo == "" {
		return ""
	}
	return p.streamInfo
}

func (p *Player) streamTitleMatchesStation(title string) bool {
	if p.playingIdx < 0 || p.playingIdx >= len(p.playlist) {
		return false
	}
	return normalizeMetadataText(title) == normalizeMetadataText(p.playlist[p.playingIdx].Name)
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

func getSettingsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "radioplayer", "settings.json"), nil
}

func loadSettings() Settings {
	path, err := getSettingsPath()
	if err != nil {
		return Settings{Volume: 75}
	}
	data, err := os.ReadFile(path)
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
	path, err := getSettingsPath()
	if err != nil {
		return
	}
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
	if p.settingsDirty {
		saveSettings(p.settings)
		p.settingsDirty = false
	}
	p.releaseCurrentMedia()
	if vlcInstance != nil {
		C.libvlc_release((*C.libvlc_instance_t)(vlcInstance))
		vlcInstance = nil
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (p *Player) startStreamInfoPolling() {
	p.stopStreamInfoPolling()
	p.infoPoll = glib.TimeoutAdd(1000, func() bool {
		if p.playingIdx < 0 || p.media == nil {
			p.infoPoll = 0
			return false
		}
		changed := false
		if info := p.readStreamInfo(); info != "" && info != p.streamInfo {
			p.streamInfo = info
			changed = true
		}
		if title := p.readStreamTitle(); title != p.streamTitle {
			p.streamTitle = title
			changed = true
		}
		if changed {
			p.refreshUI()
		}
		return true
	})
}

func (p *Player) stopStreamInfoPolling() {
	if p.infoPoll != 0 {
		glib.SourceRemove(p.infoPoll)
		p.infoPoll = 0
	}
}

func (p *Player) readStreamInfo() string {
	if p.media == nil {
		return ""
	}
	value := C.radio_stream_info((*C.libvlc_media_t)(p.media))
	if value == nil {
		return ""
	}
	defer C.radio_free_string(value)
	return C.GoString(value)
}

func (p *Player) readStreamTitle() string {
	title := p.readVLCMeta(C.libvlc_meta_NowPlaying)
	if title == "" {
		title = p.readVLCMeta(C.libvlc_meta_Title)
	}
	if title == "" {
		return ""
	}
	cleaned := cleanStreamTitle(title)
	if p.streamTitleMatchesStation(cleaned) {
		return ""
	}
	return cleaned
}

func (p *Player) readVLCMeta(meta C.libvlc_meta_t) string {
	if p.media == nil {
		return ""
	}
	value := C.radio_media_meta((*C.libvlc_media_t)(p.media), meta)
	if value == nil {
		return ""
	}
	defer C.libvlc_free(unsafe.Pointer(value))
	return C.GoString(value)
}

func cleanStreamTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.ReplaceAll(title, "|", " - ")
	return strings.Join(strings.Fields(title), " ")
}

func normalizeMetadataText(value string) string {
	value = cleanStreamTitle(value)
	value = strings.ToLower(value)
	replacer := strings.NewReplacer("-", "", "_", "", ".", "", " ", "")
	return replacer.Replace(value)
}
