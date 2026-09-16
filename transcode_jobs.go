package main

import (
	"context"
	"math"
	"sync"
)

type TranscodeProgress struct {
	Active      bool    `json:"active"`
	Path        string  `json:"path"`
	Codec       string  `json:"codec"`
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
}

func newTranscodeManager() *transcodeManager {
	return &transcodeManager{}
}

func (m *transcodeManager) progress() TranscodeProgress {
	if m == nil {
		return TranscodeProgress{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *transcodeManager) cancel() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancelFn != nil {
		m.cancelFn()
		m.cancelFn = nil
		m.status.Active = false
		return true
	}
	return false
}

func (m *transcodeManager) start(path, codec string, totalDuration float64, cancelFn context.CancelFunc) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelFn = cancelFn
	m.status = TranscodeProgress{
		Active:    true,
		Path:      path,
		Codec:     codec,
		TotalTime: totalDuration,
		Speed:     "—",
	}
}

func (m *transcodeManager) update(currentTime, fps float64, speed string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.status.Active {
		return
	}
	m.status.CurrentTime = currentTime
	m.status.FPS = fps
	if speed != "" && speed != "N/A" {
		m.status.Speed = speed
	}
	if m.status.TotalTime > 0 {
		m.status.Percent = math.Min(100.0, math.Max(0.0, (currentTime/m.status.TotalTime)*100.0))
	}
}

func (m *transcodeManager) finish() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelFn = nil
	m.status.Active = false
}
