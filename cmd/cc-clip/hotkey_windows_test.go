//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestStopHotkeyProcessWritesStopSentinel(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "hotkey.pid")
	stopFile := filepath.Join(tmpDir, "hotkey.stop")

	hotkeyPIDPathOverride = pidFile
	hotkeyStopFilePathOverride = stopFile
	origFunc := localProcessCommandFunc
	t.Cleanup(func() {
		hotkeyPIDPathOverride = ""
		hotkeyStopFilePathOverride = ""
		localProcessCommandFunc = origFunc
	})

	// Mock localProcessCommand to always report "hotkey" in command line
	// so stopHotkeyProcess doesn't refuse to kill.
	localProcessCommandFunc = func(pid int) (string, error) {
		return "cc-clip.exe hotkey --run-loop", nil
	}

	// Start a real subprocess that we can kill.
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Start-Sleep 60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start child process: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// Write the PID file pointing to our subprocess.
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		t.Fatalf("write PID file: %v", err)
	}

	// Call stopHotkeyProcess — should write stop sentinel before killing.
	stopHotkeyProcess()

	// Assert: stop sentinel was created.
	if _, err := os.Stat(stopFile); os.IsNotExist(err) {
		t.Fatal("expected stop sentinel file to be created, but it does not exist")
	}

	// Assert: PID file was removed.
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatal("expected PID file to be removed after stop")
	}
}

func TestStopHotkeyProcessNoopWhenNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "hotkey.pid")
	stopFile := filepath.Join(tmpDir, "hotkey.stop")

	hotkeyPIDPathOverride = pidFile
	hotkeyStopFilePathOverride = stopFile
	origFunc := localProcessCommandFunc
	t.Cleanup(func() {
		hotkeyPIDPathOverride = ""
		hotkeyStopFilePathOverride = ""
		localProcessCommandFunc = origFunc
	})

	localProcessCommandFunc = func(pid int) (string, error) {
		return "cc-clip.exe hotkey --run-loop", nil
	}

	// No PID file — stopHotkeyProcess should just print "not running".
	stopHotkeyProcess()

	// Stop sentinel should NOT be created when nothing was running.
	if _, err := os.Stat(stopFile); !os.IsNotExist(err) {
		t.Fatal("expected no stop sentinel file when hotkey is not running")
	}
}
