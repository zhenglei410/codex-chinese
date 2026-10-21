//go:build windows

package main

import (
	"os"
	"testing"
)

func TestPackageFamilyName(t *testing.T) {
	got := packageFamilyName("OpenAI.Codex_26.1013.4894.0_x64__2p2nqsd0c76g0")
	want := "OpenAI.Codex_2p2nqsd0c76g0"
	if got != want {
		t.Fatalf("packageFamilyName()=%q, want %q", got, want)
	}
}

func TestCodexProcessPathMatchesOnlyTheCodexPackage(t *testing.T) {
	target := `C:\Program Files\WindowsApps\OpenAI.Codex_26.1013.4894.0_x64__2p2nqsd0c76g0\app\ChatGPT.exe`
	if !isCodexProcessPath(target, target) {
		t.Fatal("exact Codex path was not matched")
	}
	if !isCodexProcessPath(
		`C:\Program Files\WindowsApps\OpenAI.Codex_26.1013.5000.0_x64__2p2nqsd0c76g0\app\ChatGPT.exe`,
		target,
	) {
		t.Fatal("another version of the Codex MSIX package was not matched")
	}
	if isCodexProcessPath(`C:\Program Files\ChatGPT\ChatGPT.exe`, target) {
		t.Fatal("an unrelated ChatGPT executable was matched")
	}
}

func TestStorePackageResolvesApplicationID(t *testing.T) {
	exe := findCodex()
	if exe == "" {
		t.Skip("Codex is not installed")
	}
	pkg, ok := storePackageForExecutable(exe)
	if !ok {
		t.Skipf("Codex is not a registered MSIX package: %s", exe)
	}
	if pkg.FullName == "" || pkg.FamilyName == "" || pkg.AppID == "" {
		t.Fatalf("incomplete package identity: %+v", pkg)
	}
}

// This opt-in integration check activates the already-running packaged app.
// It verifies the native COM path without terminating the Codex process hosting
// the test runner.
func TestNativePackagedActivation(t *testing.T) {
	if os.Getenv("CODEX_CHINESE_ACTIVATION_TEST") != "1" {
		t.Skip("set CODEX_CHINESE_ACTIVATION_TEST=1 to activate the installed Codex app")
	}
	exe := findCodex()
	pkg, ok := storePackageForExecutable(exe)
	if !ok {
		t.Skip("Codex is not a registered MSIX package")
	}
	aumid := pkg.FamilyName + "!" + pkg.AppID
	t.Logf("activating %s", aumid)
	if _, err := activatePackagedApp(aumid, "--remote-debugging-port=9341 --remote-allow-origins=http://127.0.0.1:9341"); err != nil {
		t.Fatal(err)
	}
}
