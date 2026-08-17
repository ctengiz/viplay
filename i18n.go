package main

import (
	"fmt"
	"sync"
)

const defaultLocale = "en"

var backendMessages = map[string]map[string]string{
	"en": {
		"error.unauthorised":        "Media access is not authorised",
		"error.ffmpegMissing":       "FFmpeg was not found. Install FFmpeg on your system and restart the application. macOS: brew install ffmpeg · Windows: winget install Gyan.FFmpeg · Linux: install the ffmpeg package with your package manager.",
		"error.ffprobeMissing":      "The FFmpeg installation is incomplete: ffprobe was not found. Make sure the ffmpeg and ffprobe commands are available in PATH, then restart the application.",
		"error.invalidSplitPoint":   "The split point must be inside the video.",
		"error.splitFailed":         "The video could not be split",
		"error.secondPartMissing":   "The video could not be split: no second part was created after the selected point.",
		"error.durationFailed":      "The video duration could not be read",
		"error.frameCount":          "The frame count must be between 1 and 60.",
		"error.imageWidth":          "The image width must be between 160 and 640 pixels.",
		"error.columns":             "The column count must be at least 1.",
		"error.contactSheetFailed":  "The contact sheet could not be created",
		"error.previewRequiresH264": "The preview requires H.264.",
		"error.frameDecode":         "The frame could not be decoded.",
		"error.videoTrackMissing":   "No video track was found.",
		"dialog.openVideo":          "Open video",
		"dialog.mediaFiles":         "Video and audio",
		"dialog.openSubtitle":       "Open subtitles",
		"dialog.webvttSubtitle":     "WebVTT subtitles",
	},
	"tr": {
		"error.unauthorised":        "Medya erişimi yetkilendirilmedi",
		"error.ffmpegMissing":       "FFmpeg bulunamadı. FFmpeg'i sisteminize kurup uygulamayı yeniden başlatın. macOS: brew install ffmpeg · Windows: winget install Gyan.FFmpeg · Linux: paket yöneticinizle ffmpeg paketini kurun.",
		"error.ffprobeMissing":      "FFmpeg kurulumu eksik: ffprobe bulunamadı. ffmpeg ve ffprobe komutlarının PATH içinde olduğundan emin olun, ardından uygulamayı yeniden başlatın.",
		"error.invalidSplitPoint":   "Bölme noktası videonun içinde olmalıdır.",
		"error.splitFailed":         "Video bölünemedi",
		"error.secondPartMissing":   "Video bölünemedi: seçilen noktadan sonra ikinci bölüm oluşturulamadı.",
		"error.durationFailed":      "Video süresi okunamadı",
		"error.frameCount":          "Kare sayısı 1 ile 60 arasında olmalıdır.",
		"error.imageWidth":          "Görsel genişliği 160 ile 640 piksel arasında olmalıdır.",
		"error.columns":             "Sütun sayısı en az 1 olmalıdır.",
		"error.contactSheetFailed":  "Contact sheet oluşturulamadı",
		"error.previewRequiresH264": "Önizleme için H.264 gereklidir.",
		"error.frameDecode":         "Kare çözülemedi.",
		"error.videoTrackMissing":   "Video izi bulunamadı.",
		"dialog.openVideo":          "Video aç",
		"dialog.mediaFiles":         "Video ve ses",
		"dialog.openSubtitle":       "Altyazı aç",
		"dialog.webvttSubtitle":     "WebVTT altyazı",
	},
}

func normaliseLocale(locale string) string {
	if _, ok := backendMessages[locale]; ok {
		return locale
	}
	return defaultLocale
}

func translate(locale, key string, args ...any) string {
	locale = normaliseLocale(locale)
	message, ok := backendMessages[locale][key]
	if !ok {
		message = backendMessages[defaultLocale][key]
	}
	if message == "" {
		message = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}

type localeState struct {
	mu     sync.RWMutex
	locale string
}

func (s *localeState) set(locale string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locale = normaliseLocale(locale)
	return s.locale
}

func (s *localeState) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return normaliseLocale(s.locale)
}
