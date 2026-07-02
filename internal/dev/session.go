package dev

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Maortz/android-builder/internal/adb"
	"github.com/manifoldco/promptui"
)

// Session manages adb install → app launch → flutter attach.
type Session struct {
	deviceID    string
	apkPath     string
	packageName string
	skipInstall bool
	handler     *FlutterHandler
}

func NewSession(deviceID, apkPath string, handler *FlutterHandler) *Session {
	return &Session{deviceID: deviceID, apkPath: apkPath, handler: handler}
}

func (s *Session) SetSkipInstall(skip bool, packageName string) {
	s.skipInstall = skip
	s.packageName = packageName
}

// FindAPK returns the newest .apk in distDir, prompting if multiple.
func FindAPK(distDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(distDir, "*.apk"))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("no APK in %s — run 'builder android build' first", distDir)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	prompt := promptui.Select{Label: "Select APK", Items: matches}
	_, selected, err := prompt.Run()
	return selected, err
}

func (s *Session) Start(ctx context.Context) error {
	deviceID, err := s.selectDevice()
	if err != nil {
		return err
	}
	s.deviceID = deviceID

	if !s.skipInstall {
		fmt.Printf("Installing %s...\n", s.apkPath)
		if err := adb.Install(ctx, deviceID, s.apkPath); err != nil {
			return fmt.Errorf("adb install: %w", err)
		}
		fmt.Println("Installed.")

		if s.packageName == "" {
			pkg, err := detectPackageName(s.apkPath)
			if err != nil {
				return fmt.Errorf("%w\nUse --package com.your.app or set android.packageName in builder.json", err)
			}
			s.packageName = pkg
		}

		fmt.Printf("Launching %s...\n", s.packageName)
		if err := adb.Run(ctx, deviceID, "shell", "monkey", "-p", s.packageName, "-c", "android.intent.category.LAUNCHER", "1"); err != nil {
			return fmt.Errorf("launch app: %w", err)
		}
	}

	return s.handler.Attach(ctx, deviceID, s.packageName)
}

func (s *Session) selectDevice() (string, error) {
	if s.deviceID != "" {
		return s.deviceID, nil
	}
	devices, err := adb.Devices()
	if err != nil {
		return "", err
	}
	var serials []string
	for _, d := range devices {
		if d.State == "device" {
			serials = append(serials, d.Serial)
		}
	}
	if len(serials) == 0 {
		return "", fmt.Errorf("no Android devices found\nEnable USB debugging and reconnect, then check: adb devices")
	}
	if len(serials) == 1 {
		fmt.Printf("Device: %s\n", serials[0])
		return serials[0], nil
	}
	prompt := promptui.Select{Label: "Select device", Items: serials}
	_, selected, err := prompt.Run()
	return selected, err
}

func detectPackageName(apkPath string) (string, error) {
	tools := []string{"aapt", "aapt2"}
	var out []byte
	var err error
	for _, tool := range tools {
		out, err = exec.Command(tool, "dump", "badging", apkPath).Output()
		if err == nil {
			break
		}
	}
	if err != nil {
		return "", fmt.Errorf("aapt/aapt2 not found — could not detect package name")
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "package: name=") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "name=") {
				return strings.Trim(strings.TrimPrefix(field, "name="), `'"`), nil
			}
		}
	}
	return "", fmt.Errorf("could not parse package name from aapt output")
}
