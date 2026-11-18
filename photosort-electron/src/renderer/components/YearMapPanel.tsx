import confetti from 'canvas-confetti'
import { motion } from 'framer-motion'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useT } from '../i18n'
import type { Prefs, ScrapPage } from '../prefs'
import { cachedThumbnail, loadThumbnail, request } from '../sidecar'
import type { ScrapJob, YearMapLayout, YearPin, YearScan } from '../types'
import { Select } from './Select'

interface Props {
  prefs: Prefs
  onChange: (patch: Partial<Prefs>) => void
  onClose: () => void
}

const PAGES: ScrapPage[] = ['a4-landscape', 'a4-portrait', 'letter-landscape', 'letter-portrait']
// Card geometry of the engine at scale 1 (internal/yearmap/layout.go), in mm.
const THUMB = 24
const PAD = 1.8
const CARD_H = PAD + THUMB + 6 + PAD / 2

const dirName = (path: string): string => path.replace(/[\\/][^\\/]*$/, '')

export function PinThumb({ path, x, y, size }: { path: string; x: number; y: number; size: number }): JSX.Element {
  const [url, setUrl] = useState(() => cachedThumbnail(path))
  useEffect(() => {
    const known = cachedThumbnail(path)
    setUrl(known)
    if (known === undefined) return loadThumbnail(path, setUrl)
  }, [path])
  return (
    <>
      <rect x={x} y={y} width={size} height={size} fill="#e4dfd2" />
      {url && <image href={url} x={x} y={y} width={size} height={size} preserveAspectRatio="xMidYMid slice" />}
    </>
  )
}

function Pin({ pin }: { pin: YearPin }): JSX.Element {
  const { card } = pin
  const scale = card ? card.h / CARD_H : 1
  const side = THUMB * scale
  const gap = PAD * scale
  const span = pin.shown.length * side + (pin.shown.length - 1) * gap
  const anchor = card && { x: Math.max(card.x, Math.min(pin.x, card.x + card.w)), y: Math.max(card.y, Math.min(pin.y, card.y + card.h)) }
  const more = pin.count - pin.shown.length
  return (
    <g>
      {card && anchor && <line x1={pin.x} y1={pin.y} x2={anchor.x} y2={anchor.y} stroke="#d64541" strokeWidth={0.4} />}
      {card && (
        <>
          <rect x={card.x} y={card.y} width={card.w} height={card.h} rx={1.2 * scale} fill="#fff" stroke="#cdc8be" strokeWidth={0.2} />
          {pin.shown.map((path, i) => (
            <PinThumb key={path + i} path={path} x={card.x + (card.w - span) / 2 + i * (side + gap)} y={card.y + gap} size={side} />
          ))}
          <text x={card.x + card.w / 2} y={card.y + gap + side + 4.3 * scale} textAnchor="middle" fontSize={Math.max(4, 6.5 * scale) * 0.75} fontWeight={700} fill="#3a322a">
            {pin.name}
          </text>
          {more > 0 && (
            <>
              <circle cx={card.x + card.w} cy={card.y} r={3.4 * scale} fill="#d64541" />
              <text x={card.x + card.w} y={card.y + 1.2 * scale} textAnchor="middle" fontSize={5 * scale} fontWeight={700} fill="#fff">
                {more > 99 ? '99+' : `+${more}`}
              </text>
            </>
          )}
        </>
      )}
      <circle cx={pin.x} cy={pin.y} r={2.4} fill="#d64541" stroke="#fff" strokeWidth={0.8} />
    </g>
  )
}

export function YearMapPanel({ prefs, onChange, onClose }: Props): JSX.Element {
  const t = useT()
  const [root, setRoot] = useState(prefs.destination)
  const [years, setYears] = useState<number[] | null>(null)
  const [rootOk, setRootOk] = useState<boolean | null>(null)
  const [year, setYear] = useState<number | null>(null)
  const [scan, setScan] = useState<YearScan | null>(null)
  const [scanProgress, setScanProgress] = useState<{ done: number; total: number } | null>(null)
  const [scanError, setScanError] = useState<string | null>(null)
  const [layout, setLayout] = useState<YearMapLayout | null>(null)
  const [layoutError, setLayoutError] = useState<string | null>(null)
  const [job, setJob] = useState<ScrapJob | null>(null)
  const [downloads, setDownloads] = useState('')
  const run = useRef<{ cancel: () => void } | null>(null)

  const rootDir = root.trim()
  const running = job?.state === 'running'
  const outDir = prefs.scrapOutputDir.trim() || downloads
  const places = scan?.places

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

  useEffect(() => {
    setYears(null)
    setRootOk(null)
    setYear(null)
    if (!scanRoot) {
      setYears([])
      return
    }
    let live = true
    const handle = request({ op: 'yearMapYears', root: scanRoot }, (event) => {
      if (!live) return
      if (event.type === 'yearMapYears') {
        setYears(event.years)
        setRootOk(event.rootOk)
        setYear(event.years[0] ?? null)
      } else if (event.type === 'error' && !event.cancelled) setYears([])
    })
    return () => {
      live = false
      handle.cancel()
    }
  }, [scanRoot])

  // Reading the GPS positions of a year is slow: it runs once per year.
  useEffect(() => {
    setScan(null)
    setScanError(null)
    setLayout(null)
    setJob(null)
    if (year === null || !scanRoot) return
    setScanProgress({ done: 0, total: 0 })
    let live = true
    const handle = request({ op: 'yearMapScan', root: scanRoot, year }, (event) => {
      if (!live) return
      if (event.type === 'status') setScanProgress({ done: event.done, total: event.total })
      else if (event.type === 'places') {
        setScan(event)
        setScanProgress(null)
      } else if (event.type === 'error' && !event.cancelled) {
        setScanProgress(null)
        setScanError(event.message)
      }
    })
    return () => {
      live = false
      handle.cancel()
    }
  }, [scanRoot, year])

  useEffect(() => {
    setLayoutError(null)
    if (!places || places.length === 0) {
      setLayout(null)
      return
    }
    let live = true
    const handle = request({ op: 'yearMapLayout', places, page: prefs.yearMapPage }, (event) => {
      if (!live) return
      if (event.type === 'layout') setLayout(event)
      else if (event.type === 'error' && !event.cancelled) setLayoutError(event.message)
    })
    return () => {
      live = false
      handle.cancel()
    }
  }, [places, prefs.yearMapPage])

  const summary = useMemo(() => (scan ? { places: scan.places.length, photos: scan.located, total: scan.total } : null), [scan])

  const create = (): void => {
    if (!places || places.length === 0 || year === null || !outDir || !summary) return
    setJob({ state: 'running', done: 0, total: places.reduce((sum, place) => sum + place.shown.length, 0) })
    run.current = request(
      {
        op: 'yearMapPdf',
        places,
        page: prefs.yearMapPage,
        title: t('ymap.title', { year }),
        subtitle: t('ymap.subtitle', { photos: summary.photos, places: summary.places }),
        outDir,
        fallbackDir: downloads,
        name: t('ymap.file', { year })
      },
      (event) => {
        if (event.type === 'progress') setJob((previous) => previous && { ...previous, done: event.done, total: event.total })
        else if (event.type === 'done') {
          setJob({ state: 'done', done: event.places, total: event.places, path: event.path, pages: 1, photos: event.places, fallback: event.fallback, failed: event.failed ?? [] })
          confetti({ particleCount: 90, spread: 70, origin: { y: 0.85 } })
          void window.photosort.openPath(event.path)
        } else if (event.type === 'error') setJob(event.cancelled ? null : { state: 'error', done: 0, total: 0, message: event.message })
      }
    )
  }

  const progress = job && job.total > 0 ? job.done / job.total : 0
  const scanning = scanProgress !== null

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
          <h2>🗺️ {t('ymap')}</h2>
          <p className="hint">{t('ymap.hint')}</p>
        </header>

        <div className="scrap-body">
          <aside className={`scrap-side${running ? ' locked' : ''}`}>
            <div className="field">
              <label>{t('ymap.root')}</label>
              <div className="row">
                <input value={root} disabled={running} placeholder={t('destination.placeholder')} onChange={(event) => setRoot(event.target.value)} />
                <button
                  className="btn ghost"
                  disabled={running}
                  onClick={async () => {
                    const [path] = await window.photosort.pickFolders(t('ymap.root'))
                    if (path) setRoot(path)
                  }}
                >
                  {t('choose')}
                </button>
              </div>
              {!rootDir && <p className="hint">{t('ymap.rootEmpty')}</p>}
              {rootDir && rootOk === false && <p className="hint warn">{t('ymap.rootUnknown')}</p>}
            </div>
            <div className="field">
              <label>{t('ymap.year')}</label>
              {years && years.length > 0 ? (
                <Select value={String(year ?? years[0])} options={years.map((y) => ({ value: String(y), label: String(y), icon: null }))} onChange={(value) => setYear(Number(value))} />
              ) : (
                <p className="hint">{years ? t('ymap.noYears') : t('scrap.loading')}</p>
              )}
            </div>
            <div className="field">
              <label>{t('ymap.paper')}</label>
              <Select value={prefs.yearMapPage} options={PAGES.map((page) => ({ value: page, label: t(`scrap.page.${page}`), icon: null }))} onChange={(yearMapPage) => onChange({ yearMapPage })} />
            </div>
            {summary && <p className="muted">{summary.places > 0 ? t('ymap.summary', { places: summary.places, located: summary.photos, total: summary.total }) : t('ymap.empty')}</p>}
          </aside>

          <section className="scrap-main">
            <div className="ymap-preview">
              {scanError || layoutError ? (
                <p className="muted">⚠️ {scanError ?? layoutError}</p>
              ) : scanning ? (
                <p className="muted">{t('ymap.reading', { done: scanProgress?.done ?? 0, total: scanProgress?.total ?? 0 })}</p>
              ) : !layout ? (
                <p className="muted">{summary && summary.places === 0 ? t('ymap.empty') : places ? t('ymap.layouting') : t('ymap.pick')}</p>
              ) : (
                <motion.svg className="ymap-sheet" viewBox={`0 0 ${layout.pageWidth} ${layout.pageHeight}`} style={{ aspectRatio: `${layout.pageWidth} / ${layout.pageHeight}` }} initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }}>
                  <rect width={layout.pageWidth} height={layout.pageHeight} fill="#cee1ec" />
                  <path d={layout.land} fill="#f6f0e2" fillRule="evenodd" stroke="#bab098" strokeWidth={0.2} />
                  <path d={layout.lakes} fill="#cee1ec" fillRule="evenodd" stroke="#bab098" strokeWidth={0.2} />
                  {layout.pins.map((pin, index) => (
                    <Pin key={index} pin={pin} />
                  ))}
                </motion.svg>
              )}
            </div>
            <p className="hint">{t('ymap.credit')}</p>
          </section>
        </div>

        <footer className="scrap-foot">
          <div className="scrap-save">
            <p className="hint" title={outDir}>
              {t('ymap.saveIn')} : <code>{outDir}</code>
            </p>
            {running && (
              <div className="progress">
                <i style={{ width: `${progress * 100}%`, transition: 'width 0.3s ease' }} />
              </div>
            )}
            {running && <p className="muted">{t('ymap.progress', { done: job.done, total: job.total })}</p>}
            {job?.state === 'done' && (
              <p className="muted">
                {t('ymap.done', { places: job.photos ?? 0 })}
                {job.fallback && job.path && <span className="warn"> · {t('scrap.fallback', { dir: dirName(job.path) })}</span>}
              </p>
            )}
            {job?.state === 'done' && job.failed && job.failed.length > 0 && <p className="hint warn">{t('ymap.unreadable', { n: job.failed.length })}</p>}
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
              <motion.button className="btn primary" disabled={!layout || !outDir} whileHover={{ scale: 1.04 }} whileTap={{ scale: 0.96 }} onClick={create}>
                {t('ymap.create')}
              </motion.button>
            )}
            <button className="btn" onClick={onClose}>
              {t('close')}
            </button>
          </div>
        </footer>
      </motion.div>
    </motion.div>
  )
}
