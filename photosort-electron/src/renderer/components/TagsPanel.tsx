import { motion } from 'framer-motion'
import { useState } from 'react'
import { useT } from '../i18n'
import type { Prefs } from '../prefs'
import { themeById } from '../themes'
import type { TagJob } from '../types'
import { Switch } from './Preferences'
import { Thumb } from './Thumb'

interface Props {
  job: TagJob | null
  prefs: Prefs
  /** Photos of the last sort, when the panel was opened from its link. */
  scope: string[] | null
  busy: boolean
  onChange: (patch: Partial<Prefs>) => void
  onClearScope: () => void
  onStart: (folder: string) => void
  onCancel: () => void
  onClose: () => void
}

function duration(seconds: number): string {
  if (seconds < 60) return `${Math.max(1, Math.round(seconds))} s`
  if (seconds < 3600) return `${Math.round(seconds / 60)} min`
  return `${Math.floor(seconds / 3600)} h ${Math.round((seconds % 3600) / 60)} min`
}

export function TagsPanel({ job, prefs, scope, busy, onChange, onClearScope, onStart, onCancel, onClose }: Props): JSX.Element {
  const t = useT()
  const accent = themeById(prefs.theme).vars.accent
  const [folder, setFolder] = useState(() => (job && !job.files ? job.folder : prefs.destination))
  const running = job?.state === 'loading' || job?.state === 'running'
  const kinds = prefs.tagScenes || prefs.tagMoment || prefs.tagPeople || prefs.tagMeta
  const ratio = job && job.total > 0 ? job.done / job.total : 0
  const elapsed = job ? (performance.now() - job.started) / 1000 : 0
  const remaining = job && job.state === 'running' && job.done >= 3 ? (elapsed / job.done) * (job.total - job.done) : null
  const counts = job ? Object.entries(job.counts).sort((a, b) => b[1] - a[1]) : []

  const kind = (key: 'tagScenes' | 'tagMoment' | 'tagPeople' | 'tagMeta'): JSX.Element => (
    <label className={`toggle ${running ? 'disabled' : ''}`}>
      <input type="checkbox" checked={prefs[key]} disabled={running} onChange={(event) => onChange({ [key]: event.target.checked })} />
      <Switch on={prefs[key]} accent={accent} />
      <span>{t(key)}</span>
    </label>
  )

  return (
    <motion.div className="overlay" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onClick={onClose}>
      <motion.div
        className="glass modal prefs tags-panel"
        initial={{ scale: 0.9, y: 30 }}
        animate={{ scale: 1, y: 0 }}
        exit={{ scale: 0.9, y: 30 }}
        onClick={(event) => event.stopPropagation()}
      >
        <h2>🏷️ {t('tags')}</h2>

        <div className="field">
          <label>{t('tags.folder')}</label>
          {scope ? (
            <span className="chip scope">
              {t('tags.scope', { n: scope.length })}
              <button disabled={running} aria-label={t('close')} onClick={onClearScope}>
                ×
              </button>
            </span>
          ) : (
            <div className="row">
              <input value={folder} disabled={running} placeholder={t('destination.placeholder')} onChange={(event) => setFolder(event.target.value)} />
              <button
                className="btn ghost"
                disabled={running}
                onClick={async () => {
                  const [path] = await window.photosort.pickFolders(t('tags.folder'))
                  if (path) setFolder(path)
                }}
              >
                {t('choose')}
              </button>
            </div>
          )}
        </div>

        <div className="field">
          <label>{t('tags.kinds')}</label>
          {kind('tagScenes')}
          {kind('tagMoment')}
          {kind('tagPeople')}
          {kind('tagMeta')}
          <p className="hint">{t('tags.hint')}</p>
        </div>

        {job && (
          <div className="field tag-job">
            <div className="progress">
              <i style={{ width: `${(job.state === 'done' ? 1 : ratio) * 100}%` }} />
            </div>
            <p className="muted">
              {job.state === 'loading'
                ? t('tags.loading')
                : job.state === 'running'
                  ? `${t('tags.progress', { done: job.done, total: job.total })}${remaining !== null ? ` · ${t('tags.eta', { time: duration(remaining) })}` : ''}`
                  : job.state === 'done'
                    ? job.total === 0
                      ? t('tags.nothing')
                      : t('tags.done', { tagged: job.tagged, total: job.total })
                    : job.state === 'cancelled'
                      ? t('tags.cancelled', { done: job.done, total: job.total })
                      : `⚠️ ${job.message ?? ''}`}
            </p>
            {counts.length > 0 && (
              <div className="tag-counts">
                {counts.map(([tag, count]) => (
                  <span key={tag} className="chip">
                    {tag} <em>{count}</em>
                  </span>
                ))}
              </div>
            )}
            {job.recent.length > 0 && (
              <ul className="tag-feed">
                {job.recent.map((result) => (
                  <li key={result.path} className="appear">
                    <Thumb path={result.path} size={44} />
                    <span className="tag-list">
                      {result.error ? (
                        <span className="muted">⚠️ {result.error}</span>
                      ) : result.tags && result.tags.length > 0 ? (
                        result.tags.map((tag) => (
                          <span key={tag} className="chip">
                            {tag}
                          </span>
                        ))
                      ) : (
                        <span className="muted">{t('tags.none')}</span>
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}

        <div className="row end">
          {!kinds && !running && <span className="hint grow-text">{t('tags.pickKinds')}</span>}
          {running ? (
            <button className="btn danger" onClick={onCancel}>
              {t('cancel')}
            </button>
          ) : (
            <button className="btn primary" disabled={busy || !kinds || (!scope && !folder.trim())} onClick={() => onStart(folder.trim())}>
              {t('tags.start')}
            </button>
          )}
          <button className="btn" onClick={onClose}>
            {t('close')}
          </button>
        </div>
      </motion.div>
    </motion.div>
  )
}
