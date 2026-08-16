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

type App struct {
	application *application.App
	server      *mediaServer
	recent      *recentStore
}

func NewApp(server *mediaServer) *App { return &App{server: server, recent: newRecentStore()} }

func (a *App) mediaItem(path string) MediaItem {
	abs := a.server.allow(path)
	return MediaItem{Name: filepath.Base(abs), Path: abs, URL: "/media?path=" + url.QueryEscape(abs), ThumbnailURL: "/thumbnail?path=" + url.QueryEscape(abs), Kind: mediaKind(abs)}
}

func (a *App) OpenVideos() ([]MediaItem, error) {
	paths, err := a.application.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title:   "Video aç",
		Filters: []application.FileFilter{{DisplayName: "Video ve ses", Pattern: "*.mp4;*.m4v;*.mov;*.webm;*.mkv;*.avi;*.mp3;*.m4a;*.wav;*.flac"}},
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
		Title:   "Altyazı aç",
		Filters: []application.FileFilter{{DisplayName: "WebVTT altyazı", Pattern: "*.vtt"}},
	}).PromptForSingleSelection()
	if err != nil || path == "" {
		return MediaItem{}, err
	}
	abs := a.server.allow(path)
	return MediaItem{Name: filepath.Base(abs), Path: abs, URL: "/media?path=" + url.QueryEscape(abs), Kind: "subtitle"}, nil
}

func (a *App) DirectoryVideos(path string) ([]MediaItem, error) {
	if !a.server.isAllowed(path) {
		return nil, errors.New("media not authorised")
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
		return errors.New("media not authorised")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	a.server.disallow(path)
	a.recent.remove(path)
	return nil
}

func (a *App) ProbeMedia(path string) (MediaInfo, error) {
	if !a.server.isAllowed(path) {
		return MediaInfo{}, errors.New("media not authorised")
	}
	return probeMediaPureGo(path)
}

func (a *App) MarkPlayed(path string) error {
	if !a.server.isAllowed(path) {
		return errors.New("media not authorised")
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
		return SplitResult{}, errors.New("media not authorised")
	}
	return splitMP4(path, seconds)
}

func (a *App) ExtractContactSheet(path string, frameCount int, imageWidth int) (string, error) {
	if !a.server.isAllowed(path) {
		return "", errors.New("media not authorised")
	}
	return extractContactSheet(path, frameCount, imageWidth, 4)
}

func isSupportedMedia(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".webm", ".mkv", ".avi", ".mp3", ".m4a", ".wav", ".flac":
		return true
	default:
		return false
	}
}
