package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaServerRejectsUnauthorisedFile(t *testing.T) {
	server := newMediaServer()
	req := httptest.NewRequest(http.MethodGet, "/media?path=/tmp/not-allowed.mp4", nil)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, res.Code)
	}
}

func TestMissingFFmpegHasInstallInstructions(t *testing.T) {
	err := missingMediaToolError("ffmpeg", "tr")
	message := err.Error()
	for _, expected := range []string{"FFmpeg bulunamadı", "brew install ffmpeg", "winget install Gyan.FFmpeg"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("error %q does not contain %q", message, expected)
		}
	}
}

func TestMissingFFprobeExplainsIncompleteInstall(t *testing.T) {
	err := missingMediaToolError("ffprobe", "tr")
	if !strings.Contains(err.Error(), "ffprobe bulunamadı") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFFmpegFilterDetection(t *testing.T) {
	if !ffmpegFilterListed(" .. drawtext          V->V       Draw text on top of video frames.", "drawtext") {
		t.Fatal("expected drawtext filter to be detected")
	}
	if ffmpegFilterListed(" .. tile              V->V       Tile frames.", "drawtext") {
		t.Fatal("unexpected drawtext filter detection")
	}
}

func TestExtractContactSheetWithSystemFFmpeg(t *testing.T) {
	ffmpeg, err := requireMediaTool("ffmpeg", "en")
	if err != nil {
		t.Skip("FFmpeg is not installed")
	}
	if _, err := requireMediaTool("ffprobe", "en"); err != nil {
		t.Skip("ffprobe is not installed")
	}
	input := filepath.Join(t.TempDir(), "contact-sheet-source.mp4")
	if output, err := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=duration=3:size=320x180:rate=12",
		"-c:v", "mpeg4", input,
	).CombinedOutput(); err != nil {
		t.Fatalf("test video creation failed: %v: %s", err, output)
	}
	output, err := extractContactSheet(input, 4, 160, 2, "en")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		t.Fatal("contact sheet is not a JPEG image")
	}
}

func TestBackendLocalesHaveMatchingKeys(t *testing.T) {
	for key := range backendMessages["en"] {
		if backendMessages["tr"][key] == "" {
			t.Errorf("Turkish catalog is missing %q", key)
		}
	}
	for key := range backendMessages["tr"] {
		if backendMessages["en"][key] == "" {
			t.Errorf("English catalog is missing %q", key)
		}
	}
}

func TestUnsupportedLocaleFallsBackToEnglish(t *testing.T) {
	if got := translate("de", "error.invalidSplitPoint"); got != backendMessages["en"]["error.invalidSplitPoint"] {
		t.Fatalf("unexpected fallback: %q", got)
	}
}

func TestAppLocaleSelection(t *testing.T) {
	app := NewApp(newMediaServer())
	if got := app.SetLocale("tr"); got != "tr" || app.tr("dialog.openVideo") != backendMessages["tr"]["dialog.openVideo"] {
		t.Fatalf("Turkish locale was not applied: %q", got)
	}
	if got := app.SetLocale("unsupported"); got != "en" || app.tr("dialog.openVideo") != backendMessages["en"]["dialog.openVideo"] {
		t.Fatalf("unsupported locale did not fall back to English: %q", got)
	}
}

func TestMediaServerSupportsRangeRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newMediaServer()
	server.allow(path)
	req := httptest.NewRequest(http.MethodGet, "/media?path="+path, nil)
	req.Header.Set("Range", "bytes=2-5")
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusPartialContent || res.Body.String() != "2345" {
		t.Fatalf("unexpected range response: status=%d body=%q", res.Code, res.Body.String())
	}
}

func TestMediaKind(t *testing.T) {
	if got := mediaKind("track.FLAC"); got != "audio" {
		t.Fatalf("expected audio, got %q", got)
	}
	if got := mediaKind("movie.mp4"); got != "video" {
		t.Fatalf("expected video, got %q", got)
	}
}

func TestDeleteVideoRequiresAuthorisation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(newMediaServer())
	if err := app.DeleteVideo(path); err == nil {
		t.Fatal("expected unauthorised deletion to fail")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("unauthorised file was removed")
	}
}

func TestDeleteVideoRemovesAuthorisedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delete-me.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newMediaServer()
	server.allow(path)
	app := NewApp(server)
	if err := app.DeleteVideo(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("authorised file still exists")
	}
	if server.isAllowed(path) {
		t.Fatal("deleted file remained authorised")
	}
}

func TestRecentStorePersistsNewestFirstAndDeduplicates(t *testing.T) {
	store := &recentStore{path: filepath.Join(t.TempDir(), "recent.json")}
	first := filepath.Join(t.TempDir(), "first.mp4")
	second := filepath.Join(t.TempDir(), "second.mp4")
	if err := store.add(first); err != nil {
		t.Fatal(err)
	}
	if err := store.add(second); err != nil {
		t.Fatal(err)
	}
	if err := store.add(first); err != nil {
		t.Fatal(err)
	}
	paths, err := store.list()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != first || paths[1] != second {
		t.Fatalf("unexpected recent order: %#v", paths)
	}
}
