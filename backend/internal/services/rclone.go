package services

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"log"
)

// RcloneCmd is injectable for tests (defaults to "rclone")
var RcloneCmd = "rclone"

// RcloneCopy copies local file to remote (RCLONE_REMOTE like "myremote:path").
// Special dev helper: if remote starts with "DIRECT:", copy directly to the filesystem path after the prefix.
func RcloneCopy(localPath string, remote string) error {
	if remote == "" {
		return errors.New("RCLONE_REMOTE is not configured")
	}
	if strings.HasPrefix(remote, "DIRECT:") {
		destDir := strings.TrimPrefix(remote, "DIRECT:")
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return err
		}
		dst := filepath.Join(destDir, filepath.Base(localPath))
		srcF, err := os.Open(localPath)
		if err != nil {
			return err
		}
		defer srcF.Close()
		dstF, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer dstF.Close()
		if _, err := io.Copy(dstF, srcF); err != nil {
			return err
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, RcloneCmd, "copy", localPath, remote)
	log.Printf("RcloneCopy running command: %s %v", RcloneCmd, cmd.Args)
	out, err := cmd.CombinedOutput()
	log.Printf("RcloneCopy output: %s, err: %v", string(out), err)
	if err != nil {
		return errors.New(string(out) + ": " + err.Error())
	}
	if strings.Contains(strings.ToUpper(string(out)), "ERROR") {
		return errors.New(string(out))
	}
	return nil
}
