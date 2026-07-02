package adb

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type Device struct {
	Serial string
	State  string
}

func NotFoundError(err error) error {
	return fmt.Errorf("adb not found: %w\nInstall Platform-Tools: https://developer.android.com/tools/releases/platform-tools", err)
}

func parseDevicesOutput(out string) []Device {
	var devices []Device
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			devices = append(devices, Device{Serial: fields[0], State: fields[1]})
		}
	}
	return devices
}

func Devices() ([]Device, error) {
	out, err := exec.Command("adb", "devices", "-l").Output()
	if err != nil {
		return nil, NotFoundError(err)
	}
	return parseDevicesOutput(string(out)), nil
}

func Run(ctx context.Context, deviceID string, args ...string) error {
	fullArgs := append([]string{"-s", deviceID}, args...)
	out, err := exec.CommandContext(ctx, "adb", fullArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Install(ctx context.Context, deviceID, apkPath string) error {
	return Run(ctx, deviceID, "install", "-r", apkPath)
}

func PIDof(ctx context.Context, deviceID, packageName string) (int, error) {
	out, err := exec.CommandContext(ctx, "adb", "-s", deviceID, "shell", "pidof", packageName).Output()
	if err != nil {
		return 0, fmt.Errorf("pidof %s: %w", packageName, err)
	}
	pidStr := strings.TrimSpace(string(out))
	if pidStr == "" {
		return 0, fmt.Errorf("process %s not running on device %s", packageName, deviceID)
	}
	pid, err := strconv.Atoi(strings.Fields(pidStr)[0])
	if err != nil {
		return 0, fmt.Errorf("parse pid %q: %w", pidStr, err)
	}
	return pid, nil
}

func Forward(ctx context.Context, deviceID, devicePort, hostPort string) error {
	return Run(ctx, deviceID, "forward", "tcp:"+devicePort, "tcp:"+hostPort)
}

func ForwardRemove(ctx context.Context, deviceID, devicePort string) error {
	return Run(ctx, deviceID, "forward", "--remove", "tcp:"+devicePort)
}

func reverseArgs(devicePort, hostPort string) []string {
	return []string{"tcp:" + devicePort, "tcp:" + hostPort}
}

func reverseRemoveArgs(devicePort string) []string {
	return []string{"tcp:" + devicePort}
}

func Reverse(ctx context.Context, deviceID, devicePort, hostPort string) error {
	return Run(ctx, deviceID, append([]string{"reverse"}, reverseArgs(devicePort, hostPort)...)...)
}

func ReverseRemove(ctx context.Context, deviceID, devicePort string) error {
	return Run(ctx, deviceID, append([]string{"reverse", "--remove"}, reverseRemoveArgs(devicePort)...)...)
}

func Logcat(ctx context.Context, deviceID string, pid int, clear bool, w io.Writer) error {
	if clear {
		if err := Run(ctx, deviceID, "logcat", "-c"); err != nil {
			return fmt.Errorf("clear logcat buffer: %w", err)
		}
	}
	args := []string{"-s", deviceID, "logcat"}
	if pid > 0 {
		args = append(args, "--pid="+strconv.Itoa(pid))
	}
	cmd := exec.CommandContext(ctx, "adb", args...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}
