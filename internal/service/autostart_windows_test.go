//go:build windows

package service

import (
	"errors"
	"testing"
)

func TestStatusNotInstalled(t *testing.T) {
	origQuery := regQuery
	t.Cleanup(func() { regQuery = origQuery })

	regQuery = func(key, name string) (string, error) {
		return "", errors.New("not found")
	}

	running, err := Status()
	if err == nil {
		t.Fatal("expected error for not-installed status")
	}
	if running {
		t.Fatal("expected not running when not installed")
	}
}

func TestStatusInstalledAndRunning(t *testing.T) {
	origQuery := regQuery
	origChecker := processChecker
	t.Cleanup(func() {
		regQuery = origQuery
		processChecker = origChecker
	})

	regQuery = func(key, name string) (string, error) {
		return "wscript.exe ...", nil
	}
	processChecker = func() (string, error) {
		return `cc-clip.exe serve --port 18339`, nil
	}

	running, err := Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !running {
		t.Fatal("expected running when process has 'serve' in command line")
	}
}

func TestStatusInstalledButNotRunning(t *testing.T) {
	origQuery := regQuery
	origChecker := processChecker
	t.Cleanup(func() {
		regQuery = origQuery
		processChecker = origChecker
	})

	regQuery = func(key, name string) (string, error) {
		return "wscript.exe ...", nil
	}
	processChecker = func() (string, error) {
		return "", errors.New("no process found")
	}

	running, err := Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if running {
		t.Fatal("expected not running when process check fails")
	}
}

func TestStatusInstalledButDifferentProcess(t *testing.T) {
	origQuery := regQuery
	origChecker := processChecker
	t.Cleanup(func() {
		regQuery = origQuery
		processChecker = origChecker
	})

	regQuery = func(key, name string) (string, error) {
		return "wscript.exe ...", nil
	}
	processChecker = func() (string, error) {
		return `cc-clip.exe hotkey --run-loop`, nil
	}

	running, err := Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if running {
		t.Fatal("expected not running when process doesn't have 'serve'")
	}
}
