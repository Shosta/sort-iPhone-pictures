import { AnimatePresence, motion } from 'framer-motion'
import { useState } from 'react'
import { useT } from '../i18n'

interface Props {
  sources: string[]
  disabled: boolean
  onAdd: (paths: string[]) => void
  onRemove: (path: string) => void
}

export function DropZone({ sources, disabled, onAdd, onRemove }: Props): JSX.Element {
  const t = useT()
  const [hover, setHover] = useState(false)
  return (
    <motion.div
      className={`dropzone glass ${hover ? 'hover' : ''}`}
      animate={{ scale: hover ? 1.02 : 1 }}
      transition={{ type: 'spring', stiffness: 300, damping: 20 }}
      onDragOver={(event) => {
        event.preventDefault()
        if (!disabled) setHover(true)
      }}
      onDragLeave={() => setHover(false)}
      onDrop={(event) => {
        event.preventDefault()
        setHover(false)
        if (!disabled) onAdd(window.photosort.pathsFromDrop(Array.from(event.dataTransfer.files)))
      }}
    >
      <motion.div
        className="drop-icon"
        animate={{ y: hover ? -6 : [0, -4, 0] }}
        transition={hover ? { duration: 0.2 } : { duration: 3, repeat: Infinity, ease: 'easeInOut' }}
      >
        📂
      </motion.div>
      <p className="drop-title">{hover ? t('drop.hover') : t('drop.title')}</p>
      <button
        className="btn ghost"
        disabled={disabled}
        onClick={async () => onAdd(await window.photosort.pickFolders(t('drop.pick')))}
      >
        {t('drop.browse')}
      </button>
      <div className="chips">
        <AnimatePresence>
          {sources.map((source) => (
            <motion.span
              key={source}
              layout
              className="chip"
              initial={{ opacity: 0, scale: 0.5 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.5 }}
              title={source}
            >
              {source.split(/[\\/]/).pop()}
              {!disabled && <button onClick={() => onRemove(source)}>×</button>}
            </motion.span>
          ))}
        </AnimatePresence>
      </div>
    </motion.div>
  )
}
