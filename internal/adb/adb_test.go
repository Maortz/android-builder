package adb

import "testing"

func TestParseDevicesOutput(t *testing.T) {
	sample := "List of devices attached\n" +
		"emulator-5554  device product:sdk_gphone64_x86_64 model:sdk_gphone64_x86_64 device:emulator64_x86_64 transport_id:1\n" +
		"R58M12ABCDE    unauthorized\n" +
		"\n"

	devices := parseDevicesOutput(sample)

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d: %+v", len(devices), devices)
	}
	if devices[0].Serial != "emulator-5554" || devices[0].State != "device" {
		t.Errorf("unexpected first device: %+v", devices[0])
	}
	if devices[1].Serial != "R58M12ABCDE" || devices[1].State != "unauthorized" {
		t.Errorf("unexpected second device: %+v", devices[1])
	}
}
