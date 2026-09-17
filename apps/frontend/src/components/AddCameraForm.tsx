import { useState } from 'react'
import type { FormEvent } from 'react'
import { addCamera } from '../api'
import type { Camera } from '../types'

interface Props {
  onAdded: (camera: Camera) => void
}

// Tapo cameras serve RTSP on the standard port 554, with the HD feed at
// /stream1 and the SD feed at /stream2. If the address includes its own
// port (e.g. a non-Tapo camera on a custom port), that's used instead.
const RTSP_PORT = '554'

function buildRtspUrl(address: string, stream: string): string {
  const hasPort = /:\d+$/.test(address)
  return `rtsp://${address}${hasPort ? '' : `:${RTSP_PORT}`}/${stream}`
}

export default function AddCameraForm({ onAdded }: Props) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [address, setAddress] = useState('')
  const [stream, setStream] = useState('stream1')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function reset() {
    setName('')
    setAddress('')
    setStream('stream1')
    setUsername('')
    setPassword('')
    setError(null)
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      const camera = await addCamera({
        name,
        rtspUrl: buildRtspUrl(address.trim(), stream),
        username: username || undefined,
        password: password || undefined,
      })
      onAdded(camera)
      reset()
      setOpen(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed to add camera')
    } finally {
      setSubmitting(false)
    }
  }

  if (!open) {
    return (
      <button type="button" className="btn" onClick={() => setOpen(true)}>
        + Add camera
      </button>
    )
  }

  return (
    <form className="add-camera-form" onSubmit={handleSubmit}>
      <div className="add-camera-form__row">
        <label>
          Name
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Front Door"
            required
          />
        </label>
        <label>
          IP address
          <input
            type="text"
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="192.168.1.50"
            pattern="[^/]+"
            title="IP address or hostname, optionally with :port (no rtsp:// or path)"
            required
          />
        </label>
      </div>
      <div className="add-camera-form__row">
        <label>
          Stream
          <select value={stream} onChange={(e) => setStream(e.target.value)}>
            <option value="stream1">Stream 1 (HD)</option>
            <option value="stream2">Stream 2 (SD)</option>
          </select>
        </label>
      </div>
      <div className="add-camera-form__row">
        <label>
          Username <span className="add-camera-form__optional">(optional)</span>
          <input type="text" value={username} onChange={(e) => setUsername(e.target.value)} />
        </label>
        <label>
          Password <span className="add-camera-form__optional">(optional)</span>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
      </div>

      {error && <p className="status status--error">{error}</p>}

      <div className="add-camera-form__actions">
        <button type="submit" className="btn" disabled={submitting}>
          {submitting ? 'Adding…' : 'Add camera'}
        </button>
        <button
          type="button"
          className="btn btn--secondary"
          disabled={submitting}
          onClick={() => {
            reset()
            setOpen(false)
          }}
        >
          Cancel
        </button>
      </div>
    </form>
  )
}
