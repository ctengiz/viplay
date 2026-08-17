package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/thesyncim/goh264"
)

func requireMediaTool(name, locale string) (string, error) {
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

type ffprobeResult struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func mediaDuration(path, locale string) (float64, error) {
	ffprobe, err := requireMediaTool("ffprobe", locale)
	if err != nil {
		return 0, err
	}
	output, runErr := exec.Command(ffprobe,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "json", path,
	).CombinedOutput()
	if runErr != nil {
		return 0, mediaToolError(translate(locale, "error.durationFailed"), runErr, output)
	}
	var result ffprobeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("%s: %w", translate(locale, "error.durationFailed"), err)
	}
	duration, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil || duration <= 0 {
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
	output, err := exec.Command(ffmpeg, "-hide_banner", "-filters").CombinedOutput()
	if err != nil {
		return false
	}
	return ffmpegFilterListed(string(output), name)
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
// User-triggered split and contact-sheet operations use the system FFmpeg.
func decodeH264FrameAt(path string, seconds float64) (*goh264.Frame, error) {
	f, parsed, err := decodeMP4(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var track *mp4.TrakBox
	for _, t := range parsed.Moov.Traks {
		if t.Mdia.Hdlr.HandlerType == "vide" {
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
