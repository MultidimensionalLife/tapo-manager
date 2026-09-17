import { useEffect, useState } from 'react'
import { deleteCamera, fetchCameras } from '../api'
import type { Camera } from '../types'
import AddCameraForm from './AddCameraForm'
import CameraPlayer from './CameraPlayer'

export default function CameraGrid() {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetchCameras()
      .then(setCameras)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  function handleAdded(camera: Camera) {
    setCameras((prev) => [...prev, camera])
  }

  async function handleDelete(camera: Camera) {
    await deleteCamera(camera.id)
    setCameras((prev) => prev.filter((c) => c.id !== camera.id))
  }

  if (loading) return <p className="status">Loading cameras…</p>
  if (error) return <p className="status status--error">{error}</p>

  return (
    <div className="camera-grid-wrap">
      <div className="camera-grid-wrap__toolbar">
        <AddCameraForm onAdded={handleAdded} />
      </div>

      {cameras.length === 0 ? (
        <p className="status">No cameras configured yet. Add one above.</p>
      ) : (
        <div className="camera-grid">
          {cameras.map((camera) => (
            <CameraPlayer key={camera.id} camera={camera} onDelete={handleDelete} />
          ))}
        </div>
      )}
    </div>
  )
}
