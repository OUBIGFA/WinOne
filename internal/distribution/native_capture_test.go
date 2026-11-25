package distribution

import "testing"

func TestNativeCapturePayloadRequiresCompleteNativeRuntime(t *testing.T) {
	seen := map[string]bool{"capture/native-runtime.json": true, "capture/WinOne.Capture.exe": true}
	required := requiredForPayload(seen)
	contains := func(name string) bool {
		for _, file := range required {
			if file == name {
				return true
			}
		}
		return false
	}
	for _, name := range []string{"capture/WinOne.Ocr.exe", "capture/WinOne.NativeImage.dll", "capture/Tools3000Core.dll", "capture/opencv_core4.dll", "capture/WinOne.Recording.dll", "capture/Ocr/rapidocrlite.dll"} {
		if !contains(name) || !allowed(name) {
			t.Fatalf("native capture dependency is not required/allowed: %s", name)
		}
	}
	for _, name := range []string{"capture/coreclr.dll", "capture/System.Private.CoreLib.dll", "capture/WinOne.Capture.runtimeconfig.json"} {
		if contains(name) {
			t.Fatalf("native capture still requires managed runtime: %s", name)
		}
	}
	if allowed("capture/WinOne.NativeCapture.Probe.exe") || allowed("capture/WinOne.Capture.DraftTests.exe") {
		t.Fatal("test programs are distributable")
	}
	legacy := requiredForPayload(map[string]bool{"capture/WinOne.Capture.runtimeconfig.json": true})
	found := false
	for _, name := range legacy {
		if name == "capture/coreclr.dll" {
			found = true
		}
	}
	if !found {
		t.Fatal("old verified packages lost their required runtime for rollback")
	}
}

func TestNativeCaptureRejectsMixedManagedPayload(t *testing.T) {
	seen := map[string]bool{"capture/native-runtime.json": true, "capture/coreclr.dll": true}
	if validateCaptureRuntime(seen) == nil {
		t.Fatal("mixed native and managed capture payload was accepted")
	}
	delete(seen, "capture/native-runtime.json")
	if validateCaptureRuntime(seen) != nil {
		t.Fatal("legacy rollback runtime was rejected")
	}
}
