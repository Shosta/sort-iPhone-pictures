import confetti from 'canvas-confetti'
import { AnimatePresence, animate, motion, useMotionValue, useTransform } from 'framer-motion'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { DropZone } from './components/DropZone'
import { Flights, type Flight } from './components/Flights'
import { Preferences } from './components/Preferences'
import { RemotePanel } from './components/RemotePanel'
import { ScrapbookPanel } from './components/ScrapbookPanel'
import { TravelMapPanel } from './components/TravelMapPanel'
import { YearMapPanel } from './components/YearMapPanel'
import { TagsPanel } from './components/TagsPanel'
import { Thumb } from './components/Thumb'
import { VirtualGrid } from './components/VirtualGrid'
import { IssueList } from './components/IssueList'
import { I18nContext, makeTranslate, type Translate } from './i18n'
import { remoteConfig, remoteFolder, remoteLabel, remoteReady, usePrefs } from './prefs'
import { remoteErrorText } from './remote'
import { applyTheme, themeById } from './themes'
import { cachedThumbnail, onFatal, request } from './sidecar'
import type { AssetsState, Duplicate, Entry, Phase, Plan, TagJob, TagResult } from './types'

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
const PACED_BELOW = 500
const MAX_PACED_SECONDS = 6

/**
 * Animations are decoration: on big libraries only a sample is played (about
 * one tile in `every`, and a few flights per tick), so the screen stays lively
 * without flooding the renderer. The data is never sampled.
 */
function animationBudget(count: number): { flights: number; every: number; heavy: boolean; pace: number } {
  // Small libraries are copied in a blink: the display is paced (about 3 photos
  // per second, never more than MAX_PACED_SECONDS in total) so the move can be seen.
  if (count > 0 && count < PACED_BELOW) return { flights: 3, every: 1, heavy: false, pace: Math.max(3, count / MAX_PACED_SECONDS) }
  const flights = 1
  if (count <= 5000) return { flights, every: 1, heavy: false, pace: 0 }
  return { flights, every: Math.ceil(count / 5000), heavy: count > 20000, pace: 0 }
}

function sampled(path: string, every: number): boolean {
  if (every === 1) return true
  let hash = 0
  for (let i = path.length - 1; i >= Math.max(0, path.length - 12); i--) hash = (hash * 31 + path.charCodeAt(i)) | 0
  return Math.abs(hash) % every === 0
}

/** Pause between two draft renders: none for small lists, up to 1 s for 200 000 photos. */
function draftDelay(count: number): number {
  return Math.min(1000, Math.max(0, (count - 2000) / 200))
}

function groupByFolder(entries: Entry[]): Map<string, Entry[]> {
  const map = new Map<string, Entry[]>()
  for (const entry of entries) {
    const name = folderOf(entry.target)
    const items = map.get(name)
    if (items) items.push(entry)
    else map.set(name, [entry])
  }
  return map
}

function sortedFolders(groups: Map<string, Entry[]>): [string, Entry[]][] {
  return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b))
}

function folderOf(target: string): string {
  const parts = target.split(/[\\/]/)
  if (parts.length <= 2) return parts[0] ?? ''
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
  const key = stage.name === 'hash' ? 'status.hash' : stage.name === 'similar' ? 'status.similar' : stage.name === 'blur' ? 'status.blur' : 'status.exif'
  return t(key, { done: stage.done, total: stage.total })
}

/** Overall analysis progress: reading EXIF, then fingerprints, then grouping. */
function analysisProgress(stage: { name: string; done: number; total: number } | null, deep: boolean): number {
  if (!stage) return 0
  const ratio = stage.total > 0 ? stage.done / stage.total : 1
  if (!deep) return ratio
  if (stage.name === 'exif') return 0.4 * ratio
  if (stage.name === 'hash' || stage.name === 'blur') return 0.4 + 0.55 * ratio
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

interface RemoteSummary {
  dryRun?: boolean
  sent?: number
  existing?: number
  mismatched?: number
  mismatchedSample?: string[]
  alreadyTotal?: number
  tags?: number
  warnings?: string[]
}
interface RemoteProgress {
  percent?: number
  rate?: string
  eta?: string
  done?: number
  total?: number
  file?: string
}
interface RemoteState {
  progress?: RemoteProgress
  summary?: RemoteSummary
  phase?: string
  files?: number
  total?: number
  legacy?: boolean
  dryRun?: boolean
}

function elapsedText(seconds: number): string {
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

/** What the NAS transfer is doing right now, in plain words (never empty). */
function remotePhaseText(t: Translate, state: RemoteState | null, copied: number, total: number): string {
  switch (state?.phase) {
    case 'library':
      return state.files ? t('remote.phase.library.files', { n: state.files }) : t('remote.phase.library')
    case 'dedupe':
      return t('remote.phase.dedupe')
    case 'prepare':
      return t('remote.phase.prepare')
    case 'verify':
      return t('remote.phase.verify')
    case 'index':
      return t('remote.phase.index')
    case 'send': {
      const progress = state.progress
      const done = Math.max(progress?.done ?? 0, copied)
      const count = t(state.dryRun ? 'remote.phase.simulate' : 'remote.phase.send', { done, total: progress?.total || state.total || total })
      const rate = progress?.rate ? ` — ${progress.rate}` : ''
      const eta = progress?.eta ? t('remote.status.eta', { eta: progress.eta }) : ''
      return `${count}${rate}${eta}${progress?.file ? ` — ${progress.file}` : ''}`
    }
    default:
      return t('remote.phase.connect')
  }
}

export function App(): JSX.Element {
  const [sources, setSources] = useState<string[]>([])
  const [prefs, updatePrefs] = usePrefs()
  const { destination, byLocation, landmarks, skipDuplicates, groupSimilar, dateFallback, screenshotFolder, groupBlurry, tagScenes, tagMoment, tagPeople, tagMeta } = prefs
  const t = useMemo(() => makeTranslate(prefs.language), [prefs.language])
  const [showPrefs, setShowPrefs] = useState(false)
  const [showTags, setShowTags] = useState(false)
  const [showRemote, setShowRemote] = useState(false)
  // The NAS password lives in memory only (and in the system keychain when asked).
  const [remotePassword, setRemotePassword] = useState('')
  const [remoteState, setRemoteState] = useState<RemoteState | null>(null)
  const [elapsed, setElapsed] = useState(0)
  const [resumable, setResumable] = useState(false)
  const dryRun = useRef(false)
  const [showScrapbook, setShowScrapbook] = useState(false)
  const [showYearMap, setShowYearMap] = useState(false)
  const [showTravelMap, setShowTravelMap] = useState(false)
  const [tagScope, setTagScope] = useState<string[] | null>(null)
  const accent = themeById(prefs.theme).vars.accent
  const [assets, setAssets] = useState<AssetsState>({ state: 'checking' })
  useEffect(() => applyTheme(prefs.theme), [prefs.theme])
  useEffect(() => window.photosort.setMenuLabels({ preferences: `${t('prefs')}…`, tags: `${t('tags')}…`, scrapbook: `${t('scrap')}…` }), [t])
  useEffect(() => {
    void window.photosort.getAssets().then(setAssets)
    return window.photosort.onAssets(setAssets)
  }, [])
  useEffect(() => window.photosort.onPreferences(() => setShowPrefs(true)), [])
  useEffect(() => window.photosort.onScrapbook(() => setShowScrapbook(true)), [])
  useEffect(
    () =>
      window.photosort.onTags(() => {
        setTagScope(null)
        setShowTags(true)
      }),
    []
  )
  const remote = prefs.remoteEnabled
  const remoteAccount = `${prefs.remoteUser.trim()}@${prefs.remoteHost.trim()}:${prefs.remoteModule.trim()}`
  useEffect(() => {
    if (!prefs.remoteRemember || !prefs.remoteHost.trim()) return
    void window.photosort.secrets.load(remoteAccount).then((saved) => saved && setRemotePassword((current) => current || saved))
  }, [prefs.remoteRemember, remoteAccount, prefs.remoteHost])
  useEffect(() => {
    if (!prefs.remoteHost.trim()) return
    const timer = window.setTimeout(() => {
      if (prefs.remoteRemember && remotePassword) void window.photosort.secrets.store(remoteAccount, remotePassword)
      else if (!prefs.remoteRemember) void window.photosort.secrets.forget(remoteAccount)
    }, 600)
    return () => window.clearTimeout(timer)
  }, [prefs.remoteRemember, remotePassword, remoteAccount, prefs.remoteHost])
  const setDestination = (value: string): void => updatePrefs({ destination: value })
  const setByLocation = (value: boolean): void => updatePrefs({ byLocation: value })
  const setLandmarks = (value: boolean): void => updatePrefs({ landmarks: value })
  const [stage, setStage] = useState<{ name: string; done: number; total: number } | null>(null)
  const [phase, setPhase] = useState<Phase>('idle')
  const [plan, setPlan] = useState<Plan | null>(null)
  const [draft, setDraft] = useState<Entry[]>([])
  const [draftDuplicates, setDraftDuplicates] = useState<Duplicate[]>([])
  // The copied set grows in place (cloning it each tick is O(n)); the count is what triggers renders.
  const copiedRef = useRef(new Set<string>())
  const [copiedCount, setCopiedCount] = useState(0)
  const draftRef = useRef({ entries: [] as Entry[], groups: new Map<string, Entry[]>() })
  const [draftFolders, setDraftFolders] = useState<[string, Entry[]][]>([])
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
  const budget = useRef(animationBudget(0))
  const finishing = useRef<'done' | 'error' | null>(null)
  const credit = useRef(0)
  const lastTick = useRef(performance.now())
  const pendingDraft = useRef({ entries: [] as Entry[], duplicates: [] as Duplicate[], shown: 0, timer: 0 })
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
      const now = performance.now()
      const elapsed = now - lastTick.current
      lastTick.current = now
      const pace = budget.current.pace
      let batch: Entry[]
      if (pace > 0 && finishing.current !== 'error') {
        // Photos are already copied; only their display is spread out.
        credit.current = Math.min(credit.current + (pace * elapsed) / 1000, 10)
        const count = Math.min(Math.floor(credit.current), buffer.current.length)
        credit.current -= count
        batch = buffer.current.splice(0, count)
      } else {
        batch = buffer.current.splice(0)
      }
      if (finishing.current && buffer.current.length === 0 && batch.length === 0) {
        const outcome = finishing.current
        finishing.current = null
        credit.current = 0
        if (outcome === 'done' && dryRun.current) {
          copiedRef.current = new Set()
          setCopiedCount(0)
          setDoneByFolder(new Map())
          setPhase('ready')
        } else if (outcome === 'done') {
          setTimeout(() => {
            setPhase('done')
            confetti({ particleCount: 140, spread: 80, origin: { y: 0.7 } })
          }, 250)
        }
        return
      }
      if (batch.length === 0) return
      const panels = document.querySelectorAll('.column .panel')
      const left = panels[0]?.getBoundingClientRect()
      const right = panels[1]?.getBoundingClientRect()
      const view = document.querySelector('.panel.list .vgrid')?.getBoundingClientRect()
      // Overscan cells are rendered but clipped: they take off from the panel instead.
      const visible = (rect: DOMRect | undefined): rect is DOMRect =>
        !!rect && !!view && rect.top >= view.top - 1 && rect.bottom <= view.bottom + 1
      // Flights only go to folders that are on screen, and leave from the real
      // tile when it is visible. Other copies borrow the picture of a visible
      // tile, so something always flies while the engine works (decoding a
      // thumbnail just for the animation would compete with the copy).
      const inside = (rect: DOMRect | undefined, box: DOMRect | undefined): rect is DOMRect =>
        !!rect && !!box && rect.top >= box.top && rect.bottom <= box.bottom
      const tiles = new Map<string, { rect: DOMRect; url: string }>()
      document.querySelectorAll<HTMLElement>('[data-pending]').forEach((tile) => {
        const image = tile.querySelector('img')
        const rect = tile.getBoundingClientRect()
        if (image?.src && visible(rect)) tiles.set(tile.dataset.pending as string, { rect, url: image.src })
      })
      const decoys = [...tiles.values()]
      const folderRects = new Map<string, DOMRect | null>()
      const launched: Flight[] = []
      const limit = budget.current.flights
      for (let i = 0; i < batch.length && launched.length < limit; i++) {
        const entry = batch[i]
        const name = folderOf(entry.target)
        if (!folderRects.has(name)) {
          const rect = document.querySelector(`[data-month="${CSS.escape(name)}"] .month-bar`)?.getBoundingClientRect()
          folderRects.set(name, inside(rect, right) ? rect : null)
        }
        const folder = folderRects.get(name)
        if (!folder) continue
        const real = tiles.get(entry.source)
        const source = real ?? (decoys.length > 0 ? decoys[launched.length % decoys.length] : undefined)
        const url = source?.url ?? cachedThumbnail(entry.source)
        if (!url) continue
        const rect = source?.rect
        const index = launched.length
        launched.push({
          id: `${entry.source}-${performance.now()}-${index}`,
          url,
          from: rect
            ? { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2, size: rect.width }
            : { x: (left?.x ?? 0) + (left?.width ?? 0) / 2, y: (left?.y ?? 0) + (left?.height ?? 0) / 2, size: 96 },
          to: { x: folder.x + folder.width / 2, y: folder.y },
          delay: index * 0.07
        })
      }
      if (launched.length > 0) setFlights((previous) => [...previous, ...launched])
      for (const entry of batch) copiedRef.current.add(entry.source)
      setCopiedCount(copiedRef.current.size)
      const added = new Map<string, Entry[]>()
      for (const entry of batch) {
        const name = folderOf(entry.target)
        const list = added.get(name)
        if (list) list.push(entry)
        else added.set(name, [entry])
      }
      setDoneByFolder((previous) => {
        const next = new Map(previous)
        for (const [name, list] of added) next.set(name, (next.get(name) ?? NO_ENTRY).concat(list))
        return next
      })
    }, 120)
    return () => clearInterval(timer)
  }, [])

  // Draft batches are accumulated and applied at a pace that follows the list
  // size: the grouping below is O(n), so applying every 80 ms batch makes big
  // libraries quadratic. Nothing is dropped; the final plan replaces the draft.
  const flushDraft = useCallback((): void => {
    const state = pendingDraft.current
    window.clearTimeout(state.timer)
    state.timer = 0
    const entries = state.entries
    const duplicates = state.duplicates
    if (entries.length === 0 && duplicates.length === 0) return
    state.entries = []
    state.duplicates = []
    const mirror = draftRef.current
    if (duplicates.length > 0) {
      const gone = new Set(duplicates.map((duplicate) => duplicate.path))
      mirror.entries = mirror.entries.concat(entries).filter((entry) => !gone.has(entry.source))
      mirror.groups = groupByFolder(mirror.entries)
    } else {
      mirror.entries = mirror.entries.concat(entries)
      const touched = new Set<string>()
      for (const entry of entries) {
        const name = folderOf(entry.target)
        const list = mirror.groups.get(name)
        if (!list) mirror.groups.set(name, [entry])
        else {
          // Mutated until it is published below, then replaced so memoised folders see the change.
          list.push(entry)
        }
        touched.add(name)
      }
      for (const name of touched) mirror.groups.set(name, mirror.groups.get(name)!.slice())
    }
    state.shown = mirror.entries.length
    setDraft(mirror.entries)
    setDraftFolders(sortedFolders(mirror.groups))
    if (duplicates.length > 0) setDraftDuplicates((previous) => previous.concat(duplicates))
  }, [])

  const clearDraftBuffer = (): void => {
    draftRef.current = { entries: [], groups: new Map() }
    const state = pendingDraft.current
    window.clearTimeout(state.timer)
    state.timer = 0
    state.entries = []
    state.duplicates = []
    state.shown = 0
  }

  const resetResults = (): void => {
    clearDraftBuffer()
    buffer.current = []
    copiedTargets.current = []
    arrived.current = new Map()
    setPlan(null)
    setDraft([])
    setDraftFolders([])
    setDraftDuplicates([])
    copiedRef.current = new Set()
    setCopiedCount(0)
    setDoneByFolder(new Map())
    setPhase('idle')
    setError(null)
    setRemoteState(null)
    setResumable(false)
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
    const planRequest = request({ op: 'plan', sources, destination: remote ? '' : destination, remote: remote ? remoteConfig(prefs, remotePassword) : undefined, byLocation, landmarks, dedupe: skipDuplicates, similar: groupSimilar, dateFallback, screenshots: screenshotFolder, blurry: groupBlurry }, (event) => {
      if (event.type === 'status') setStage({ name: event.stage, done: event.done, total: event.total })
      else if (event.type === 'draft') {
        const entries: Entry[] = event.entries ?? []
        const duplicates: Duplicate[] = event.duplicates ?? []
        const now = performance.now()
        for (const entry of entries) arrived.current.set(entry.source, now)
        const state = pendingDraft.current
        for (const entry of entries) state.entries.push(entry)
        for (const duplicate of duplicates) state.duplicates.push(duplicate)
        if (!state.timer) state.timer = window.setTimeout(flushDraft, draftDelay(state.shown + state.entries.length))
      } else if (event.type === 'plan') {
        clearDraftBuffer()
        setStage(null)
        setPlan(event.plan)
        setPhase('ready')
      } else if (event.type === 'error') {
        flushDraft()
        setStage(null)
        setPhase('idle')
        if (!event.cancelled) setError(remoteErrorText(t, event))
      }
    })
    planId.current = planRequest.id
    running.current = planRequest
  }

  const sort = (simulate = false): void => {
    if (!plan) return
    // OK runs the sort and the send; Cancel launches nothing (the user can start the simulation themselves).
    if (remote && !simulate && !prefs.remoteUsed.includes(remoteLabel(prefs)) && !window.confirm(t('remote.firstTime'))) return
    dryRun.current = simulate
    setRemoteState(remote ? { phase: 'connect', dryRun: simulate } : null)
    setElapsed(0)
    setError(null)
    setResumable(false)
    setPhase('copying')
    copiedRef.current = new Set()
    setCopiedCount(0)
    setDoneByFolder(new Map())
    copiedTargets.current = []
    finishing.current = null
    credit.current = 0
    running.current = request({ op: 'copy', planId: planId.current, ...(remote ? { remote: remoteConfig(prefs, remotePassword), dryRun: simulate, remoteNoDedupe: !prefs.remoteDedupe, remoteRefresh: simulate } : {}) }, (event) => {
      if (event.type === 'progress') {
        buffer.current.push(event.entry)
        copiedTargets.current.push(`${plan.destination}/${event.entry.target}`)
      } else if (event.type === 'remotePhase') {
        setRemoteState((state) => ({
          ...(state ?? {}),
          phase: event.phase,
          ...(event.files !== undefined ? { files: event.files } : {}),
          ...(event.phase === 'send' ? { total: event.photos ?? event.total, dryRun: !!event.dryRun } : {}),
          ...(event.legacy !== undefined ? { legacy: !!event.legacy } : {})
        }))
      } else if (event.type === 'remoteProgress') {
        setRemoteState((state) => ({ ...(state ?? {}), progress: event }))
      } else if (event.type === 'done') {
        if (event.remote) {
          setRemoteState((state) => ({ legacy: state?.legacy, summary: event }))
          const label = remoteLabel(prefs)
          if (!event.dryRun && !prefs.remoteUsed.includes(label)) updatePrefs({ remoteUsed: [...prefs.remoteUsed, label] })
        }
        // The tick finishes once every copied photo has been displayed.
        finishing.current = 'done'
      } else if (event.type === 'error') {
        finishing.current = 'error'
        setPhase('ready')
        if (!event.cancelled) {
          setError(remote ? remoteErrorText(t, event) : event.message)
          if (remote && event.legacyRsync) setRemoteState((state) => ({ ...(state ?? {}), legacy: true }))
          setResumable(!!event.resumable)
        }
      }
    })
  }

  const startTags = (folder: string): void => {
    const files = tagScope ?? []
    tagBuffer.current = []
    const run = request({ op: 'tag', folder, files, tagScenes, tagMoment, tagPeople, tagMeta }, (event) => {
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
  const pending = useMemo(
    () => (copiedCount === 0 ? entries : entries.filter((entry) => !copiedRef.current.has(entry.source))),
    [entries, copiedCount]
  )
  const planFolders = useMemo(() => (plan ? sortedFolders(groupByFolder(plan.entries)) : null), [plan])
  const folders = planFolders ?? draftFolders
  const busy = phase === 'analyzing' || phase === 'copying'
  const tagging = tagJob?.state === 'loading' || tagJob?.state === 'running'
  const busyEnded = useRef(0)
  const wasBusy = useRef(false)
  if (wasBusy.current && !busy) busyEnded.current = performance.now()
  wasBusy.current = busy
  const cleanable = !!plan || !!error || sources.length > 0
  const canAnalyze = !busy && sources.length > 0 && (remote ? remoteReady(prefs) : !!destination.trim())
  const canSort = !busy && !tagging && phase === 'ready' && entries.length > 0
  const deep = skipDuplicates || groupSimilar || groupBlurry
  const progress =
    phase === 'analyzing' ? analysisProgress(stage, deep) : phase === 'copying' || phase === 'done' ? (entries.length ? copiedCount / entries.length : 0) : 0

  const sending = remote && phase === 'copying'
  useEffect(() => {
    if (!sending) return
    const timer = setInterval(() => setElapsed((value) => value + 1), 1000)
    return () => clearInterval(timer)
  }, [sending])
  const remoteTotal = remoteState?.progress?.total || remoteState?.total || 0
  const remoteDone = Math.max(remoteState?.progress?.done ?? 0, copiedCount)
  const indeterminate = sending && !(remoteState?.phase === 'send' && remoteTotal > 0)
  const barProgress = sending && !indeterminate ? Math.min(1, remoteDone / remoteTotal) : progress

  const animation = animationBudget(entries.length)
  budget.current = animation
  const renderPending = useCallback((entry: Entry, size: number) => {
    const since = arrived.current.get(entry.source)
    const fresh = since !== undefined && performance.now() - since < FRESH && sampled(entry.source, animation.every)
    return <Thumb path={entry.source} size={size} label={entry.source} pending appearDelay={fresh ? 0 : undefined} />
  }, [animation.every])

  return (
    <I18nContext.Provider value={t}>
    <div className={`app${animation.heavy && busy ? ' heavy' : ''}`}>
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
          <button className="gear" title={t('scrap')} aria-label={t('scrap')} onClick={() => setShowScrapbook(true)}>
            📅
          </button>
          <button className="gear" title={t('ymap')} aria-label={t('ymap')} onClick={() => setShowYearMap(true)}>
            🗺️
          </button>
          <button className="gear" title={t('travel')} aria-label={t('travel')} onClick={() => setShowTravelMap(true)}>
            🚐
          </button>
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
            {remote ? (
              <>
              <div className="row">
                <input value={remoteLabel(prefs)} readOnly disabled={busy} title={remoteLabel(prefs)} />
                <button className="btn ghost" disabled={busy} onClick={() => setShowRemote(true)}>
                  {t('remote.change')}
                </button>
              </div>
              <p className="remote-folder-line">{remoteFolder(prefs) ? t('remote.folderLine', { path: remoteFolder(prefs) }) : `⚠️ ${t('remote.folderLine.none')}`}</p>
              </>
            ) : (
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
            )}
            <button className="link remote-link" disabled={busy} onClick={() => (remote ? (updatePrefs({ remoteEnabled: false }), resetResults()) : setShowRemote(true))}>
              {remote ? t('remote.local') : t('remote.open')}
            </button>
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
              {t('sorted')} <em><Counter value={copiedCount} /></em>
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
        <div className={`progress${indeterminate ? ' indeterminate' : ''}`} role="progressbar" aria-busy={sending}>
          <motion.i animate={{ width: indeterminate ? '35%' : `${barProgress * 100}%` }} transition={{ type: 'spring', stiffness: 120, damping: 20 }} />
        </div>
        <div className="status">
          <AnimatePresence mode="wait">
            <motion.span key={error ?? phase} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -8 }}>
              {error
                ? `⚠️ ${error}`
                : phase === 'analyzing'
                  ? analysisStatus(t, stage)
                  : phase === 'copying'
                    ? remote
                      ? `${remotePhaseText(t, remoteState, copiedCount, entries.length)} · ${elapsedText(elapsed)}`
                      : t('status.copying', { done: copiedCount, total: entries.length })
                    : phase === 'done'
                      ? remote && remoteState?.summary
                        ? t('remote.status.done', { n: copiedCount, sent: remoteState.summary.sent ?? 0, existing: remoteState.summary.existing ?? 0 })
                        : t('status.done', { n: copiedCount })
                      : remoteState?.summary?.dryRun
                        ? t('remote.status.dryDone', { sent: remoteState.summary.sent ?? 0, existing: remoteState.summary.existing ?? 0 })
                        : plan
                        ? t('status.ready', { n: entries.length })
                        : t('status.idle')}
            </motion.span>
          </AnimatePresence>
          {(remoteState?.legacy || remoteState?.summary?.warnings?.includes('legacyRsync')) && <span className="remote-warn">{t('remote.warn.legacyRsync')}</span>}
          {remoteState?.summary?.warnings?.includes('libraryTimeout') && <span className="remote-warn">{t('remote.warn.libraryTimeout')}</span>}
          {(remoteState?.summary?.alreadyTotal ?? 0) > 0 && <span className="remote-warn">{t('remote.status.already', { n: remoteState?.summary?.alreadyTotal ?? 0 })}</span>}
          {(remoteState?.summary?.tags ?? 0) > 0 && <span className="remote-warn">{t('remote.status.tags', { n: remoteState?.summary?.tags ?? 0 })}</span>}
          {(remoteState?.summary?.mismatched ?? 0) > 0 && (
            <span className="remote-warn">
              {t('remote.warn.mismatch', { n: remoteState?.summary?.mismatched ?? 0, sample: (remoteState?.summary?.mismatchedSample ?? [])[0] ?? '' })}
            </span>
          )}
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
          {phase === 'done' && plan && !remote && (
            <button className="link" onClick={() => window.photosort.openPath(plan.destination)}>
              {t('openFolder')}
            </button>
          )}
          {phase === 'done' && !remote && copiedTargets.current.length > 0 && !tagging && (
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
          <motion.button
            className={`btn ${cleanable && phase === 'ready' ? 'hot' : 'ghost'}`}
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            disabled={busy || !cleanable}
            title={t('clean.hint')}
            onClick={() => {
              // A click aimed at a button that moved right when the work ended must not wipe the results.
              if (performance.now() - busyEnded.current < 800) return
              setSources([])
              resetResults()
            }}
          >
            {t('clean')}
          </motion.button>
          <motion.button
            className={`btn${canAnalyze && !plan ? ' hot' : ''}`}
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            disabled={!canAnalyze}
            onClick={analyze}
          >
            {t('analyze')}
          </motion.button>
          <motion.button
            className={`btn primary${canSort ? ' hot' : ''}`}
            whileHover={{ scale: 1.05 }}
            whileTap={{ scale: 0.95 }}
            disabled={!canSort}
            title={tagging ? t('tags.busy') : undefined}
            onClick={() => sort()}
          >
            {resumable ? t('remote.resume') : t('sort')}
          </motion.button>
          {remote && (
            <motion.button
              className="btn ghost"
              whileHover={{ scale: 1.05 }}
              whileTap={{ scale: 0.95 }}
              disabled={!canSort}
              title={t('remote.dryRun.hint')}
              onClick={() => sort(true)}
            >
              {t('remote.dryRun')}
            </motion.button>
          )}
          {busy && (
            <motion.button className="btn danger" whileTap={{ scale: 0.95 }} onClick={() => running.current?.cancel()}>
              {t('cancel')}
            </motion.button>
          )}
        </div>
      </footer>

      <AnimatePresence>
        {showRemote && (
          <RemotePanel
            prefs={prefs}
            busy={busy}
            password={remotePassword}
            onPassword={setRemotePassword}
            onChange={(patch) => {
              updatePrefs(patch)
              if (!('remoteChecksum' in patch || 'remoteRemember' in patch)) resetResults()
            }}
            onClose={() => setShowRemote(false)}
          />
        )}
      </AnimatePresence>

      <AnimatePresence>
        {showPrefs && <Preferences prefs={prefs} busy={busy} onChange={updatePrefs} onClose={() => setShowPrefs(false)} />}
      </AnimatePresence>

      <AnimatePresence>
        {showTags && (
          <TagsPanel
            job={tagJob}
            assets={assets}
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
        {showTravelMap && <TravelMapPanel prefs={prefs} onChange={updatePrefs} onClose={() => setShowTravelMap(false)} />}
        {showYearMap && <YearMapPanel prefs={prefs} onChange={updatePrefs} onClose={() => setShowYearMap(false)} />}
        {showScrapbook && <ScrapbookPanel prefs={prefs} onChange={updatePrefs} onClose={() => setShowScrapbook(false)} />}
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
              <IssueList failures={plan?.skipped ?? []} duplicates={duplicates} />
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
