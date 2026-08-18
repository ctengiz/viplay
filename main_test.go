package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitForThumbnailStatus(t *testing.T, manager *thumbnailManager, predicate func(ThumbnailProgress) bool) ThumbnailProgress {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := manager.progress()
		if predicate(status) {
			return status
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for thumbnail status")
	return ThumbnailProgress{}
}

func TestThumbnailManagerPauseResumeAndStop(t *testing.T) {
	manager := newThumbnailManager()
	started := make(chan string, 2)
	release := make(chan struct{}, 2)
	done := make(chan struct{})
	go func() {
		manager.run([]string{"first.mp4", "second.mp4"}, func(path string) error {
			started <- path
			<-release
			return nil
		})
		close(done)
	}()
	if path := <-started; path != "first.mp4" {
		t.Fatalf("unexpected first path: %q", path)
	}
	manager.pause()
	release <- struct{}{}
	waitForThumbnailStatus(t, manager, func(status ThumbnailProgress) bool { return status.Paused && status.Completed == 1 })
	select {
	case path := <-started:
		t.Fatalf("second thumbnail started while paused: %q", path)
	case <-time.After(30 * time.Millisecond):
	}
	manager.resume()
	if path := <-started; path != "second.mp4" {
		t.Fatalf("unexpected second path: %q", path)
	}
	stopDone := make(chan struct{})
	go func() {
		manager.stopAndWait()
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("stop returned before the active thumbnail finished")
	case <-time.After(30 * time.Millisecond):
	}
	release <- struct{}{}
	<-stopDone
	<-done
	if status := manager.progress(); status.Active || status.Paused {
		t.Fatalf("unexpected stopped status: %#v", status)
	}
}

func TestClearThumbnailCacheRemovesCacheDirectory(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "thumbnails")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "cached.jpg"), []byte("thumbnail"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := clearThumbnailCacheAt(cacheDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatalf("cache directory still exists: %v", err)
	}
}

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

func TestVideoEncoderDetection(t *testing.T) {
	encoders := parseVideoEncoders(" V....D libx265 H.265 encoder\n V..... libsvtav1 AV1 encoder\n A..... aac AAC encoder")
	if !encoders["libx265"] || !encoders["libsvtav1"] || encoders["aac"] {
		t.Fatalf("unexpected encoders: %#v", encoders)
	}
}

func TestTranscodeWithSystemFFmpeg(t *testing.T) {
	ffmpeg, err := requireMediaTool("ffmpeg", "en")
	if err != nil {
		t.Skip(err)
	}
	if _, err := requireMediaTool("ffprobe", "en"); err != nil {
		t.Skip(err)
	}
	if !availableVideoEncoders(ffmpeg)["libx265"] {
		t.Skip("libx265 is not available")
	}
	input := filepath.Join(t.TempDir(), "transcode-source.mp4")
	if output, runErr := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=duration=2:size=480x270:rate=24",
		"-c:v", "mpeg4", "-q:v", "2", input,
	).CombinedOutput(); runErr != nil {
		t.Fatalf("test video creation failed: %v: %s", runErr, output)
	}
	result, err := transcodeVideoWithDelete(input, "hevc", "en", os.Remove)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(input); !os.IsNotExist(err) {
		t.Fatal("original remained after successful transcode")
	}
	info, err := os.Stat(result.Item.Path)
	if err != nil || info.Size() == 0 || result.OutputSize >= result.OriginalSize {
		t.Fatalf("unexpected transcode result: %#v, stat=%v", result, err)
	}
	probe, err := probeWithFFprobe(result.Item.Path, "en")
	if err != nil || firstVideoCodec(probe) != "hevc" {
		t.Fatalf("unexpected output codec: %q, error=%v", firstVideoCodec(probe), err)
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

func TestNormaliseSplitMarkersSortsAndValidates(t *testing.T) {
	markers, err := normaliseSplitMarkers([]float64{7, 2, 4}, 10, "en")
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{2, 4, 7}
	for i := range want {
		if markers[i] != want[i] {
			t.Fatalf("marker %d = %v, want %v", i, markers[i], want[i])
		}
	}
	if _, err := normaliseSplitMarkers([]float64{2, 2}, 10, "en"); err == nil {
		t.Fatal("duplicate markers were accepted")
	}
}

func TestMultiSplitWithSystemFFmpeg(t *testing.T) {
	ffmpeg, err := requireMediaTool("ffmpeg", "en")
	if err != nil {
		t.Skip(err)
	}
	input := filepath.Join(t.TempDir(), "multi.mp4")
	command := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=duration=5:size=320x180:rate=12",
		"-c:v", "libx264", "-g", "12", "-keyint_min", "12", "-sc_threshold", "0", input,
	)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		t.Fatalf("create synthetic video: %v: %s", runErr, output)
	}
	result, err := splitMP4AtMarkers(input, []float64{1.5, 3.5}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Paths) != 3 || len(result.SplitTimes) != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, path := range result.Paths {
		if info, statErr := os.Stat(path); statErr != nil || info.Size() == 0 {
			t.Fatalf("missing split output %q: %v", path, statErr)
		}
	}
}

func TestEmbeddedLocalizationCatalogIsValid(t *testing.T) {
	if err := validateLocalization(localizationData); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedLocaleFallsBackToEnglish(t *testing.T) {
	if got := translate("de", "error.invalidSplitPoint"); got != localizationData.Messages["en"]["error.invalidSplitPoint"] {
		t.Fatalf("unexpected fallback: %q", got)
	}
}

func TestAppLocaleSelection(t *testing.T) {
	app := NewApp(newMediaServer())
	if payload := app.GetLocalization("tr"); payload.Locale != "tr" || payload.Messages["library.title"] == "" || app.tr("dialog.openVideo") != localizationData.Messages["tr"]["dialog.openVideo"] {
		t.Fatalf("Turkish locale was not applied: %#v", payload)
	}
	if payload := app.GetLocalization("unsupported"); payload.Locale != "en" || app.tr("dialog.openVideo") != localizationData.Messages["en"]["dialog.openVideo"] {
		t.Fatalf("unsupported locale did not fall back to English: %#v", payload)
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

func TestProbeMediaWithoutMovieBoxDoesNotPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incomplete.mp4")
	ftypOnly := []byte{
		0, 0, 0, 24, 'f', 't', 'y', 'p',
		'i', 's', 'o', 'm', 0, 0, 0, 0,
		'i', 's', 'o', 'm', 'i', 's', 'o', '2',
	}
	if err := os.WriteFile(path, ftypOnly, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := probeMediaPureGo(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.ProbeAvailable || info.Size != int64(len(ftypOnly)) || info.Container != "mp4" {
		t.Fatalf("unexpected partial media info: %#v", info)
	}
	if _, err := thumbnailFor(path); err == nil {
		t.Fatal("expected thumbnail generation for incomplete MP4 to fail safely")
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

func TestDeleteFileFallsBackToPermanentRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mounted-video.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	trashErr := errors.New("trash is unavailable")
	if err := deleteFileWithTrash(path, func(string) error { return trashErr }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file remained after Trash fallback")
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
