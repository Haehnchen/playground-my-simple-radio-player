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
	"sync"
	"unsafe"

	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
)

type audioCommandType uint8

const (
	audioPlay audioCommandType = iota
	audioStop
	audioSetVolume
	audioSetMuted
	audioReadMetadata
	audioClose
)

type audioMetadata struct {
	info  string
	title string
}

type audioCommand struct {
	kind         audioCommandType
	url          string
	volume       int
	muted        bool
	playDone     func(string)
	metadataDone func(audioMetadata)
}

type audioBackend struct {
	instance *C.libvlc_instance_t
	player   *C.libvlc_media_player_t
	media    *C.libvlc_media_t

	mu     sync.Mutex
	queue  []audioCommand
	wake   chan struct{}
	done   chan struct{}
	closed bool
}

func initAudioBackend() *audioBackend {
	instance := C.radio_new_vlc_instance()
	if instance == nil {
		return nil
	}
	backend := &audioBackend{
		instance: instance,
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	go backend.run()
	return backend
}

func (a *audioBackend) enqueue(command audioCommand) bool {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return false
	}
	if command.kind == audioClose {
		a.closed = true
	}
	if command.kind == audioPlay || command.kind == audioStop || command.kind == audioClose {
		for i := range a.queue {
			a.queue[i] = audioCommand{}
		}
		a.queue = append(a.queue[:0], command)
		a.mu.Unlock()
		select {
		case a.wake <- struct{}{}:
		default:
		}
		return true
	}
	if command.kind == audioSetVolume || command.kind == audioSetMuted {
		for i := len(a.queue) - 1; i >= 0; i-- {
			queued := a.queue[i]
			if queued.kind == command.kind {
				a.queue[i] = command
				a.mu.Unlock()
				return true
			}
		}
	}
	a.queue = append(a.queue, command)
	a.mu.Unlock()

	select {
	case a.wake <- struct{}{}:
	default:
	}
	return true
}

func (a *audioBackend) nextCommand() audioCommand {
	for {
		a.mu.Lock()
		if len(a.queue) > 0 {
			command := a.queue[0]
			a.queue[0] = audioCommand{}
			a.queue = a.queue[1:]
			a.mu.Unlock()
			return command
		}
		a.mu.Unlock()
		<-a.wake
	}
}

func (a *audioBackend) run() {
	for {
		command := a.nextCommand()
		switch command.kind {
		case audioPlay:
			a.releaseMedia()
			errorMessage := a.startMedia(command.url, command.volume, command.muted)
			dispatchToUI(func() { command.playDone(errorMessage) })
		case audioStop:
			a.releaseMedia()
		case audioSetVolume:
			if a.player != nil {
				C.libvlc_audio_set_volume(a.player, C.int(command.volume))
			}
		case audioSetMuted:
			if a.player != nil {
				C.libvlc_audio_set_mute(a.player, C.int(boolToInt(command.muted)))
			}
		case audioReadMetadata:
			metadata := a.metadata()
			dispatchToUI(func() { command.metadataDone(metadata) })
		case audioClose:
			a.releaseMedia()
			C.libvlc_release(a.instance)
			close(a.done)
			return
		}
	}
}

func (a *audioBackend) startMedia(url string, volume int, muted bool) string {
	curl := C.CString(url)
	defer C.free(unsafe.Pointer(curl))

	media := C.libvlc_media_new_location(a.instance, curl)
	if media == nil {
		return "Error loading stream"
	}
	C.radio_media_add_playback_options(media)

	player := C.libvlc_media_player_new_from_media(media)
	if player == nil {
		C.libvlc_media_release(media)
		return "Error creating player"
	}
	C.libvlc_audio_set_volume(player, C.int(volume))
	C.libvlc_audio_set_mute(player, C.int(boolToInt(muted)))
	if C.libvlc_media_player_play(player) != 0 {
		C.libvlc_media_player_release(player)
		C.libvlc_media_release(media)
		return "Error starting stream"
	}

	a.media = media
	a.player = player
	return ""
}

func (a *audioBackend) releaseMedia() {
	if a.player != nil {
		C.libvlc_media_player_stop(a.player)
		C.libvlc_media_player_release(a.player)
		a.player = nil
	}
	if a.media != nil {
		C.libvlc_media_release(a.media)
		a.media = nil
	}
}

func (a *audioBackend) play(url string, volume int, muted bool, done func(string)) {
	a.enqueue(audioCommand{kind: audioPlay, url: url, volume: volume, muted: muted, playDone: done})
}

func (a *audioBackend) stop() {
	a.enqueue(audioCommand{kind: audioStop})
}

func (a *audioBackend) setVolume(volume int) {
	a.enqueue(audioCommand{kind: audioSetVolume, volume: volume})
}

func (a *audioBackend) setMuted(muted bool) {
	a.enqueue(audioCommand{kind: audioSetMuted, muted: muted})
}

func (a *audioBackend) readMetadata(done func(audioMetadata)) bool {
	return a.enqueue(audioCommand{kind: audioReadMetadata, metadataDone: done})
}

func (a *audioBackend) close() {
	if a == nil {
		return
	}
	if a.enqueue(audioCommand{kind: audioClose}) {
		<-a.done
		return
	}
	<-a.done
}

func dispatchToUI(callback func()) {
	if callback == nil {
		return
	}
	glib.IdleAdd(callback)
}

func (p *Player) playTrack(id int) {
	if id < 0 || id >= len(p.filteredList) {
		return
	}
	track := p.filteredList[id]

	playlistIndex := -1
	for i, t := range p.playlist {
		if t.URL == track.URL {
			playlistIndex = i
			break
		}
	}
	if playlistIndex < 0 {
		return
	}

	p.playVersion++
	version := p.playVersion
	p.stopStreamInfoPolling()
	p.infoPending = false
	p.playingIdx = playlistIndex
	p.statusMsg = "Loading " + track.Name + "..."
	p.streamInfo = ""
	p.streamTitle = ""
	p.refreshUI()
	p.audio.play(track.URL, p.settings.Volume, p.isMuted, func(errorMessage string) {
		if version != p.playVersion {
			return
		}
		if errorMessage != "" {
			p.statusMsg = errorMessage + ": " + track.Name
			p.playingIdx = -1
			p.refreshUI()
			return
		}
		p.statusMsg = ""
		p.settings.LastTrackURL = track.URL
		saveSettings(p.settings)
		p.refreshUI()
		p.startStreamInfoPolling()
	})
}

func (p *Player) stopPlayback() {
	p.playVersion++
	p.stopStreamInfoPolling()
	p.infoPending = false
	p.audio.stop()
	p.playingIdx = -1
	p.streamInfo = ""
	p.streamTitle = ""
	p.settings.LastTrackURL = ""
	saveSettings(p.settings)
	p.refreshUI()
}

func (p *Player) setVolume(vol int) {
	p.audio.setVolume(vol)
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
	p.audio.setMuted(muted)
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
	p.stopStreamInfoPolling()
	p.audio.close()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (p *Player) startStreamInfoPolling() {
	p.stopStreamInfoPolling()
	version := p.playVersion
	p.infoPoll = glib.TimeoutAdd(1000, func() bool {
		if version != p.playVersion || p.playingIdx < 0 {
			p.infoPoll = 0
			return false
		}
		if p.infoPending {
			return true
		}
		p.infoPending = true
		if !p.audio.readMetadata(func(metadata audioMetadata) {
			if version != p.playVersion {
				return
			}
			p.infoPending = false
			changed := false
			if metadata.info != "" && metadata.info != p.streamInfo {
				p.streamInfo = metadata.info
				changed = true
			}
			title := cleanStreamTitle(metadata.title)
			if p.streamTitleMatchesStation(title) {
				title = ""
			}
			if title != p.streamTitle {
				p.streamTitle = title
				changed = true
			}
			if changed {
				p.refreshUI()
			}
		}) {
			p.infoPending = false
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

func (a *audioBackend) metadata() audioMetadata {
	if a.media == nil {
		return audioMetadata{}
	}
	metadata := audioMetadata{info: readStreamInfo(a.media)}
	metadata.title = readMediaMeta(a.media, C.libvlc_meta_NowPlaying)
	if metadata.title == "" {
		metadata.title = readMediaMeta(a.media, C.libvlc_meta_Title)
	}
	return metadata
}

func readStreamInfo(media *C.libvlc_media_t) string {
	value := C.radio_stream_info(media)
	if value == nil {
		return ""
	}
	defer C.radio_free_string(value)
	return C.GoString(value)
}

func readMediaMeta(media *C.libvlc_media_t, meta C.libvlc_meta_t) string {
	value := C.radio_media_meta(media, meta)
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
