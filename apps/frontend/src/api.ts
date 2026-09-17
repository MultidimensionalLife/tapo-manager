import type { Camera, NewCamera } from './types'

export async function fetchCameras(): Promise<Camera[]> {
  const res = await fetch('/api/cameras')
  if (!res.ok) {
    throw new Error(`failed to load cameras: ${res.status}`)
  }
  return res.json()
}

export async function addCamera(camera: NewCamera): Promise<Camera> {
  const res = await fetch('/api/cameras', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(camera),
  })
  if (!res.ok) {
    const detail = await res.text()
    throw new Error(detail || `failed to add camera: ${res.status}`)
  }
  return res.json()
}

export async function deleteCamera(id: string): Promise<void> {
  const res = await fetch(`/api/cameras/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) {
    const detail = await res.text()
    throw new Error(detail || `failed to delete camera: ${res.status}`)
  }
}
