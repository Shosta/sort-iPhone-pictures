import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'

interface Props<T> {
  items: T[]
  itemKey: (item: T) => string
  render: (item: T, size: number) => ReactNode
  /** Tiles are at least this wide; columns stretch to fill the width. */
  minSize?: number
  gap?: number
  /** Extra rows rendered above and below the visible ones. */
  overscan?: number
  children?: ReactNode
}

/**
 * A grid that only mounts the tiles near the visible area, so thousands of
 * photos scroll smoothly. Tiles are absolutely positioned: when an item
 * leaves, the following ones glide to their new place.
 */
export function VirtualGrid<T>({ items, itemKey, render, minSize = 96, gap = 8, overscan = 3, children }: Props<T>): JSX.Element {
  const ref = useRef<HTMLDivElement>(null)
  const [view, setView] = useState({ width: 0, height: 0, top: 0 })

  useLayoutEffect(() => {
    const element = ref.current
    if (!element) return
    const measure = (): void => {
      // The padding leaves room for the hover zoom of the edge tiles.
      const style = getComputedStyle(element)
      const width = element.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight)
      const height = element.clientHeight
      const top = Math.max(0, element.scrollTop - parseFloat(style.paddingTop))
      setView((previous) => (previous.width === width && previous.height === height && previous.top === top ? previous : { width, height, top }))
    }
    let frame = 0
    const onScroll = (): void => {
      if (!frame) frame = requestAnimationFrame(() => {
        frame = 0
        measure()
      })
    }
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    element.addEventListener('scroll', onScroll, { passive: true })
    measure()
    return () => {
      observer.disconnect()
      element.removeEventListener('scroll', onScroll)
      cancelAnimationFrame(frame)
    }
  }, [])

  const columns = Math.max(1, Math.floor((view.width + gap) / (minSize + gap)))
  const size = view.width > 0 ? Math.floor((view.width - gap * (columns - 1)) / columns) : minSize
  const step = size + gap
  const rows = Math.ceil(items.length / columns)
  const firstRow = Math.max(0, Math.floor(view.top / step) - overscan)
  const lastRow = Math.min(rows, Math.ceil((view.top + view.height) / step) + overscan)
  const start = firstRow * columns
  const visible = items.slice(start, lastRow * columns)

  return (
    <div ref={ref} className="vgrid">
      <div className="vgrid-space" style={{ height: rows > 0 ? rows * step - gap : 0 }}>
        {visible.map((item, offset) => {
          const index = start + offset
          const key = itemKey(item)
          return (
            <div
              key={key}
              className="cell"
              style={{ width: size, height: size, transform: `translate(${(index % columns) * step}px, ${Math.floor(index / columns) * step}px)` }}
            >
              {render(item, size)}
            </div>
          )
        })}
      </div>
      {children}
    </div>
  )
}
