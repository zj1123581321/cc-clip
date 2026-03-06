package shim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalBinaryHash(t *testing.T) {
	// Create a temp file with known content
	dir := t.TempDir()
	binPath := filepath.Join(dir, "test-binary")
	content := []byte("hello world test binary content")
	if err := os.WriteFile(binPath, content, 0755); err != nil {
		t.Fatalf("failed to write test binary: %v", err)
	}

	hash, err := LocalBinaryHash(binPath)
	if err != nil {
		t.Fatalf("LocalBinaryHash failed: %v", err)
	}

	// Verify format: sha256:<hex>
	if !strings.HasPrefix(hash, "sha256:") {
		t.Fatalf("hash should start with 'sha256:', got %q", hash)
	}

	hexPart := strings.TrimPrefix(hash, "sha256:")
	if len(hexPart) != 64 {
		t.Fatalf("expected 64-char hex hash, got %d chars: %q", len(hexPart), hexPart)
	}

	// Same file should produce same hash (deterministic)
	hash2, err := LocalBinaryHash(binPath)
	if err != nil {
		t.Fatalf("second hash failed: %v", err)
	}
	if hash != hash2 {
		t.Fatalf("same file produced different hashes: %q vs %q", hash, hash2)
	}
}

func TestLocalBinaryHashDifferentContent(t *testing.T) {
	dir := t.TempDir()

	bin1 := filepath.Join(dir, "bin1")
	bin2 := filepath.Join(dir, "bin2")

	os.WriteFile(bin1, []byte("content version 1"), 0755)
	os.WriteFile(bin2, []byte("content version 2"), 0755)

	hash1, err := LocalBinaryHash(bin1)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := LocalBinaryHash(bin2)
	if err != nil {
		t.Fatal(err)
	}

	if hash1 == hash2 {
		t.Fatal("different content should produce different hashes")
	}
}

func TestLocalBinaryHashNonExistent(t *testing.T) {
	_, err := LocalBinaryHash("/nonexistent/path/binary")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestNeedsUpload(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "test-binary")
	os.WriteFile(binPath, []byte("binary content"), 0755)

	hash, _ := LocalBinaryHash(binPath)

	tests := []struct {
		name   string
		remote *DeployState
		want   bool
	}{
		{
			name:   "nil remote state",
			remote: nil,
			want:   true,
		},
		{
			name: "matching hash",
			remote: &DeployState{
				BinaryHash: hash,
			},
			want: false,
		},
		{
			name: "different hash",
			remote: &DeployState{
				BinaryHash: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			},
			want: true,
		},
		{
			name: "empty hash in remote state",
			remote: &DeployState{
				BinaryHash: "",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsUpload(binPath, tt.remote)
			if got != tt.want {
				t.Errorf("NeedsUpload() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsUploadNonExistentBinary(t *testing.T) {
	// If local binary doesn't exist, NeedsUpload returns true
	remote := &DeployState{BinaryHash: "sha256:abc"}
	got := NeedsUpload("/nonexistent/binary", remote)
	if !got {
		t.Error("expected NeedsUpload=true when local binary doesn't exist")
	}
}

func TestNeedsShimInstall(t *testing.T) {
	tests := []struct {
		name   string
		remote *DeployState
		want   bool
	}{
		{
			name:   "nil remote state",
			remote: nil,
			want:   true,
		},
		{
			name: "shim not installed",
			remote: &DeployState{
				ShimInstalled: false,
			},
			want: true,
		},
		{
			name: "shim installed",
			remote: &DeployState{
				ShimInstalled: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsShimInstall(tt.remote)
			if got != tt.want {
				t.Errorf("NeedsShimInstall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeployStateJSON(t *testing.T) {
	state := DeployState{
		BinaryHash:    "sha256:abc123",
		BinaryVersion: "v0.1.0",
		ShimInstalled: true,
		ShimTarget:    "xclip",
		PathFixed:     true,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded DeployState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.BinaryHash != state.BinaryHash {
		t.Errorf("BinaryHash mismatch: %q vs %q", decoded.BinaryHash, state.BinaryHash)
	}
	if decoded.BinaryVersion != state.BinaryVersion {
		t.Errorf("BinaryVersion mismatch: %q vs %q", decoded.BinaryVersion, state.BinaryVersion)
	}
	if decoded.ShimInstalled != state.ShimInstalled {
		t.Errorf("ShimInstalled mismatch: %v vs %v", decoded.ShimInstalled, state.ShimInstalled)
	}
	if decoded.ShimTarget != state.ShimTarget {
		t.Errorf("ShimTarget mismatch: %q vs %q", decoded.ShimTarget, state.ShimTarget)
	}
	if decoded.PathFixed != state.PathFixed {
		t.Errorf("PathFixed mismatch: %v vs %v", decoded.PathFixed, state.PathFixed)
	}
}

func TestDeployStateJSONFromRaw(t *testing.T) {
	// Simulate reading from a remote file
	raw := `{
  "binary_hash": "sha256:deadbeef",
  "binary_version": "v0.2.0",
  "shim_installed": true,
  "shim_target": "wl-paste",
  "path_fixed": false
}`

	var state DeployState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		t.Fatalf("failed to unmarshal raw JSON: %v", err)
	}

	if state.BinaryHash != "sha256:deadbeef" {
		t.Errorf("unexpected BinaryHash: %q", state.BinaryHash)
	}
	if state.BinaryVersion != "v0.2.0" {
		t.Errorf("unexpected BinaryVersion: %q", state.BinaryVersion)
	}
	if !state.ShimInstalled {
		t.Error("expected ShimInstalled=true")
	}
	if state.ShimTarget != "wl-paste" {
		t.Errorf("unexpected ShimTarget: %q", state.ShimTarget)
	}
	if state.PathFixed {
		t.Error("expected PathFixed=false")
	}
}

func TestDeployStateCorruptedJSON(t *testing.T) {
	// Corrupted JSON should not parse
	raw := `{broken json`
	var state DeployState
	err := json.Unmarshal([]byte(raw), &state)
	if err == nil {
		t.Error("expected error for corrupted JSON")
	}
}
