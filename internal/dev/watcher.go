package dev

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Maortz/android-builder/internal/config"
)

type Watcher struct {
	cfg      *config.WatchConfig
	onChange func()
	seen     map[string]time.Time
}

func NewWatcher(cfg *config.WatchConfig, onChange func()) *Watcher {
	if cfg == nil {
		cfg = &config.WatchConfig{Dirs: []string{"lib"}, Patterns: []string{".dart"}, Debounce: 100}
	}
	return &Watcher{cfg: cfg, onChange: onChange, seen: make(map[string]time.Time)}
}

func (w *Watcher) Run(ctx context.Context) {
	debounce := time.Duration(w.cfg.Debounce) * time.Millisecond
	if debounce == 0 {
		debounce = 100 * time.Millisecond
	}

	var pending bool
	var timer *time.Timer
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if w.scanChanges() && !pending {
				pending = true
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(debounce, func() {
					pending = false
					w.onChange()
				})
			}
		}
	}
}

func (w *Watcher) scanChanges() bool {
	changed := false
	for _, dir := range w.cfg.Dirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if !w.matches(path) || w.ignored(path) {
				return nil
			}
			prev, seen := w.seen[path]
			if !seen || info.ModTime().After(prev) {
				w.seen[path] = info.ModTime()
				if seen {
					changed = true
				}
			}
			return nil
		})
	}
	return changed
}

func (w *Watcher) matches(path string) bool {
	if len(w.cfg.Patterns) == 0 {
		return true
	}
	for _, p := range w.cfg.Patterns {
		if strings.HasSuffix(path, p) {
			return true
		}
	}
	return false
}

func (w *Watcher) ignored(path string) bool {
	for _, ig := range w.cfg.Ignore {
		if strings.HasSuffix(path, ig) {
			return true
		}
	}
	return false
}
