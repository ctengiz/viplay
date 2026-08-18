package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

type mediaServer struct {
	mu      sync.RWMutex
	allowed map[string]struct{}
}

func newMediaServer() *mediaServer { return &mediaServer{allowed: make(map[string]struct{})} }

func (m *mediaServer) allow(path string) string {
	abs, _ := filepath.Abs(path)
	m.mu.Lock()
	m.allowed[abs] = struct{}{}
	m.mu.Unlock()
	return abs
}

func (m *mediaServer) isAllowed(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	m.mu.RLock()
	_, ok := m.allowed[abs]
	m.mu.RUnlock()
	return ok
}

func (m *mediaServer) disallow(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	m.mu.Lock()
	delete(m.allowed, abs)
	m.mu.Unlock()
}

func (m *mediaServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/media" && r.URL.Path != "/thumbnail" {
		http.NotFound(w, r)
		return
	}
	path := r.URL.Query().Get("path")
	abs, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, "invalid media path", http.StatusBadRequest)
		return
	}
	m.mu.RLock()
	_, ok := m.allowed[abs]
	m.mu.RUnlock()
	if !ok {
		http.Error(w, "media not authorised", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/thumbnail" {
		data, err := thumbnailFor(abs)
		if err != nil {
			http.Error(w, "thumbnail unavailable", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (m *mediaServer) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media" || r.URL.Path == "/thumbnail" {
			m.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	server := newMediaServer()
	service := NewApp(server)
	app := application.New(application.Options{
		Name:        "ViPlay",
		Description: "Cross-platform desktop video player",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Services: []application.Service{application.NewService(service)},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: server.middleware,
		},
	})
	service.application = app
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "ViPlay",
		Width:            1440,
		Height:           900,
		MinWidth:         980,
		MinHeight:        640,
		URL:              "/",
		BackgroundColour: application.NewRGB(9, 10, 12),
		Mac:              application.MacWindow{TitleBar: application.MacTitleBarHidden},
	})
	err := app.Run()
	if err != nil {
		fmt.Println("ViPlay:", err)
	}
}

func mediaKind(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".mp3" || ext == ".m4a" || ext == ".wav" || ext == ".flac" {
		return "audio"
	}
	return "video"
}
