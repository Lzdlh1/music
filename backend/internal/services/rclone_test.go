package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Test RcloneCopy by pointing RcloneCmd to a small shell script that echoes success
func TestRcloneCopy_Success(t *testing.T) {
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

func TestRcloneCopy_DirectLocal(t *testing.T) {
	src, err := os.CreateTemp("", "src-file-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(src.Name())
	src.WriteString("hi")
	src.Close()

	dir, err := os.MkdirTemp("", "rclone-dest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	if err := RcloneCopy(src.Name(), "DIRECT:"+dir); err != nil {
		t.Fatalf("expected direct copy to succeed, got %v", err)
	}
	// check file exists
	dst := filepath.Join(dir, filepath.Base(src.Name()))
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("expected dest file, got error: %v", err)
	}
}

// Ensure exec is available for tests
func init() {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		// tests will fail, but that's fine in non-unix envs
		_ = err
	}
}
