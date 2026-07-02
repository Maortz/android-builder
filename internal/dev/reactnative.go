package dev

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/Maortz/android-builder/internal/adb"
)

type ReactNativeHandler struct {
	metroPort int
	showLogs  bool
	metroCmd  *exec.Cmd
}

func NewReactNativeHandler(metroPort int, showLogs bool) *ReactNativeHandler {
	return &ReactNativeHandler{metroPort: metroPort, showLogs: showLogs}
}

// Run sets up the Metro reverse-proxy, starts the Metro bundler, launches
// the app, and blocks until the bundler exits or ctx is cancelled.
func (h *ReactNativeHandler) Run(ctx context.Context, deviceID, packageName string) error {
	port := strconv.Itoa(h.metroPort)

	if err := adb.Reverse(ctx, deviceID, port, port); err != nil {
		return fmt.Errorf("adb reverse: %w", err)
	}
	defer func() {
		_ = adb.ReverseRemove(context.Background(), deviceID, port)
	}()

	fmt.Printf("Starting Metro bundler on port %s...\n", port)
	h.metroCmd = exec.CommandContext(ctx, "npx", "react-native", "start", "--port", port)
	h.metroCmd.Stdout = os.Stdout
	h.metroCmd.Stderr = os.Stderr
	if err := h.metroCmd.Start(); err != nil {
		return fmt.Errorf("start Metro (is Node.js/React Native CLI installed?): %w", err)
	}

	// Give Metro a moment to bind its port before launching the app.
	time.Sleep(2 * time.Second)

	fmt.Printf("Launching %s...\n", packageName)
	if err := adb.Run(ctx, deviceID, "shell", "am", "start", "-n", packageName+"/.MainActivity"); err != nil {
		return fmt.Errorf("launch app: %w", err)
	}

	if h.showLogs {
		h.startLogcat(ctx, deviceID, packageName)
	}

	return h.metroCmd.Wait()
}

func (h *ReactNativeHandler) startLogcat(ctx context.Context, deviceID, packageName string) {
	go func() {
		pid, err := adb.PIDof(ctx, deviceID, packageName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not resolve PID for %s, skipping logcat: %v\n", packageName, err)
			return
		}
		if err := adb.Logcat(ctx, deviceID, pid, false, os.Stderr); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "Warning: logcat stream ended: %v\n", err)
		}
	}()
}

func (h *ReactNativeHandler) Stop() {
	if h.metroCmd != nil && h.metroCmd.Process != nil {
		_ = h.metroCmd.Process.Kill()
	}
}
