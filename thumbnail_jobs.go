package main

import (
	"os"
	"path/filepath"
	"sync"
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

func (m *thumbnailManager) run(paths []string, generate func(string) error) {
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

		err := generate(path)

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

func thumbnailCacheDir() string {
	cache, _ := os.UserCacheDir()
	return filepath.Join(cache, "ViPlay", "thumbnails")
}

func clearThumbnailCache() error {
	return clearThumbnailCacheAt(thumbnailCacheDir())
}

func clearThumbnailCacheAt(path string) error {
	return os.RemoveAll(path)
}
