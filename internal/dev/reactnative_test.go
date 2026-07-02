package dev

import "testing"

func TestNewReactNativeHandlerDefaults(t *testing.T) {
	h := NewReactNativeHandler(8081, true)

	if h.metroPort != 8081 {
		t.Errorf("expected metroPort 8081, got %d", h.metroPort)
	}
	if !h.showLogs {
		t.Error("expected showLogs true")
	}
}
