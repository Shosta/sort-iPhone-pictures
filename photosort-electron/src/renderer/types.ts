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

/** A photo found in the sorted library, with the day it was taken (YYYY-MM-DD). */
export interface ScrapPhoto {
  path: string
  date: string
}

/** Position of a photo on a sheet, in millimetres from the top-left corner. */
export interface ScrapPlacement {
  path: string
  x: number
  y: number
  w: number
  h: number
  /** Turn of the piece in degrees, counter-clockwise, around its centre. */
  angle: number
  /** The piece before turning, relative to the box above. */
  ux: number
  uy: number
  uw: number
  uh: number
  /** The photo inside its frame, relative to the piece. */
  px: number
  py: number
  pw: number
  ph: number
}

export interface ScrapLayout {
  pageWidth: number
  pageHeight: number
  background: string
  sheets: { placements: ScrapPlacement[] }[]
  failed: { path: string; error: string }[]
}

/** The PDF being written, or the last one written. */
export interface ScrapJob {
  state: 'running' | 'done' | 'error'
  done: number
  total: number
  path?: string
  pages?: number
  photos?: number
  /** The chosen folder was unusable: the PDF went to Downloads. */
  fallback?: boolean
  failed?: { path: string; error: string }[]
  message?: string
}

export interface YearPin {
  x: number
  y: number
  name: string
  count: number
  shown: string[]
  /** The card of photos and name, in millimetres; null when it did not fit. */
  card: { x: number; y: number; w: number; h: number } | null
}

/** The map of the year drawn for a page, in millimetres. */
export interface YearMapLayout {
  pageWidth: number
  pageHeight: number
  land: string
  lakes: string
  pins: YearPin[]
}

export interface YearPlace {
  name: string
  lat: number
  lon: number
  count: number
  shown: string[]
}

export interface YearScan {
  places: YearPlace[]
  total: number
  located: number
}
