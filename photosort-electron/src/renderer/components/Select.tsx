import { AnimatePresence, motion } from 'framer-motion'
import { type ReactNode, useEffect, useRef, useState } from 'react'

export interface Option<T extends string> {
  value: T
  label: string
  icon: ReactNode
}

interface Props<T extends string> {
  value: T
  options: Option<T>[]
  onChange: (value: T) => void
}

export function Select<T extends string>({ value, options, onChange }: Props<T>): JSX.Element {
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const current = options.find((option) => option.value === value) ?? options[0]

  useEffect(() => {
    const close = (event: MouseEvent): void => {
      if (!root.current?.contains(event.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [])

  return (
    <div className="select" ref={root}>
      <button className="select-button" onClick={() => setOpen((previous) => !previous)} aria-haspopup="listbox" aria-expanded={open}>
        {current.icon}
        <span>{current.label}</span>
        <span className="chevron">▾</span>
      </button>
      <AnimatePresence>
        {open && (
          <motion.ul className="select-list glass" role="listbox" initial={{ opacity: 0, y: -6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -6 }}>
            {options.map((option) => (
              <li
                key={option.value}
                role="option"
                aria-selected={option.value === value}
                className={option.value === value ? 'selected' : ''}
                onClick={() => {
                  onChange(option.value)
                  setOpen(false)
                }}
              >
                {option.icon}
                <span>{option.label}</span>
              </li>
            ))}
          </motion.ul>
        )}
      </AnimatePresence>
    </div>
  )
}
