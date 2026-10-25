//go:build darwin

package main

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativePlayerLoadsMatroskaWithSystemFFmpeg(t *testing.T) {
	ffmpeg, err := requireMediaTool("ffmpeg", "en")
	if err != nil {
		t.Skip(err)
	}
	input := filepath.Join(t.TempDir(), "source.mkv")
	if output, runErr := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=duration=2:size=160x90:rate=12",
		"-f", "lavfi", "-i", "sine=duration=2",
		"-c:v", "mpeg4", "-c:a", "ac3", input,
	).CombinedOutput(); runErr != nil {
		t.Fatalf("create synthetic video: %v: %s", runErr, output)
	}
	events := make(chan PlayerEvent, 64)
	player, err := newNativePlayer(func(event PlayerEvent) {
		select {
		case events <- event:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := player.load(input, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for loaded, duration := false, 0.0; !loaded || duration < 1.5; {
		select {
		case event := <-events:
			switch event.Type {
			case "loaded":
				loaded = true
			case "duration":
				duration = event.Value
			case "error":
				t.Fatalf("mpv error: %s", event.Message)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for file load: loaded=%v duration=%v", loaded, duration)
		}
	}
	if err := player.seek(1); err != nil {
		t.Fatal(err)
	}
}
