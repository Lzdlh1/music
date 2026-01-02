import subprocess
from pathlib import Path
from typing import Optional

RCLONE_REMOTE = None  # Expect user to set env RCLONE_REMOTE like "myremote:my/path"


def rclone_copy(local_path: Path, remote: Optional[str] = None) -> subprocess.CompletedProcess:
    remote_to_use = remote or RCLONE_REMOTE
    if not remote_to_use:
        raise RuntimeError("RCLONE_REMOTE not configured. Set env RCLONE_REMOTE or pass remote param.")
    cmd = ["rclone", "copy", str(local_path), remote_to_use]
    return subprocess.run(cmd, check=False, capture_output=True, text=True)
