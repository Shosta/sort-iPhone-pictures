import { useEffect, useRef } from 'react'

export interface Flight {
  id: string
  /** Thumbnail already in cache: flights never trigger a decode. */
  url: string
  from: { x: number; y: number; size: number }
  to: { x: number; y: number }
  delay: number
}

const DURATION = 1.1

function Drop({ flight, onDone }: { flight: Flight; onDone: (id: string) => void }): JSX.Element {
  const ref = useRef<HTMLImageElement>(null)
  const { url, from, to } = flight
  const half = from.size / 2
  const dx = to.x - from.x
  const dy = to.y - from.y

  // Web Animations keep the whole flight on the compositor (transform and
  // opacity only), so a dozen photos in the air cost almost no main-thread time.
  useEffect(() => {
    const element = ref.current
    if (!element) return
    const move = (x: number, y: number, scale: number, rotate: number): string => `translate(${x}px, ${y}px) scale(${scale}) rotate(${rotate}deg)`
    const animation = element.animate(
      [
        { transform: move(0, 0, 1, 0), opacity: 1, offset: 0 },
        { transform: move(dx * 0.55, dy * 0.25, 1.12, -4), opacity: 1, offset: 0.6, easing: 'cubic-bezier(0.3, 0, 0.3, 1)' },
        { transform: move(dx, dy, 0.35, 0), opacity: 0.9, offset: 0.85 },
        { transform: move(dx, dy, 0.35, 0), opacity: 0, offset: 1 }
      ],
      { duration: DURATION * 1000, delay: flight.delay * 1000, easing: 'cubic-bezier(0.45, 0.05, 0.25, 1)', fill: 'both' }
    )
    animation.onfinish = () => onDone(flight.id)
    return () => animation.cancel()
  }, [flight, dx, dy, onDone])

  return (
    <img
      ref={ref}
      src={url}
      alt=""
      className="flight"
      style={{ left: from.x - half, top: from.y - half, width: from.size, height: from.size, opacity: 0 }}
    />
  )
}

export function Flights({ flights, onDone }: { flights: Flight[]; onDone: (id: string) => void }): JSX.Element {
  return (
    <div className="flights" aria-hidden>
      {flights.map((flight) => (
        <Drop key={flight.id} flight={flight} onDone={onDone} />
      ))}
    </div>
  )
}
