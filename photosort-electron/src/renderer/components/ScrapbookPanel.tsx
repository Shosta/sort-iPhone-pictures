import confetti from 'canvas-confetti'
import { motion } from 'framer-motion'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useT } from '../i18n'
import type { Prefs, ScrapPage } from '../prefs'
import { cachedThumbnail, loadThumbnail, request } from '../sidecar'
import type { ScrapJob, ScrapLayout, ScrapPhoto } from '../types'
import { addDays, Calendar, isoDay, isoWeek, mondayOf, monthWeeks } from './Calendar'
import { Flights, type Flight } from './Flights'
import { Select } from './Select'
import { Thumb } from './Thumb'
import { VirtualGrid } from './VirtualGrid'

interface Props {
  prefs: Prefs
  onChange: (patch: Partial<Prefs>) => void
  onClose: () => void
}

const PAGES: ScrapPage[] = [
  'a4-landscape', 'a4-portrait', 'letter-landscape', 'letter-portrait',
  'a5-landscape', 'a5-portrait', '10x15-landscape', '10x15-portrait',
  '13x18-landscape', '13x18-portrait', 'square-20', 'square-21'
]
/** Thumbnails that take off when the PDF is created. */
const MAX_FLIGHTS = 14

const parseDay = (iso: string): Date => {
  const [year, month, day] = iso.split('-').map(Number)
  return new Date(year, month - 1, day)
}
const baseName = (path: string): string => path.split(/[\\/]/).pop() ?? path
const dirName = (path: string): string => path.replace(/[\\/][^\\/]*$/, '')
const squash = (text: string): string => text.replace(/\s+/g, ' ')

/** A photo placed on the preview sheet, drawn from the thumbnail already in cache. */
function PreviewPhoto({ x, y, w, h, path }: { x: number; y: number; w: number; h: number; path: string }): JSX.Element {
  const [url, setUrl] = useState(() => cachedThumbnail(path))
  useEffect(() => {
    const known = cachedThumbnail(path)
    setUrl(known)
    if (known === undefined) return loadThumbnail(path, setUrl)
  }, [path])
  return (
    <g className="scrap-ph" style={{ transform: `translate(${x}px, ${y}px)` }}>
      <rect width={w} height={h} className="scrap-ph-bg" />
      {url && <image href={url} width={w} height={h} preserveAspectRatio="xMidYMid slice" />}
      <rect width={w} height={h} className="scrap-ph-edge" />
    </g>
  )
}

export function ScrapbookPanel({ prefs, onChange, onClose }: Props): JSX.Element {
  const t = useT()
  const locale = prefs.language
  const [root, setRoot] = useState(prefs.destination)
  const [month, setMonth] = useState(() => new Date(new Date().getFullYear(), new Date().getMonth(), 1))
  const [monday, setMonday] = useState<Date>(() => mondayOf(new Date()))
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [rootOk, setRootOk] = useState<boolean | null>(null)
  const [photos, setPhotos] = useState<ScrapPhoto[] | null>(null)
  const [photosError, setPhotosError] = useState<string | null>(null)
  const [checked, setChecked] = useState<Set<string>>(new Set())
  const [layout, setLayout] = useState<ScrapLayout | null>(null)
  const [layoutBusy, setLayoutBusy] = useState(false)
  const [layoutError, setLayoutError] = useState<string | null>(null)
  const [job, setJob] = useState<ScrapJob | null>(null)
  const [downloads, setDownloads] = useState('')
  const [flights, setFlights] = useState<Flight[]>([])
  const previewRef = useRef<HTMLDivElement>(null)
  const run = useRef<{ cancel: () => void } | null>(null)
  const removeFlight = useCallback((id: string) => setFlights((previous) => previous.filter((flight) => flight.id !== id)), [])

  const rootDir = root.trim()
  const running = job?.state === 'running'
  const monthKey = `${month.getFullYear()}-${month.getMonth()}`
  const weekKey = isoDay(monday)

  useEffect(() => {
    void window.photosort.downloadsDir().then(setDownloads)
    return () => run.current?.cancel()
  }, [])

  // The root is typed by hand: wait for a pause before scanning it.
  const [scanRoot, setScanRoot] = useState(rootDir)
  useEffect(() => {
    const timer = setTimeout(() => setScanRoot(rootDir), 350)
    return () => clearTimeout(timer)
  }, [rootDir])

  // Photos per day of the visible calendar grid (adjacent months included).
  useEffect(() => {
    if (!scanRoot) {
      setCounts({})
      setRootOk(null)
      return
    }
    const weeks = monthWeeks(month)
    let live = true
    const handle = request({ op: 'weekDays', root: scanRoot, from: isoDay(weeks[0][0]), to: isoDay(weeks[weeks.length - 1][6]) }, (event) => {
      if (!live) return
      if (event.type === 'weekDays') {
        setCounts(event.days ?? {})
        setRootOk(event.rootOk)
      } else if (event.type === 'error' && !event.cancelled) {
        setCounts({})
      }
    })
    return () => {
      live = false
      handle.cancel()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scanRoot, monthKey])

  // Photos of the picked week; nothing is ticked at first.
  useEffect(() => {
    setChecked(new Set())
    setPhotosError(null)
    setJob(null)
    if (!scanRoot) {
      setPhotos([])
      return
    }
    setPhotos(null)
    let live = true
    const handle = request({ op: 'weekPhotos', root: scanRoot, from: weekKey, to: isoDay(addDays(monday, 6)) }, (event) => {
      if (!live) return
      if (event.type === 'photos') setPhotos(event.photos ?? [])
      else if (event.type === 'error' && !event.cancelled) {
        setPhotos([])
        setPhotosError(event.message)
      }
    })
    return () => {
      live = false
      handle.cancel()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scanRoot, weekKey])

  const files = useMemo(() => (photos ?? []).filter((photo) => checked.has(photo.path)).map((photo) => photo.path), [photos, checked])
  const filesKey = files.join('\n')

  // Live page preview: the engine measures the ticked photos and lays them out.
  useEffect(() => {
    setLayoutError(null)
    if (files.length === 0) {
      setLayout(null)
      setLayoutBusy(false)
      return
    }
    setLayoutBusy(true)
    let live = true
    let handle: { cancel: () => void } | null = null
    const timer = setTimeout(() => {
      handle = request({ op: 'scrapbookLayout', files, page: prefs.scrapPage, longSide: prefs.scrapLongSide, gutter: prefs.scrapGutter, bleed: prefs.scrapBleed ? 3 : 0 }, (event) => {
        if (!live) return
        if (event.type === 'layout') {
          setLayout(event)
          setLayoutBusy(false)
        } else if (event.type === 'error' && !event.cancelled) {
          setLayout(null)
          setLayoutBusy(false)
          setLayoutError(event.message)
        }
      })
    }, 200)
    return () => {
      live = false
      clearTimeout(timer)
      handle?.cancel()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filesKey, prefs.scrapPage, prefs.scrapLongSide, prefs.scrapGutter, prefs.scrapBleed])

  const range = useMemo(() => {
    const format = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'long' })
    return squash(format.formatRange(monday, addDays(monday, 6)))
  }, [locale, monday])
  const week = isoWeek(monday)
  const dayFormat = useMemo(() => new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric' }), [locale])
  const now = new Date()
  const onThisWeek = weekKey === isoDay(mondayOf(now)) && monthKey === `${now.getFullYear()}-${now.getMonth()}`
  const outDir = prefs.scrapOutputDir.trim() || downloads

  const toggle = useCallback((path: string) => {
    setChecked((previous) => {
      const next = new Set(previous)
      if (!next.delete(path)) next.add(path)
      return next
    })
  }, [])

  const goToday = (): void => {
    const now = new Date()
    setMonth(new Date(now.getFullYear(), now.getMonth(), 1))
    setMonday(mondayOf(now))
  }

  const launchFlights = (): void => {
    const target = previewRef.current?.getBoundingClientRect()
    if (!target) return
    const launched: Flight[] = []
    document.querySelectorAll<HTMLElement>('.scrap-tile.on').forEach((tile, index) => {
      const url = tile.querySelector('img')?.src
      if (!url || launched.length >= MAX_FLIGHTS) return
      const box = tile.getBoundingClientRect()
      launched.push({
        id: `${Date.now()}-${index}`,
        url,
        from: { x: box.left + box.width / 2, y: box.top + box.height / 2, size: Math.min(box.width, box.height) },
        to: { x: target.left + target.width / 2, y: target.top + target.height / 2 },
        delay: index * 0.045
      })
    })
    if (launched.length > 0) setFlights((previous) => previous.concat(launched))
  }

  const create = (): void => {
    if (files.length === 0 || !outDir) return
    launchFlights()
    const name = squash(t('scrap.file', { year: week.year, week: String(week.week).padStart(2, '0'), range }))
    setJob({ state: 'running', done: 0, total: files.length })
    run.current = request(
      { op: 'scrapbook', files, page: prefs.scrapPage, longSide: prefs.scrapLongSide, gutter: prefs.scrapGutter, bleed: prefs.scrapBleed ? 3 : 0, outDir, fallbackDir: downloads, name },
      (event) => {
        if (event.type === 'progress') {
          setJob((previous) => previous && { ...previous, done: event.done, total: event.total })
        } else if (event.type === 'done') {
          setJob({ state: 'done', done: event.photos, total: event.photos, path: event.path, pages: event.pages, photos: event.photos, fallback: event.fallback, failed: event.failed ?? [] })
          confetti({ particleCount: 90, spread: 70, origin: { y: 0.85 } })
          void window.photosort.openPath(event.path)
        } else if (event.type === 'error') {
          setJob(event.cancelled ? null : { state: 'error', done: 0, total: files.length, message: event.message })
        }
      }
    )
  }

  const sheets = layout?.sheets ?? []
  const placed = sheets.reduce((sum, sheet) => sum + sheet.placements.length, 0)
  const progress = job && job.total > 0 ? job.done / job.total : 0

  return (
    <motion.div className="overlay" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onClick={() => !running && onClose()}>
      <motion.div
        className="glass modal prefs scrap-panel"
        initial={{ scale: 0.92, y: 30 }}
        animate={{ scale: 1, y: 0 }}
        exit={{ scale: 0.92, y: 30 }}
        onClick={(event) => event.stopPropagation()}
      >
        <header className="scrap-head">
          <h2>📅 {t('scrap')}</h2>
          <p className="hint">{t('scrap.hint')}</p>
        </header>

        <div className="scrap-body">
          <aside className={`scrap-side${running ? ' locked' : ''}`}>
            <div className="field">
              <label>{t('scrap.root')}</label>
              <div className="row">
                <input value={root} disabled={running} placeholder={t('destination.placeholder')} onChange={(event) => setRoot(event.target.value)} />
                <button
                  className="btn ghost"
                  disabled={running}
                  onClick={async () => {
                    const [path] = await window.photosort.pickFolders(t('scrap.root'))
                    if (path) setRoot(path)
                  }}
                >
                  {t('choose')}
                </button>
              </div>
              {!rootDir && <p className="hint">{t('scrap.rootEmpty')}</p>}
              {rootDir && rootOk === false && <p className="hint warn">{t('scrap.rootUnknown')}</p>}
            </div>

            <Calendar
              locale={locale}
              month={month}
              counts={counts}
              selected={weekKey}
              prevLabel={t('scrap.prev')}
              nextLabel={t('scrap.next')}
              onMonth={(delta) => setMonth((previous) => new Date(previous.getFullYear(), previous.getMonth() + delta, 1))}
              onPick={setMonday}
            />

            <div className="scrap-weekbar">
              <strong className="scrap-week">{t('scrap.week', { week: week.week, range })}</strong>
              <button className="link" disabled={onThisWeek} onClick={goToday}>
                {t('scrap.today')}
              </button>
            </div>

            <details className="scrap-settings">
              <summary>{t('scrap.settings')}</summary>
              <div className="field">
                <label>{t('scrap.paper')}</label>
                <Select
                  value={prefs.scrapPage}
                  options={PAGES.map((page) => ({ value: page, label: t(`scrap.page.${page}`), icon: null }))}
                  onChange={(scrapPage) => onChange({ scrapPage })}
                />
              </div>
              <div className="field">
                <label>
                  {t('scrap.longSide')} <em>{prefs.scrapLongSide / 10} cm</em>
                </label>
                <input type="range" min={50} max={80} step={1} value={prefs.scrapLongSide} onChange={(event) => onChange({ scrapLongSide: Number(event.target.value) })} />
              </div>
              <div className="field">
                <label>
                  {t('scrap.gutter')} <em>{prefs.scrapGutter / 10} cm</em>
                </label>
                <input type="range" min={5} max={10} step={1} value={prefs.scrapGutter} onChange={(event) => onChange({ scrapGutter: Number(event.target.value) })} />
              </div>
              <label className="check">
                <input type="checkbox" checked={prefs.scrapBleed} onChange={(event) => onChange({ scrapBleed: event.target.checked })} /> {t('scrap.bleed')}
              </label>
            </details>
          </aside>

          <section className="scrap-main">
            <div className="scrap-bar">
              <span className="muted">{photos ? t('scrap.selected', { n: files.length, total: photos.length }) : t('scrap.loading')}</span>
              <span className="scrap-bar-actions">
                <button className="link" disabled={!photos || photos.length === 0 || running} onClick={() => setChecked(new Set((photos ?? []).map((photo) => photo.path)))}>
                  {t('scrap.all')}
                </button>
                <button className="link" disabled={checked.size === 0 || running} onClick={() => setChecked(new Set())}>
                  {t('scrap.none')}
                </button>
              </span>
            </div>

            <div className="scrap-photos">
              {photos && photos.length > 0 ? (
                <VirtualGrid
                  items={photos}
                  itemKey={(photo) => photo.path}
                  minSize={104}
                  render={(photo, size) => {
                    const on = checked.has(photo.path)
                    return (
                      <button type="button" className={`scrap-tile${on ? ' on' : ''}`} aria-pressed={on} aria-label={t('scrap.toggle')} disabled={running} onClick={() => toggle(photo.path)}>
                        <Thumb path={photo.path} size={size} label={baseName(photo.path)} />
                        <span className="scrap-day">{dayFormat.format(parseDay(photo.date))}</span>
                        <motion.span className="scrap-check" initial={false} animate={{ scale: on ? 1 : 0, opacity: on ? 1 : 0 }} transition={{ type: 'spring', stiffness: 620, damping: 26 }}>
                          ✓
                        </motion.span>
                      </button>
                    )
                  }}
                />
              ) : (
                <p className="empty">{photosError ? `⚠️ ${photosError}` : photos === null ? t('scrap.loading') : rootDir ? t('scrap.empty') : t('scrap.pickWeek')}</p>
              )}
            </div>

            <div className="scrap-preview" ref={previewRef}>
              {layoutError ? (
                <p className="muted">⚠️ {layoutError}</p>
              ) : files.length === 0 ? (
                <p className="muted">{t('scrap.nopreview')}</p>
              ) : (
                <>
                  <div className={`scrap-sheets${layoutBusy ? ' busy' : ''}`}>
                    {layout &&
                      sheets.map((sheet, index) => (
                        <motion.svg
                          key={index}
                          className="scrap-sheet"
                          viewBox={`0 0 ${layout.pageWidth} ${layout.pageHeight}`}
                          style={{ aspectRatio: `${layout.pageWidth} / ${layout.pageHeight}` }}
                          initial={{ opacity: 0, y: 12, rotate: index % 2 ? 1.2 : -1.2 }}
                          animate={{ opacity: 1, y: 0, rotate: 0 }}
                          transition={{ type: 'spring', stiffness: 260, damping: 24, delay: index * 0.06 }}
                        >
                          {sheet.placements.map((placement) => (
                            <PreviewPhoto key={placement.path} {...placement} />
                          ))}
                        </motion.svg>
                      ))}
                  </div>
                  <p className="muted scrap-count">{layout ? t('scrap.sheets', { pages: sheets.length, n: placed }) : t('scrap.layouting')}</p>
                </>
              )}
              {layout && layout.failed.length > 0 && <p className="hint warn">{t('scrap.unreadable', { n: layout.failed.length })}</p>}
            </div>
          </section>
        </div>

        <footer className="scrap-foot">
          <div className="scrap-save">
            <p className="hint" title={outDir}>
              {t('scrap.saveIn')} : <code>{outDir}</code>
            </p>
            <p className="hint">{t('scrap.print')}</p>
            {job?.state === 'running' && (
              <div className="progress">
                <i style={{ width: `${progress * 100}%`, transition: 'width 0.3s ease' }} />
              </div>
            )}
            {job?.state === 'running' && <p className="muted">{t('scrap.progress', { done: job.done, total: job.total })}</p>}
            {job?.state === 'done' && (
              <p className="muted">
                {t('scrap.done', { pages: job.pages ?? 0, photos: job.photos ?? 0 })}
                {job.fallback && job.path && <span className="warn"> · {t('scrap.fallback', { dir: dirName(job.path) })}</span>}
              </p>
            )}
            {job?.state === 'done' && job.failed && job.failed.length > 0 && <p className="hint warn">{t('scrap.unreadable', { n: job.failed.length })}</p>}
            {job?.state === 'error' && <p className="muted">⚠️ {job.message}</p>}
          </div>
          <div className="row end">
            {job?.state === 'done' && job.path && (
              <button className="btn ghost" onClick={() => void window.photosort.openPath(job.path as string)}>
                {t('scrap.open')}
              </button>
            )}
            {running ? (
              <button className="btn danger" onClick={() => run.current?.cancel()}>
                {t('cancel')}
              </button>
            ) : (
              <motion.button className="btn primary" disabled={files.length === 0 || layoutBusy || !outDir} whileHover={{ scale: 1.04 }} whileTap={{ scale: 0.96 }} onClick={create}>
                {t('scrap.create')}
              </motion.button>
            )}
            <button className="btn" onClick={onClose}>
              {t('close')}
            </button>
          </div>
        </footer>
      </motion.div>
      <Flights flights={flights} onDone={removeFlight} />
    </motion.div>
  )
}
