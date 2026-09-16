package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var playbackProxyMu sync.Mutex

func requiresPlaybackProxy(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mpg", ".mpeg", ".flv", ".avi", ".wmv", ".asf", ".mkv":
		return true
	default:
		return false
	}
}

func canStreamCopyVideo(path, locale string) bool {
	probe, err := probeWithFFprobe(path, locale)
	if err != nil {
		return false
	}
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			codec := strings.ToLower(stream.CodecName)
			pixFmt := strings.ToLower(stream.PixFmt)
			if (codec == "h264" || codec == "hevc") && (pixFmt == "" || pixFmt == "yuv420p" || pixFmt == "yuvj420p") {
				return true
			}
			return false
		}
	}
	return false
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

	playbackProxyMu.Lock()
	defer playbackProxyMu.Unlock()
	if info, statErr := os.Stat(outputPath); statErr == nil && info.Size() > 0 {
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
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)

	ext := strings.ToLower(filepath.Ext(path))
	if (ext == ".mkv" || ext == ".avi" || ext == ".mp4" || ext == ".mov") && canStreamCopyVideo(path, locale) {
		copyArgs := []string{
			"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
			"-i", path,
			"-map", "0:v:0", "-map", "0:a?", "-sn", "-dn",
			"-c:v", "copy",
			"-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart",
			tempPath,
		}
		if _, copyErr := exec.Command(ffmpeg, copyArgs...).CombinedOutput(); copyErr == nil {
			if info, statErr := os.Stat(tempPath); statErr == nil && info.Size() > 0 {
				if err := os.Rename(tempPath, outputPath); err == nil {
					return outputPath, nil
				}
			}
		}
		_ = os.Remove(tempPath)
	}

	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", path,
		"-map", "0:v:0", "-map", "0:a?", "-sn", "-dn",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart",
		tempPath,
	}
	if output, runErr := exec.Command(ffmpeg, args...).CombinedOutput(); runErr != nil {
		return "", mediaToolError(translate(locale, "error.playbackConversionFailed"), runErr, output)
	}
	if info, statErr := os.Stat(tempPath); statErr != nil || info.Size() == 0 {
		return "", fmt.Errorf("%s", translate(locale, "error.playbackConversionFailed"))
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		return "", err
	}
	return outputPath, nil
}
