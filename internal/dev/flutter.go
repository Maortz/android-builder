package dev

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Maortz/android-builder/internal/adb"
	"github.com/Maortz/android-builder/internal/config"
)

type FlutterHandler struct {
	noAttach bool
	noWatch  bool
	showLogs bool
	watchCfg *config.WatchConfig
	cmd      *exec.Cmd
	stdin    io.WriteCloser
}

func NewFlutterHandler(noAttach, noWatch, showLogs bool, watchCfg *config.WatchConfig) *FlutterHandler {
	return &FlutterHandler{noAttach: noAttach, noWatch: noWatch, showLogs: showLogs, watchCfg: watchCfg}
}

func (h *FlutterHandler) Attach(ctx context.Context, deviceID, packageName string) error {
	args := []string{"attach", "--device-id", deviceID}

	if h.noAttach {
		fmt.Printf("\nRun manually:\n  flutter %s\n", strings.Join(args, " "))
		return nil
	}

	if h.showLogs {
		h.startLogcat(ctx, deviceID, packageName)
	}

	fmt.Println("\nStarting flutter attach...")
	h.cmd = exec.CommandContext(ctx, "flutter", args...)

	stdin, err := h.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	h.stdin = stdin

	stdout, err := h.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	h.cmd.Stderr = h.cmd.Stdout

	if err := h.cmd.Start(); err != nil {
		return fmt.Errorf("flutter attach: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	if !h.noWatch {
		w := NewWatcher(h.watchCfg, func() {
			if h.stdin != nil {
				_, _ = fmt.Fprintln(h.stdin, "r")
				fmt.Printf("[%s] Hot reload\n", time.Now().Format("15:04:05"))
			}
		})
		go w.Run(ctx)
	}

	return h.cmd.Wait()
}

func (h *FlutterHandler) startLogcat(ctx context.Context, deviceID, packageName string) {
	if packageName == "" {
		fmt.Println("Warning: package name unknown, skipping --logs (pass --package to enable)")
		return
	}
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

func (h *FlutterHandler) Stop() {
	if h.stdin != nil {
		_ = h.stdin.Close()
	}
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
}
