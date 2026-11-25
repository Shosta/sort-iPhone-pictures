import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useT, type Translate } from '../i18n'
import { remoteErrorText } from '../remote'
import { request } from '../sidecar'

export interface ImportDevice {
  id: string
  name: string
  path: string
}

export interface ImportSortOptions {
  destination: string
  remote?: Record<string, unknown>
  remoteNoDedupe?: boolean
  byLocation: boolean
  landmarks: boolean
  dedupe: boolean
  similar: boolean
  dateFallback: boolean
  screenshots: boolean
  blurry: boolean
}

interface Props {
  options: ImportSortOptions
  /** Why importing is not possible right now (NAS destination, no destination), or null. */
  blocked: string | null
  /** Parent is busy with a regular plan/copy. */
  disabled: boolean
  open: boolean
  onClose: () => void
  /** True while the card replaces the drop zone. */
  onVisible: (visible: boolean) => void
  /** True while an import is running (locks the rest of the UI). */
  onRunning: (running: boolean) => void
  onFinished: () => void
  locale: string
}

type Mode = 'all' | 'new' | 'period'
type Stage = 'ready' | 'running' | 'done'

interface Scan {
  count: number
  videos: number
  from: string
  to: string
}

interface Progress {
  phase: 'copy' | 'sort' | 'send'
  batch: number
  batches: number
  copied: number
  done: number
  total: number
  elapsedMs: number
}

interface Summary {
  imported: number
  duplicates: number
  videos: number
}

const POLL_MS = 3000

function clock(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

function errorText(t: Translate, event: { code?: string; detail?: string; message?: string; resumable?: boolean }): string {
  if (event.code === 'lowDisk') return t('import.err.lowDisk')
  if (event.code === 'noDevice') return t('import.err.noDevice')
  if (event.code === 'copy') return t('import.err.copy', { detail: event.detail ?? '' })
  return remoteErrorText(t, event)
}

function localDate(iso: string, locale: string): string {
  const day = new Date(`${iso.slice(0, 10)}T00:00:00`)
  return Number.isNaN(day.getTime()) ? iso : day.toLocaleDateString(locale)
}

function baseName(path: string): string {
  const parts = path.split(/[\\/]+/).filter(Boolean)
  return parts[parts.length - 1] ?? path
}

/** Same frame as the folder drop zone: icon, text, actions, then a footer line. */
function Frame({ title, footer, children }: { title: string; footer: string; children: ReactNode }): JSX.Element {
  return (
    <div className="dropzone glass import-card" role="region" aria-label={title}>
      <div className="card-body">
        <div className="drop-icon" aria-hidden="true">📱</div>
        {children}
      </div>
      <p className="card-foot">{footer}</p>
    </div>
  )
}

export function ImportCard({ options, blocked, disabled, open, onClose, onVisible, onRunning, onFinished, locale }: Props): JSX.Element | null {
  const t = useT()
  const [devices, setDevices] = useState<ImportDevice[]>([])
  const [manual, setManual] = useState<ImportDevice | null>(null)
  const [mode, setMode] = useState<Mode>('new')
  const year = new Date().getFullYear()
  const [from, setFrom] = useState(`${year}-01-01`)
  const [to, setTo] = useState(`${year}-12-31`)
  const [scan, setScan] = useState<Scan | null>(null)
  const [stage, setStage] = useState<Stage>('ready')
  const [progress, setProgress] = useState<Progress | null>(null)
  const [paused, setPaused] = useState<string | null>(null)
  const [summary, setSummary] = useState<Summary | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [menu, setMenu] = useState(false)
  const job = useRef<{ id: string; cancel: () => void } | null>(null)

  const device = devices[0] ?? manual
  const deviceId = device?.id
  const devicePath = device?.path
  const running = stage === 'running'
  const visible = running || stage === 'done' || device !== null || open
  const badRange = mode === 'period' && from > to

  useEffect(() => onVisible(visible), [visible, onVisible])
  useEffect(() => onRunning(running), [running, onRunning])

  useEffect(() => {
    if (running || stage === 'done') return
    let live = true
    const poll = (): void => {
      request({ op: 'importDetect' }, (event) => {
        if (live && event.type === 'importDevices') setDevices(event.devices ?? [])
      })
    }
    poll()
    const timer = setInterval(poll, POLL_MS)
    return () => {
      live = false
      clearInterval(timer)
    }
  }, [running, stage])

  useEffect(() => {
    if (devicePath === undefined || stage !== 'ready' || badRange) {
      setScan(null)
      return
    }
    let live = true
    setScan(null)
    const run = request({ op: 'importScan', deviceId: deviceId ?? '', devicePath, mode, from, to }, (event) => {
      if (!live) return
      if (event.type === 'importScan') setScan({ count: event.count, videos: event.videos, from: event.from, to: event.to })
    })
    return () => {
      live = false
      run.cancel()
    }
  }, [deviceId, devicePath, mode, from, to, stage, badRange])

  useEffect(() => () => job.current?.cancel(), [])

  const start = useCallback(
    (runMode: Mode) => {
      if (!device) return
      setError(null)
      setPaused(null)
      setSummary(null)
      setProgress(null)
      setStage('running')
      job.current = request(
        { op: 'importRun', deviceId: device.id, devicePath: device.path, mode: runMode, from, to, ...options },
        (event) => {
          if (event.type === 'importProgress') setProgress(event)
          else if (event.type === 'importPaused') setPaused(event.reason ?? 'manual')
          else if (event.type === 'importResumed') setPaused(null)
          else if (event.type === 'importDone') {
            setSummary({ imported: event.imported, duplicates: event.duplicates, videos: event.videos })
            setStage('done')
            job.current = null
            onFinished()
          } else if (event.type === 'error') {
            job.current = null
            setStage('ready')
            if (!event.cancelled) setError(errorText(t, event))
            onFinished()
          }
        }
      )
    },
    [device, from, to, options, onFinished, t]
  )

  const control = (action: 'pause' | 'resume'): void => {
    if (!job.current) return
    window.photosort.send({ op: 'importControl', id: `ctl-${Date.now()}`, target: job.current.id, action })
  }

  const pick = async (): Promise<void> => {
    const [folder] = await window.photosort.pickFolders(t('import.pickTitle'))
    if (folder) setManual({ id: '', name: baseName(folder), path: folder })
  }

  const forget = (): void => {
    if (!device) return
    setMenu(false)
    request({ op: 'importForget', deviceId: device.id, devicePath: device.path }, () => setScan(null))
  }

  if (!visible) return null

  const modeLine = (value: Mode): string =>
    value === 'new'
      ? t('import.foot.new')
      : value === 'all'
        ? t('import.foot.all')
        : badRange
          ? t('import.badRange')
          : t('import.foot.period', { from: localDate(from, locale), to: localDate(to, locale) })

  const back = (
    <button className="link card-link" onClick={() => (setManual(null), onClose())}>
      <span className="card-link-icon" aria-hidden="true">📂</span>
      {t('import.back')}
    </button>
  )

  if (running) {
    const total = progress?.total || scan?.count || 0
    const done = progress ? (progress.copied + progress.done) / Math.max(1, 2 * total) : 0
    return (
      <Frame title={t('import.title')} footer={modeLine(mode)}>
        <p className="drop-title">{device?.name ?? t('import.title')}</p>
        <p className="import-phase" aria-live="polite">
          {paused === 'disconnected'
            ? t('import.disconnected')
            : paused === 'network'
              ? t('import.network')
            : paused
              ? t('import.paused')
              : progress
                ? `${t(progress.phase === 'copy' ? 'import.phase.copy' : progress.phase === 'send' ? 'import.phase.send' : 'import.phase.sort')} · ${t('import.batch', { n: progress.batch, total: progress.batches })}`
                : t('import.counting')}
        </p>
        <div className={`progress import-bar${progress ? '' : ' indeterminate'}`} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(done * 100)}>
          <i style={{ width: `${Math.round(done * 100)}%` }} />
        </div>
        <p className="import-meta">{progress ? `${t('import.progress', { done: progress.done, total })} · ${clock(progress.elapsedMs)}` : ''}</p>
        <div className="import-actions">
          {paused !== 'disconnected' && paused !== 'network' && (
            <button className="btn ghost" onClick={() => control(paused ? 'resume' : 'pause')}>
              {paused ? t('import.resume') : t('import.pause')}
            </button>
          )}
          <button className="btn danger" onClick={() => job.current?.cancel()}>
            {t('import.cancel')}
          </button>
        </div>
      </Frame>
    )
  }

  if (stage === 'done' && summary) {
    return (
      <Frame title={t('import.done.title')} footer={modeLine(mode)}>
        <p className="drop-title">✅ {t('import.done.title')}</p>
        <p className="import-count" role="status">{t('import.done.summary', { ...summary })}</p>
        <div className="import-actions">
          <button className="btn primary" autoFocus onClick={() => setStage('ready')}>
            {t('import.done.ok')}
          </button>
        </div>
      </Frame>
    )
  }

  if (!device) {
    return (
      <Frame title={t('import.none.title')} footer={t('import.foot.none')}>
        <p className="drop-title">{t('import.none.title')}</p>
        <p className="import-help">{t('import.none.help')}</p>
        <div className="import-actions">
          <button className="btn ghost" disabled={disabled} onClick={() => void pick()}>
            {t('import.pickFolder')}
          </button>
        </div>
        {back}
      </Frame>
    )
  }

  // Only real preconditions disable the button; the live count never does.
  const zero = scan !== null && scan.count === 0
  const countText = !scan
    ? badRange
      ? t('import.badRange')
      : t('import.counting')
    : zero
      ? t(mode === 'new' ? 'import.zero.new' : mode === 'period' ? 'import.zero.period' : 'import.zero.all')
      : scan.count === 1
        ? t('import.count.one', { from: localDate(scan.from, locale) })
        : t('import.count', { n: scan.count, from: localDate(scan.from, locale), to: localDate(scan.to, locale) })
  const reason = disabled ? null : (blocked ?? (badRange ? t('import.badRange') : null))
  const cannot = disabled || !!blocked || badRange
  const modes: Mode[] = ['all', 'new', 'period']

  return (
    <Frame title={device.name} footer={modeLine(mode)}>
      <p className="drop-title">{t(devices[0] ? 'import.ready' : 'import.ready.folder', { name: device.name })}</p>
      {!devices[0] && <p className="import-help" title={device.path}>{device.name}</p>}
      <div className="import-modes" role="radiogroup" aria-label={t('import.mode')}>
        {modes.map((value) => (
          <button key={value} role="radio" aria-checked={mode === value} className={`seg${mode === value ? ' on' : ''}`} onClick={() => setMode(value)}>
            {t(`import.mode.${value}` as const)}
          </button>
        ))}
      </div>
      {mode === 'period' && (
        <div className="import-period">
          <label>
            {t('import.from')}
            <input type="date" value={from} max={to} onChange={(event) => setFrom(event.target.value)} />
          </label>
          <label>
            {t('import.to')}
            <input type="date" value={to} min={from} onChange={(event) => setTo(event.target.value)} />
          </label>
        </div>
      )}
      <p className="import-count" aria-live="polite">
        {countText}
        {scan && scan.videos > 0 && <span className="import-videos"> · {t('import.videos', { n: scan.videos })}</span>}
      </p>
      <div className="import-actions">
        <button className="btn hot" disabled={cannot} onClick={() => start(mode)}>
          {t('import.start')}
        </button>
        <div className="import-more">
          <button className="btn ghost" aria-haspopup="menu" aria-expanded={menu} aria-label={t('import.more')} title={t('import.more')} onClick={() => setMenu((value) => !value)}>
            ⋯
          </button>
          {menu && (
            <div className="import-menu glass" role="menu">
              <button role="menuitem" disabled={cannot} onClick={() => (setMenu(false), setMode('all'), start('all'))}>
                {t('import.reimport')}
              </button>
              <button role="menuitem" onClick={forget}>
                {t('import.forget')}
              </button>
            </div>
          )}
        </div>
      </div>
      {(reason || error) && (
        <p className="import-error" role="alert">
          {error ?? reason}
        </p>
      )}

      {back}
    </Frame>
  )
}
