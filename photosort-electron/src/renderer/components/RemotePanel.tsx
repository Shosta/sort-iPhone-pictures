import { motion } from 'framer-motion'
import { useEffect, useState } from 'react'
import { useT } from '../i18n'
import { type Prefs, remoteConfig, remoteReady } from '../prefs'
import { request } from '../sidecar'
import { themeById } from '../themes'
import { remoteErrorText } from '../remote'
import { Switch } from './Preferences'

interface Props {
  prefs: Prefs
  busy: boolean
  password: string
  onPassword: (value: string) => void
  onChange: (patch: Partial<Prefs>) => void
  onClose: () => void
}

type Test = { state: 'idle' } | { state: 'running' } | { state: 'ok'; text: string } | { state: 'error'; text: string }

export function RemotePanel({ prefs, busy, password, onPassword, onChange, onClose }: Props): JSX.Element {
  const t = useT()
  const accent = themeById(prefs.theme).vars.accent
  const [test, setTest] = useState<Test>({ state: 'idle' })
  const [secure, setSecure] = useState(false)
  const daemon = prefs.remoteProtocol === 'daemon'
  const locked = busy || test.state === 'running'

  useEffect(() => {
    void window.photosort.secrets.available().then(setSecure)
  }, [])

  const edit = (patch: Partial<Prefs>): void => {
    setTest({ state: 'idle' })
    onChange(patch)
  }

  const runTest = (): void => {
    setTest({ state: 'running' })
    request({ op: 'remoteTest', remote: remoteConfig(prefs, password) }, (event) => {
      if (event.type === 'remoteTest') {
        setTest({
          state: 'ok',
          text: event.pathMissing ? t('remote.test.pathMissing') : t('remote.test.ok', { n: event.entries ?? 0 })
        })
      } else if (event.type === 'error') {
        setTest({ state: 'error', text: remoteErrorText(t, event) })
      }
    })
  }

  return (
    <motion.div className="overlay" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onClick={onClose}>
      <motion.div
        className="glass modal prefs remote-panel"
        initial={{ scale: 0.9, y: 30 }}
        animate={{ scale: 1, y: 0 }}
        exit={{ scale: 0.9, y: 30 }}
        onClick={(event) => event.stopPropagation()}
      >
        <h2>☁️ {t('remote.title')}</h2>
        <p className="hint">{t('remote.intro')}</p>

        <label className={`toggle ${busy ? 'disabled' : ''}`}>
          <input type="checkbox" checked={prefs.remoteEnabled} disabled={busy} onChange={(event) => edit({ remoteEnabled: event.target.checked })} />
          <Switch on={prefs.remoteEnabled} accent={accent} />
          <span>{t('remote.enable')}</span>
        </label>

        <fieldset className="remote-fields" disabled={locked || !prefs.remoteEnabled}>
          <div className="field">
            <label>{t('remote.protocol')}</label>
            <div className="seg">
              {(['daemon', 'ssh'] as const).map((value) => (
                <button
                  key={value}
                  type="button"
                  className={`btn ghost${prefs.remoteProtocol === value ? ' on' : ''}`}
                  onClick={() => edit({ remoteProtocol: value })}
                >
                  {t(value === 'daemon' ? 'remote.protocol.daemon' : 'remote.protocol.ssh')}
                </button>
              ))}
            </div>
          </div>

          <div className="field">
            <label>{t('remote.host')}</label>
            <div className="row">
              <input value={prefs.remoteHost} placeholder={t('remote.host.placeholder')} onChange={(event) => edit({ remoteHost: event.target.value })} />
              <input
                className="narrow"
                inputMode="numeric"
                value={prefs.remotePort}
                aria-label={t('remote.port')}
                placeholder={daemon ? '873' : '22'}
                onChange={(event) => edit({ remotePort: event.target.value.replace(/\D/g, '') })}
              />
            </div>
          </div>

          <div className="field">
            <label>{t('remote.user')}</label>
            <input value={prefs.remoteUser} autoCapitalize="off" spellCheck={false} onChange={(event) => edit({ remoteUser: event.target.value })} />
          </div>

          {daemon && (
            <div className="field">
              <label>{t('remote.module')}</label>
              <input value={prefs.remoteModule} placeholder={t('remote.module.placeholder')} onChange={(event) => edit({ remoteModule: event.target.value })} />
            </div>
          )}

          <div className="field">
            <label>{t(daemon ? 'remote.path.daemon' : 'remote.path.ssh')}</label>
            <input
              value={prefs.remotePath}
              placeholder={daemon ? '' : t('remote.path.ssh.placeholder')}
              onChange={(event) => edit({ remotePath: event.target.value })}
            />
          </div>

          {daemon ? (
            <div className="field">
              <label>{t('remote.password')}</label>
              <input
                type="password"
                autoComplete="off"
                value={password}
                onChange={(event) => {
                  setTest({ state: 'idle' })
                  onPassword(event.target.value)
                }}
              />
              <label className={`toggle ${secure ? '' : 'disabled'}`}>
                <input type="checkbox" checked={prefs.remoteRemember && secure} disabled={!secure} onChange={(event) => onChange({ remoteRemember: event.target.checked })} />
                <Switch on={prefs.remoteRemember && secure} accent={accent} />
                <span>{t('remote.remember')}</span>
              </label>
              <p className="hint">{t(secure ? 'remote.remember.off' : 'remote.remember.unavailable')}</p>
            </div>
          ) : (
            <div className="field">
              <label>{t('remote.keyFile')}</label>
              <div className="row">
                <input value={prefs.remoteKeyFile} onChange={(event) => edit({ remoteKeyFile: event.target.value })} />
                <button
                  className="btn ghost"
                  type="button"
                  onClick={async () => {
                    const path = await window.photosort.pickFile(t('remote.keyFile.pick'))
                    if (path) edit({ remoteKeyFile: path })
                  }}
                >
                  {t('choose')}
                </button>
              </div>
              <p className="hint">{t('remote.password.ssh')}</p>
            </div>
          )}

          <div className="field">
            <label className="toggle">
              <input type="checkbox" checked={prefs.remoteChecksum} onChange={(event) => onChange({ remoteChecksum: event.target.checked })} />
              <Switch on={prefs.remoteChecksum} accent={accent} />
              <span>{t('remote.checksum')}</span>
            </label>
            <p className="hint">{t('remote.checksum.hint')}</p>
          </div>

          <div className="row">
            <button className="btn" type="button" disabled={!remoteReady(prefs)} onClick={runTest}>
              {test.state === 'running' ? t('remote.testing') : t('remote.test')}
            </button>
          </div>
          {(test.state === 'ok' || test.state === 'error') && (
            <p className={`hint remote-test ${test.state}`}>{test.state === 'error' ? `⚠️ ${test.text}` : test.text}</p>
          )}
        </fieldset>

        <button className="btn" onClick={onClose}>
          {t('remote.close')}
        </button>
      </motion.div>
    </motion.div>
  )
}
