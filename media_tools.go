package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/thesyncim/goh264"
)

func requireMediaTool(name, locale string) (string, error) {
	// Packaged builds ship FFmpeg next to the application executable.
	if executable, err := os.Executable(); err == nil {
		if bundled, err := exec.LookPath(filepath.Join(filepath.Dir(executable), name)); err == nil {
			return bundled, nil
		}
	}
	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}
	if runtime.GOOS == "darwin" {
		for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
			candidate := filepath.Join(dir, name)
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return candidate, nil
			}
		}
	}
	return "", missingMediaToolError(name, locale)
}

func missingMediaToolError(name, locale string) error {
	if name == "ffprobe" {
		return fmt.Errorf("%s", translate(locale, "error.ffprobeMissing"))
	}
	return fmt.Errorf("%s", translate(locale, "error.ffmpegMissing"))
}

func splitMP4(path string, seconds float64, locale string) (SplitResult, error) {
	ffmpeg, err := requireMediaTool("ffmpeg", locale)
	if err != nil {
		return SplitResult{}, err
	}
	duration, err := mediaDuration(path, locale)
	if err != nil {
		return SplitResult{}, err
	}
	if seconds <= .05 || seconds >= duration-.05 {
		return SplitResult{}, fmt.Errorf("%s", translate(locale, "error.invalidSplitPoint"))
	}

	dir := filepath.Dir(path)
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = ".mp4"
	}
	base := strings.TrimSuffix(path, filepath.Ext(path))
	first := availableOutput(base+"_part1", ext)
	second := availableOutput(base+"_part2", ext)
	tempDir, err := os.MkdirTemp(dir, ".viplay-split-")
	if err != nil {
		return SplitResult{}, err
	}
	defer os.RemoveAll(tempDir)
	pattern := filepath.Join(tempDir, "part_%d"+ext)

	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", path,
		"-map", "0:v:0", "-map", "0:a?", "-map", "0:s?",
		"-c", "copy",
		"-f", "segment", "-segment_times", formatSeconds(seconds),
		"-reset_timestamps", "1", pattern,
	}
	if output, runErr := exec.Command(ffmpeg, args...).CombinedOutput(); runErr != nil {
		return SplitResult{}, mediaToolError(translate(locale, "error.splitFailed"), runErr, output)
	}
	tempFirst := filepath.Join(tempDir, "part_0"+ext)
	tempSecond := filepath.Join(tempDir, "part_1"+ext)
	if _, err := os.Stat(tempSecond); err != nil {
		return SplitResult{}, fmt.Errorf("%s", translate(locale, "error.secondPartMissing"))
	}
	actual, err := mediaDuration(tempFirst, locale)
	if err != nil {
		return SplitResult{}, err
	}
	if err := os.Rename(tempFirst, first); err != nil {
		return SplitResult{}, err
	}
	if err := os.Rename(tempSecond, second); err != nil {
		_ = os.Remove(first)
		return SplitResult{}, err
	}
	return SplitResult{FirstPath: first, SecondPath: second, SplitTime: actual}, nil
}

func splitMP4AtMarkers(path string, points []float64, locale string) (MultiSplitResult, error) {
	ffmpeg, err := requireMediaTool("ffmpeg", locale)
	if err != nil {
		return MultiSplitResult{}, err
	}
	duration, err := mediaDuration(path, locale)
	if err != nil {
		return MultiSplitResult{}, err
	}
	markers, err := normaliseSplitMarkers(points, duration, locale)
	if err != nil {
		return MultiSplitResult{}, err
	}

	dir := filepath.Dir(path)
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = ".mp4"
	}
	base := strings.TrimSuffix(path, filepath.Ext(path))
	tempDir, err := os.MkdirTemp(dir, ".viplay-multi-split-")
	if err != nil {
		return MultiSplitResult{}, err
	}
	defer os.RemoveAll(tempDir)
	pattern := filepath.Join(tempDir, "part_%d"+ext)
	formatted := make([]string, len(markers))
	for i, marker := range markers {
		formatted[i] = formatSeconds(marker)
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", path,
		"-map", "0:v:0", "-map", "0:a?", "-map", "0:s?",
		"-c", "copy",
		"-f", "segment", "-segment_times", strings.Join(formatted, ","),
		"-reset_timestamps", "1", pattern,
	}
	if output, runErr := exec.Command(ffmpeg, args...).CombinedOutput(); runErr != nil {
		return MultiSplitResult{}, mediaToolError(translate(locale, "error.splitFailed"), runErr, output)
	}

	paths := make([]string, len(markers)+1)
	actualTimes := make([]float64, 0, len(markers))
	elapsed := 0.0
	for i := range paths {
		tempPath := filepath.Join(tempDir, fmt.Sprintf("part_%d%s", i, ext))
		if _, statErr := os.Stat(tempPath); statErr != nil {
			return MultiSplitResult{}, fmt.Errorf("%s", translate(locale, "error.splitPartMissing", i+1))
		}
		partDuration, durationErr := mediaDuration(tempPath, locale)
		if durationErr != nil {
			return MultiSplitResult{}, durationErr
		}
		elapsed += partDuration
		if i < len(markers) {
			actualTimes = append(actualTimes, elapsed)
		}
		paths[i] = availableOutput(fmt.Sprintf("%s_part%d", base, i+1), ext)
	}
	for i, outputPath := range paths {
		tempPath := filepath.Join(tempDir, fmt.Sprintf("part_%d%s", i, ext))
		if renameErr := os.Rename(tempPath, outputPath); renameErr != nil {
			for _, moved := range paths[:i] {
				_ = os.Remove(moved)
			}
			return MultiSplitResult{}, renameErr
		}
	}
	return MultiSplitResult{Paths: paths, SplitTimes: actualTimes}, nil
}

func normaliseSplitMarkers(points []float64, duration float64, locale string) ([]float64, error) {
	if len(points) < 1 || len(points) > 100 {
		return nil, fmt.Errorf("%s", translate(locale, "error.invalidMarkerCount"))
	}
	markers := append([]float64(nil), points...)
	sort.Float64s(markers)
	for i, marker := range markers {
		if math.IsNaN(marker) || math.IsInf(marker, 0) || marker <= .05 || marker >= duration-.05 || (i > 0 && marker-markers[i-1] < .05) {
			return nil, fmt.Errorf("%s", translate(locale, "error.invalidSplitMarkers"))
		}
	}
	return markers, nil
}

type mergeStreamSignature struct {
	ext        string
	videoCodec string
	audioCodec string
	width      int
	height     int
	duration   float64
}

func probeMergeSignature(path, locale string) (mergeStreamSignature, error) {
	probe, err := probeWithFFprobe(path, locale)
	if err != nil {
		return mergeStreamSignature{}, err
	}
	sig := mergeStreamSignature{
		ext:      strings.ToLower(filepath.Ext(path)),
		duration: probeDuration(probe),
	}
	for _, stream := range probe.Streams {
		switch {
		case stream.CodecType == "video" && sig.videoCodec == "":
			sig.videoCodec = strings.ToLower(stream.CodecName)
			sig.width = stream.Width
			sig.height = stream.Height
		case stream.CodecType == "audio" && sig.audioCodec == "":
			sig.audioCodec = strings.ToLower(stream.CodecName)
		}
	}
	if sig.videoCodec == "" || sig.duration <= 0 {
		return mergeStreamSignature{}, fmt.Errorf("%s", translate(locale, "error.videoTrackMissing"))
	}
	return sig, nil
}

func formatConcatDemuxerList(paths []string) string {
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	for _, p := range paths {
		escaped := strings.ReplaceAll(p, "'", `'\''`)
		b.WriteString("file '")
		b.WriteString(escaped)
		b.WriteString("'\n")
	}
	return b.String()
}

func mergeVideosWithDelete(paths []string, locale string, removeOriginal func(string) error) (MergeResult, error) {
	if len(paths) < 2 {
		return MergeResult{}, fmt.Errorf("%s", translate(locale, "error.mergeMinCount"))
	}
	ffmpeg, err := requireMediaTool("ffmpeg", locale)
	if err != nil {
		return MergeResult{}, err
	}
	var firstSig mergeStreamSignature
	var totalDuration float64
	for i, path := range paths {
		sig, sigErr := probeMergeSignature(path, locale)
		if sigErr != nil {
			return MergeResult{}, sigErr
		}
		if i == 0 {
			firstSig = sig
		} else if sig.ext != firstSig.ext ||
			sig.videoCodec != firstSig.videoCodec ||
			sig.width != firstSig.width ||
			sig.height != firstSig.height ||
			sig.audioCodec != firstSig.audioCodec {
			return MergeResult{}, fmt.Errorf("%s", translate(locale, "error.mergeIncompatible"))
		}
		totalDuration += sig.duration
	}

	firstPath := paths[0]
	dir := filepath.Dir(firstPath)
	ext := firstSig.ext
	if ext == "" {
		ext = ".mp4"
	}
	base := strings.TrimSuffix(firstPath, filepath.Ext(firstPath)) + "_merged"
	outputPath := availableOutput(base, ext)

	tempDir, err := os.MkdirTemp(dir, ".viplay-merge-")
	if err != nil {
		return MergeResult{}, err
	}
	defer os.RemoveAll(tempDir)

	listPath := filepath.Join(tempDir, "concat.txt")
	if err := os.WriteFile(listPath, []byte(formatConcatDemuxerList(paths)), 0o600); err != nil {
		return MergeResult{}, err
	}
	tempOutput := filepath.Join(tempDir, "merged"+ext)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-f", "concat", "-safe", "0",
		"-i", listPath,
		"-map", "0:v:0", "-map", "0:a?", "-map", "0:s?",
		"-c", "copy",
	}
	if ext == ".mp4" || ext == ".m4v" || ext == ".mov" {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, tempOutput)

	if output, runErr := exec.Command(ffmpeg, args...).CombinedOutput(); runErr != nil {
		return MergeResult{}, mediaToolError(translate(locale, "error.mergeFailed"), runErr, output)
	}
	outInfo, err := os.Stat(tempOutput)
	if err != nil || outInfo.Size() == 0 {
		return MergeResult{}, fmt.Errorf("%s", translate(locale, "error.mergeFailed"))
	}
	if outDuration, durErr := mediaDuration(tempOutput, locale); durErr != nil || outDuration <= 0 || math.Abs(outDuration-totalDuration) > math.Max(3, totalDuration*0.05) {
		return MergeResult{}, fmt.Errorf("%s", translate(locale, "error.mergeFailed"))
	}
	if err := os.Rename(tempOutput, outputPath); err != nil {
		return MergeResult{}, err
	}
	if removeOriginal != nil {
		for _, src := range paths {
			_ = removeOriginal(src)
		}
	}
	return MergeResult{
		Item:    MediaItem{Path: outputPath},
		Deleted: removeOriginal != nil,
	}, nil
}

type ffprobeResult struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecName    string            `json:"codec_name"`
		CodecType    string            `json:"codec_type"`
		Duration     string            `json:"duration,omitempty"`
		PixFmt       string            `json:"pix_fmt,omitempty"`
		Width        int               `json:"width,omitempty"`
		Height       int               `json:"height,omitempty"`
		AvgFrameRate string            `json:"avg_frame_rate,omitempty"`
		Tags         map[string]string `json:"tags,omitempty"`
	} `json:"streams"`
}

type transcodeProfile struct {
	option TranscodeOption
	args   []string
}

var transcodeProfiles = []transcodeProfile{
	{option: TranscodeOption{ID: "mp4", Codec: "MP4 (H.264)", EstimatedSaving: 20, OutputExtension: ".mp4"}, args: []string{"-preset", "medium", "-crf", "22", "-pix_fmt", "yuv420p"}},
	{option: TranscodeOption{ID: "hevc", Codec: "HEVC / H.265", EstimatedSaving: 35, OutputExtension: ".mkv"}, args: []string{"-preset", "medium", "-crf", "24"}},
	{option: TranscodeOption{ID: "av1", Codec: "AV1", EstimatedSaving: 50, OutputExtension: ".mkv"}, args: []string{"-preset", "6", "-crf", "30"}},
}

func transcodeOptions(path, locale string) ([]TranscodeOption, error) {
	ffmpeg, err := requireMediaTool("ffmpeg", locale)
	if err != nil {
		return nil, err
	}
	probe, err := probeWithFFprobe(path, locale)
	if err != nil {
		return nil, err
	}
	return transcodeOptionsFromProbe(path, probe, ffmpeg, locale)
}

func transcodeOptionsFromProbe(path string, probe ffprobeResult, ffmpeg, locale string) ([]TranscodeOption, error) {
	currentCodec := firstVideoCodec(probe)
	if currentCodec == "" {
		return nil, fmt.Errorf("%s", translate(locale, "error.videoTrackMissing"))
	}
	encoders := availableVideoEncoders(ffmpeg)
	options := make([]TranscodeOption, 0, len(transcodeProfiles))
	ext := strings.ToLower(filepath.Ext(path))
	for _, profile := range transcodeProfiles {
		if (profile.option.ID == "hevc" && (currentCodec == "hevc" || currentCodec == "h265" || currentCodec == "av1")) ||
			(profile.option.ID == "av1" && currentCodec == "av1") ||
			(profile.option.ID == "mp4" && (ext == ".mp4" || ext == ".m4v") && (currentCodec == "h264" || currentCodec == "avc")) {
			continue
		}
		encoder := preferredEncoder(profile.option.ID, encoders)
		if encoder == "" {
			continue
		}
		option := profile.option
		option.Encoder = encoder
		options = append(options, option)
	}
	return options, nil
}

func transcodeVideo(path, optionID, locale string) (TranscodeResult, error) {
	return transcodeVideoWithManager(path, optionID, locale, nil, deleteFile)
}

func transcodeVideoWithDelete(path, optionID, locale string, removeOriginal func(string) error) (TranscodeResult, error) {
	return transcodeVideoWithManager(path, optionID, locale, nil, removeOriginal)
}

func transcodeVideoWithManager(path, optionID, locale string, manager *transcodeManager, removeOriginal func(string) error) (TranscodeResult, error) {
	ffmpeg, err := requireMediaTool("ffmpeg", locale)
	if err != nil {
		return TranscodeResult{}, err
	}
	originalInfo, err := os.Stat(path)
	if err != nil {
		return TranscodeResult{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if manager != nil {
		manager.start(path, profileByID(optionID).option.Codec, 0, cancel)
		defer manager.finish()
	}

	probe, err := probeWithFFprobe(path, locale)
	if err != nil {
		return TranscodeResult{}, err
	}
	options, err := transcodeOptionsFromProbe(path, probe, ffmpeg, locale)
	if err != nil {
		return TranscodeResult{}, err
	}
	var selected *TranscodeOption
	for i := range options {
		if options[i].ID == optionID {
			selected = &options[i]
			break
		}
	}
	if selected == nil {
		return TranscodeResult{}, fmt.Errorf("%s", translate(locale, "error.transcodeOptionUnavailable"))
	}
	if ctx.Err() != nil {
		return TranscodeResult{}, fmt.Errorf("%s", translate(locale, "error.transcodeCancelled"))
	}

	duration := probeDuration(probe)
	if manager != nil {
		manager.setMeta(selected.Codec, duration)
	}

	dir := filepath.Dir(path)
	base := strings.TrimSuffix(path, filepath.Ext(path)) + "_" + selected.ID
	outputPath := availableOutput(base, selected.OutputExtension)
	temp, err := os.CreateTemp(dir, ".viplay-transcode-*"+selected.OutputExtension)
	if err != nil {
		return TranscodeResult{}, err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return TranscodeResult{}, err
	}
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)

	profile := profileByID(selected.ID)
	hasSubs := hasTextSubtitles(probe)

	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-progress", "pipe:1",
		"-i", path,
		"-map", "0:v:0", "-map", "0:a?",
	}
	if selected.OutputExtension == ".mp4" {
		if hasSubs {
			args = append(args, "-map", "0:s?")
		}
		args = append(args,
			"-map_metadata", "0", "-map_chapters", "0",
			"-c", "copy", "-c:v", selected.Encoder,
			"-c:a", "aac", "-b:a", "192k",
		)
		if hasSubs {
			args = append(args, "-c:s", "mov_text")
		}
		args = append(args, profile.args...)
		args = append(args, "-movflags", "+faststart", tempPath)
	} else {
		args = append(args,
			"-map", "0:s?",
			"-map_metadata", "0", "-map_chapters", "0",
			"-c", "copy", "-c:v", selected.Encoder,
		)
		args = append(args, profile.args...)
		args = append(args, tempPath)
	}

	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return TranscodeResult{}, err
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return TranscodeResult{}, err
	}

	var parseWg sync.WaitGroup
	parseWg.Add(1)
	go func() {
		defer parseWg.Done()
		scanner := bufio.NewScanner(stdout)
		var currentSec, fps float64
		var speed string
		for scanner.Scan() {
			line := scanner.Text()
			if key, val, ok := strings.Cut(line, "="); ok {
				key = strings.TrimSpace(key)
				val = strings.TrimSpace(val)
				switch key {
				case "out_time_us":
					if us, parseErr := strconv.ParseInt(val, 10, 64); parseErr == nil && us >= 0 {
						currentSec = float64(us) / 1000000.0
					}
				case "out_time":
					if parsed, ok := parseFFmpegTimestamp(val); ok && parsed > currentSec {
						currentSec = parsed
					}
				case "fps":
					if f, parseErr := strconv.ParseFloat(val, 64); parseErr == nil {
						fps = f
					}
				case "speed":
					speed = val
				case "progress":
					if manager != nil {
						if val == "end" {
							manager.update(currentSec, fps, speed)
							manager.setStage("validating")
						} else {
							manager.update(currentSec, fps, speed)
						}
					}
				}
			}
		}
	}()

	runErr := cmd.Wait()
	parseWg.Wait()

	if ctx.Err() != nil {
		return TranscodeResult{}, fmt.Errorf("%s", translate(locale, "error.transcodeCancelled"))
	}
	if runErr != nil {
		return TranscodeResult{}, mediaToolError(translate(locale, "error.transcodeFailed"), runErr, stderrBuf.Bytes())
	}
	if manager != nil {
		manager.setStage("validating")
	}
	requireSmaller := removeOriginal != nil
	if err := validateTranscodeWithSourceProbe(probe, tempPath, selected.ID, originalInfo.Size(), requireSmaller, locale); err != nil {
		return TranscodeResult{}, err
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		return TranscodeResult{}, err
	}
	outputInfo, err := os.Stat(outputPath)
	if err != nil {
		_ = os.Remove(outputPath)
		return TranscodeResult{}, err
	}
	if removeOriginal != nil {
		if err := removeOriginal(path); err != nil {
			_ = os.Remove(outputPath)
			return TranscodeResult{}, fmt.Errorf("%s: %w", translate(locale, "error.transcodeOriginalDeleteFailed"), err)
		}
	}
	return TranscodeResult{
		Item:         MediaItem{Path: outputPath},
		OriginalSize: originalInfo.Size(),
		OutputSize:   outputInfo.Size(),
		Deleted:      removeOriginal != nil,
	}, nil
}

func profileByID(id string) transcodeProfile {
	for _, profile := range transcodeProfiles {
		if profile.option.ID == id {
			return profile
		}
	}
	return transcodeProfile{}
}

var ffmpegListings sync.Map

// ffmpegListing caches capability listings because they cannot change while the binary path stays the same.
func ffmpegListing(ffmpeg, flag string) (string, bool) {
	key := ffmpeg + "\x00" + flag
	if cached, ok := ffmpegListings.Load(key); ok {
		return cached.(string), true
	}
	output, err := exec.Command(ffmpeg, "-hide_banner", flag).CombinedOutput()
	if err != nil {
		return "", false
	}
	ffmpegListings.Store(key, string(output))
	return string(output), true
}

func availableVideoEncoders(ffmpeg string) map[string]bool {
	output, ok := ffmpegListing(ffmpeg, "-encoders")
	if !ok {
		return nil
	}
	return parseVideoEncoders(output)
}

func parseVideoEncoders(output string) map[string]bool {
	result := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasPrefix(fields[0], "V") {
			result[fields[1]] = true
		}
	}
	return result
}

func preferredEncoder(codec string, encoders map[string]bool) string {
	var candidates []string
	switch codec {
	case "mp4":
		candidates = []string{"libx264"}
	case "hevc":
		candidates = []string{"libx265"}
	case "av1":
		candidates = []string{"libsvtav1"}
	}
	for _, candidate := range candidates {
		if encoders[candidate] {
			return candidate
		}
	}
	return ""
}

func hasTextSubtitles(probe ffprobeResult) bool {
	for _, stream := range probe.Streams {
		if stream.CodecType == "subtitle" {
			switch strings.ToLower(stream.CodecName) {
			case "subrip", "srt", "ass", "ssa", "mov_text", "webvtt":
				return true
			}
		}
	}
	return false
}

func probeWithFFprobe(path, locale string) (ffprobeResult, error) {
	ffprobe, err := requireMediaTool("ffprobe", locale)
	if err != nil {
		return ffprobeResult{}, err
	}
	output, runErr := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=duration:stream=codec_name,codec_type,duration,pix_fmt,width,height,avg_frame_rate:stream_tags=DURATION", "-of", "json", path).CombinedOutput()
	if runErr != nil {
		return ffprobeResult{}, mediaToolError(translate(locale, "error.durationFailed"), runErr, output)
	}
	var result ffprobeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return ffprobeResult{}, fmt.Errorf("%s: %w", translate(locale, "error.durationFailed"), err)
	}
	return result, nil
}

func parseFFmpegTimestamp(value string) (float64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "N/A") || strings.HasPrefix(trimmed, "-") {
		return 0, false
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) != 3 {
		return 0, false
	}
	hours, hErr := strconv.ParseFloat(parts[0], 64)
	minutes, mErr := strconv.ParseFloat(parts[1], 64)
	seconds, sErr := strconv.ParseFloat(parts[2], 64)
	if hErr != nil || mErr != nil || sErr != nil {
		return 0, false
	}
	total := hours*3600 + minutes*60 + seconds
	return total, total >= 0 && !math.IsNaN(total) && !math.IsInf(total, 0)
}

func probeDuration(probe ffprobeResult) float64 {
	if duration, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil && duration > 0 {
		return duration
	}
	for _, stream := range probe.Streams {
		if duration, err := strconv.ParseFloat(stream.Duration, 64); err == nil && duration > 0 {
			return duration
		}
		for key, value := range stream.Tags {
			if strings.EqualFold(key, "DURATION") {
				if duration, ok := parseFFmpegTimestamp(value); ok && duration > 0 {
					return duration
				}
			}
		}
	}
	return 0
}

// mediaInfoFromProbe fills the fields the pure Go MP4 reader leaves empty for other containers.
func mediaInfoFromProbe(info MediaInfo, probe ffprobeResult) MediaInfo {
	info.ProbeAvailable = true
	if duration := probeDuration(probe); duration > 0 {
		info.Duration = duration
	}
	for _, stream := range probe.Streams {
		switch {
		case stream.CodecType == "video" && info.VideoCodec == "":
			info.VideoCodec, info.Width, info.Height = strings.ToLower(stream.CodecName), stream.Width, stream.Height
			if numerator, denominator, ok := strings.Cut(stream.AvgFrameRate, "/"); ok {
				n, nErr := strconv.ParseFloat(numerator, 64)
				d, dErr := strconv.ParseFloat(denominator, 64)
				if nErr == nil && dErr == nil && n > 0 && d > 0 {
					info.FPS = fmt.Sprintf("%.3g", n/d)
				}
			}
		case stream.CodecType == "audio" && info.AudioCodec == "":
			info.AudioCodec = strings.ToLower(stream.CodecName)
		}
	}
	return info
}

func firstVideoCodec(result ffprobeResult) string {
	for _, stream := range result.Streams {
		if stream.CodecType == "video" {
			return strings.ToLower(stream.CodecName)
		}
	}
	return ""
}

func validateTranscode(source, output, codec string, sourceSize int64, requireSmaller bool, locale string) error {
	sourceProbe, err := probeWithFFprobe(source, locale)
	if err != nil {
		return err
	}
	return validateTranscodeWithSourceProbe(sourceProbe, output, codec, sourceSize, requireSmaller, locale)
}

func validateTranscodeWithSourceProbe(sourceProbe ffprobeResult, output, codec string, sourceSize int64, requireSmaller bool, locale string) error {
	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("%s", translate(locale, "error.transcodeValidationFailed"))
	}
	if requireSmaller && info.Size() >= sourceSize {
		return fmt.Errorf("%s", translate(locale, "error.transcodeNotSmaller"))
	}
	outputProbe, err := probeWithFFprobe(output, locale)
	if err != nil {
		return err
	}
	wantCodec := map[string]string{"hevc": "hevc", "av1": "av1", "mp4": "h264"}[codec]
	if firstVideoCodec(outputProbe) != wantCodec {
		return fmt.Errorf("%s", translate(locale, "error.transcodeValidationFailed"))
	}
	sourceDuration := probeDuration(sourceProbe)
	outputDuration := probeDuration(outputProbe)
	tolerance := math.Max(2, sourceDuration*.01)
	if sourceDuration <= 0 || outputDuration <= 0 || math.Abs(sourceDuration-outputDuration) > tolerance {
		return fmt.Errorf("%s", translate(locale, "error.transcodeValidationFailed"))
	}
	return nil
}

func mediaDuration(path, locale string) (float64, error) {
	result, err := probeWithFFprobe(path, locale)
	if err != nil {
		return 0, err
	}
	duration := probeDuration(result)
	if duration <= 0 {
		return 0, fmt.Errorf("%s", translate(locale, "error.durationFailed"))
	}
	return duration, nil
}

func extractContactSheet(path string, count, cellW, columns int, locale string) (string, error) {
	ffmpeg, err := requireMediaTool("ffmpeg", locale)
	if err != nil {
		return "", err
	}
	if count < 1 || count > 60 {
		return "", fmt.Errorf("%s", translate(locale, "error.frameCount"))
	}
	if cellW < 160 || cellW > 640 {
		return "", fmt.Errorf("%s", translate(locale, "error.imageWidth"))
	}
	if columns < 1 {
		return "", fmt.Errorf("%s", translate(locale, "error.columns"))
	}
	duration, err := mediaDuration(path, locale)
	if err != nil {
		return "", err
	}
	interval := duration / float64(count+1)
	rows := (count + columns - 1) / columns
	cellH := cellW * 9 / 16
	filters := []string{
		fmt.Sprintf("fps=fps=1/%s:start_time=%s", formatSeconds(interval), formatSeconds(interval)),
		fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", cellW, cellH),
		fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black", cellW, cellH),
	}
	if ffmpegSupportsFilter(ffmpeg, "drawtext") {
		filters = append(filters, "drawtext=text='%{pts\\:hms}':x=8:y=8:fontsize=18:fontcolor=white:box=1:boxcolor=black@0.65")
	}
	filters = append(filters, fmt.Sprintf("tile=%dx%d:nb_frames=%d:padding=2:margin=2", columns, rows, count))
	filter := strings.Join(filters, ",")
	output := availableOutput(strings.TrimSuffix(path, filepath.Ext(path))+"_contact-sheet", ".jpg")
	temp, err := os.CreateTemp(filepath.Dir(output), ".viplay-contact-*.jpg")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return "", err
	}
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)

	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
		"-i", path, "-vf", filter, "-frames:v", "1", "-q:v", "2", tempPath,
	}
	if data, runErr := exec.Command(ffmpeg, args...).CombinedOutput(); runErr != nil {
		return "", mediaToolError(translate(locale, "error.contactSheetFailed"), runErr, data)
	}
	if err := os.Rename(tempPath, output); err != nil {
		return "", err
	}
	return output, nil
}

func ffmpegSupportsFilter(ffmpeg, name string) bool {
	output, ok := ffmpegListing(ffmpeg, "-filters")
	return ok && ffmpegFilterListed(output, name)
}

func ffmpegFilterListed(output, name string) bool {
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return true
		}
	}
	return false
}

func formatSeconds(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func mediaToolError(action string, err error, output []byte) error {
	detail := strings.TrimSpace(string(output))
	if len(detail) > 800 {
		detail = detail[len(detail)-800:]
	}
	if detail == "" {
		return fmt.Errorf("%s: %w", strings.ToLower(action), err)
	}
	return fmt.Errorf("%s: %s", strings.ToLower(action), detail)
}

func availableOutput(base, ext string) string {
	candidate := base + ext
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d%s", base, i, ext)
	}
}

// decodeH264FrameAt is retained for the lightweight in-app thumbnail cache.
// User-triggered split, contact-sheet, and transcoding operations use the system FFmpeg.
// The frame is taken at fraction of the duration so the moov box is parsed only once.
func decodeH264FrameAt(path string, fraction float64) (*goh264.Frame, error) {
	f, parsed, err := decodeMP4(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if parsed.Moov == nil {
		return nil, fmt.Errorf("%s", translate(defaultLocale, "error.videoTrackMissing"))
	}
	var track *mp4.TrakBox
	for _, t := range parsed.Moov.Traks {
		if t != nil && t.Mdia != nil && t.Mdia.Hdlr != nil && t.Mdia.Minf != nil && t.Mdia.Minf.Stbl != nil && t.Mdia.Minf.Stbl.Stsd != nil && t.Mdia.Minf.Stbl.Stts != nil && t.Mdia.Minf.Stbl.Stsz != nil && t.Mdia.Mdhd != nil && t.Mdia.Mdhd.Timescale > 0 && t.Mdia.Hdlr.HandlerType == "vide" {
			track = t
			break
		}
	}
	if track == nil {
		return nil, fmt.Errorf("%s", translate(defaultLocale, "error.videoTrackMissing"))
	}
	stbl := track.Mdia.Minf.Stbl
	if stbl.Stsd.AvcX == nil || stbl.Stsd.AvcX.AvcC == nil {
		return nil, fmt.Errorf("%s", translate(defaultLocale, "error.previewRequiresH264"))
	}
	seconds := .1
	if parsed.Moov.Mvhd != nil && parsed.Moov.Mvhd.Timescale > 0 {
		seconds = max(seconds, float64(parsed.Moov.Mvhd.Duration)/float64(parsed.Moov.Mvhd.Timescale)*fraction)
	}
	target, err := stbl.Stts.GetSampleNrAtTime(uint64(seconds * float64(track.Mdia.Mdhd.Timescale)))
	if err != nil {
		return nil, err
	}
	start := target
	if stbl.Stss != nil {
		for start > 1 && !stbl.Stss.IsSyncSample(start) {
			start--
		}
	}
	var configBuffer bytes.Buffer
	if err := stbl.Stsd.AvcX.AvcC.DecConfRec.Encode(&configBuffer); err != nil {
		return nil, err
	}
	decoder := goh264.NewDecoder()
	if _, err := decoder.ConfigureAVCC(configBuffer.Bytes()); err != nil {
		return nil, err
	}
	workspace := make([]byte, 2*1024*1024)
	for n := start; n <= target+8 && n <= track.GetNrSamples(); n++ {
		var packet bytes.Buffer
		if err := parsed.CopySampleData(&packet, f, track, n, n, workspace); err != nil {
			return nil, err
		}
		frames, decodeErr := decoder.DecodeConfiguredAVCFrames(packet.Bytes())
		if decodeErr == nil && len(frames) > 0 && n >= target {
			return frames[len(frames)-1], nil
		}
	}
	frames, err := decoder.DecodeConfiguredAVCFrames(nil)
	if err == nil && len(frames) > 0 {
		return frames[len(frames)-1], nil
	}
	return nil, fmt.Errorf("%s", translate(defaultLocale, "error.frameDecode"))
}
