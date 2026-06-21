package dev_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Maortz/android-builder/internal/config"
	"github.com/Maortz/android-builder/internal/dev"
)

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "lib")
	os.MkdirAll(libDir, 0755)
	dartFile := filepath.Join(libDir, "main.dart")
	os.WriteFile(dartFile, []byte("// initial"), 0644)

	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	fired := make(chan struct{}, 1)
	cfg := &config.WatchConfig{Dirs: []string{"lib"}, Patterns: []string{".dart"}, Debounce: 50}
	w := dev.NewWatcher(cfg, func() { fired <- struct{}{} })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go w.Run(ctx)

	time.Sleep(300 * time.Millisecond) // let initial scan populate seen map

	os.WriteFile(dartFile, []byte("// changed"), 0644)

	select {
	case <-fired:
	case <-ctx.Done():
		t.Fatal("watcher did not fire")
	}
}

func TestWatcherIgnoresPattern(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "lib")
	os.MkdirAll(libDir, 0755)
	genFile := filepath.Join(libDir, "foo.g.dart")
	os.WriteFile(genFile, []byte("// initial"), 0644)

	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	fired := make(chan struct{}, 1)
	cfg := &config.WatchConfig{Dirs: []string{"lib"}, Patterns: []string{".dart"}, Ignore: []string{".g.dart"}, Debounce: 50}
	w := dev.NewWatcher(cfg, func() { fired <- struct{}{} })

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	go w.Run(ctx)

	time.Sleep(300 * time.Millisecond)
	os.WriteFile(genFile, []byte("// changed"), 0644)

	select {
	case <-fired:
		t.Fatal("watcher fired for ignored file")
	case <-ctx.Done():
		// expected
	}
}
