package services

import (
	"os"
	"os/exec"
	"testing"
)

// Test RcloneCopy by pointing rcloneCmd to a small shell script that echoes success
func TestRcloneCopy_Success(t *testing.T) {
	// create a temp script
	f, err := os.CreateTemp("", "fake-rclone-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	content := "#!/bin/sh\necho 'Success'\nexit 0\n"
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Chmod(0755)
	f.Close()

old := RcloneCmd
	RcloneCmd = f.Name()
	defer func() { RcloneCmd = old }()

	// call RcloneCopy (remote must not be empty)
	if err := RcloneCopy("/tmp/somefile", "myremote:base"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestRcloneCopy_NoRemote(t *testing.T) {
	if err := RcloneCopy("/tmp/x", ""); err == nil {
		t.Fatalf("expected error when remote is empty")
	}
}

func TestRcloneCopy_Failure(t *testing.T) {
	// script that returns non-zero and prints ERROR
	f, err := os.CreateTemp("", "fake-rclone-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	content := "#!/bin/sh\necho 'ERROR something' >&2\nexit 1\n"
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Chmod(0755)
	f.Close()

	old := RcloneCmd
	RcloneCmd = f.Name()
	defer func() { RcloneCmd = old }()

	if err := RcloneCopy("/tmp/somefile", "myremote:base"); err == nil {
		t.Fatalf("expected error when rclone fails")
	}
}

// Ensure exec is available for tests
func init() {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		// tests will fail, but that's fine in non-unix envs
		_ = err
	}
}
