import { motion } from 'framer-motion'
import { useEffect, useState } from 'react'
import { thumbnail } from '../sidecar'

interface Props {
  path: string
  label?: string
  size?: number
  index?: number
  pending?: boolean
  appearDelay?: number
}

export function Thumb({ path, label, size = 96, index = 0, pending = false, appearDelay = 0 }: Props): JSX.Element {
  const [url, setUrl] = useState<string | null | undefined>(undefined)
  useEffect(() => {
    let alive = true
    thumbnail(path, size * 2).then((value) => alive && setUrl(value))
    return () => {
      alive = false
    }
  }, [path, size])

  return (
    <motion.div
      data-pending={pending ? path : undefined}
      className="thumb"
      style={{ width: size, height: size }}
      initial={{ opacity: 0, scale: pending ? 1 : 0.6 }}
      animate={{ opacity: 1, scale: 1 }}
      exit={{ opacity: 0 }}
      transition={{ opacity: { duration: 0.5, delay: appearDelay + Math.min(index, 24) * 0.02 }, scale: { duration: 0.4, delay: appearDelay } }}
      whileHover={{ scale: 1.08, zIndex: 5 }}
      title={label ?? path}
    >
      {url === undefined && <div className="shimmer" />}
      {url === null && <span className="thumb-missing">🖼</span>}
      {url && <img src={url} alt="" draggable={false} />}
    </motion.div>
  )
}
