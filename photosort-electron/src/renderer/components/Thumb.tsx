import { memo, useEffect, useRef, useState } from 'react'
import { cachedThumbnail, loadThumbnail } from '../sidecar'

interface Props {
  path: string
  label?: string
  size?: number
  pending?: boolean
  /** Plays the appear animation, after this many seconds. */
  appearDelay?: number
  /** Load only while on screen (for tiles that are not virtualized). */
  lazy?: boolean
  /** Shown when the image cannot be decoded. */
  fallback?: string
}

export const Thumb = memo(function Thumb({ path, label, size, pending = false, appearDelay, lazy = false, fallback }: Props): JSX.Element {
  const ref = useRef<HTMLDivElement>(null)
  const [url, setUrl] = useState<string | null | undefined>(() => cachedThumbnail(path))

  useEffect(() => {
    const known = cachedThumbnail(path)
    setUrl(known)
    if (known !== undefined) return
    let drop: (() => void) | null = null
    const load = (): void => {
      if (!drop) drop = loadThumbnail(path, setUrl)
    }
    if (!lazy || !ref.current) {
      load()
      return () => drop?.()
    }
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) load()
      else if (drop) {
        drop()
        drop = null
      }
    })
    observer.observe(ref.current)
    return () => {
      observer.disconnect()
      drop?.()
    }
  }, [path, lazy])

  return (
    <div
      ref={ref}
      data-pending={pending ? path : undefined}
      className={`thumb${appearDelay !== undefined ? ' appear' : ''}`}
      style={{ width: size, height: size, animationDelay: appearDelay ? `${appearDelay}s` : undefined }}
      title={label ?? path}
    >
      {url === undefined && <div className="shimmer" />}
      {url === null && <span className="thumb-missing">{fallback ?? '🖼'}</span>}
      {url && <img src={url} alt="" draggable={false} decoding="async" />}
    </div>
  )
})
