package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ThumbnailProgress struct {
	Active         bool     `json:"active"`
	Paused         bool     `json:"paused"`
	Completed      int      `json:"completed"`
	Total          int      `json:"total"`
	Current        string   `json:"current"`
	CompletedPaths []string `json:"completedPaths"`
}

type thumbnailManager struct {
	mu         sync.Mutex
	cond       *sync.Cond
	generation uint64
	workers    int
	status     ThumbnailProgress
}

func newThumbnailManager() *thumbnailManager {
	manager := &thumbnailManager{}
	manager.cond = sync.NewCond(&manager.mu)
	return manager
}

func (m *thumbnailManager) run(paths []string, generate func(string) (cached bool, err error)) {
	m.mu.Lock()
	m.generation++
	generation := m.generation
	m.status = ThumbnailProgress{Active: len(paths) > 0, Total: len(paths), CompletedPaths: []string{}}
	m.cond.Broadcast()
	for m.workers > 0 {
		m.cond.Wait()
	}
	if generation != m.generation {
		m.mu.Unlock()
		return
	}
	m.workers++
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.workers--
		if generation == m.generation {
			m.status.Active = false
			m.status.Paused = false
			m.status.Current = ""
		}
		m.cond.Broadcast()
		m.mu.Unlock()
	}()

	for _, path := range paths {
		m.mu.Lock()
		for generation == m.generation && m.status.Paused {
			m.cond.Wait()
		}
		if generation != m.generation {
			m.mu.Unlock()
			return
		}
		m.status.Current = filepath.Base(path)
		m.mu.Unlock()

		cached, err := generate(path)

		m.mu.Lock()
		if generation != m.generation {
			m.mu.Unlock()
			return
		}
		m.status.Completed++
		if err == nil {
			m.status.CompletedPaths = append(m.status.CompletedPaths, path)
		}
		m.mu.Unlock()

		if !cached {
			time.Sleep(15 * time.Millisecond)
		}
	}
}

func (m *thumbnailManager) progress() ThumbnailProgress {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := m.status
	result.CompletedPaths = append([]string(nil), m.status.CompletedPaths...)
	return result
}

func (m *thumbnailManager) pause() {
	m.mu.Lock()
	if m.status.Active {
		m.status.Paused = true
	}
	m.mu.Unlock()
}

func (m *thumbnailManager) resume() {
	m.mu.Lock()
	m.status.Paused = false
	m.cond.Broadcast()
	m.mu.Unlock()
}

func (m *thumbnailManager) stopAndWait() {
	m.mu.Lock()
	m.generation++
	m.status.Active = false
	m.status.Paused = false
	m.status.Current = ""
	m.cond.Broadcast()
	for m.workers > 0 {
		m.cond.Wait()
	}
	m.mu.Unlock()
}

var (
	thumbIndexMu     sync.RWMutex
	thumbIndexLoaded bool
	thumbIndexDir    string
	thumbIndexFiles  map[string]struct{}
)

func ensureThumbIndexLocked(cacheDir string) {
	if thumbIndexLoaded && thumbIndexDir == cacheDir {
		return
	}
	entries, err := os.ReadDir(cacheDir)
	files := make(map[string]struct{}, len(entries))
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				files[entry.Name()] = struct{}{}
			}
		}
	}
	thumbIndexDir = cacheDir
	thumbIndexFiles = files
	thumbIndexLoaded = true
}

func recordCachedThumbnailName(cacheDir, fileName string) {
	thumbIndexMu.Lock()
	ensureThumbIndexLocked(cacheDir)
	thumbIndexFiles[fileName] = struct{}{}
	thumbIndexMu.Unlock()
}

func forgetCachedThumbnailName(cacheDir, fileName string) {
	thumbIndexMu.Lock()
	if thumbIndexLoaded && thumbIndexDir == cacheDir {
		delete(thumbIndexFiles, fileName)
	}
	thumbIndexMu.Unlock()
}

func resetCachedThumbnailIndex(cacheDir string) {
	thumbIndexMu.Lock()
	if !thumbIndexLoaded || thumbIndexDir == cacheDir {
		thumbIndexFiles = make(map[string]struct{})
		thumbIndexDir = cacheDir
		thumbIndexLoaded = true
	}
	thumbIndexMu.Unlock()
}

func thumbnailCacheFileName(path string, stat os.FileInfo) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", path, stat.Size(), stat.ModTime().UnixNano())))
	return hex.EncodeToString(hash[:]) + ".jpg"
}

func hasCachedThumbnailInfo(path string, stat os.FileInfo) bool {
	if stat == nil || stat.IsDir() {
		return false
	}
	cacheDir := thumbnailCacheDir()
	name := thumbnailCacheFileName(path, stat)
	thumbIndexMu.RLock()
	if thumbIndexLoaded && thumbIndexDir == cacheDir {
		_, ok := thumbIndexFiles[name]
		thumbIndexMu.RUnlock()
		return ok
	}
	thumbIndexMu.RUnlock()

	thumbIndexMu.Lock()
	ensureThumbIndexLocked(cacheDir)
	_, ok := thumbIndexFiles[name]
	thumbIndexMu.Unlock()
	return ok
}

func thumbnailCacheDir() string {
	cache, _ := os.UserCacheDir()
	return filepath.Join(cache, "ViPlay", "thumbnails")
}

func thumbnailCachePathAt(path, cacheDir string) (string, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, thumbnailCacheFileName(path, stat)), nil
}

func removeThumbnailFor(path string) error {
	return removeThumbnailForAt(path, thumbnailCacheDir())
}

func removeThumbnailForAt(path, cacheDir string) error {
	cachePath, err := thumbnailCachePathAt(path, cacheDir)
	if err != nil {
		return err
	}
	forgetCachedThumbnailName(cacheDir, filepath.Base(cachePath))
	return os.Remove(cachePath)
}

func clearThumbnailCache() error {
	return clearThumbnailCacheAt(thumbnailCacheDir())
}

func clearThumbnailCacheAt(path string) error {
	err := os.RemoveAll(path)
	resetCachedThumbnailIndex(path)
	return err
}
