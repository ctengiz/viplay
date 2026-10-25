package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"
)

// durationCache keys on path, size, and modification time so edited files are probed again.
var durationCache sync.Map

func mediaDurationCached(path, locale string) (float64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	key := fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())
	if cached, ok := durationCache.Load(key); ok {
		return cached.(float64), true
	}
	duration := 0.0
	if probe, err := probeMediaPureGo(path); err == nil && probe.Duration > 0 {
		duration = probe.Duration
	} else if result, err := probeWithFFprobe(path, locale); err == nil {
		duration, _ = strconv.ParseFloat(result.Format.Duration, 64)
	}
	if duration <= 0 {
		return 0, false
	}
	durationCache.Store(key, duration)
	return duration, true
}

// MediaDurations returns durations in seconds for the authorised paths the playlist currently shows.
func (a *App) MediaDurations(paths []string) map[string]float64 {
	locale := a.locale.get()
	result := make(map[string]float64, len(paths))
	var mu sync.Mutex
	work := make(chan string)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range work {
				if duration, ok := mediaDurationCached(path, locale); ok {
					mu.Lock()
					result[path] = duration
					mu.Unlock()
				}
			}
		}()
	}
	for _, path := range paths {
		if a.server.isAllowed(path) {
			work <- path
		}
	}
	close(work)
	wg.Wait()
	return result
}
