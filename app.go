package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var mediaInfoCache sync.Map

type MediaItem struct {
	Name           string  `json:"name"`
	Path           string  `json:"path"`
	URL            string  `json:"url"`
	ThumbnailURL   string  `json:"thumbnailUrl"`
	ThumbnailReady bool    `json:"thumbnailReady,omitempty"`
	Duration       float64 `json:"duration,omitempty"`
	Kind           string  `json:"kind"`
	Size           int64   `json:"size"`
	ModTime        int64   `json:"modTime"`
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

type MergeResult struct {
	Item    MediaItem `json:"item"`
	Deleted bool      `json:"deleted"`
}

type App struct {
	application *application.App
	server      *mediaServer
	recent      *recentStore
	thumbnails  *thumbnailManager
	transcode   *transcodeManager
	jobs        *jobManager
	player      *nativePlayer
	window      *application.WebviewWindow
	systemOpen  systemOpenQueue
	locale      localeState
}

func NewApp(server *mediaServer) *App {
	app := &App{server: server, recent: newRecentStore(), thumbnails: newThumbnailManager(), transcode: newTranscodeManager(), jobs: newJobManager()}
	app.locale.set(defaultLocale)
	app.transcode.setObserver(func(status TranscodeProgress) {
		if app.application != nil {
			app.application.Event.Emit("transcode:progress", status)
		}
	})
	app.jobs.setObserver(func(jobs []MediaJob) {
		if app.application != nil {
			app.application.Event.Emit("jobs:progress", jobs)
		}
	})
	return app
}

func (a *App) GetLocalization(locale string) LocalizationPayload {
	payload := localizationPayload(a.locale.set(locale))
	a.buildPlaylistMenu()
	return payload
}

func (a *App) tr(key string, args ...any) string {
	return translate(a.locale.get(), key, args...)
}

func (a *App) mediaItem(path string) MediaItem {
	return a.mediaItemWithInfo(path, nil)
}

func (a *App) mediaItemWithInfo(path string, info os.FileInfo) MediaItem {
	abs := a.server.allow(path)
	item := MediaItem{Name: filepath.Base(abs), Path: abs, URL: "/media?path=" + url.QueryEscape(abs), ThumbnailURL: "/thumbnail?path=" + url.QueryEscape(abs), Kind: mediaKind(abs)}
	if info == nil {
		info, _ = os.Stat(abs)
	}
	if info != nil {
		item.Size, item.ModTime = info.Size(), info.ModTime().UnixMilli()
		if item.Kind == "video" && hasCachedThumbnailInfo(abs, info) {
			item.ThumbnailReady = true
		}
		key := fmt.Sprintf("%s:%d:%d", abs, info.Size(), info.ModTime().UnixNano())
		if cachedDur, ok := durationCache.Load(key); ok {
			item.Duration = cachedDur.(float64)
		}
	}
	return item
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
						info, _ := entry.Info()
						items = append(items, a.mediaItemWithInfo(filepath.Join(trimmed, entry.Name()), info))
					}
				}
			}
			continue
		}
		if isSupportedMedia(trimmed) {
			items = append(items, a.mediaItemWithInfo(trimmed, fi))
		}
	}
	return items, nil
}

func (a *App) DirectoryVideos(path string) ([]MediaItem, error) {
	if !a.server.isAllowed(path) {
		return nil, errors.New(a.tr("error.unauthorised"))
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	items := make([]MediaItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isSupportedMedia(entry.Name()) {
			continue
		}
		info, _ := entry.Info()
		items = append(items, a.mediaItemWithInfo(filepath.Join(dir, entry.Name()), info))
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}

func (a *App) DeleteVideo(path string) error {
	if !a.server.isAllowed(path) {
		return errors.New(a.tr("error.unauthorised"))
	}
	if a.transcode.isEncoding(path) {
		return errors.New(a.tr("delete.encodingBusy"))
	}
	if a.jobs.isBusy(path) {
		return errors.New(a.tr("error.fileBusy"))
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
	var cacheKey string
	if stat, statErr := os.Stat(path); statErr == nil {
		cacheKey = fmt.Sprintf("%s:%d:%d", path, stat.Size(), stat.ModTime().UnixNano())
		if cached, ok := mediaInfoCache.Load(cacheKey); ok {
			return cached.(MediaInfo), nil
		}
	}
	info, err := probeMediaPureGo(path)
	if err != nil {
		return info, err
	}
	if !info.ProbeAvailable {
		if probe, probeErr := probeWithFFprobe(path, a.locale.get()); probeErr == nil {
			info = mediaInfoFromProbe(info, probe)
		}
	}
	if cacheKey != "" && info.ProbeAvailable {
		mediaInfoCache.Store(cacheKey, info)
		if info.Duration > 0 {
			durationCache.Store(cacheKey, info.Duration)
		}
	}
	return info, nil
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
			items = append(items, a.mediaItemWithInfo(path, stat))
		}
	}
	return items, nil
}

func (a *App) SplitVideo(path string, seconds float64) (SplitResult, error) {
	if !a.server.isAllowed(path) {
		return SplitResult{}, errors.New(a.tr("error.unauthorised"))
	}
	if a.transcode.isEncoding(path) || a.jobs.isBusy(path) {
		return SplitResult{}, errors.New(a.tr("error.fileBusy"))
	}
	result, err := splitMP4WithJobs(path, seconds, a.locale.get(), a.jobs)
	if err != nil {
		return SplitResult{}, err
	}
	a.server.allow(result.FirstPath)
	a.server.allow(result.SecondPath)
	return result, nil
}

func (a *App) SplitVideoAtMarkers(path string, seconds []float64) (MultiSplitResult, error) {
	if !a.server.isAllowed(path) {
		return MultiSplitResult{}, errors.New(a.tr("error.unauthorised"))
	}
	if a.transcode.isEncoding(path) || a.jobs.isBusy(path) {
		return MultiSplitResult{}, errors.New(a.tr("error.fileBusy"))
	}
	result, err := splitMP4AtMarkersWithJobs(path, seconds, a.locale.get(), a.jobs)
	if err != nil {
		return MultiSplitResult{}, err
	}
	for _, outPath := range result.Paths {
		a.server.allow(outPath)
	}
	return result, nil
}

func (a *App) ExtractContactSheet(path string, frameCount int, imageWidth int) (string, error) {
	if !a.server.isAllowed(path) {
		return "", errors.New(a.tr("error.unauthorised"))
	}
	if a.transcode.isEncoding(path) || a.jobs.isBusy(path) {
		return "", errors.New(a.tr("error.fileBusy"))
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
	a.thumbnails.run(videoPaths, func(path string) (bool, error) {
		_, cached, err := ensureThumbnailCached(path)
		return cached, err
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
	if a.transcode.isEncoding(path) || a.jobs.isBusy(path) {
		return nil, errors.New(a.tr("error.fileBusy"))
	}
	return transcodeOptions(path, a.locale.get())
}

func (a *App) TranscodeVideo(path string, optionID string, trashOriginal bool) (TranscodeResult, error) {
	if !a.server.isAllowed(path) {
		return TranscodeResult{}, errors.New(a.tr("error.unauthorised"))
	}
	if a.transcode.isEncoding(path) || a.jobs.isBusy(path) {
		return TranscodeResult{}, errors.New(a.tr("error.fileBusy"))
	}
	if a.transcode.progress().Active {
		return TranscodeResult{}, errors.New(a.tr("error.transcodeBusy"))
	}
	var removeFn func(string) error
	if trashOriginal {
		removeFn = deleteFile
	}
	result, err := transcodeVideoWithJobs(path, optionID, a.locale.get(), a.transcode, a.jobs, removeFn)
	if err != nil {
		return TranscodeResult{}, err
	}
	if trashOriginal {
		_ = removeThumbnailFor(path)
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
	for _, job := range a.jobs.list() {
		if job.Kind == "transcode" {
			a.jobs.cancel(job.ID)
		}
	}
	a.transcode.cancel()
}

func (a *App) ActiveJobs() []MediaJob {
	return a.jobs.list()
}

func (a *App) CancelJob(id string) {
	for _, job := range a.jobs.list() {
		if job.ID == id && job.Kind == "transcode" {
			a.transcode.cancel()
		}
	}
	a.jobs.cancel(id)
}

func (a *App) MergeVideos(paths []string, trashOriginals bool) (MergeResult, error) {
	if len(paths) < 2 {
		return MergeResult{}, errors.New(a.tr("error.mergeMinCount"))
	}
	seen := make(map[string]struct{}, len(paths))
	cleaned := make([]string, 0, len(paths))
	for _, path := range paths {
		if !a.server.isAllowed(path) {
			return MergeResult{}, errors.New(a.tr("error.unauthorised"))
		}
		if a.transcode.isEncoding(path) || a.jobs.isBusy(path) {
			return MergeResult{}, errors.New(a.tr("error.fileBusy"))
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		cleaned = append(cleaned, abs)
	}
	if len(cleaned) < 2 {
		return MergeResult{}, errors.New(a.tr("error.mergeMinCount"))
	}
	var removeFn func(string) error
	if trashOriginals {
		removeFn = deleteFile
	}
	result, err := mergeVideosWithJobs(cleaned, a.locale.get(), a.jobs, removeFn)
	if err != nil {
		return MergeResult{}, err
	}
	if trashOriginals {
		for i, src := range cleaned {
			_ = removeThumbnailFor(src)
			a.server.disallow(src)
			if i == 0 {
				a.recent.replace(src, result.Item.Path)
			} else {
				a.recent.remove(src)
			}
		}
	} else {
		a.recent.add(result.Item.Path)
	}
	result.Item = a.mediaItem(result.Item.Path)
	result.Deleted = trashOriginals
	return result, nil
}

func isSupportedMedia(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".webm", ".mkv", ".avi", ".wmv", ".asf", ".mpg", ".mpeg", ".flv", ".mp3", ".m4a", ".wav", ".flac":
		return true
	default:
		return false
	}
}
