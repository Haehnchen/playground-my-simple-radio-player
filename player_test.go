package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseM3U8(t *testing.T) {
	path := writeTestFile(t, "stations.m3u8", `#EXTM3U
#EXTINF:-1,Station One
https://example.com/one

# A comment
https://example.com/two.mp3
`)

	tracks, err := parseM3U8(path)
	if err != nil {
		t.Fatalf("parseM3U8() error = %v", err)
	}
	want := []Track{
		{Name: "Station One", URL: "https://example.com/one"},
		{Name: "two", URL: "https://example.com/two.mp3"},
	}
	if !reflect.DeepEqual(tracks, want) {
		t.Fatalf("parseM3U8() = %#v, want %#v", tracks, want)
	}
}

func TestParseXSPF(t *testing.T) {
	path := writeTestFile(t, "stations.xspf", `<?xml version="1.0" encoding="UTF-8"?>
<playlist xmlns="http://xspf.org/ns/0/" version="1">
  <trackList>
    <track><title>Station One</title><location>https://example.com/one</location></track>
    <track><location>https://example.com/two</location></track>
  </trackList>
</playlist>`)

	tracks, err := parseXSPF(path)
	if err != nil {
		t.Fatalf("parseXSPF() error = %v", err)
	}
	want := []Track{
		{Name: "Station One", URL: "https://example.com/one"},
		{Name: "two", URL: "https://example.com/two"},
	}
	if !reflect.DeepEqual(tracks, want) {
		t.Fatalf("parseXSPF() = %#v, want %#v", tracks, want)
	}
}

func TestCleanStreamTitle(t *testing.T) {
	if got, want := cleanStreamTitle("  Artist  |  Track  "), "Artist - Track"; got != want {
		t.Fatalf("cleanStreamTitle() = %q, want %q", got, want)
	}
}

func TestNormalizeMetadataText(t *testing.T) {
	left := normalizeMetadataText("Radio_One.FM")
	right := normalizeMetadataText("radio one fm")
	if left != right {
		t.Fatalf("normalized values differ: %q != %q", left, right)
	}
}

func TestAudioQueueKeepsLatestIntent(t *testing.T) {
	backend := &audioBackend{wake: make(chan struct{}, 1), done: make(chan struct{})}
	backend.enqueue(audioCommand{kind: audioPlay, url: "https://example.com/one"})
	backend.enqueue(audioCommand{kind: audioSetVolume, volume: 25})
	backend.enqueue(audioCommand{kind: audioSetVolume, volume: 50})

	if got := len(backend.queue); got != 2 {
		t.Fatalf("queue length = %d, want 2", got)
	}
	if got := backend.queue[1].volume; got != 50 {
		t.Fatalf("queued volume = %d, want 50", got)
	}

	backend.enqueue(audioCommand{kind: audioPlay, url: "https://example.com/two"})
	if got := len(backend.queue); got != 1 {
		t.Fatalf("queue length after newer play = %d, want 1", got)
	}
	if got := backend.queue[0].url; got != "https://example.com/two" {
		t.Fatalf("queued URL = %q, want latest URL", got)
	}

	backend.enqueue(audioCommand{kind: audioStop})
	if got := len(backend.queue); got != 1 || backend.queue[0].kind != audioStop {
		t.Fatalf("stop did not supersede pending commands: %#v", backend.queue)
	}
}

func writeTestFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path
}
