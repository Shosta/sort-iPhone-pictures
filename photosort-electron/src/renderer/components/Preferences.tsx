import { motion } from 'framer-motion'
import { LANGUAGES, useT } from '../i18n'
import type { Prefs } from '../prefs'
import { THEMES, themeById } from '../themes'
import { Flag } from './Flag'
import { Select } from './Select'

interface Props {
  prefs: Prefs
  busy: boolean
  onChange: (patch: Partial<Prefs>) => void
  onClose: () => void
}

function Switch({ on, accent }: { on: boolean; accent: string }): JSX.Element {
  return (
    <motion.span className="switch" animate={{ backgroundColor: on ? accent : 'rgba(255,255,255,0.18)' }}>
      <motion.span className="knob" animate={{ x: on ? 18 : 0 }} transition={{ type: 'spring', stiffness: 500, damping: 30 }} />
    </motion.span>
  )
}

export function Preferences({ prefs, busy, onChange, onClose }: Props): JSX.Element {
  const t = useT()
  const accent = themeById(prefs.theme).vars.accent

  return (
    <motion.div className="overlay" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onClick={onClose}>
      <motion.div
        className="glass modal prefs"
        initial={{ scale: 0.9, y: 30 }}
        animate={{ scale: 1, y: 0 }}
        exit={{ scale: 0.9, y: 30 }}
        onClick={(event) => event.stopPropagation()}
      >
        <h2>{t('prefs')}</h2>

        <div className="field">
          <label>{t('prefs.language')}</label>
          <Select
            value={prefs.language}
            options={LANGUAGES.map((language) => ({ value: language.code, label: language.label, icon: <Flag code={language.code} /> }))}
            onChange={(language) => onChange({ language })}
          />
        </div>

        <div className="field">
          <label>{t('prefs.theme')}</label>
          <Select
            value={prefs.theme}
            options={THEMES.map((theme) => ({
              value: theme.id,
              label: t(`theme.${theme.id}`),
              icon: (
                <span className="swatches">
                  {theme.swatches.map((color) => (
                    <i key={color} style={{ background: color }} />
                  ))}
                </span>
              )
            }))}
            onChange={(theme) => onChange({ theme })}
          />
        </div>

        <div className="field">
          <label>{t('prefs.output')}</label>
          <div className="row">
            <input
              value={prefs.destination}
              disabled={busy}
              placeholder={t('destination.placeholder')}
              onChange={(event) => onChange({ destination: event.target.value })}
            />
            <button
              className="btn ghost"
              disabled={busy}
              onClick={async () => {
                const [path] = await window.photosort.pickFolders(t('destination.pick'))
                if (path) onChange({ destination: path })
              }}
            >
              {t('choose')}
            </button>
          </div>
        </div>

        <div className="field">
          <label>{t('prefs.location')}</label>
          <label className={`toggle ${busy ? 'disabled' : ''}`}>
            <input type="checkbox" checked={prefs.byLocation} disabled={busy} onChange={(event) => onChange({ byLocation: event.target.checked })} />
            <Switch on={prefs.byLocation} accent={accent} />
            <span>{t('byLocation')}</span>
          </label>
          <label className={`toggle ${busy || !prefs.byLocation ? 'disabled' : ''}`}>
            <input type="checkbox" checked={prefs.landmarks} disabled={busy || !prefs.byLocation} onChange={(event) => onChange({ landmarks: event.target.checked })} />
            <Switch on={prefs.landmarks && prefs.byLocation} accent={accent} />
            <span>{t('landmarks')}</span>
          </label>
        </div>

        <div className="field">
          <label>{t('prefs.duplicates')}</label>
          <label className={`toggle ${busy ? 'disabled' : ''}`}>
            <input type="checkbox" checked={prefs.skipDuplicates} disabled={busy} onChange={(event) => onChange({ skipDuplicates: event.target.checked })} />
            <Switch on={prefs.skipDuplicates} accent={accent} />
            <span>{t('skipDuplicates')}</span>
          </label>
          <label className={`toggle ${busy ? 'disabled' : ''}`}>
            <input type="checkbox" checked={prefs.groupSimilar} disabled={busy} onChange={(event) => onChange({ groupSimilar: event.target.checked })} />
            <Switch on={prefs.groupSimilar} accent={accent} />
            <span>{t('groupSimilar')}</span>
          </label>
        </div>

        <p className="hint">{t('prefs.saved')}</p>
        <button className="btn" onClick={onClose}>
          {t('close')}
        </button>
      </motion.div>
    </motion.div>
  )
}
