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

export interface Duplicate {
  path: string
  of: string
  inDestination: boolean
}

export interface Plan {
  destination: string
  entries: Entry[]
  skipped: Issue[]
  duplicates: Duplicate[]
}

export type Phase = 'idle' | 'analyzing' | 'ready' | 'copying' | 'done'

export interface TagResult {
  path: string
  tags: string[] | null
  error?: string
}

/** An AI tagging run, which lives on when its panel is closed. */
export interface TagJob {
  id: string
  folder: string
  /** Number of files when the run is limited to the last sort. */
  files: number
  state: 'loading' | 'running' | 'done' | 'error' | 'cancelled'
  done: number
  total: number
  tagged: number
  failed: number
  counts: Record<string, number>
  recent: TagResult[]
  started: number
  message?: string
}
