package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// playbackCacheLimit bounds the proxy cache; least recently used proxies are evicted first.
const playbackCacheLimit = 20 << 30

var playbackProxyLocks sync.Map

func requiresPlaybackProxy(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mpg", ".mpeg", ".flv", ".avi", ".wmv", ".asf", ".mkv":
		return true
	default:
		return false
	}
}

// proxyStreamArgs returns codec arguments that remux WebKit-compatible streams and convert only the rest.
func proxyStreamArgs(probe ffprobeResult) (video []string, audio []string) {
	audio = []string{"-c:a", "copy"}
	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" && strings.ToLower(stream.CodecName) != "aac" {
			audio = []string{"-c:a", "aac", "-b:a", "192k"}
		}
	}
	for _, stream := range probe.Streams {
		if stream.CodecType != "video" {
			continue
		}
		codec, pixFmt := strings.ToLower(stream.CodecName), strings.ToLower(stream.PixFmt)
		eightBit := pixFmt == "" || pixFmt == "yuv420p" || pixFmt == "yuvj420p"
		switch {
		case codec == "h264" && eightBit:
			return []string{"-c:v", "copy"}, audio
		case codec == "hevc" && (eightBit || pixFmt == "yuv420p10le"):
			// WebKit plays HEVC in MP4 only with the hvc1 sample entry tag.
			return []string{"-c:v", "copy", "-tag:v", "hvc1"}, audio
		}
		return nil, audio
	}
	return nil, audio
}

func proxyEncoderArgs(ffmpeg string) [][]string {
	software := []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p"}
	if runtime.GOOS == "darwin" && availableVideoEncoders(ffmpeg)["h264_videotoolbox"] {
		return [][]string{{"-c:v", "h264_videotoolbox", "-b:v", "12M", "-allow_sw", "1", "-pix_fmt", "yuv420p"}, software}
	}
	return [][]string{software}
}

func preparePlaybackFile(path, locale string, force bool) (string, error) {
	if !force && !requiresPlaybackProxy(path) {
		return path, nil
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return preparePlaybackFileAt(path, locale, cacheRoot)
}

func preparePlaybackFileAt(path, locale, cacheRoot string) (string, error) {
	ffmpeg, err := requireMediaTool("ffmpeg", locale)
	if err != nil {
		return "", err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", path, stat.Size(), stat.ModTime().UnixNano())))
	cacheDir := filepath.Join(cacheRoot, "ViPlay", "playback")
	outputPath := filepath.Join(cacheDir, hex.EncodeToString(hash[:])+".mp4")

	lock, _ := playbackProxyLocks.LoadOrStore(outputPath, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	if info, statErr := os.Stat(outputPath); statErr == nil && info.Size() > 0 {
		now := time.Now()
		_ = os.Chtimes(outputPath, now, now)
		return outputPath, nil
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(cacheDir, ".viplay-playback-*.mp4")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return "", err
	}
	defer os.Remove(tempPath)

	var videoAttempts [][]string
	audioArgs := []string{"-c:a", "aac", "-b:a", "192k"}
	if probe, probeErr := probeWithFFprobe(path, locale); probeErr == nil {
		var copyArgs []string
		copyArgs, audioArgs = proxyStreamArgs(probe)
		if copyArgs != nil {
			videoAttempts = append(videoAttempts, copyArgs)
		}
	}
	videoAttempts = append(videoAttempts, proxyEncoderArgs(ffmpeg)...)

	var lastErr error
	var lastOutput []byte
	for _, videoArgs := range videoAttempts {
		args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", path, "-map", "0:v:0", "-map", "0:a?", "-sn", "-dn"}
		args = append(append(append(args, videoArgs...), audioArgs...), "-movflags", "+faststart", tempPath)
		lastOutput, lastErr = exec.Command(ffmpeg, args...).CombinedOutput()
		if info, statErr := os.Stat(tempPath); lastErr == nil && statErr == nil && info.Size() > 0 {
			if err := os.Rename(tempPath, outputPath); err != nil {
				return "", err
			}
			prunePlaybackCache(cacheDir, playbackCacheLimit, outputPath)
			return outputPath, nil
		}
	}
	if lastErr == nil {
		return "", fmt.Errorf("%s", translate(locale, "error.playbackConversionFailed"))
	}
	return "", mediaToolError(translate(locale, "error.playbackConversionFailed"), lastErr, lastOutput)
}

// prunePlaybackCache removes the least recently used proxies until the cache fits within limit.
func prunePlaybackCache(cacheDir string, limit int64, keep string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	type cached struct {
		path    string
		size    int64
		modTime time.Time
	}
	var files []cached
	var total int64
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil || entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		files = append(files, cached{filepath.Join(cacheDir, entry.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.Before(files[j].modTime) })
	for _, file := range files {
		if total <= limit {
			return
		}
		if file.path != keep && os.Remove(file.path) == nil {
			total -= file.size
		}
	}
}
