import confetti from 'canvas-confetti'
import { AnimatePresence, LayoutGroup, animate, motion, useMotionValue, useTransform } from 'framer-motion'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { DropZone } from './components/DropZone'
import { Flights, type Flight } from './components/Flights'
import { Preferences } from './components/Preferences'
import { Thumb } from './components/Thumb'
import { I18nContext, makeTranslate } from './i18n'
import { usePrefs } from './prefs'
import { applyTheme, themeById } from './themes'
import { onFatal, request } from './sidecar'
import type { Entry, Phase, Plan } from './types'

const VISIBLE = 80

function Counter({ value }: { value: number }): JSX.Element {
  const motionValue = useMotionValue(value)
  const rounded = useTransform(motionValue, (latest) => Math.round(latest))
  useEffect(() => {
    const controls = animate(motionValue, value, { duration: 0.4 })
    return controls.stop
  }, [value, motionValue])
  return <motion.span>{rounded}</motion.span>
}

export function App(): JSX.Element {
  const [sources, setSources] = useState<string[]>([])
  const [prefs, updatePrefs] = usePrefs()
  const { destination, byLocation, landmarks, skipDuplicates } = prefs
  const t = useMemo(() => makeTranslate(prefs.language), [prefs.language])
  const [showPrefs, setShowPrefs] = useState(false)
  const accent = themeById(prefs.theme).vars.accent
  useEffect(() => applyTheme(prefs.theme), [prefs.theme])
  useEffect(() => window.photosort.setMenuLabel(`${t('prefs')}…`), [t])
  useEffect(() => window.photosort.onPreferences(() => setShowPrefs(true)), [])
  const setDestination = (value: string): void => updatePrefs({ destination: value })
  const setByLocation = (value: boolean): void => updatePrefs({ byLocation: value })
  const setLandmarks = (value: boolean): void => updatePrefs({ landmarks: value })
  const [phase, setPhase] = useState<Phase>('idle')
  const [plan, setPlan] = useState<Plan | null>(null)
  const [copied, setCopied] = useState<Set<string>>(new Set())
  const [error, setError] = useState<string | null>(null)
  const [showSkipped, setShowSkipped] = useState(false)
  const running = useRef<{ cancel: () => void } | null>(null)
  const buffer = useRef<string[]>([])
  const planId = useRef('')
  const monthOf = useRef(new Map<string, string>())
  const [flights, setFlights] = useState<Flight[]>([])
  const removeFlight = useCallback((id: string) => setFlights((previous) => previous.filter((flight) => flight.id !== id)), [])

  useEffect(() => onFatal((event) => setError(event.message)), [])
  useEffect(() => {
    const timer = setInterval(() => {
      if (buffer.current.length === 0) return
      const batch = buffer.current.splice(0)
      const fallback = document.querySelector('.column .panel')?.getBoundingClientRect()
      const launched: Flight[] = batch.slice(0, 30).map((source, index) => {
        const origin = document.querySelector(`[data-pending="${CSS.escape(source)}"]`)?.getBoundingClientRect()
        const folder = document.querySelector(`[data-month="${CSS.escape(monthOf.current.get(source) ?? '')}"] .month-bar`)?.getBoundingClientRect()
        const panel = document.querySelectorAll('.column .panel')[1]?.getBoundingClientRect()
        const clamp = (value: number, low: number, high: number): number => Math.min(Math.max(value, low), high)
        return {
          id: `${source}-${performance.now()}-${index}`,
          path: source,
          from: origin
            ? { x: origin.x + origin.width / 2, y: origin.y + origin.height / 2, size: origin.width }
            : { x: (fallback?.x ?? 0) + (fallback?.width ?? 0) / 2, y: (fallback?.y ?? 0) + (fallback?.height ?? 0) / 2, size: 96 },
          to: folder && panel
            ? { x: folder.x + folder.width / 2, y: clamp(folder.y, panel.y + 20, panel.bottom - 20) }
            : { x: (panel?.x ?? 0) + (panel?.width ?? 0) / 2, y: (panel?.y ?? 0) + 40 },
          delay: index * 0.07
        }
      })
      setFlights((previous) => [...previous, ...launched])
      setCopied((previous) => new Set([...previous, ...batch]))
    }, 120)
    return () => clearInterval(timer)
  }, [])

  const resetResults = (): void => {
    buffer.current = []
    setPlan(null)
    setCopied(new Set())
    setPhase('idle')
    setError(null)
  }

  const addSources = useCallback((paths: string[]) => {
    if (paths.length === 0) return
    setSources((previous) => [...new Set([...previous, ...paths])])
    setPlan(null)
    setPhase('idle')
  }, [])

  const analyze = (): void => {
    resetResults()
    setPhase('analyzing')
    const planRequest = request({ op: 'plan', sources, destination, byLocation, landmarks, dedupe: skipDuplicates }, (event) => {
      if (event.type === 'plan') {
        setPlan(event.plan)
        setPhase('ready')
      } else if (event.type === 'error') {
        setPhase('idle')
        if (!event.cancelled) setError(event.message)
      }
    })
    planId.current = planRequest.id
    running.current = planRequest
  }

  const sort = (): void => {
    if (!plan) return
    setPhase('copying')
    setCopied(new Set())
    running.current = request({ op: 'copy', planId: planId.current }, (event) => {
      if (event.type === 'progress') buffer.current.push(event.entry.source)
      else if (event.type === 'done') {
        setTimeout(() => {
          setPhase('done')
          confetti({ particleCount: 140, spread: 80, origin: { y: 0.7 } })
        }, 250)
      } else if (event.type === 'error') {
        setPhase('ready')
        if (!event.cancelled) setError(event.message)
      }
    })
  }

  const entries = plan?.entries ?? []
  const pending = useMemo(() => entries.filter((entry) => !copied.has(entry.source)), [entries, copied])
  const months = useMemo(() => {
    const map = new Map<string, Entry[]>()
    for (const entry of entries) {
      const parts = entry.target.split(/[\\/]/)
      const key = parts.length > 3 ? `${parts[1]} · ${parts[2]}` : (parts[1] ?? '')
      map.set(key, [...(map.get(key) ?? []), entry])
    }
    const lookup = new Map<string, string>()
    for (const [key, items] of map) for (const item of items) lookup.set(item.source, key)
    monthOf.current = lookup
    return [...map.entries()]
  }, [entries])
  const progress = entries.length ? copied.size / entries.length : 0
  const busy = phase === 'analyzing' || phase === 'copying'

  return (
    <I18nContext.Provider value={t}>
    <div className="app">
      <div className="aurora">
        <span /> <span /> <span />
      </div>
      <header className="titlebar">
        <motion.h1 initial={{ opacity: 0, y: -10 }} animate={{ opacity: 1, y: 0 }}>
          📸 Photosort
        </motion.h1>
        <div className="steps">
          {([t('step.sources'), t('step.analysis'), t('step.sort')] as const).map((label, index) => {
            const active = [sources.length > 0, phase === 'ready' || phase === 'copying' || phase === 'done', phase === 'done'][index]
            return (
              <motion.span key={label} className={`step ${active ? 'on' : ''}`} animate={{ opacity: active ? 1 : 0.4 }}>
                {index + 1}. {label}
              </motion.span>
            )
          })}
        </div>
        <button className="gear" disabled={busy} title={t('prefs')} aria-label={t('prefs')} onClick={() => setShowPrefs(true)}>
          ⚙️
        </button>
      </header>

      <LayoutGroup>
        <main className="columns">
          <section className="column">
            <div className="top">
            <DropZone
              sources={sources}
              disabled={busy}
              onAdd={addSources}
              onRemove={(path) => {
                setSources((previous) => previous.filter((item) => item !== path))
                resetResults()
              }}
            />
            </div>
            <div className="glass panel grow">
              <h2>
                {t('pending')} <em>{plan ? <Counter value={pending.length} /> : 0}</em>
              </h2>
              <div className="grid">
                <AnimatePresence>
                  {pending.slice(0, VISIBLE).map((entry, index) => (
                    <Thumb key={entry.source} path={entry.source} index={index} label={entry.source} pending />
                  ))}
                </AnimatePresence>
              </div>
              {pending.length > VISIBLE && <p className="more">{t('more', { n: pending.length - VISIBLE })}</p>}
              {!plan && <p className="empty">{t('pending.empty')}</p>}
            </div>
          </section>

          <section className="column">
            <div className="top">
            <div className="glass destination">
              <label>{t('destination')}</label>
              <div className="row">
                <input
                  value={destination}
                  disabled={busy}
                  placeholder={t('destination.placeholder')}
                  onChange={(event) => {
                    setDestination(event.target.value)
                    setPlan(null)
                    setPhase('idle')
                  }}
                />
                <button
                  className="btn ghost"
                  disabled={busy}
                  onClick={async () => {
                    const [path] = await window.photosort.pickFolders(t('destination.pick'))
                    if (path) {
                      setDestination(path)
                      resetResults()
                    }
                  }}
                >
                  {t('choose')}
                </button>
              </div>
            </div>
            <label className={`toggle glass ${busy ? 'disabled' : ''}`}>
              <input
                type="checkbox"
                checked={byLocation}
                disabled={busy}
                onChange={(event) => {
                  setByLocation(event.target.checked)
                  resetResults()
                }}
              />
              <motion.span className="switch" animate={{ backgroundColor: byLocation ? accent : 'rgba(255,255,255,0.18)' }}>
                <motion.span className="knob" animate={{ x: byLocation ? 18 : 0 }} transition={{ type: 'spring', stiffness: 500, damping: 30 }} />
              </motion.span>
              <span>{t('byLocation')}</span>
            </label>
            <label className={`toggle glass ${busy || !byLocation ? 'disabled' : ''}`}>
              <input
                type="checkbox"
                checked={landmarks}
                disabled={busy || !byLocation}
                onChange={(event) => {
                  setLandmarks(event.target.checked)
                  resetResults()
                }}
              />
              <motion.span className="switch" animate={{ backgroundColor: landmarks && byLocation ? accent : 'rgba(255,255,255,0.18)' }}>
                <motion.span className="knob" animate={{ x: landmarks && byLocation ? 18 : 0 }} transition={{ type: 'spring', stiffness: 500, damping: 30 }} />
              </motion.span>
              <span>{t('landmarks')}</span>
            </label>
            <label className={`toggle glass ${busy ? 'disabled' : ''}`}>
              <input
                type="checkbox"
                checked={skipDuplicates}
                disabled={busy}
                onChange={(event) => {
                  updatePrefs({ skipDuplicates: event.target.checked })
                  resetResults()
                }}
              />
              <motion.span className="switch" animate={{ backgroundColor: skipDuplicates ? accent : 'rgba(255,255,255,0.18)' }}>
                <motion.span className="knob" animate={{ x: skipDuplicates ? 18 : 0 }} transition={{ type: 'spring', stiffness: 500, damping: 30 }} />
              </motion.span>
              <span>{t('skipDuplicates')}</span>
            </label>
            </div>
            <div className="glass panel grow">
              <h2>
                {t('sorted')} <em><Counter value={copied.size} /></em>
              </h2>
              {months.map(([month, items]) => {
                const done = items.filter((entry) => copied.has(entry.source))
                return (
                  <motion.div key={month} data-month={month} layout className="month" initial={{ opacity: 0, x: 20 }} animate={{ opacity: 1, x: 0 }}>
                    <div className="month-head">
                      <strong>{month}</strong>
                      <span>
                        {done.length}/{items.length}
                      </span>
                    </div>
                    <div className="month-bar">
                      <motion.i animate={{ width: `${(done.length / items.length) * 100}%` }} />
                    </div>
                    <div className="grid small">
                      <AnimatePresence>
                        {done.slice(0, 24).map((entry, index) => (
                          <Thumb key={entry.source} path={entry.source} size={56} index={index} label={entry.target} appearDelay={1.3} />
                        ))}
                      </AnimatePresence>
                    </div>
                  </motion.div>
                )
              })}
              {!plan && <p className="empty">{t('sorted.empty')}</p>}
            </div>
          </section>
        </main>
      </LayoutGroup>

      <Flights flights={flights} onDone={removeFlight} />

      <footer className="actionbar glass">
        <div className="progress">
          <motion.i animate={{ width: `${progress * 100}%` }} transition={{ type: 'spring', stiffness: 120, damping: 20 }} />
        </div>
        <div className="status">
          <AnimatePresence mode="wait">
            <motion.span key={error ?? phase} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -8 }}>
              {error
                ? `⚠️ ${error}`
                : phase === 'analyzing'
                  ? t('status.analyzing')
                  : phase === 'copying'
                    ? t('status.copying', { done: copied.size, total: entries.length })
                    : phase === 'done'
                      ? t('status.done', { n: copied.size })
                      : plan
                        ? t('status.ready', { n: entries.length })
                        : t('status.idle')}
            </motion.span>
          </AnimatePresence>
          {plan && plan.duplicates?.length > 0 && <span className="muted">{t('duplicates', { n: plan.duplicates.length })}</span>}
          {plan && plan.skipped.length > 0 && (
            <button className="link" onClick={() => setShowSkipped(true)}>
              {t('issues', { n: plan.skipped.length })}
            </button>
          )}
          {phase === 'done' && plan && (
            <button className="link" onClick={() => window.photosort.openPath(plan.destination)}>
              {t('openFolder')}
            </button>
          )}
        </div>
        <div className="actions">
          {busy && (
            <motion.button className="btn danger" whileTap={{ scale: 0.95 }} onClick={() => running.current?.cancel()}>
              {t('cancel')}
            </motion.button>
          )}
          <motion.button
            className="btn ghost"
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            disabled={busy || (!plan && !error && sources.length === 0)}
            title={t('clean.hint')}
            onClick={() => {
              setSources([])
              resetResults()
            }}
          >
            {t('clean')}
          </motion.button>
          <motion.button
            className="btn"
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            disabled={busy || sources.length === 0 || !destination.trim()}
            onClick={analyze}
          >
            {t('analyze')}
          </motion.button>
          <motion.button
            className="btn primary"
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            disabled={busy || phase !== 'ready' || entries.length === 0}
            onClick={sort}
          >
            {t('sort')}
          </motion.button>
        </div>
      </footer>

      <AnimatePresence>
        {showPrefs && <Preferences prefs={prefs} busy={busy} onChange={updatePrefs} onClose={() => setShowPrefs(false)} />}
      </AnimatePresence>

      <AnimatePresence>
        {showSkipped && plan && (
          <motion.div className="overlay" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onClick={() => setShowSkipped(false)}>
            <motion.div
              className="glass modal"
              initial={{ scale: 0.9, y: 30 }}
              animate={{ scale: 1, y: 0 }}
              exit={{ scale: 0.9, y: 30 }}
              onClick={(event) => event.stopPropagation()}
            >
              <h2>{t('issues.title')}</h2>
              <ul>
                {plan.skipped.map((issue) => (
                  <li key={issue.path}>
                    <code>{issue.path}</code>
                    <span>{issue.reason}</span>
                  </li>
                ))}
              </ul>
              <button className="btn" onClick={() => setShowSkipped(false)}>
                {t('close')}
              </button>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
    </I18nContext.Provider>
  )
}
