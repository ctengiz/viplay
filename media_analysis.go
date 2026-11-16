package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/thesyncim/goh264"
)

func probeMediaPureGo(path string) (MediaInfo, error) {
	result := MediaInfo{Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")}
	stat, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	result.Size = stat.Size()
	if !isISOBaseMedia(path) {
		return result, nil
	}
	f, parsed, err := decodeMP4(path)
	if err != nil {
		return result, nil
	}
	defer f.Close()
	if parsed.Moov == nil {
		return result, nil
	}
	result.ProbeAvailable = true
	result.Container = "mp4/mov"
	if parsed.Moov.Mvhd != nil && parsed.Moov.Mvhd.Timescale > 0 {
		result.Duration = float64(parsed.Moov.Mvhd.Duration) / float64(parsed.Moov.Mvhd.Timescale)
	}
	for _, track := range parsed.Moov.Traks {
		if track == nil || track.Mdia == nil || track.Mdia.Hdlr == nil || track.Mdia.Minf == nil || track.Mdia.Minf.Stbl == nil {
			continue
		}
		stsd := track.Mdia.Minf.Stbl.Stsd
		if stsd == nil {
			continue
		}
		switch track.Mdia.Hdlr.HandlerType {
		case "vide":
			entry, codec := videoEntry(stsd)
			if result.VideoCodec == "" && entry != nil {
				result.VideoCodec, result.Width, result.Height = codec, int(entry.Width), int(entry.Height)
				if track.Mdia.Mdhd != nil && track.Mdia.Mdhd.Duration > 0 && track.Mdia.Minf.Stbl.Stsz != nil {
					fps := float64(track.GetNrSamples()) * float64(track.Mdia.Mdhd.Timescale) / float64(track.Mdia.Mdhd.Duration)
					result.FPS = fmt.Sprintf("%.3g", fps)
				}
			}
		case "soun":
			if result.AudioCodec == "" {
				result.AudioCodec = audioCodec(stsd)
			}
		}
	}
	return result, nil
}

func isISOBaseMedia(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mov", ".m4a":
		return true
	}
	return false
}

func decodeMP4(path string) (*os.File, *mp4.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := mp4.DecodeFile(f, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, parsed, nil
}

func videoEntry(s *mp4.StsdBox) (*mp4.VisualSampleEntryBox, string) {
	switch {
	case s.AvcX != nil:
		return s.AvcX, "h264"
	case s.HvcX != nil:
		return s.HvcX, "hevc"
	case s.Av01 != nil:
		return s.Av01, "av1"
	case s.VpXX != nil:
		return s.VpXX, s.VpXX.Type()
	case s.Mjpg != nil:
		return s.Mjpg, "mjpeg"
	case s.Mp4v != nil:
		return s.Mp4v, "mpeg4"
	}
	return nil, ""
}

func audioCodec(s *mp4.StsdBox) string {
	switch {
	case s.Mp4a != nil:
		return "aac"
	case s.AC3 != nil:
		return "ac3"
	case s.EC3 != nil:
		return "eac3"
	case s.Opus != nil:
		return "opus"
	}
	return ""
}

func ensureThumbnailCached(path string) (string, bool, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return "", false, err
	}
	cacheDir := thumbnailCacheDir()
	fileName := thumbnailCacheFileName(path, stat)
	cachePath := filepath.Join(cacheDir, fileName)
	if hasCachedThumbnailInfo(path, stat) {
		if info, statErr := os.Stat(cachePath); statErr == nil && info.Size() > 0 {
			return cachePath, true, nil
		}
		forgetCachedThumbnailName(cacheDir, fileName)
	} else if info, statErr := os.Stat(cachePath); statErr == nil && info.Size() > 0 {
		recordCachedThumbnailName(cacheDir, fileName)
		return cachePath, true, nil
	}
	data, err := decodeH264Thumbnail(path)
	if err != nil {
		data, err = decodeFFmpegThumbnail(path)
		if err != nil {
			return "", false, err
		}
	}
	if os.MkdirAll(cacheDir, 0o700) == nil {
		if writeErr := os.WriteFile(cachePath, data, 0o600); writeErr == nil {
			recordCachedThumbnailName(cacheDir, fileName)
		}
	}
	return cachePath, false, nil
}

func thumbnailFor(path string) ([]byte, error) {
	cachePath, _, err := ensureThumbnailCached(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(cachePath)
}

func decodeH264Thumbnail(path string) ([]byte, error) {
	frame, err := decodeH264FrameAt(path, .1)
	if err != nil {
		return nil, err
	}
	return frameJPEG(frame)
}

func frameJPEG(frame *goh264.Frame) ([]byte, error) {
	if frame.BitDepthLuma != 8 || frame.Width < 1 || frame.Height < 1 {
		return nil, fmt.Errorf("unsupported frame format")
	}
	w, h := frame.Width, frame.Height
	outW := 240
	outH := h * outW / w
	if outH > 150 {
		outH = 150
		outW = w * outH / h
	}
	if outW < 1 {
		outW = 1
	}
	if outH < 1 {
		outH = 1
	}
	// Encode directly from YCbCr 4:2:0 planes so image/jpeg uses its fast path without RGBA round-trips.
	img := image.NewYCbCr(image.Rect(0, 0, outW, outH), image.YCbCrSubsampleRatio420)
	yMax := len(frame.Y)
	cbMax := len(frame.Cb)
	crMax := len(frame.Cr)
	for oy := 0; oy < outH; oy++ {
		sy := oy*h/outH + frame.CropTop
		yRowOff := sy * frame.YStride
		dstYRow := oy * img.YStride
		for ox := 0; ox < outW; ox++ {
			sx := ox*w/outW + frame.CropLeft
			yi := yRowOff + sx
			if yi >= 0 && yi < yMax {
				img.Y[dstYRow+ox] = frame.Y[yi]
			}
		}
	}
	cw := (outW + 1) / 2
	ch := (outH + 1) / 2
	for cy := 0; cy < ch; cy++ {
		sy := (cy*2)*h/outH + frame.CropTop
		dstCRow := cy * img.CStride
		for cx := 0; cx < cw; cx++ {
			sx := (cx*2)*w/outW + frame.CropLeft
			ci := -1
			switch frame.ChromaFormatIDC {
			case 0:
				ci = -1
			case 1:
				ci = (sy/2)*frame.CStride + sx/2
			case 2:
				ci = sy*frame.CStride + sx/2
			default:
				ci = sy*frame.CStride + sx
			}
			cb, cr := uint8(128), uint8(128)
			if ci >= 0 && ci < cbMax && ci < crMax {
				cb, cr = frame.Cb[ci], frame.Cr[ci]
			}
			img.Cb[dstCRow+cx] = cb
			img.Cr[dstCRow+cx] = cr
		}
	}
	var data bytes.Buffer
	data.Grow(12 * 1024)
	if err := jpeg.Encode(&data, img, &jpeg.Options{Quality: 78}); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

type recentStore struct {
	mu     sync.Mutex
	path   string
	loaded bool
	items  []string
}

func newRecentStore() *recentStore {
	dir, _ := os.UserConfigDir()
	return &recentStore{path: filepath.Join(dir, "ViPlay", "recent.json")}
}
func (r *recentStore) readLocked() []string {
	if r.loaded {
		return r.items
	}
	data, err := os.ReadFile(r.path)
	if err == nil {
		var paths []string
		if json.Unmarshal(data, &paths) == nil {
			r.items = paths
		}
	}
	r.loaded = true
	return r.items
}
func (r *recentStore) add(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	paths := r.readLocked()
	if len(paths) > 0 && paths[0] == abs {
		return nil
	}
	next := make([]string, 0, min(len(paths)+1, 50))
	next = append(next, abs)
	for _, p := range paths {
		if p != abs && len(next) < 50 {
			next = append(next, p)
		}
	}
	return r.writeLocked(next)
}
func (r *recentStore) remove(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	abs, _ := filepath.Abs(path)
	paths := r.readLocked()
	changed := false
	next := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == abs {
			changed = true
			continue
		}
		next = append(next, p)
	}
	if changed {
		_ = r.writeLocked(next)
	}
}
func (r *recentStore) replace(oldPath, newPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	oldAbs, _ := filepath.Abs(oldPath)
	newAbs, _ := filepath.Abs(newPath)
	paths := r.readLocked()
	next := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == oldAbs {
			p = newAbs
		}
		if p != "" && !containsPath(next, p) {
			next = append(next, p)
		}
	}
	_ = r.writeLocked(next)
}
func containsPath(paths []string, path string) bool {
	for _, candidate := range paths {
		if candidate == path {
			return true
		}
	}
	return false
}
func (r *recentStore) list() ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := r.readLocked()
	return append([]string(nil), paths...), nil
}
func (r *recentStore) writeLocked(paths []string) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	if err := os.WriteFile(r.path, data, 0o600); err != nil {
		return err
	}
	r.items = paths
	r.loaded = true
	return nil
}
