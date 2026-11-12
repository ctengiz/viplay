package main

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"sync"
)

type TranscodeProgress struct {
	Active      bool    `json:"active"`
	Path        string  `json:"path"`
	Name        string  `json:"name"`
	Codec       string  `json:"codec"`
	Stage       string  `json:"stage"`
	Percent     float64 `json:"percent"`
	Speed       string  `json:"speed"`
	FPS         float64 `json:"fps"`
	CurrentTime float64 `json:"currentTime"`
	TotalTime   float64 `json:"totalTime"`
}

type transcodeManager struct {
	mu       sync.Mutex
	status   TranscodeProgress
	cancelFn context.CancelFunc
	onChange func(TranscodeProgress)
}

func newTranscodeManager() *transcodeManager {
	return &transcodeManager{}
}

func (m *transcodeManager) setObserver(fn func(TranscodeProgress)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *transcodeManager) emitLocked() (func(TranscodeProgress), TranscodeProgress) {
	return m.onChange, m.status
}

func (m *transcodeManager) progress() TranscodeProgress {
	if m == nil {
		return TranscodeProgress{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *transcodeManager) isEncoding(path string) bool {
	if m == nil || strings.TrimSpace(path) == "" {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.status.Active || m.status.Path == "" {
		return false
	}
	activeAbs, err := filepath.Abs(m.status.Path)
	if err != nil {
		activeAbs = m.status.Path
	}
	return activeAbs == abs
}

func (m *transcodeManager) cancel() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	if m.cancelFn == nil {
		m.mu.Unlock()
		return false
	}
	cancelFn := m.cancelFn
	m.cancelFn = nil
	m.status.Active = false
	m.status.Stage = ""
	fn, snapshot := m.emitLocked()
	m.mu.Unlock()
	cancelFn()
	if fn != nil {
		fn(snapshot)
	}
	return true
}

func (m *transcodeManager) start(path, codec string, totalDuration float64, cancelFn context.CancelFunc) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.cancelFn = cancelFn
	m.status = TranscodeProgress{
		Active:    true,
		Path:      path,
		Name:      filepath.Base(path),
		Codec:     codec,
		Stage:     "preparing",
		TotalTime: totalDuration,
	}
	fn, snapshot := m.emitLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
}

func (m *transcodeManager) setMeta(codec string, totalDuration float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.status.Active {
		m.mu.Unlock()
		return
	}
	if codec != "" {
		m.status.Codec = codec
	}
	if totalDuration > 0 {
		m.status.TotalTime = totalDuration
	}
	fn, snapshot := m.emitLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
}

func (m *transcodeManager) setStage(stage string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.status.Active {
		m.mu.Unlock()
		return
	}
	m.status.Stage = stage
	if stage == "validating" {
		m.status.Percent = 100
		if m.status.TotalTime > 0 {
			m.status.CurrentTime = m.status.TotalTime
		}
	}
	fn, snapshot := m.emitLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
}

func (m *transcodeManager) update(currentTime, fps float64, speed string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.status.Active {
		m.mu.Unlock()
		return
	}
	if currentTime > 0 {
		m.status.CurrentTime = currentTime
	}
	if fps > 0 {
		m.status.FPS = fps
	}
	cleanSpeed := strings.TrimSpace(speed)
	if cleanSpeed != "" && !strings.EqualFold(cleanSpeed, "N/A") {
		m.status.Speed = cleanSpeed
	}
	if currentTime > 0 || fps > 0 || m.status.Speed != "" {
		m.status.Stage = "encoding"
	}
	if m.status.TotalTime > 0 && m.status.CurrentTime > 0 {
		m.status.Percent = math.Min(100.0, math.Max(0.0, (m.status.CurrentTime/m.status.TotalTime)*100.0))
	}
	fn, snapshot := m.emitLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
}

func (m *transcodeManager) finish() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.cancelFn = nil
	wasActive := m.status.Active
	m.status = TranscodeProgress{}
	fn, snapshot := m.emitLocked()
	m.mu.Unlock()
	if wasActive && fn != nil {
		fn(snapshot)
	}
}
