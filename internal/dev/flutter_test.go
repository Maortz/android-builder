package dev

import (
	"testing"

	"github.com/Maortz/android-builder/internal/config"
)

func TestNewFlutterHandlerSetsShowLogs(t *testing.T) {
	watchCfg := &config.WatchConfig{Dirs: []string{"lib"}}

	h := NewFlutterHandler(false, false, true, watchCfg)

	if !h.showLogs {
		t.Fatal("expected showLogs to be true")
	}
}
