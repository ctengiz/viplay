package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
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

func thumbnailFor(path string) ([]byte, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", path, stat.Size(), stat.ModTime().UnixNano())))
	cachePath := filepath.Join(thumbnailCacheDir(), hex.EncodeToString(hash[:])+".jpg")
	if data, readErr := os.ReadFile(cachePath); readErr == nil {
		return data, nil
	}
	data, err := decodeH264Thumbnail(path)
	if err != nil {
		return nil, err
	}
	if os.MkdirAll(filepath.Dir(cachePath), 0o700) == nil {
		_ = os.WriteFile(cachePath, data, 0o600)
	}
	return data, nil
}

func decodeH264Thumbnail(path string) ([]byte, error) {
	info, err := probeMediaPureGo(path)
	if err != nil {
		return nil, err
	}
	timestamp := info.Duration * .1
	if timestamp < .1 {
		timestamp = .1
	}
	frame, err := decodeH264FrameAt(path, timestamp)
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
	img := image.NewRGBA(image.Rect(0, 0, outW, outH))
	for oy := 0; oy < outH; oy++ {
		for ox := 0; ox < outW; ox++ {
			x, y := ox*w/outW+frame.CropLeft, oy*h/outH+frame.CropTop
			ci := 0
			switch frame.ChromaFormatIDC {
			case 0:
				ci = -1
			case 1:
				ci = (y/2)*frame.CStride + x/2
			case 2:
				ci = y*frame.CStride + x/2
			default:
				ci = y*frame.CStride + x
			}
			cb, cr := uint8(128), uint8(128)
			if ci >= 0 && ci < len(frame.Cb) && ci < len(frame.Cr) {
				cb, cr = frame.Cb[ci], frame.Cr[ci]
			}
			r, g, b := color.YCbCrToRGB(frame.Y[y*frame.YStride+x], cb, cr)
			img.SetRGBA(ox, oy, color.RGBA{r, g, b, 255})
		}
	}
	var data bytes.Buffer
	if err := jpeg.Encode(&data, img, &jpeg.Options{Quality: 78}); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

type recentStore struct {
	mu   sync.Mutex
	path string
}

func newRecentStore() *recentStore {
	dir, _ := os.UserConfigDir()
	return &recentStore{path: filepath.Join(dir, "ViPlay", "recent.json")}
}
func (r *recentStore) read() []string {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil
	}
	var paths []string
	_ = json.Unmarshal(data, &paths)
	return paths
}
func (r *recentStore) add(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	paths := r.read()
	next := []string{abs}
	for _, p := range paths {
		if p != abs && len(next) < 50 {
			next = append(next, p)
		}
	}
	return r.write(next)
}
func (r *recentStore) remove(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	abs, _ := filepath.Abs(path)
	var next []string
	for _, p := range r.read() {
		if p != abs {
			next = append(next, p)
		}
	}
	_ = r.write(next)
}
func (r *recentStore) replace(oldPath, newPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	oldAbs, _ := filepath.Abs(oldPath)
	newAbs, _ := filepath.Abs(newPath)
	paths := r.read()
	next := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == oldAbs {
			p = newAbs
		}
		if p != "" && !containsPath(next, p) {
			next = append(next, p)
		}
	}
	_ = r.write(next)
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
	return r.read(), nil
}
func (r *recentStore) write(paths []string) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o600)
}
