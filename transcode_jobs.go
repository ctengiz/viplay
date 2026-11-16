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

type MediaJob struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Paths       []string `json:"paths"`
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Stage       string   `json:"stage"`
	Percent     float64  `json:"percent"`
	Speed       string   `json:"speed"`
	FPS         float64  `json:"fps"`
	CurrentTime float64  `json:"currentTime"`
	TotalTime   float64  `json:"totalTime"`
}

type mediaJobEntry struct {
	seq      uint64
	job      MediaJob
	pathSet  map[string]struct{}
	cancelFn context.CancelFunc
}

type jobManager struct {
	mu       sync.Mutex
	nextSeq  uint64
	entries  map[string]*mediaJobEntry
	onChange func([]MediaJob)
}

func newJobManager() *jobManager {
	return &jobManager{entries: make(map[string]*mediaJobEntry)}
}

func normaliseJobPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			abs = trimmed
		}
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

func (m *jobManager) setObserver(fn func([]MediaJob)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *jobManager) snapshotLocked() []MediaJob {
	ordered := make([]*mediaJobEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		ordered = append(ordered, entry)
	}
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[j].seq < ordered[i].seq {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	jobs := make([]MediaJob, 0, len(ordered))
	for _, entry := range ordered {
		jobCopy := entry.job
		jobCopy.Paths = append([]string(nil), entry.job.Paths...)
		jobs = append(jobs, jobCopy)
	}
	return jobs
}

func (m *jobManager) list() []MediaJob {
	if m == nil {
		return []MediaJob{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

func (m *jobManager) isBusy(paths ...string) bool {
	if m == nil || len(paths) == 0 {
		return false
	}
	norm := normaliseJobPaths(paths)
	if len(norm) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.entries {
		for _, p := range norm {
			if _, exists := entry.pathSet[p]; exists {
				return true
			}
		}
	}
	return false
}

func (m *jobManager) start(kind string, paths []string, name, label string, totalDuration float64, cancelFn context.CancelFunc) (string, bool) {
	if m == nil {
		return "", true
	}
	norm := normaliseJobPaths(paths)
	if len(norm) == 0 {
		return "", false
	}
	m.mu.Lock()
	for _, entry := range m.entries {
		for _, p := range norm {
			if _, exists := entry.pathSet[p]; exists {
				m.mu.Unlock()
				return "", false
			}
		}
	}
	m.nextSeq++
	id := kind + "-" + strings.TrimLeft(filepath.Base(norm[0]), ".") + "-" + string(rune('0'+m.nextSeq%10))
	id = "job-" + strconvFormatUint(m.nextSeq)
	pathSet := make(map[string]struct{}, len(norm))
	for _, p := range norm {
		pathSet[p] = struct{}{}
	}
	if name == "" {
		name = filepath.Base(norm[0])
	}
	m.entries[id] = &mediaJobEntry{
		seq:      m.nextSeq,
		pathSet:  pathSet,
		cancelFn: cancelFn,
		job: MediaJob{
			ID:        id,
			Kind:      kind,
			Paths:     norm,
			Name:      name,
			Label:     label,
			Stage:     "preparing",
			TotalTime: totalDuration,
		},
	}
	fn := m.onChange
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
	return id, true
}

func strconvFormatUint(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func (m *jobManager) setMeta(id, label string, totalDuration float64) {
	if m == nil || id == "" {
		return
	}
	m.mu.Lock()
	entry, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if label != "" {
		entry.job.Label = label
	}
	if totalDuration > 0 {
		entry.job.TotalTime = totalDuration
	}
	fn := m.onChange
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
}

func (m *jobManager) setStage(id, stage string) {
	if m == nil || id == "" {
		return
	}
	m.mu.Lock()
	entry, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	entry.job.Stage = stage
	if stage == "validating" {
		entry.job.Percent = 100
		if entry.job.TotalTime > 0 {
			entry.job.CurrentTime = entry.job.TotalTime
		}
	}
	fn := m.onChange
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
}

func (m *jobManager) update(id string, currentTime, fps float64, speed string) {
	if m == nil || id == "" {
		return
	}
	m.mu.Lock()
	entry, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if currentTime > 0 {
		entry.job.CurrentTime = currentTime
	}
	if fps > 0 {
		entry.job.FPS = fps
	}
	cleanSpeed := strings.TrimSpace(speed)
	if cleanSpeed != "" && !strings.EqualFold(cleanSpeed, "N/A") {
		entry.job.Speed = cleanSpeed
	}
	if currentTime > 0 || fps > 0 || entry.job.Speed != "" {
		entry.job.Stage = "running"
	}
	if entry.job.TotalTime > 0 && entry.job.CurrentTime > 0 {
		entry.job.Percent = math.Min(100.0, math.Max(0.0, (entry.job.CurrentTime/entry.job.TotalTime)*100.0))
	}
	fn := m.onChange
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
}

func (m *jobManager) cancel(id string) bool {
	if m == nil || id == "" {
		return false
	}
	m.mu.Lock()
	entry, ok := m.entries[id]
	if !ok || entry.cancelFn == nil {
		m.mu.Unlock()
		return false
	}
	cancelFn := entry.cancelFn
	delete(m.entries, id)
	fn := m.onChange
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	cancelFn()
	if fn != nil {
		fn(snapshot)
	}
	return true
}

func (m *jobManager) cancelAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(m.entries))
	for _, entry := range m.entries {
		if entry.cancelFn != nil {
			cancels = append(cancels, entry.cancelFn)
		}
	}
	m.entries = make(map[string]*mediaJobEntry)
	fn := m.onChange
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	for _, cancelFn := range cancels {
		cancelFn()
	}
	if fn != nil && len(cancels) > 0 {
		fn(snapshot)
	}
}

func (m *jobManager) finish(id string) {
	if m == nil || id == "" {
		return
	}
	m.mu.Lock()
	if _, ok := m.entries[id]; !ok {
		m.mu.Unlock()
		return
	}
	delete(m.entries, id)
	fn := m.onChange
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	if fn != nil {
		fn(snapshot)
	}
}
