export interface Camera {
  id: string
  name: string
  streamUrl: string
}

export interface NewCamera {
  name: string
  rtspUrl: string
  username?: string
  password?: string
}
