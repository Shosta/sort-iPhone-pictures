import { motion } from 'framer-motion'
import { useEffect } from 'react'

export interface Flight {
  id: string
  /** Thumbnail already in cache: flights never trigger a decode. */
  url: string
  from: { x: number; y: number; size: number }
  to: { x: number; y: number }
  delay: number
}

const DURATION = 1.5

function Drop({ flight, onDone }: { flight: Flight; onDone: (id: string) => void }): JSX.Element {
  useEffect(() => {
    const timer = setTimeout(() => onDone(flight.id), (DURATION + flight.delay) * 1000 + 100)
    return () => clearTimeout(timer)
  }, [flight, onDone])

  const { url, from, to } = flight
  const half = from.size / 2
  // x eases in-out while y eases in, which bends the trajectory into a smooth arc.
  return (
    <motion.img
      src={url}
      alt=""
      className="flight"
      style={{ left: from.x - half, top: from.y - half, width: from.size, height: from.size }}
      initial={{ x: 0, y: 0, scale: 1, opacity: 1, rotate: 0 }}
      animate={{
        x: [0, (to.x - from.x) * 0.55, to.x - from.x],
        y: [0, (to.y - from.y) * 0.25, to.y - from.y],
        scale: [1, 1.12, 0.35],
        opacity: [1, 1, 0.9, 0],
        rotate: [0, -4, 0]
      }}
      transition={{
        duration: DURATION,
        delay: flight.delay,
        ease: [0.45, 0.05, 0.25, 1],
        opacity: { duration: DURATION, delay: flight.delay, times: [0, 0.6, 0.85, 1] }
      }}
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
