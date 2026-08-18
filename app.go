package main

import (
	"errors"
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
}

type App struct {
	application *application.App
	server      *mediaServer
	recent      *recentStore
	locale      localeState
}

func NewApp(server *mediaServer) *App {
	app := &App{server: server, recent: newRecentStore()}
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
		Filters: []application.FileFilter{{DisplayName: a.tr("dialog.mediaFiles"), Pattern: "*.mp4;*.m4v;*.mov;*.webm;*.mkv;*.avi;*.mp3;*.m4a;*.wav;*.flac"}},
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

func (a *App) TranscodeOptions(path string) ([]TranscodeOption, error) {
	if !a.server.isAllowed(path) {
		return nil, errors.New(a.tr("error.unauthorised"))
	}
	return transcodeOptions(path, a.locale.get())
}

func (a *App) TranscodeVideo(path string, optionID string) (TranscodeResult, error) {
	if !a.server.isAllowed(path) {
		return TranscodeResult{}, errors.New(a.tr("error.unauthorised"))
	}
	result, err := transcodeVideo(path, optionID, a.locale.get())
	if err != nil {
		return TranscodeResult{}, err
	}
	a.server.disallow(path)
	result.Item = a.mediaItem(result.Item.Path)
	a.recent.replace(path, result.Item.Path)
	return result, nil
}

func isSupportedMedia(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".webm", ".mkv", ".avi", ".mp3", ".m4a", ".wav", ".flac":
		return true
	default:
		return false
	}
}
