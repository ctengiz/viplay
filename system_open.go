package main

import "sync"

// systemOpenQueue holds files the OS asks ViPlay to open (Finder "Open With", file associations)
// until the frontend is ready, because launch-time requests arrive before the webview has loaded.
type systemOpenQueue struct {
	mu      sync.Mutex
	ready   bool
	pending []string
}

// add returns true when the frontend is ready and the files should be delivered immediately.
func (q *systemOpenQueue) add(paths ...string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.ready {
		return true
	}
	q.pending = append(q.pending, paths...)
	return false
}

func (q *systemOpenQueue) take() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ready = true
	pending := q.pending
	q.pending = nil
	return pending
}

func (a *App) openFromSystem(path string) {
	if path == "" || !a.systemOpen.add(path) {
		return
	}
	a.application.Event.Emit("files-dropped", map[string]any{"files": []string{path}})
	if a.window != nil {
		a.window.Show()
		a.window.Focus()
	}
}

// PendingSystemFiles returns files the OS asked to open before the frontend loaded and marks it ready.
func (a *App) PendingSystemFiles() []string {
	return a.systemOpen.take()
}
