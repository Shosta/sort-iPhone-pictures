import confetti from 'canvas-confetti'
import { motion } from 'framer-motion'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useT } from '../i18n'
import type { Prefs, ScrapPage } from '../prefs'
import { request } from '../sidecar'
import type { ScrapJob, TravelLayout, TravelStop, TravelTrip } from '../types'
import { Select } from './Select'
import { PinThumb, PROVIDERS } from './YearMapPanel'

interface Props {
  prefs: Prefs
  onChange: (patch: Partial<Prefs>) => void
  onClose: () => void
}

const PAGES: ScrapPage[] = ['a4-portrait', 'a4-landscape', 'letter-portrait', 'letter-landscape']
// Card geometry of the engine at scale 1 (internal/yearmap/layout.go), in mm.
const THUMB = 24
const PAD = 1.8
const CARD_H = PAD + THUMB + 6 + PAD / 2
const RED = '#b04034'
const INK = '#423428'

const dirName = (path: string): string => path.replace(/[\\/][^\\/]*$/, '')
const today = (): string => new Date().toISOString().slice(0, 10)

function Stop({ stop }: { stop: TravelStop }): JSX.Element {
  const { card } = stop
  const scale = card ? card.h / CARD_H : 1
  const side = THUMB * scale
  const gap = PAD * scale
  const anchor = card && { x: Math.max(card.x, Math.min(stop.x, card.x + card.w)), y: Math.max(card.y, Math.min(stop.y, card.y + card.h)) }
  return (
    <g>
      {card && anchor && <line x1={stop.x} y1={stop.y} x2={anchor.x} y2={anchor.y} stroke={RED} strokeWidth={0.25} />}
      {card && (
        <>
          <rect x={card.x + 0.6} y={card.y + 0.8} width={card.w} height={card.h} fill="#000" opacity={0.2} />
          <rect x={card.x} y={card.y} width={card.w} height={card.h} fill="#fff" stroke="#cdc4b4" strokeWidth={0.15} />
          <PinThumb path={stop.photo} x={card.x + (card.w - side) / 2} y={card.y + gap} size={side} />
          <text x={card.x + card.w / 2} y={card.y + gap + side + 2.9 * scale} textAnchor="middle" fontSize={Math.max(4, 6.8 * scale) * 0.75} fontWeight={700} fontFamily="Georgia, serif" fill={INK}>
            {stop.name}
          </text>
          <text x={card.x + card.w / 2} y={card.y + gap + side + 5.6 * scale} textAnchor="middle" fontSize={Math.max(3.5, 5 * scale) * 0.75} fontStyle="italic" fontFamily="Georgia, serif" fill="#806f5f">
            {stop.dates}
          </text>
          <Badge x={card.x} y={card.y} r={2.6 * scale} n={stop.n} />
        </>
      )}
      <Badge x={stop.x} y={stop.y} r={2.4} n={stop.n} />
    </g>
  )
}

function Badge({ x, y, r, n }: { x: number; y: number; r: number; n: number }): JSX.Element {
  const size = Math.max(4, r * 2.7) * (n >= 10 ? 0.8 : 1) * 0.75
  return (
    <>
      <circle cx={x} cy={y} r={r + 0.5} fill="#fff" />
      <circle cx={x} cy={y} r={r} fill={RED} />
      <text x={x} y={y + size * 0.36} textAnchor="middle" fontSize={size} fontWeight={700} fill="#fff">
        {n}
      </text>
    </>
  )
}

function safeFile(name: string): string {
  return name.replace(/[\\/:*?"<>|]+/g, ' ').replace(/\s+/g, ' ').trim()
}

export function TravelMapPanel({ prefs, onChange, onClose }: Props): JSX.Element {
  const t = useT()
  const lang = prefs.language.slice(0, 2)
  const [root, setRoot] = useState(prefs.destination)
  const [years, setYears] = useState<number[] | null>(null)
  const [rootOk, setRootOk] = useState<boolean | null>(null)
  const [year, setYear] = useState(0)
  const [trips, setTrips] = useState<TravelTrip[] | null>(null)
  const [selected, setSelected] = useState<number | 'custom'>(0)
  const [from, setFrom] = useState(today())
  const [to, setTo] = useState(today())
  const [custom, setCustom] = useState<TravelTrip | null>(null)
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [layout, setLayout] = useState<TravelLayout | null>(null)
  const [layoutError, setLayoutError] = useState<string | null>(null)
  const [title, setTitle] = useState('')
  const [job, setJob] = useState<ScrapJob | null>(null)
  const [downloads, setDownloads] = useState('')
  const run = useRef<{ cancel: () => void } | null>(null)

  const rootDir = root.trim()
  const running = job?.state === 'running'
  const outDir = prefs.travelMapOutputDir.trim() || downloads
  const trip = selected === 'custom' ? custom : (trips?.[selected] ?? null)
  const steps = trip?.steps

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
    setYear(0)
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
      } else if (event.type === 'error' && !event.cancelled) setYears([])
    })
    return () => {
      live = false
      handle.cancel()
    }
  }, [scanRoot])

  // Reading the GPS positions is slow: trips are detected once per root and year.
  useEffect(() => {
    setTrips(null)
    setError(null)
    setSelected(0)
    setJob(null)
    if (!scanRoot || years === null || years.length === 0) return
    setProgress({ done: 0, total: 0 })
    let live = true
    const handle = request({ op: 'travelTrips', root: scanRoot, year }, (event) => {
      if (!live) return
      if (event.type === 'status') setProgress({ done: event.done, total: event.total })
      else if (event.type === 'trips') {
        setTrips(event.trips)
        setProgress(null)
        setSelected(event.trips.length > 0 ? 0 : 'custom')
      } else if (event.type === 'error' && !event.cancelled) {
        setProgress(null)
        setError(event.message)
      }
    })
    return () => {
      live = false
      handle.cancel()
    }
  }, [scanRoot, years, year])

  // A custom period is read on its own.
  useEffect(() => {
    setCustom(null)
    if (selected !== 'custom' || !scanRoot || !from || !to || to < from) return
    setError(null)
    setProgress({ done: 0, total: 0 })
    let live = true
    const timer = setTimeout(() => {
      const handle = request({ op: 'travelScan', root: scanRoot, from, to }, (event) => {
        if (!live) return
        if (event.type === 'status') setProgress({ done: event.done, total: event.total })
        else if (event.type === 'trip') {
          setCustom(event.trip)
          setProgress(null)
        } else if (event.type === 'error' && !event.cancelled) {
          setProgress(null)
          setError(event.message)
        }
      })
      cancel = handle.cancel
    }, 400)
    let cancel = (): void => {}
    return () => {
      live = false
      clearTimeout(timer)
      cancel()
    }
  }, [selected, scanRoot, from, to])

  const place = useMemo(() => steps?.reduce((best, step) => (step.count > best.count ? step : best), steps[0])?.name ?? '', [steps])
  useEffect(() => setTitle(place ? t('travel.defaultTitle', { place }) : ''), [place, t])

  useEffect(() => {
    setLayoutError(null)
    setLayout(null)
    if (!steps || steps.length === 0) return
    let live = true
    const handle = request({ op: 'travelLayout', steps, page: prefs.travelMapPage, provider: prefs.travelMapProvider, lang }, (event) => {
      if (!live) return
      if (event.type === 'layout') setLayout(event)
      else if (event.type === 'error' && !event.cancelled) setLayoutError(event.message)
    })
    return () => {
      live = false
      handle.cancel()
    }
  }, [steps, prefs.travelMapPage, prefs.travelMapProvider, lang])

  const dates = (a: string, b: string): string => {
    const format = new Intl.DateTimeFormat(prefs.language, { day: 'numeric', month: 'long', year: 'numeric' })
    return a === b ? format.format(new Date(a)) : `${format.format(new Date(a))} – ${format.format(new Date(b))}`
  }

  const create = (): void => {
    if (!trip || !steps || steps.length === 0 || !outDir || !layout) return
    setJob({ state: 'running', done: 0, total: steps.filter((step) => step.key).length })
    run.current = request(
      {
        op: 'travelPdf',
        steps,
        page: prefs.travelMapPage,
        provider: prefs.travelMapProvider,
        lang,
        title: title.trim() || t('travel.defaultTitle', { place }),
        subtitle: t('travel.subtitle', { dates: dates(trip.from, trip.to), steps: steps.length, km: Math.round(layout.distanceKm) }),
        outDir,
        fallbackDir: downloads,
        name: `${safeFile(t('travel.file', { place, year: trip.from.slice(0, 4) }))}.pdf`
      },
      (event) => {
        if (event.type === 'progress') setJob((previous) => previous && { ...previous, done: event.done, total: event.total })
        else if (event.type === 'done') {
          setJob({ state: 'done', done: event.steps, total: event.steps, path: event.path, pages: 1, photos: event.steps, fallback: event.fallback, failed: event.failed ?? [] })
          confetti({ particleCount: 90, spread: 70, origin: { y: 0.85 } })
          void window.photosort.openPath(event.path)
        } else if (event.type === 'error') setJob(event.cancelled ? null : { state: 'error', done: 0, total: 0, message: event.message })
      }
    )
  }

  const ratio = job && job.total > 0 ? job.done / job.total : 0
  const busy = progress !== null

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
          <h2>🚐 {t('travel')}</h2>
          <p className="hint">{t('travel.hint')}</p>
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
            {years && years.length > 0 && (
              <div className="field">
                <label>{t('travel.year')}</label>
                <Select
                  value={String(year)}
                  options={[{ value: '0', label: t('travel.allYears'), icon: null }, ...years.map((y) => ({ value: String(y), label: String(y), icon: null }))]}
                  onChange={(value) => setYear(Number(value))}
                />
              </div>
            )}
            {years && years.length === 0 && rootDir && <p className="hint">{t('ymap.noYears')}</p>}
            {trips && (
              <div className="field">
                <label>{t('travel.trips')}</label>
                <div className="travel-trips">
                  {trips.length === 0 && <p className="hint">{t('travel.noTrips')}</p>}
                  {trips.map((item, index) => (
                    <button key={item.from} className={`btn ghost travel-trip${selected === index ? ' on' : ''}`} onClick={() => setSelected(index)}>
                      <strong>{item.steps.find((step) => step.count === Math.max(...item.steps.map((s) => s.count)))?.name}</strong>
                      <span>{dates(item.from, item.to)}</span>
                      <span className="muted">{t('travel.tripLine', { steps: item.steps.length, photos: item.photos, km: Math.round(item.distanceKm) })}</span>
                    </button>
                  ))}
                  <button className={`btn ghost travel-trip${selected === 'custom' ? ' on' : ''}`} onClick={() => setSelected('custom')}>
                    <strong>{t('travel.custom')}</strong>
                  </button>
                </div>
              </div>
            )}
            {selected === 'custom' && trips && (
              <div className="field">
                <div className="row">
                  <label>{t('travel.from')}</label>
                  <input type="date" value={from} onChange={(event) => setFrom(event.target.value)} />
                </div>
                <div className="row">
                  <label>{t('travel.to')}</label>
                  <input type="date" value={to} min={from} onChange={(event) => setTo(event.target.value)} />
                </div>
              </div>
            )}
            <div className="field">
              <label>{t('travel.titleLabel')}</label>
              <input value={title} disabled={running} onChange={(event) => setTitle(event.target.value)} />
            </div>
            <div className="field">
              <label>{t('travel.paper')}</label>
              <Select value={prefs.travelMapPage} options={PAGES.map((page) => ({ value: page, label: t(`scrap.page.${page}`), icon: null }))} onChange={(travelMapPage) => onChange({ travelMapPage })} />
            </div>
            <div className="field">
              <label>{t('ymap.provider')}</label>
              <Select
                value={prefs.travelMapProvider}
                options={PROVIDERS.map((p) => ({ value: p.id, label: p.label ?? t('ymap.provider.offline'), icon: null }))}
                onChange={(travelMapProvider) => onChange({ travelMapProvider })}
              />
            </div>
            <div className="field">
              <label>{t('ymap.outDir')}</label>
              <div className="row">
                <input value={prefs.travelMapOutputDir} disabled={running} placeholder={downloads || t('ymap.outDirHint')} onChange={(event) => onChange({ travelMapOutputDir: event.target.value })} />
                <button
                  className="btn ghost"
                  disabled={running}
                  onClick={async () => {
                    const [path] = await window.photosort.pickFolders(t('ymap.outDir'))
                    if (path) onChange({ travelMapOutputDir: path })
                  }}
                >
                  {t('choose')}
                </button>
              </div>
              <p className="hint">{t('ymap.outDirHint')}</p>
            </div>
          </aside>

          <section className="scrap-main">
            <div className="ymap-preview">
              {error || layoutError ? (
                <p className="muted">⚠️ {error ?? layoutError}</p>
              ) : busy ? (
                <p className="muted">{t(trips === null ? 'travel.detecting' : 'travel.reading', { done: progress?.done ?? 0, total: progress?.total ?? 0 })}</p>
              ) : trip && trip.steps.length === 0 ? (
                <p className="muted">{t('travel.noSteps')}</p>
              ) : !layout ? (
                <p className="muted">{steps ? t('travel.layouting') : t('travel.pick')}</p>
              ) : (
                <motion.svg className="ymap-sheet" viewBox={`0 0 ${layout.pageWidth} ${layout.pageHeight}`} style={{ aspectRatio: `${layout.pageWidth} / ${layout.pageHeight}` }} initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }}>
                  <rect width={layout.pageWidth} height={layout.pageHeight} fill="#faf5e8" />
                  {layout.backdrop && <image href={layout.backdrop} width={layout.pageWidth} height={layout.pageHeight} preserveAspectRatio="none" />}
                  {layout.land && <path d={layout.land} fill="#ecdfc2" fillRule="evenodd" stroke="#a08a68" strokeWidth={0.2} />}
                  {layout.lakes && <path d={layout.lakes} fill="#faf5e8" fillRule="evenodd" stroke="#a08a68" strokeWidth={0.2} />}
                  <rect x={3} y={3} width={layout.pageWidth - 6} height={layout.pageHeight - 6} fill="none" stroke={INK} strokeWidth={0.5} />
                  <rect x={4.2} y={4.2} width={layout.pageWidth - 8.4} height={layout.pageHeight - 8.4} fill="none" stroke={INK} strokeWidth={0.15} />
                  {layout.stops.length > 1 && <path d={layout.route} fill="none" stroke={RED} strokeWidth={0.6} strokeLinecap="round" strokeDasharray="0.01 1.7" />}
                  <rect x={6} y={6} width={90} height={18} fill="#fffcf4" fillOpacity={0.85} stroke={INK} strokeWidth={0.4} />
                  <text x={10} y={15.2} fontSize={14} fontWeight={700} fontFamily="Georgia, serif" fill={INK}>
                    {title}
                  </text>
                  <text x={10} y={20.6} fontSize={6} fontStyle="italic" fontFamily="Georgia, serif" fill="#806f5f">
                    {trip && t('travel.subtitle', { dates: dates(trip.from, trip.to), steps: layout.stops.length, km: Math.round(layout.distanceKm) })}
                  </text>
                  {layout.stops.map((stop) => (
                    <Stop key={stop.n} stop={stop} />
                  ))}
                </motion.svg>
              )}
            </div>
            <p className="hint">{layout?.backdropError ? `⚠️ ${t('ymap.backdropError', { error: layout.backdropError })}` : (layout?.credit ?? t('ymap.credit'))}</p>
          </section>
        </div>

        <footer className="scrap-foot">
          <div className="scrap-save">
            <p className="hint" title={outDir}>
              {t('travel.saveIn')} : <code>{outDir}</code>
            </p>
            {running && (
              <div className="progress">
                <i style={{ width: `${ratio * 100}%`, transition: 'width 0.3s ease' }} />
              </div>
            )}
            {running && <p className="muted">{t('travel.progress', { done: job.done, total: job.total })}</p>}
            {job?.state === 'done' && (
              <p className="muted">
                {t('travel.done', { steps: job.photos ?? 0 })}
                {job.fallback && job.path && <span className="warn"> · {t('scrap.fallback', { dir: dirName(job.path) })}</span>}
              </p>
            )}
            {job?.state === 'done' && job.failed && job.failed.length > 0 && <p className="hint warn">{t('travel.unreadable', { n: job.failed.length })}</p>}
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
                {t('travel.create')}
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
