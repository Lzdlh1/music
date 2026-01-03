package services

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// RcloneCmd is injectable for tests (defaults to "rclone")
var RcloneCmd = "rclone"

// RcloneCopy copies local file to remote (RCLONE_REMOTE like "myremote:path")
func RcloneCopy(localPath string, remote string) error {
	if remote == "" {
		return errors.New("RCLONE_REMOTE is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, RcloneCmd, "copy", localPath, remote)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(string(out) + ": " + err.Error())
	}
	if strings.Contains(strings.ToUpper(string(out)), "ERROR") {
		return errors.New(string(out))
	}
	return nil
}
