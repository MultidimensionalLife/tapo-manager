import { useEffect, useRef, useState } from 'react'
import type { Camera } from '../types'

interface Props {
  camera: Camera
  onDelete: (camera: Camera) => void
}

// IP cameras overwhelmingly encode standard 8-bit 4:2:0 H.264 (Main/High
// profile, level <= 4.1); this codec string is a capability check for
// fMP4-over-MSE support, not a strict match against the actual stream, and
// works broadly across camera H.264 profiles in practice.
const MIME = 'video/mp4; codecs="avc1.640029, mp4a.40.2"'

// Bound how much a live view accumulates in the SourceBuffer: trim back to
// TRIM_TO_SECONDS whenever the buffered range exceeds MAX_BUFFER_SECONDS, so
// memory doesn't grow unbounded on an always-on camera tile.
const MAX_BUFFER_SECONDS = 30
const TRIM_TO_SECONDS = 15

// If playback falls this far behind the buffered live edge (e.g. after a
// tab was backgrounded), jump forward instead of catching up in real time.
const LIVE_EDGE_SLACK_SECONDS = 4

const RECONNECT_DELAY_MS = 1000

export default function CameraPlayer({ camera, onDelete }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [error, setError] = useState<string | null>(null)
  const [deleting, setDeleting] = useState(false)

  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    if (!('MediaSource' in window) || !MediaSource.isTypeSupported(MIME)) {
      setError('Live video playback is not supported in this browser')
      return
    }

    let cancelled = false
    let ws: WebSocket | null = null
    let mediaSource: MediaSource | null = null
    let sourceBuffer: SourceBuffer | null = null
    let objectUrl: string | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    const queue: ArrayBuffer[] = []
    let busy = false

    const onUpdateEnd = () => {
      busy = false
      pump()
    }

    const pump = () => {
      if (busy || cancelled || !sourceBuffer || sourceBuffer.updating) return
      if (queue.length === 0) {
        maybeTrim()
        return
      }
      const chunk = queue.shift()!
      busy = true
      try {
        sourceBuffer.appendBuffer(chunk)
      } catch {
        busy = false
      }
    }

    const maybeTrim = () => {
      if (!sourceBuffer || sourceBuffer.updating || sourceBuffer.buffered.length === 0) return
      const start = sourceBuffer.buffered.start(0)
      const end = sourceBuffer.buffered.end(sourceBuffer.buffered.length - 1)
      if (end - start <= MAX_BUFFER_SECONDS) return
      busy = true
      try {
        sourceBuffer.remove(start, end - TRIM_TO_SECONDS)
      } catch {
        busy = false
      }
    }

    const catchUpToLiveEdge = () => {
      if (!sourceBuffer || sourceBuffer.buffered.length === 0) return
      const end = sourceBuffer.buffered.end(sourceBuffer.buffered.length - 1)
      if (end - video.currentTime > LIVE_EDGE_SLACK_SECONDS) {
        video.currentTime = end - 0.5
      }
    }

    const teardown = () => {
      if (ws) {
        ws.onclose = null
        ws.onerror = null
        ws.onmessage = null
        ws.close()
        ws = null
      }
      if (sourceBuffer) {
        sourceBuffer.removeEventListener('updateend', onUpdateEnd)
        sourceBuffer = null
      }
      mediaSource = null
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl)
        objectUrl = null
      }
      queue.length = 0
      busy = false
    }

    const scheduleReconnect = () => {
      if (cancelled) return
      reconnectTimer = setTimeout(connect, RECONNECT_DELAY_MS)
    }

    const connect = () => {
      if (cancelled) return

      mediaSource = new MediaSource()
      objectUrl = URL.createObjectURL(mediaSource)
      video.src = objectUrl

      mediaSource.addEventListener(
        'sourceopen',
        () => {
          if (cancelled || !mediaSource) return

          try {
            sourceBuffer = mediaSource.addSourceBuffer(MIME)
          } catch {
            setError('failed to initialize video buffer')
            return
          }
          sourceBuffer.mode = 'sequence'
          sourceBuffer.addEventListener('updateend', onUpdateEnd)

          const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
          ws = new WebSocket(`${proto}//${window.location.host}${camera.streamUrl}`)
          ws.binaryType = 'arraybuffer'

          ws.onmessage = (event) => {
            setError(null)
            queue.push(event.data as ArrayBuffer)
            pump()
          }
          ws.onclose = () => {
            if (cancelled) return
            setError('connection lost, reconnecting…')
            teardown()
            scheduleReconnect()
          }
          ws.onerror = () => {
            ws?.close()
          }
        },
        { once: true },
      )
    }

    video.addEventListener('timeupdate', catchUpToLiveEdge)
    connect()

    return () => {
      cancelled = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      video.removeEventListener('timeupdate', catchUpToLiveEdge)
      teardown()
      video.removeAttribute('src')
      video.load()
    }
  }, [camera.streamUrl])

  async function handleDelete() {
    if (!window.confirm(`Delete camera "${camera.name}"?`)) return
    setDeleting(true)
    setError(null)
    try {
      await onDelete(camera)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed to delete camera')
      setDeleting(false)
    }
  }

  return (
    <div className="camera-player">
      <div className="camera-player__header">
        <span>{camera.name}</span>
        <button
          type="button"
          className="camera-player__delete"
          onClick={handleDelete}
          disabled={deleting}
          aria-label={`Delete ${camera.name}`}
          title={`Delete ${camera.name}`}
        >
          {deleting ? '…' : '✕'}
        </button>
      </div>
      <video ref={videoRef} className="camera-player__video" autoPlay muted playsInline controls />
      {error && <div className="camera-player__error">{error}</div>}
    </div>
  )
}
