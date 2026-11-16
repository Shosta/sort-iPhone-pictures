import confetti from 'canvas-confetti'
import { AnimatePresence, animate, motion, useMotionValue, useTransform } from 'framer-motion'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { DropZone } from './components/DropZone'
import { Flights, type Flight } from './components/Flights'
import { Preferences } from './components/Preferences'
import { TagsPanel } from './components/TagsPanel'
import { Thumb } from './components/Thumb'
import { VirtualGrid } from './components/VirtualGrid'
import { I18nContext, makeTranslate, type Translate } from './i18n'
import { usePrefs } from './prefs'
import { applyTheme, themeById } from './themes'
import { cachedThumbnail, onFatal, request } from './sidecar'
import type { Duplicate, Entry, Phase, Plan, TagJob, TagResult } from './types'

/** Thumbnails shown per destination folder in the "Sorted" panel. */
const PER_FOLDER = 24
/** Tiles that arrived less than this long ago (ms) play the appear animation. */
const FRESH = 900
const NO_ENTRY: Entry[] = []

function Counter({ value }: { value: number }): JSX.Element {
  const motionValue = useMotionValue(value)
  const rounded = useTransform(motionValue, (latest) => Math.round(latest))
  useEffect(() => {
    const controls = animate(motionValue, value, { duration: 0.4 })
    return controls.stop
  }, [value, motionValue])
  return <motion.span>{rounded}</motion.span>
}

/** "2026/2026-10/Paris/IMG.jpg" → "2026-10 · Paris". */
function folderOf(target: string): string {
  const parts = target.split(/[\\/]/)
  return parts.length > 3 ? `${parts[1]} · ${parts[2]}` : (parts[1] ?? '')
}

const Folder = memo(function Folder({ name, items, done }: { name: string; items: Entry[]; done: Entry[] }): JSX.Element {
  return (
    <div data-month={name} className="month">
      <div className="month-head">
        <strong>{name}</strong>
        <span>
          {done.length}/{items.length}
        </span>
      </div>
      <div className="month-bar">
        <i style={{ width: `${(done.length / items.length) * 100}%` }} />
      </div>
      {done.length > 0 && (
        <div className="grid small">
          {done.slice(0, PER_FOLDER).map((entry) => (
            <Thumb key={entry.source} path={entry.source} size={56} label={entry.target} appearDelay={1.3} lazy />
          ))}
          {done.length > PER_FOLDER && <span className="more-chip">+{done.length - PER_FOLDER}</span>}
        </div>
      )}
    </div>
  )
})

function analysisStatus(t: Translate, stage: { name: string; done: number; total: number } | null): string {
  if (!stage) return t('status.analyzing')
  const key = stage.name === 'hash' ? 'status.hash' : stage.name === 'similar' ? 'status.similar' : 'status.exif'
  return t(key, { done: stage.done, total: stage.total })
}

/** Overall analysis progress: reading EXIF, then fingerprints, then grouping. */
function analysisProgress(stage: { name: string; done: number; total: number } | null, deep: boolean): number {
  if (!stage) return 0
  const ratio = stage.total > 0 ? stage.done / stage.total : 1
  if (!deep) return ratio
  if (stage.name === 'exif') return 0.4 * ratio
  if (stage.name === 'hash') return 0.4 + 0.55 * ratio
  return 0.97
}

function applyTagResults(job: TagJob, results: TagResult[]): TagJob {
  if (results.length === 0) return job
  const counts = { ...job.counts }
  let tagged = job.tagged
  let failed = job.failed
  for (const result of results) {
    if (result.error) failed++
    else if (result.tags && result.tags.length > 0) {
      tagged++
      for (const tag of result.tags) counts[tag] = (counts[tag] ?? 0) + 1
    }
  }
  return { ...job, tagged, failed, counts, done: job.done + results.length, recent: [...results.reverse(), ...job.recent].slice(0, 12) }
}

export function App(): JSX.Element {
  const [sources, setSources] = useState<string[]>([])
  const [prefs, updatePrefs] = usePrefs()
  const { destination, byLocation, landmarks, skipDuplicates, groupSimilar, tagScenes, tagMoment, tagPeople } = prefs
  const t = useMemo(() => makeTranslate(prefs.language), [prefs.language])
  const [showPrefs, setShowPrefs] = useState(false)
  const [showTags, setShowTags] = useState(false)
  const [tagScope, setTagScope] = useState<string[] | null>(null)
  const accent = themeById(prefs.theme).vars.accent
  useEffect(() => applyTheme(prefs.theme), [prefs.theme])
  useEffect(() => window.photosort.setMenuLabels({ preferences: `${t('prefs')}…`, tags: `${t('tags')}…` }), [t])
  useEffect(() => window.photosort.onPreferences(() => setShowPrefs(true)), [])
  useEffect(
    () =>
      window.photosort.onTags(() => {
        setTagScope(null)
        setShowTags(true)
      }),
    []
  )
  const setDestination = (value: string): void => updatePrefs({ destination: value })
  const setByLocation = (value: boolean): void => updatePrefs({ byLocation: value })
  const setLandmarks = (value: boolean): void => updatePrefs({ landmarks: value })
  const [stage, setStage] = useState<{ name: string; done: number; total: number } | null>(null)
  const [phase, setPhase] = useState<Phase>('idle')
  const [plan, setPlan] = useState<Plan | null>(null)
  const [draft, setDraft] = useState<Entry[]>([])
  const [draftDuplicates, setDraftDuplicates] = useState<Duplicate[]>([])
  const [copied, setCopied] = useState<Set<string>>(new Set())
  const [doneByFolder, setDoneByFolder] = useState<Map<string, Entry[]>>(new Map())
  const [error, setError] = useState<string | null>(null)
  const [showSkipped, setShowSkipped] = useState(false)
  const [tagJob, setTagJob] = useState<TagJob | null>(null)
  const running = useRef<{ cancel: () => void } | null>(null)
  const tagRun = useRef<{ id: string; cancel: () => void } | null>(null)
  const tagBuffer = useRef<TagResult[]>([])
  const buffer = useRef<Entry[]>([])
  const copiedTargets = useRef<string[]>([])
  const arrived = useRef(new Map<string, number>())
  const planId = useRef('')
  const [flights, setFlights] = useState<Flight[]>([])
  const removeFlight = useCallback((id: string) => setFlights((previous) => previous.filter((flight) => flight.id !== id)), [])

  useEffect(() => onFatal((event) => setError(event.message)), [])

  // Copied photos and tagging results are applied in batches, so the UI
  // stays fluid however fast the engine goes.
  useEffect(() => {
    const timer = setInterval(() => {
      if (tagBuffer.current.length > 0) {
        const results = tagBuffer.current.splice(0)
        setTagJob((job) => (job ? applyTagResults(job, results) : job))
      }
      if (buffer.current.length === 0) return
      const batch = buffer.current.splice(0)
      const panels = document.querySelectorAll('.column .panel')
      const left = panels[0]?.getBoundingClientRect()
      const right = panels[1]?.getBoundingClientRect()
      const view = document.querySelector('.panel.list .vgrid')?.getBoundingClientRect()
      const clamp = (value: number, low: number, high: number): number => Math.min(Math.max(value, low), high)
      // Overscan cells are rendered but clipped: they take off from the panel instead.
      const visible = (rect: DOMRect | undefined): rect is DOMRect =>
        !!rect && !!view && rect.top >= view.top - 1 && rect.bottom <= view.bottom + 1
      // Only photos whose thumbnail is already loaded take off: decoding others
      // just for the animation would compete with the copy.
      const launched: Flight[] = []
      for (const entry of batch) {
        const url = cachedThumbnail(entry.source)
        if (!url) continue
        if (launched.length === 30) break
        const index = launched.length
        const origin = document.querySelector(`[data-pending="${CSS.escape(entry.source)}"]`)?.getBoundingClientRect()
        const folder = document.querySelector(`[data-month="${CSS.escape(folderOf(entry.target))}"] .month-bar`)?.getBoundingClientRect()
        launched.push({
          id: `${entry.source}-${performance.now()}-${index}`,
          url,
          from: visible(origin)
            ? { x: origin.x + origin.width / 2, y: origin.y + origin.height / 2, size: origin.width }
            : { x: (left?.x ?? 0) + (left?.width ?? 0) / 2, y: (left?.y ?? 0) + (left?.height ?? 0) / 2, size: 96 },
          to: folder && right
            ? { x: folder.x + folder.width / 2, y: clamp(folder.y, right.y + 20, right.bottom - 20) }
            : { x: (right?.x ?? 0) + (right?.width ?? 0) / 2, y: (right?.y ?? 0) + 40 },
          delay: index * 0.07
        })
      }
      if (launched.length > 0) setFlights((previous) => [...previous, ...launched])
      setCopied((previous) => {
        const next = new Set(previous)
        for (const entry of batch) next.add(entry.source)
        return next
      })
      setDoneByFolder((previous) => {
        const next = new Map(previous)
        for (const entry of batch) {
          const name = folderOf(entry.target)
          next.set(name, [...(next.get(name) ?? []), entry])
        }
        return next
      })
    }, 120)
    return () => clearInterval(timer)
  }, [])

  const resetResults = (): void => {
    buffer.current = []
    copiedTargets.current = []
    arrived.current = new Map()
    setPlan(null)
    setDraft([])
    setDraftDuplicates([])
    setCopied(new Set())
    setDoneByFolder(new Map())
    setPhase('idle')
    setError(null)
  }

  const addSources = useCallback((paths: string[]) => {
    if (paths.length === 0) return
    setSources((previous) => [...new Set([...previous, ...paths])])
    setPlan(null)
    setDraft([])
    setDraftDuplicates([])
    setPhase('idle')
  }, [])

  const analyze = (): void => {
    resetResults()
    setPhase('analyzing')
    setStage(null)
    const planRequest = request({ op: 'plan', sources, destination, byLocation, landmarks, dedupe: skipDuplicates, similar: groupSimilar }, (event) => {
      if (event.type === 'status') setStage({ name: event.stage, done: event.done, total: event.total })
      else if (event.type === 'draft') {
        const entries: Entry[] = event.entries ?? []
        const duplicates: Duplicate[] = event.duplicates ?? []
        const now = performance.now()
        for (const entry of entries) arrived.current.set(entry.source, now)
        setDraft((previous) => {
          let next = entries.length > 0 ? previous.concat(entries) : previous
          if (duplicates.length > 0) {
            const gone = new Set(duplicates.map((duplicate) => duplicate.path))
            next = next.filter((entry) => !gone.has(entry.source))
          }
          return next
        })
        if (duplicates.length > 0) setDraftDuplicates((previous) => previous.concat(duplicates))
      } else if (event.type === 'plan') {
        setStage(null)
        setPlan(event.plan)
        setPhase('ready')
      } else if (event.type === 'error') {
        setStage(null)
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
    setDoneByFolder(new Map())
    copiedTargets.current = []
    running.current = request({ op: 'copy', planId: planId.current }, (event) => {
      if (event.type === 'progress') {
        buffer.current.push(event.entry)
        copiedTargets.current.push(`${plan.destination}/${event.entry.target}`)
      } else if (event.type === 'done') {
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

  const startTags = (folder: string): void => {
    const files = tagScope ?? []
    tagBuffer.current = []
    const run = request({ op: 'tag', folder, files, tagScenes, tagMoment, tagPeople }, (event) => {
      if (tagRun.current?.id !== run.id) return
      if (event.type === 'status') {
        setTagJob((job) => job && { ...job, state: 'running', total: event.total, started: performance.now() })
      } else if (event.type === 'tagged') {
        tagBuffer.current.push({ path: event.path, tags: event.tags, error: event.error })
      } else if (event.type === 'done') {
        const results = tagBuffer.current.splice(0)
        setTagJob((job) => job && { ...applyTagResults(job, results), state: 'done' })
      } else if (event.type === 'error') {
        const results = tagBuffer.current.splice(0)
        setTagJob((job) => job && { ...applyTagResults(job, results), state: event.cancelled ? 'cancelled' : 'error', message: event.message })
      }
    })
    tagRun.current = run
    setTagJob({
      id: run.id,
      folder,
      files: files.length,
      state: 'loading',
      done: 0,
      total: files.length,
      tagged: 0,
      failed: 0,
      counts: {},
      recent: [],
      started: performance.now()
    })
  }

  const entries = plan?.entries ?? draft
  const duplicates = plan?.duplicates ?? draftDuplicates
  const pending = useMemo(() => (copied.size === 0 ? entries : entries.filter((entry) => !copied.has(entry.source))), [entries, copied])
  const folders = useMemo(() => {
    const map = new Map<string, Entry[]>()
    for (const entry of entries) {
      const name = folderOf(entry.target)
      const items = map.get(name)
      if (items) items.push(entry)
      else map.set(name, [entry])
    }
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b))
  }, [entries])
  const busy = phase === 'analyzing' || phase === 'copying'
  const tagging = tagJob?.state === 'loading' || tagJob?.state === 'running'
  const deep = skipDuplicates || groupSimilar
  const progress =
    phase === 'analyzing' ? analysisProgress(stage, deep) : phase === 'copying' || phase === 'done' ? (entries.length ? copied.size / entries.length : 0) : 0

  const renderPending = useCallback((entry: Entry, size: number) => {
    const since = arrived.current.get(entry.source)
    const fresh = since !== undefined && performance.now() - since < FRESH
    return <Thumb path={entry.source} size={size} label={entry.source} pending appearDelay={fresh ? 0 : undefined} />
  }, [])

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
        <div className="title-actions">
          <button
            className="gear"
            title={t('tags')}
            aria-label={t('tags')}
            onClick={() => {
              setTagScope(null)
              setShowTags(true)
            }}
          >
            🏷️
            {tagging && tagJob && <span className="badge">{tagJob.state === 'loading' ? '…' : `${tagJob.done}/${tagJob.total}`}</span>}
          </button>
          <button className="gear" disabled={busy} title={t('prefs')} aria-label={t('prefs')} onClick={() => setShowPrefs(true)}>
            ⚙️
          </button>
        </div>
      </header>

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
          <div className="glass panel grow list">
            <h2>
              {t('pending')} <em><Counter value={pending.length} /></em>
            </h2>
            <VirtualGrid items={pending} itemKey={(entry) => entry.source} render={renderPending}>
              {entries.length === 0 && phase !== 'analyzing' && <p className="empty">{t('pending.empty')}</p>}
            </VirtualGrid>
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
                  resetResults()
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
          </div>
          <div className="glass panel grow">
            <h2>
              {t('sorted')} <em><Counter value={copied.size} /></em>
            </h2>
            {folders.map(([name, items]) => (
              <Folder key={name} name={name} items={items} done={doneByFolder.get(name) ?? NO_ENTRY} />
            ))}
            {entries.length === 0 && <p className="empty">{t('sorted.empty')}</p>}
          </div>
        </section>
      </main>

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
                  ? analysisStatus(t, stage)
                  : phase === 'copying'
                    ? t('status.copying', { done: copied.size, total: entries.length })
                    : phase === 'done'
                      ? t('status.done', { n: copied.size })
                      : plan
                        ? t('status.ready', { n: entries.length })
                        : t('status.idle')}
            </motion.span>
          </AnimatePresence>
          {duplicates.length > 0 && (
            <button className="link" onClick={() => setShowSkipped(true)}>
              {t('duplicates', { n: duplicates.length })}
            </button>
          )}
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
          {phase === 'done' && copiedTargets.current.length > 0 && !tagging && (
            <button
              className="link"
              onClick={() => {
                setTagScope([...copiedTargets.current])
                setShowTags(true)
              }}
            >
              🏷️ {t('tags.link')}
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
            disabled={busy || tagging || phase !== 'ready' || entries.length === 0}
            title={tagging ? t('tags.busy') : undefined}
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
        {showTags && (
          <TagsPanel
            job={tagJob}
            prefs={prefs}
            scope={tagScope}
            busy={busy}
            onChange={updatePrefs}
            onClearScope={() => setTagScope(null)}
            onStart={startTags}
            onCancel={() => tagRun.current?.cancel()}
            onClose={() => setShowTags(false)}
          />
        )}
      </AnimatePresence>

      <AnimatePresence>
        {showSkipped && (
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
                {(plan?.skipped ?? []).map((issue) => (
                  <li key={issue.path}>
                    <code>{issue.path}</code>
                    <span>{issue.reason}</span>
                  </li>
                ))}
                {duplicates.map((duplicate) => (
                  <li key={duplicate.path}>
                    <code>{duplicate.path}</code>
                    <span>
                      {duplicate.inDestination ? t('duplicateInDest') : t('duplicateOf')} {duplicate.of}
                    </span>
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
