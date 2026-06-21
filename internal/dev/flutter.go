package dev

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/Maortz/android-builder/internal/config"
)

type FlutterHandler struct {
	noAttach bool
	noWatch  bool
	watchCfg *config.WatchConfig
	cmd      *exec.Cmd
	stdin    io.WriteCloser
}

func NewFlutterHandler(noAttach, noWatch bool, watchCfg *config.WatchConfig) *FlutterHandler {
	return &FlutterHandler{noAttach: noAttach, noWatch: noWatch, watchCfg: watchCfg}
}

func (h *FlutterHandler) Attach(ctx context.Context, deviceID, _ string) error {
	args := []string{"attach", "--device-id", deviceID}

	if h.noAttach {
		fmt.Printf("\nRun manually:\n  flutter %s\n", strings.Join(args, " "))
		return nil
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

func (h *FlutterHandler) Stop() {
	if h.stdin != nil {
		_ = h.stdin.Close()
	}
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
}
