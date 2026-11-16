import type { PhotosortApi } from '../preload'

declare global {
  interface Window {
    photosort: PhotosortApi
  }
}

export interface Entry {
  source: string
  target: string
  metadata: { TakenAt: string; Camera: string }
}

export interface Issue {
  path: string
  reason: string
}

export interface Plan {
  destination: string
  entries: Entry[]
  skipped: Issue[]
}

export type Phase = 'idle' | 'analyzing' | 'ready' | 'copying' | 'done'
