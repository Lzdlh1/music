from fastapi import FastAPI, Depends, HTTPException, status, BackgroundTasks
from fastapi.security import OAuth2PasswordRequestForm
from fastapi.middleware.cors import CORSMiddleware
from sqlmodel import Session, select
from .db import init_db, engine
from .models import User, Task
from .auth import get_password_hash, verify_password, create_access_token, decode_access_token
from .tasks import process_task
import os
from typing import List

app = FastAPI(title="music-uploader")

app.add_middleware(
    CORSMiddleware,
    allow_origins=[os.getenv("CORS_ALLOW", "*")],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Serve simple web UI from / (set WEB_DIR env if needed)
from fastapi.staticfiles import StaticFiles
WEB_DIR = os.getenv("WEB_DIR", "./web")
if os.path.isdir(WEB_DIR):
    app.mount("/", StaticFiles(directory=WEB_DIR, html=True), name="web")

@app.on_event("startup")
def on_startup():
    init_db()

# --- Auth ---
@app.post('/auth/register')
def register(username: str, password: str):
    with Session(engine) as session:
        q = select(User).where(User.username == username)
        user = session.exec(q).first()
        if user:
            raise HTTPException(status_code=400, detail="User exists")
        u = User(username=username, hashed_password=get_password_hash(password))
        session.add(u)
        session.commit()
        return {"msg": "ok"}

@app.post('/auth/login')
def login(form_data: OAuth2PasswordRequestForm = Depends()):
    with Session(engine) as session:
        q = select(User).where(User.username == form_data.username)
        user = session.exec(q).first()
        if not user or not verify_password(form_data.password, user.hashed_password):
            raise HTTPException(status_code=400, detail="Incorrect username or password")
        token = create_access_token({"sub": user.username})
        return {"access_token": token, "token_type": "bearer"}

# Dep
from fastapi.security import OAuth2PasswordBearer
oauth2_scheme = OAuth2PasswordBearer(tokenUrl="/auth/login")

def get_current_user(token: str = Depends(oauth2_scheme)):
    payload = decode_access_token(token)
    if not payload:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid token")
    username = payload.get("sub")
    with Session(engine) as session:
        q = select(User).where(User.username == username)
        user = session.exec(q).first()
        if not user:
            raise HTTPException(status_code=401, detail="User not found")
        return user

# --- Tasks ---
@app.post('/tasks')
def create_task(url: str, title: str = None, background_tasks: BackgroundTasks = Depends(), user=Depends(get_current_user)):
    t = Task(url=url, title=title or "")
    with Session(engine) as session:
        session.add(t)
        session.commit()
        session.refresh(t)
    # Launch background processing
    background_tasks.add_task(process_task, t.id)
    return {"id": t.id, "status": t.status}

@app.get('/tasks')
def list_tasks(user=Depends(get_current_user)):
    with Session(engine) as session:
        tasks = session.exec(select(Task).order_by(Task.created_at.desc())).all()
        return tasks

@app.get('/tasks/{task_id}')
def get_task(task_id: int, user=Depends(get_current_user)):
    with Session(engine) as session:
        t = session.get(Task, task_id)
        if not t:
            raise HTTPException(status_code=404, detail="Not found")
        return t

# --- Search (GDStudio) ---
@app.get('/search')
async def search(q: str, source: str = 'netease', count: int = 20, pages: int = 1):
    # For public-domain or user-provided URLs only.
    from .downloaders import gdstudio_search_stub
    results = await gdstudio_search_stub(q, source=source, count=count, pages=pages)
    return results

# --- Import a track from GDStudio by track id and source, specify quality (br) ---
from pydantic import BaseModel
class ImportRequest(BaseModel):
    source: str
    id: str
    br: int = 320
    title: str = None

@app.post('/import')
def import_track(req: ImportRequest, background_tasks: BackgroundTasks = Depends(), user=Depends(get_current_user)):
    # Use adapter to resolve track URL and create a task
    from .adapters.gdstudio import GdStudioAdapter
    adapter = GdStudioAdapter()
    # Resolve streaming info synchronously in this context by calling async method
    import asyncio
    info = asyncio.get_event_loop().run_until_complete(adapter.get_track_url(req.source, req.id, req.br))
    url = info.get('url')
    if not url:
        raise HTTPException(status_code=502, detail='Unable to resolve track URL')
    title = req.title or f"{req.source}:{req.id}"
    t = Task(url=url, title=title)
    with Session(engine) as session:
        session.add(t)
        session.commit()
        session.refresh(t)
    background_tasks.add_task(process_task, t.id)
    return {"id": t.id, "status": t.status, "resolved_br": info.get('br'), 'size_kb': info.get('size')}
