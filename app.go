package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type MediaItem struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnailUrl"`
	Kind         string `json:"kind"`
}

type MediaInfo struct {
	Container      string  `json:"container"`
	VideoCodec     string  `json:"videoCodec"`
	AudioCodec     string  `json:"audioCodec"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	FPS            string  `json:"fps"`
	Duration       float64 `json:"duration"`
	Size           int64   `json:"size"`
	ProbeAvailable bool    `json:"probeAvailable"`
}

type SplitResult struct {
	FirstPath  string  `json:"firstPath"`
	SecondPath string  `json:"secondPath"`
	SplitTime  float64 `json:"splitTime"`
}

type MultiSplitResult struct {
	Paths      []string  `json:"paths"`
	SplitTimes []float64 `json:"splitTimes"`
}

type TranscodeOption struct {
	ID              string `json:"id"`
	Codec           string `json:"codec"`
	Encoder         string `json:"encoder"`
	EstimatedSaving int    `json:"estimatedSaving"`
	OutputExtension string `json:"outputExtension"`
}

type TranscodeResult struct {
	Item         MediaItem `json:"item"`
	OriginalSize int64     `json:"originalSize"`
	OutputSize   int64     `json:"outputSize"`
	Deleted      bool      `json:"deleted"`
}

type App struct {
	application *application.App
	server      *mediaServer
	recent      *recentStore
	thumbnails  *thumbnailManager
	transcode   *transcodeManager
	locale      localeState
}

func NewApp(server *mediaServer) *App {
	app := &App{server: server, recent: newRecentStore(), thumbnails: newThumbnailManager(), transcode: newTranscodeManager()}
	app.locale.set(defaultLocale)
	return app
}

func (a *App) GetLocalization(locale string) LocalizationPayload {
	return localizationPayload(a.locale.set(locale))
}

func (a *App) tr(key string, args ...any) string {
	return translate(a.locale.get(), key, args...)
}

func (a *App) mediaItem(path string) MediaItem {
	abs := a.server.allow(path)
	return MediaItem{Name: filepath.Base(abs), Path: abs, URL: "/media?path=" + url.QueryEscape(abs), ThumbnailURL: "/thumbnail?path=" + url.QueryEscape(abs), Kind: mediaKind(abs)}
}

func (a *App) OpenVideos() ([]MediaItem, error) {
	paths, err := a.application.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:   a.tr("dialog.openVideo"),
		Filters: []application.FileFilter{{DisplayName: a.tr("dialog.mediaFiles"), Pattern: "*.mp4;*.m4v;*.mov;*.webm;*.mkv;*.avi;*.wmv;*.asf;*.mpg;*.mpeg;*.flv;*.mp3;*.m4a;*.wav;*.flac"}},
	}).PromptForMultipleSelection()
	if err != nil {
		return nil, err
	}
	items := make([]MediaItem, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		items = append(items, a.mediaItem(path))
	}
	return items, nil
}

func (a *App) OpenSubtitle() (MediaItem, error) {
	path, err := a.application.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:   a.tr("dialog.openSubtitle"),
		Filters: []application.FileFilter{{DisplayName: a.tr("dialog.webvttSubtitle"), Pattern: "*.vtt"}},
	}).PromptForSingleSelection()
	if err != nil || path == "" {
		return MediaItem{}, err
	}
	abs := a.server.allow(path)
	return MediaItem{Name: filepath.Base(abs), Path: abs, URL: "/media?path=" + url.QueryEscape(abs), Kind: "subtitle"}, nil
}

func (a *App) OpenSubtitlePath(path string) (MediaItem, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return MediaItem{}, errors.New("empty path")
	}
	fi, err := os.Stat(trimmed)
	if err != nil || fi.IsDir() {
		return MediaItem{}, fmt.Errorf("invalid subtitle path: %s", trimmed)
	}
	abs := a.server.allow(trimmed)
	return MediaItem{Name: filepath.Base(abs), Path: abs, URL: "/media?path=" + url.QueryEscape(abs), Kind: "subtitle"}, nil
}

func (a *App) OpenDroppedFiles(paths []string) ([]MediaItem, error) {
	items := make([]MediaItem, 0, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		fi, err := os.Stat(trimmed)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			entries, err := os.ReadDir(trimmed)
			if err == nil {
				for _, entry := range entries {
					if !entry.IsDir() && isSupportedMedia(entry.Name()) {
						items = append(items, a.mediaItem(filepath.Join(trimmed, entry.Name())))
					}
				}
			}
			continue
		}
		if isSupportedMedia(trimmed) {
			items = append(items, a.mediaItem(trimmed))
		}
	}
	return items, nil
}

func (a *App) DirectoryVideos(path string) ([]MediaItem, error) {
	if !a.server.isAllowed(path) {
		return nil, errors.New(a.tr("error.unauthorised"))
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	items := make([]MediaItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isSupportedMedia(entry.Name()) {
			continue
		}
		items = append(items, a.mediaItem(filepath.Join(filepath.Dir(path), entry.Name())))
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}

func (a *App) DeleteVideo(path string) error {
	if !a.server.isAllowed(path) {
		return errors.New(a.tr("error.unauthorised"))
	}
	_ = removeThumbnailFor(path)
	if err := deleteFile(path); err != nil {
		return err
	}
	a.server.disallow(path)
	a.recent.remove(path)
	return nil
}

func (a *App) ProbeMedia(path string) (MediaInfo, error) {
	if !a.server.isAllowed(path) {
		return MediaInfo{}, errors.New(a.tr("error.unauthorised"))
	}
	return probeMediaPureGo(path)
}

func (a *App) PreparePlayback(path string) (string, error) {
	if !a.server.isAllowed(path) {
		return "", errors.New(a.tr("error.unauthorised"))
	}
	playbackPath, err := preparePlaybackFile(path, a.locale.get(), false)
	if err != nil {
		return "", err
	}
	abs := a.server.allow(playbackPath)
	return "/media?path=" + url.QueryEscape(abs), nil
}

func (a *App) PreparePlaybackForce(path string) (string, error) {
	if !a.server.isAllowed(path) {
		return "", errors.New(a.tr("error.unauthorised"))
	}
	playbackPath, err := preparePlaybackFile(path, a.locale.get(), true)
	if err != nil {
		return "", err
	}
	abs := a.server.allow(playbackPath)
	return "/media?path=" + url.QueryEscape(abs), nil
}

func (a *App) MarkPlayed(path string) error {
	if !a.server.isAllowed(path) {
		return errors.New(a.tr("error.unauthorised"))
	}
	return a.recent.add(path)
}

func (a *App) RecentVideos() ([]MediaItem, error) {
	paths, err := a.recent.list()
	if err != nil {
		return nil, err
	}
	items := make([]MediaItem, 0, len(paths))
	for _, path := range paths {
		if stat, statErr := os.Stat(path); statErr == nil && !stat.IsDir() && isSupportedMedia(path) {
			items = append(items, a.mediaItem(path))
		}
	}
	return items, nil
}

func (a *App) SplitVideo(path string, seconds float64) (SplitResult, error) {
	if !a.server.isAllowed(path) {
		return SplitResult{}, errors.New(a.tr("error.unauthorised"))
	}
	return splitMP4(path, seconds, a.locale.get())
}

func (a *App) SplitVideoAtMarkers(path string, seconds []float64) (MultiSplitResult, error) {
	if !a.server.isAllowed(path) {
		return MultiSplitResult{}, errors.New(a.tr("error.unauthorised"))
	}
	return splitMP4AtMarkers(path, seconds, a.locale.get())
}

func (a *App) ExtractContactSheet(path string, frameCount int, imageWidth int) (string, error) {
	if !a.server.isAllowed(path) {
		return "", errors.New(a.tr("error.unauthorised"))
	}
	return extractContactSheet(path, frameCount, imageWidth, 4, a.locale.get())
}

func (a *App) GenerateThumbnails(paths []string) error {
	videoPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if !a.server.isAllowed(path) {
			return errors.New(a.tr("error.unauthorised"))
		}
		if mediaKind(path) == "video" {
			videoPaths = append(videoPaths, path)
		}
	}
	a.thumbnails.run(videoPaths, func(path string) error {
		_, err := thumbnailFor(path)
		return err
	})
	return nil
}

func (a *App) ThumbnailGenerationStatus() ThumbnailProgress {
	return a.thumbnails.progress()
}

func (a *App) PauseThumbnailGeneration() {
	a.thumbnails.pause()
}

func (a *App) ResumeThumbnailGeneration() {
	a.thumbnails.resume()
}

func (a *App) StopThumbnailGeneration() {
	a.thumbnails.stopAndWait()
}

func (a *App) ClearThumbnailCache() error {
	a.thumbnails.stopAndWait()
	return clearThumbnailCache()
}

func (a *App) TranscodeOptions(path string) ([]TranscodeOption, error) {
	if !a.server.isAllowed(path) {
		return nil, errors.New(a.tr("error.unauthorised"))
	}
	return transcodeOptions(path, a.locale.get())
}

func (a *App) TranscodeVideo(path string, optionID string, trashOriginal bool) (TranscodeResult, error) {
	if !a.server.isAllowed(path) {
		return TranscodeResult{}, errors.New(a.tr("error.unauthorised"))
	}
	if a.transcode.progress().Active {
		return TranscodeResult{}, errors.New(a.tr("error.transcodeBusy"))
	}
	var removeFn func(string) error
	if trashOriginal {
		removeFn = deleteFile
	}
	result, err := transcodeVideoWithManager(path, optionID, a.locale.get(), a.transcode, removeFn)
	if err != nil {
		return TranscodeResult{}, err
	}
	if trashOriginal {
		a.server.disallow(path)
		a.recent.replace(path, result.Item.Path)
	} else {
		a.recent.add(result.Item.Path)
	}
	result.Item = a.mediaItem(result.Item.Path)
	result.Deleted = trashOriginal
	return result, nil
}

func (a *App) TranscodeStatus() TranscodeProgress {
	return a.transcode.progress()
}

func (a *App) CancelTranscode() {
	a.transcode.cancel()
}

func isSupportedMedia(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".webm", ".mkv", ".avi", ".wmv", ".asf", ".mpg", ".mpeg", ".flv", ".mp3", ".m4a", ".wav", ".flac":
		return true
	default:
		return false
	}
}
