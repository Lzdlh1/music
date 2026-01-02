import pytest
from pathlib import Path
from src.app.uploaders import rclone_copy

def test_rclone_copy_raises_without_remote(tmp_path):
    p = tmp_path / 'x.txt'
    p.write_text('x')
    import os
    # Ensure env not set
    os.environ.pop('RCLONE_REMOTE', None)
    with pytest.raises(RuntimeError):
        rclone_copy(p)
