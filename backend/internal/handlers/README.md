Handlers expose:
- POST /api/register
- POST /api/login
- POST /api/tasks (auth required) body: { title, url, cookie? }
- GET /api/tasks (auth required)
- GET /api/tasks/:id (auth required)

Note: cookie is optional and is used only for that single task request; backend will NOT store cookies persistently.
