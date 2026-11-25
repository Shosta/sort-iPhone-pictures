import { useId, useLayoutEffect, useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useT } from '../i18n'
import { request } from '../sidecar'

export interface ImportDevice {
  id: string
  name: string
  path: string
}

export type DcimMode = 'all' | 'new' | 'period'

/** Source reported to the parent: the card only picks a DCIM volume and a mode. */
export interface DcimSource {
  deviceId: string
  devicePath: string
  name: string
  mode: DcimMode
  from: string
  to: string
}

interface Props {
  /** Parent is busy (analysing / sorting): the source is frozen. */
  disabled: boolean
  open: boolean
  onClose: () => void
  /** True while the card replaces the drop zone. */
  onVisible: (visible: boolean) => void
  onSource: (source: DcimSource | null) => void
  locale: string
}

interface Scan {
  count: number
  videos: number
  from: string
  to: string
}

const POLL_MS = 3000


function InfoTip({ text }: { text: string }) {
  const [open, setOpen] = useState(false)
  const [hover, setHover] = useState(false)
  const [pos, setPos] = useState<{ left: number; top: number; below: boolean } | null>(null)
  const btn = useRef<HTMLButtonElement>(null)
  const pop = useRef<HTMLSpanElement>(null)
  const id = useId()
  const shown = open || hover

  useLayoutEffect(() => {
    if (!shown || !btn.current || !pop.current) return setPos(null)
    const a = btn.current.getBoundingClientRect()
    const w = pop.current.offsetWidth
    const h = pop.current.offsetHeight
    const below = a.top - h - 10 < 8
    const left = Math.min(Math.max(a.left + a.width / 2 - w / 2, 8), window.innerWidth - w - 8)
    setPos({ left, top: below ? a.bottom + 8 : a.top - h - 8, below })
  }, [shown, text])

  return (
    <span className="info-tip" onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}>
      <button
        ref={btn}
        type="button"
        className="info-tip-btn"
        aria-label={text}
        aria-describedby={shown ? id : undefined}
        onClick={() => setOpen((v) => !v)}
        onBlur={() => setOpen(false)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            setOpen(false)
            setHover(false)
            e.stopPropagation()
          }
        }}
      >
        i
      </button>
      {shown &&
        createPortal(
          <span
            ref={pop}
            id={id}
            role="tooltip"
            className="info-tip-pop"
            style={{ left: pos?.left ?? 0, top: pos?.top ?? 0, visibility: pos ? 'visible' : 'hidden' }}
          >
            {text}
          </span>,
          document.body
        )}
    </span>
  )
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


/** Overflow menu rendered in a portal, anchored under its button. */
function MoreMenu({ label, items }: { label: string; items: { label: string; onClick: () => void }[] }): JSX.Element {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ right: number; top: number } | null>(null)
  const btn = useRef<HTMLButtonElement>(null)
  const pop = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    if (!open || !btn.current) return
    const a = btn.current.getBoundingClientRect()
    setPos({ right: Math.max(8, window.innerWidth - a.right), top: a.bottom + 6 })
  }, [open])

  useEffect(() => {
    if (!open) return
    const close = (event: Event): void => {
      if (event.target instanceof Node && (pop.current?.contains(event.target) || btn.current?.contains(event.target))) return
      setOpen(false)
    }
    const key = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        setOpen(false)
        btn.current?.focus()
      }
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', key)
    window.addEventListener('resize', close)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', key)
      window.removeEventListener('resize', close)
    }
  }, [open])

  return (
    <>
      <button ref={btn} className="btn ghost import-more-btn" aria-haspopup="menu" aria-expanded={open} aria-label={label} title={label} onClick={() => setOpen((v) => !v)}>
        ⋯
      </button>
      {open &&
        createPortal(
          <div ref={pop} className="import-menu" role="menu" style={{ right: pos?.right ?? 8, top: pos?.top ?? 0, visibility: pos ? 'visible' : 'hidden' }}>
            {items.map((item) => (
              <button key={item.label} role="menuitem" autoFocus={items[0] === item} onClick={() => (setOpen(false), item.onClick())}>
                {item.label}
              </button>
            ))}
          </div>,
          document.body
        )}
    </>
  )
}

export function ImportCard({ disabled, open, onClose, onVisible, onSource, locale }: Props): JSX.Element | null {
  const t = useT()
  const [devices, setDevices] = useState<ImportDevice[]>([])
  const [manual, setManual] = useState<ImportDevice | null>(null)
  const [dismissed, setDismissed] = useState<string | null>(null)
  const [mode, setMode] = useState<DcimMode>('new')
  const year = new Date().getFullYear()
  const [from, setFrom] = useState(`${year}-01-01`)
  const [to, setTo] = useState(`${year}-12-31`)
  const [scan, setScan] = useState<Scan | null>(null)
  const [rescan, setRescan] = useState(0)
  const frozen = useRef(disabled)
  frozen.current = disabled

  const detected = devices.find((d) => d.id !== dismissed) ?? null
  const device = manual ?? detected
  const deviceId = device?.id
  const devicePath = device?.path
  const deviceName = device?.name
  const visible = device !== null || open
  const badRange = mode === 'period' && from > to

  useEffect(() => onVisible(visible), [visible, onVisible])

  useEffect(() => {
    onSource(devicePath === undefined ? null : { deviceId: deviceId ?? '', devicePath, name: deviceName ?? '', mode, from, to })
  }, [deviceId, devicePath, deviceName, mode, from, to, onSource])

  // The device list is not refreshed while the parent works, so a cable
  // glitch mid-sort does not drop the plan (the engine pauses by itself).
  useEffect(() => {
    let live = true
    const poll = (): void => {
      if (frozen.current) return
      request({ op: 'importDetect' }, (event) => {
        if (live && !frozen.current && event.type === 'importDevices') setDevices(event.devices ?? [])
      })
    }
    poll()
    const timer = setInterval(poll, POLL_MS)
    return () => {
      live = false
      clearInterval(timer)
    }
  }, [])

  useEffect(() => {
    if (devicePath === undefined || badRange) {
      setScan(null)
      return
    }
    if (disabled) return
    let live = true
    setScan(null)
    const run = request({ op: 'importScan', deviceId: deviceId ?? '', devicePath, mode, from, to }, (event) => {
      if (live && event.type === 'importScan') setScan({ count: event.count, videos: event.videos, from: event.from, to: event.to })
    })
    return () => {
      live = false
      run.cancel()
    }
  }, [deviceId, devicePath, mode, from, to, badRange, disabled, rescan])

  const pick = async (): Promise<void> => {
    const [folder] = await window.photosort.pickFolders(t('import.pickTitle'))
    if (folder) setManual({ id: '', name: baseName(folder), path: folder })
  }

  const forget = (): void => {
    if (!device) return
    request({ op: 'importForget', deviceId: device.id, devicePath: device.path }, () => setRescan((n) => n + 1))
  }

  const goBack = (): void => {
    setManual(null)
    if (detected) setDismissed(detected.id)
    onClose()
  }

  if (!visible) return null

  const modeLine = (value: DcimMode): string =>
    value === 'new'
      ? t('import.foot.new')
      : value === 'all'
        ? t('import.foot.all')
        : badRange
          ? t('import.badRange')
          : t('import.foot.period', { from: localDate(from, locale), to: localDate(to, locale) })

  const back = (
    <button className="link card-link" disabled={disabled} onClick={goBack}>
      <span className="card-link-icon" aria-hidden="true">📂</span>
      {t('import.back')}
    </button>
  )

  if (!device) {
    return (
      <Frame title={t('import.none.title')} footer={t('import.foot.none')}>
        <p className="drop-title">
          {t('import.none.title')}
          <InfoTip text={t('import.none.help')} />
        </p>
        <div className="import-actions">
          <button className="btn ghost" disabled={disabled} onClick={() => void pick()}>
            {t('import.pickFolder')}
          </button>
        </div>
        {back}
      </Frame>
    )
  }

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
  const modes: DcimMode[] = ['all', 'new', 'period']
  const isFolder = manual !== null

  return (
    <Frame title={isFolder ? t('import.ready.folder') : device.name} footer={modeLine(mode)}>
      <p className="drop-title">{isFolder ? t('import.ready.folder') : device.name}</p>
      {isFolder && <p className="import-help" title={device.path}>{device.name}</p>}
      <div className="import-modes" role="radiogroup" aria-label={t('import.mode')}>
        {modes.map((value) => (
          <button key={value} role="radio" aria-checked={mode === value} disabled={disabled} className={`seg${mode === value ? ' on' : ''}`} onClick={() => setMode(value)}>
            {t(`import.mode.${value}` as const)}
          </button>
        ))}
      </div>
      {mode === 'period' && (
        <div className="import-period">
          <label>
            {t('import.from')}
            <input type="date" value={from} max={to} disabled={disabled} onChange={(event) => setFrom(event.target.value)} />
          </label>
          <label>
            {t('import.to')}
            <input type="date" value={to} min={from} disabled={disabled} onChange={(event) => setTo(event.target.value)} />
          </label>
        </div>
      )}
      <p className="import-count" aria-live="polite">
        {countText}
        {scan && scan.videos > 0 && <span className="import-videos"> · {t('import.videos', { n: scan.videos })}</span>}
      </p>
      <div className="import-actions">
        <MoreMenu
          label={t('import.more')}
          items={[
            { label: t('import.reimport'), onClick: () => setMode('all') },
            { label: t('import.forget'), onClick: forget }
          ]}
        />
      </div>
      {back}
    </Frame>
  )
}
