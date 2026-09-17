# tapo-manager

Monorepo for viewing Tapo (or any RTSP) camera feeds in the browser.

- `apps/backend` — Go server. Pulls each camera's RTSP feed and remuxes it to
  fragmented MP4 via `ffmpeg`, broadcasting it live to viewers over a
  WebSocket, and serves the camera list over HTTP. (Browsers can't play RTSP
  directly, so this bridges it to something a `<video>` element can play.)
- `apps/frontend` — React + TypeScript (Vite) app. Fetches the camera list
  and plays each stream by feeding the WebSocket's fMP4 chunks into a
  MediaSource-backed `<video>` element.

Managed as a pnpm workspace (`pnpm-workspace.yaml` → `apps/*`); the Go module
isn't a pnpm package, it just lives alongside the frontend in `apps/`.

## Architecture

```
IP cameras (RTSP)         backend (Go)                    frontend (React)
┌────────────┐   TCP   ┌───────────────────┐    WS    ┌───────────────────┐
│  Tapo cam  │────────▶│ ffmpeg (1/camera) │─────────▶│ MediaSource-backed │
│  (LAN)     │         │ -f mp4, fragments │          │  <video> element   │
└────────────┘         ├───────────────────┤   HTTP   ├───────────────────┤
                        │  camera.Store      │◀────────▶│ camera list,       │
                        │  (base + runtime)  │          │ add/delete forms   │
                        └─────────┬──────────┘          └───────────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    ▼                            ▼
         cameras.json (base,          cameras.runtime.json
         read-only, gitignored)       (API-added, camera-data volume)
```

- **`apps/backend/internal/stream`** — one ffmpeg process per camera,
  restarted with exponential backoff on failure. Its fragmented-MP4 stdout
  is fanned out live to WebSocket viewers by a per-camera `hub`
  (`hub.go`); a viewer that connects after the stream already started still
  gets the cached init segment first (`fmp4.go`).
- **`apps/backend/internal/camera`** — the in-memory camera registry
  (`Store`). Cameras loaded from the base config file are immutable via the
  API; cameras added through `POST /api/cameras` live in a separate,
  writable file, so the base file (often a credentials file you'd want
  mounted read-only) never needs to be written back to.
- **`apps/backend/internal/httpapi`** — the HTTP/WebSocket surface:
  `GET/POST/DELETE /api/cameras[/id]` for the camera list, and
  `GET /ws/cameras/<id>` for live video.
- **`apps/frontend`** — fetches the camera list once, then opens one
  WebSocket per camera and feeds the incoming fMP4 chunks straight into a
  `MediaSource`/`SourceBuffer` on that camera's `<video>` element
  (`src/components/CameraPlayer.tsx`).

See "How streaming works" below for the RTSP → browser data flow in detail.

## Prerequisites

- Node.js 20+, pnpm 9+
- Go 1.23+ (only needed to run the backend outside Docker)
- `ffmpeg` on PATH (only needed to run the backend outside Docker)

## Configure cameras

```bash
cp apps/backend/cameras.example.json apps/backend/cameras.json
```

Edit `apps/backend/cameras.json` with your cameras' RTSP URLs. For Tapo
cameras, enable "Camera Account" (Advanced Settings → Camera Account) in the
Tapo app, then use:

```
rtsp://<camAccountUser>:<camAccountPass>@<camera-ip>:554/stream1
```

(`stream1` = HD, `stream2` = SD). `cameras.json` is gitignored since it holds
credentials.

## Run locally (no Docker)

```bash
pnpm install

# terminal 1
pnpm dev:backend

# terminal 2
pnpm dev:frontend
```

Frontend dev server runs on http://localhost:5173 and proxies `/api` and
`/ws` to the backend on http://localhost:8080.

Other root scripts: `build:frontend`, `lint:frontend`, `build:backend`,
`test:backend`.

## Run with Docker Compose

```bash
cp apps/backend/cameras.example.json apps/backend/cameras.json
# edit apps/backend/cameras.json first — it's bind-mounted into the
# container, so it must exist before `docker compose up` or Docker will
# create it as an empty directory instead.

docker compose up --build
```

- Frontend: http://localhost:3000 (nginx, also proxies `/api` and `/ws` to
  the backend container)
- Backend directly: http://localhost:8081

Cameras added at runtime (via the "+ Add camera" button, as opposed to the
bind-mounted `cameras.json`) are persisted to a named volume (`camera-data`)
inside the backend container, so they survive container restarts.

## How streaming works

1. On boot, the backend starts one `ffmpeg -rtsp_transport tcp -i <rtsp-url>
   ... -f mp4 -movflags frag_keyframe+empty_moov+default_base_moof pipe:1`
   process per configured camera, restarting it with backoff if the camera
   drops off the network.
2. Each camera's fragmented-MP4 stdout is broadcast live to any connected
   viewers over `GET /ws/cameras/<id>` (a WebSocket). `GET /api/cameras`
   returns each camera's id/name and that WebSocket path; RTSP URLs and
   credentials are never sent to the frontend.
3. The frontend connects to that WebSocket and appends the incoming chunks
   to a `MediaSource`-backed `<video>` element (no hls.js, no polling) — the
   backend pushes new video data the moment ffmpeg produces it, rather than
   the frontend pulling a playlist on an interval.

This favors lower latency and no per-viewer polling over the operational
simplicity of plain file-based HLS. If you later need sub-second latency
(this setup is closer to 1-3s, gated by the camera's own keyframe interval),
swap the backend's ffmpeg/MSE pipeline for WebRTC (e.g. `pion/webrtc`)
without touching the frontend's camera-list contract.
