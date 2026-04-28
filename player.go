package main

/*
#cgo pkg-config: gstreamer-1.0
#include <stdlib.h>
#include <gst/gst.h>

typedef struct {
	gchar *codec;
	gchar *title;
	guint bitrate;
	guint nominal_bitrate;
	gint rate;
	gint channels;
	guint version;
} RadioStreamInfo;

static void radio_start_bus_watch(GstElement *player);
static void radio_stop_bus_watch(GstElement *player);
static void radio_update_player_info_from_caps(GstElement *player, GstCaps *caps);
static GstPadProbeReturn radio_on_audio_pad_event(GstPad *pad, GstPadProbeInfo *probe_info, gpointer data);

static void radio_stream_info_free(gpointer data) {
	RadioStreamInfo *info = data;
	if (info != NULL) {
		g_free(info->codec);
		g_free(info->title);
		g_free(info);
	}
}

static gboolean radio_has_property(GObject *object, const char *name) {
	return object != NULL && g_object_class_find_property(G_OBJECT_GET_CLASS(object), name) != NULL;
}

static void radio_set_bool_if_property(GObject *object, const char *name, gboolean value) {
	if (radio_has_property(object, name)) {
		g_object_set(object, name, value, NULL);
	}
}

static void radio_set_int_if_property(GObject *object, const char *name, gint value) {
	if (radio_has_property(object, name)) {
		g_object_set(object, name, value, NULL);
	}
}

static void radio_set_uint_if_property(GObject *object, const char *name, guint value) {
	if (radio_has_property(object, name)) {
		g_object_set(object, name, value, NULL);
	}
}

static void radio_set_int64_if_property(GObject *object, const char *name, gint64 value) {
	if (radio_has_property(object, name)) {
		g_object_set(object, name, value, NULL);
	}
}

static void radio_set_uint64_if_property(GObject *object, const char *name, guint64 value) {
	if (radio_has_property(object, name)) {
		g_object_set(object, name, value, NULL);
	}
}

static void radio_set_double_if_property(GObject *object, const char *name, gdouble value) {
	if (radio_has_property(object, name)) {
		g_object_set(object, name, value, NULL);
	}
}

static void radio_set_string_if_property(GObject *object, const char *name, const char *value) {
	if (radio_has_property(object, name)) {
		g_object_set(object, name, value, NULL);
	}
}

static void radio_on_source_setup(GstElement *bin, GstElement *source, gpointer data) {
	GObject *object = G_OBJECT(source);
	radio_set_bool_if_property(object, "automatic-redirect", TRUE);
	radio_set_bool_if_property(object, "iradio-mode", TRUE);
	radio_set_bool_if_property(object, "keep-alive", TRUE);
	radio_set_uint_if_property(object, "blocksize", 32768);
	radio_set_string_if_property(object, "user-agent", "Radio Player/1.0 GStreamer");
}

static void radio_on_pad_added(GstElement *src, GstPad *pad, gpointer data) {
	GstElement *queue = GST_ELEMENT(data);
	GstPad *sink = gst_element_get_static_pad(queue, "sink");
	GstElement *player = GST_ELEMENT(gst_element_get_parent(src));
	if (sink == NULL || gst_pad_is_linked(sink)) {
		if (sink != NULL) {
			gst_object_unref(sink);
		}
		if (player != NULL) {
			gst_object_unref(player);
		}
		return;
	}

	gboolean is_audio = FALSE;
	GstCaps *caps = gst_pad_get_current_caps(pad);
	if (caps == NULL) {
		caps = gst_pad_query_caps(pad, NULL);
	}
	if (caps != NULL && !gst_caps_is_empty(caps) && !gst_caps_is_any(caps)) {
		GstStructure *structure = gst_caps_get_structure(caps, 0);
		const char *name = gst_structure_get_name(structure);
		if (name != NULL && g_str_has_prefix(name, "audio/")) {
			is_audio = TRUE;
		}
	}
	if (caps != NULL) {
		gst_caps_unref(caps);
	}
	if (is_audio) {
		gst_pad_add_probe(pad, GST_PAD_PROBE_TYPE_EVENT_DOWNSTREAM, radio_on_audio_pad_event, player, NULL);
		if (gst_pad_link(pad, sink) == GST_PAD_LINK_OK) {
			caps = gst_pad_get_current_caps(pad);
			if (caps != NULL) {
				radio_update_player_info_from_caps(player, caps);
				gst_caps_unref(caps);
			}
		}
	}
	gst_object_unref(sink);
	if (player != NULL) {
		gst_object_unref(player);
	}
}

static GstElement* radio_new_pipeline(const char *uri) {
	GstElement *pipeline = gst_pipeline_new("radio-player");
	GstElement *source = gst_element_factory_make("uridecodebin", "source");
	GstElement *queue = gst_element_factory_make("queue", "audio-buffer");
	GstElement *convert = gst_element_factory_make("audioconvert", "convert");
	GstElement *resample = gst_element_factory_make("audioresample", "resample");
	GstElement *volume = gst_element_factory_make("volume", "radio-volume");
	GstElement *sink = gst_element_factory_make("autoaudiosink", "sink");

	if (pipeline == NULL || source == NULL || queue == NULL || convert == NULL || resample == NULL || volume == NULL || sink == NULL) {
		if (pipeline != NULL) {
			gst_object_unref(pipeline);
		}
		return NULL;
	}

	g_object_set(G_OBJECT(source), "uri", uri, NULL);

	g_object_set(G_OBJECT(queue),
		"max-size-buffers", 0,
		"max-size-bytes", 0,
		"max-size-time", (guint64)(250 * GST_MSECOND),
		"min-threshold-time", (guint64)0,
		"silent", TRUE,
		NULL);

	gst_bin_add_many(GST_BIN(pipeline), source, queue, convert, resample, volume, sink, NULL);
	if (!gst_element_link_many(queue, convert, resample, volume, sink, NULL)) {
		gst_object_unref(pipeline);
		return NULL;
	}
	g_object_set_data_full(G_OBJECT(pipeline), "radio-info", g_new0(RadioStreamInfo, 1), radio_stream_info_free);
	g_signal_connect(source, "source-setup", G_CALLBACK(radio_on_source_setup), NULL);
	g_signal_connect(source, "pad-added", G_CALLBACK(radio_on_pad_added), queue);
	radio_start_bus_watch(pipeline);
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
	radio_stop_bus_watch(player);
	gst_object_unref(player);
}

static gboolean radio_update_info_from_tags(RadioStreamInfo *info, GstTagList *tags) {
	gboolean changed = FALSE;
	gchar *codec = NULL;
	gchar *title = NULL;
	guint bitrate = 0;
	if (gst_tag_list_get_string(tags, GST_TAG_AUDIO_CODEC, &codec)) {
		if (g_strcmp0(info->codec, codec) != 0) {
			g_free(info->codec);
			info->codec = codec;
			changed = TRUE;
		} else {
			g_free(codec);
		}
	}
	if (gst_tag_list_get_string(tags, GST_TAG_TITLE, &title)) {
		if (g_strcmp0(info->title, title) != 0) {
			g_free(info->title);
			info->title = title;
			changed = TRUE;
		} else {
			g_free(title);
		}
	}
	if (gst_tag_list_get_uint(tags, GST_TAG_BITRATE, &bitrate) && info->bitrate != bitrate) {
		info->bitrate = bitrate;
		changed = TRUE;
	}
	if (gst_tag_list_get_uint(tags, GST_TAG_NOMINAL_BITRATE, &bitrate) && info->nominal_bitrate != bitrate) {
		info->nominal_bitrate = bitrate;
		changed = TRUE;
	}
	return changed;
}

static gboolean radio_update_info_from_caps(RadioStreamInfo *info, GstCaps *caps) {
	if (info == NULL || caps == NULL || gst_caps_is_empty(caps) || gst_caps_is_any(caps)) {
		return FALSE;
	}

	GstStructure *structure = gst_caps_get_structure(caps, 0);
	if (structure == NULL) {
		return FALSE;
	}

	gboolean changed = FALSE;
	gint rate = 0;
	gint channels = 0;
	if (gst_structure_get_int(structure, "rate", &rate) && info->rate != rate) {
		info->rate = rate;
		changed = TRUE;
	}
	if (gst_structure_get_int(structure, "channels", &channels) && info->channels != channels) {
		info->channels = channels;
		changed = TRUE;
	}
	return changed;
}

static void radio_update_player_info_from_caps(GstElement *player, GstCaps *caps) {
	if (player == NULL) {
		return;
	}
	RadioStreamInfo *info = g_object_get_data(G_OBJECT(player), "radio-info");
	if (radio_update_info_from_caps(info, caps)) {
		info->version++;
	}
}

static GstPadProbeReturn radio_on_audio_pad_event(GstPad *pad, GstPadProbeInfo *probe_info, gpointer data) {
	if ((GST_PAD_PROBE_INFO_TYPE(probe_info) & GST_PAD_PROBE_TYPE_EVENT_DOWNSTREAM) == 0) {
		return GST_PAD_PROBE_OK;
	}

	GstEvent *event = GST_PAD_PROBE_INFO_EVENT(probe_info);
	if (event != NULL && GST_EVENT_TYPE(event) == GST_EVENT_CAPS) {
		GstCaps *caps = NULL;
		gst_event_parse_caps(event, &caps);
		radio_update_player_info_from_caps((GstElement*)data, caps);
	}
	return GST_PAD_PROBE_OK;
}

static gboolean radio_on_bus_message(GstBus *bus, GstMessage *message, gpointer data) {
	GstElement *player = GST_ELEMENT(data);
	RadioStreamInfo *info = g_object_get_data(G_OBJECT(player), "radio-info");
	if (info == NULL) {
		return G_SOURCE_CONTINUE;
	}
	if (GST_MESSAGE_TYPE(message) == GST_MESSAGE_TAG) {
		GstTagList *tags = NULL;
		gst_message_parse_tag(message, &tags);
		if (tags != NULL) {
			if (radio_update_info_from_tags(info, tags)) {
				info->version++;
			}
			gst_tag_list_unref(tags);
		}
	}
	return G_SOURCE_CONTINUE;
}

static void radio_start_bus_watch(GstElement *player) {
	GstBus *bus = gst_element_get_bus(player);
	guint watch_id = gst_bus_add_watch(bus, radio_on_bus_message, player);
	g_object_set_data(G_OBJECT(player), "radio-bus-watch-id", GUINT_TO_POINTER(watch_id));
	gst_object_unref(bus);
}

static void radio_stop_bus_watch(GstElement *player) {
	guint watch_id = GPOINTER_TO_UINT(g_object_get_data(G_OBJECT(player), "radio-bus-watch-id"));
	if (watch_id != 0) {
		g_source_remove(watch_id);
		g_object_set_data(G_OBJECT(player), "radio-bus-watch-id", GUINT_TO_POINTER(0));
	}
}

static char* radio_stream_info(GstElement *player) {
	RadioStreamInfo *info = g_object_get_data(G_OBJECT(player), "radio-info");
	if (info == NULL) {
		return NULL;
	}

	GString *out = g_string_new(NULL);
	if (info->codec != NULL && info->codec[0] != '\0') {
		g_string_append(out, info->codec);
	}
	guint bitrate = info->bitrate > 0 ? info->bitrate : info->nominal_bitrate;
	if (bitrate > 0) {
		if (out->len > 0) {
			g_string_append(out, ", ");
		}
		g_string_append_printf(out, "%u kbps", bitrate / 1000);
	}
	if (info->rate > 0) {
		if (out->len > 0) {
			g_string_append(out, ", ");
		}
		if (info->rate % 1000 == 0) {
			g_string_append_printf(out, "%d kHz", info->rate / 1000);
		} else {
			g_string_append_printf(out, "%.1f kHz", info->rate / 1000.0);
		}
	}
	if (info->channels > 0) {
		if (out->len > 0) {
			g_string_append(out, ", ");
		}
		if (info->channels == 1) {
			g_string_append(out, "mono");
		} else if (info->channels == 2) {
			g_string_append(out, "stereo");
		} else {
			g_string_append_printf(out, "%d ch", info->channels);
		}
	}
	if (out->len == 0) {
		g_string_free(out, TRUE);
		return NULL;
	}
	return g_string_free(out, FALSE);
}

static char* radio_stream_title(GstElement *player) {
	RadioStreamInfo *info = g_object_get_data(G_OBJECT(player), "radio-info");
	if (info == NULL) {
		return NULL;
	}
	if (info->title != NULL && info->title[0] != '\0') {
		return g_strdup(info->title);
	}
	return NULL;
}

static guint radio_stream_version(GstElement *player) {
	RadioStreamInfo *info = g_object_get_data(G_OBJECT(player), "radio-info");
	if (info == NULL) {
		return 0;
	}
	return info->version;
}

static void radio_free_string(char *value) {
	g_free(value);
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
		p.stopStreamInfoPolling()
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
	p.streamInfo = ""
	p.streamTitle = ""
	p.streamVersion = 0
	p.settings.LastTrackURL = track.URL
	saveSettings(p.settings)
	p.refreshUI()
	p.startStreamInfoPolling()
}

func (p *Player) stopPlayback() {
	p.stopStreamInfoPolling()
	if p.gstPlayer != nil {
		C.radio_stop((*C.GstElement)(p.gstPlayer))
	}
	p.playingIdx = -1
	p.streamInfo = ""
	p.streamTitle = ""
	p.streamVersion = 0
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
	if p.gstPlayer != nil {
		C.radio_set_mute((*C.GstElement)(p.gstPlayer), gboolean(muted))
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
	p.stopStreamInfoPolling()
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

func (p *Player) startStreamInfoPolling() {
	p.stopStreamInfoPolling()
	p.infoPoll = glib.TimeoutAdd(1000, func() bool {
		if p.playingIdx < 0 || p.gstPlayer == nil {
			p.infoPoll = 0
			return false
		}
		version := uint(C.radio_stream_version((*C.GstElement)(p.gstPlayer)))
		if version == p.streamVersion {
			return true
		}
		p.streamVersion = version
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
	if p.gstPlayer == nil {
		return ""
	}
	info := C.radio_stream_info((*C.GstElement)(p.gstPlayer))
	if info == nil {
		return ""
	}
	defer C.radio_free_string(info)
	return C.GoString(info)
}

func (p *Player) readStreamTitle() string {
	if p.gstPlayer == nil {
		return ""
	}
	title := C.radio_stream_title((*C.GstElement)(p.gstPlayer))
	if title == nil {
		return ""
	}
	defer C.radio_free_string(title)
	cleaned := cleanStreamTitle(C.GoString(title))
	if p.streamTitleMatchesStation(cleaned) {
		return ""
	}
	return cleaned
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
