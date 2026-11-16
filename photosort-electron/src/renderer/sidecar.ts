type Listener = (event: any) => void

const listeners = new Map<string, Listener>()
let counter = 0
let started = false
let fatal: Listener | null = null

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
    listener(event)
    if (event.type === 'plan' || event.type === 'done' || event.type === 'error' || event.type === 'thumbnail') {
      listeners.delete(id)
    }
  })
  window.photosort.send({ ...payload, id })
  return {
    id,
    cancel: () => window.photosort.send({ op: 'cancel', id: String(++counter), target: id })
  }
}

const cache = new Map<string, Promise<string | null>>()

export function thumbnail(path: string, size = 200): Promise<string | null> {
  let promise = cache.get(path)
  if (!promise) {
    promise = new Promise((resolve) => {
      request({ op: 'thumbnail', path, size }, (event) => resolve(event.type === 'thumbnail' ? event.dataUrl : null))
    })
    cache.set(path, promise)
  }
  return promise
}
