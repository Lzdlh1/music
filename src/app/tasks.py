from fastapi import BackgroundTasks
from pathlib import Path
import os
from .downloaders import download_to_path
from .uploaders import rclone_copy
from .db import engine
from .models import Task
from sqlmodel import Session, select
from datetime import datetime

DOWNLOAD_DIR = Path(os.getenv("DOWNLOAD_DIR", "./downloads"))

async def process_task(task_id: int):
    """Background runner: download then upload using rclone."""
    with Session(engine) as session:
        task = session.get(Task, task_id)
        if not task:
            return
        try:
            task.status = "downloading"
            task.updated_at = datetime.utcnow()
            session.add(task)
            session.commit()

            filename = Path(task.url).name or f"{task_id}.dat"
            local_path = DOWNLOAD_DIR / str(task_id) / filename
            await download_to_path(task.url, local_path)

            task.local_path = str(local_path)
            task.status = "uploading"
            task.updated_at = datetime.utcnow()
            session.add(task)
            session.commit()

            result = rclone_copy(local_path)
            if result.returncode == 0:
                task.status = "done"
                task.remote_path = os.getenv("RCLONE_REMOTE")
                task.message = result.stdout[:1000]
            else:
                task.status = "failed"
                task.message = result.stderr[:1000]
            task.updated_at = datetime.utcnow()
            session.add(task)
            session.commit()
        except Exception as e:
            task.status = "failed"
            task.message = str(e)
            task.updated_at = datetime.utcnow()
            session.add(task)
            session.commit()
