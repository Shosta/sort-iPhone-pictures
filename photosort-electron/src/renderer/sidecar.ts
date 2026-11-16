type Listener = (event: any) => void

const listeners = new Map<string, Listener>()
let counter = 0
let started = false
let fatal: Listener | null = null

const TERMINAL = new Set(['plan', 'done', 'error', 'thumbnail', 'weekDays', 'photos', 'layout'])

function start(): void {
  if (started) return
  started = true
  window.photosort.onEvent((event) => {
    const listener = listeners.get(event.id)
    if (listener) listener(event)
    else if (event.type === 'error' && fatal) fatal(event)
  })
}

export function onFatal(listener: Listener): void {
  start()
  fatal = listener
}

export function request(
  payload: Record<string, unknown>,
  listener: Listener
): { id: string; cancel: () => void } {
  start()
  const id = String(++counter)
  listeners.set(id, (event) => {
    if (TERMINAL.has(event.type)) listeners.delete(id)
    listener(event)
  })
  window.photosort.send({ ...payload, id })
  return {
    id,
    cancel: () => window.photosort.send({ op: 'cancel', id: String(++counter), target: id })
  }
}

// Thumbnails: one size for every tile (it matches the previews the engine
// caches while planning), a bounded LRU of data URLs, and a LIFO queue so the
// tiles the user is looking at right now are decoded first. Requests for tiles
// scrolled out of view are dropped or cancelled.
export const THUMB_SIZE = 256
const CACHE_LIMIT = 1500
const IN_FLIGHT = Math.max(4, Math.min(navigator.hardwareConcurrency || 8, 12))

type Waiter = (url: string | null) => void
const cache = new Map<string, string | null>()
const waiters = new Map<string, Set<Waiter>>()
const queue: string[] = []
const inFlight = new Map<string, { cancel: () => void }>()

function remember(path: string, url: string | null): void {
  cache.delete(path)
  cache.set(path, url)
  if (cache.size > CACHE_LIMIT) cache.delete(cache.keys().next().value as string)
}

function settle(path: string, url: string | null): void {
  remember(path, url)
  const pending = waiters.get(path)
  waiters.delete(path)
  pending?.forEach((waiter) => waiter(url))
}

function pump(): void {
  while (inFlight.size < IN_FLIGHT && queue.length > 0) {
    const path = queue.pop() as string
    const handle = request({ op: 'thumbnail', path, size: THUMB_SIZE }, (event) => {
      const current = inFlight.get(path) === handle
      if (current) inFlight.delete(path)
      if (event.type === 'thumbnail') settle(path, event.dataUrl)
      else if (current && !event.cancelled) settle(path, null)
      pump()
    })
    inFlight.set(path, handle)
  }
}

export function cachedThumbnail(path: string): string | null | undefined {
  return cache.get(path)
}

/** Loads a thumbnail; the returned function drops the request if still pending. */
export function loadThumbnail(path: string, onReady: Waiter): () => void {
  const hit = cache.get(path)
  if (hit !== undefined) {
    remember(path, hit)
    onReady(hit)
    return () => undefined
  }
  let pending = waiters.get(path)
  if (!pending) waiters.set(path, (pending = new Set()))
  pending.add(onReady)
  if (!inFlight.has(path)) {
    const index = queue.lastIndexOf(path)
    if (index >= 0) queue.splice(index, 1)
    queue.push(path)
    pump()
  }
  return () => {
    const set = waiters.get(path)
    if (!set) return
    set.delete(onReady)
    if (set.size > 0) return
    waiters.delete(path)
    const index = queue.lastIndexOf(path)
    if (index >= 0) queue.splice(index, 1)
    const running = inFlight.get(path)
    if (running) {
      inFlight.delete(path)
      running.cancel()
      pump()
    }
  }
}
