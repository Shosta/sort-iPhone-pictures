import type { Key, Translate } from './i18n'

interface RemoteErrorEvent {
  code?: string
  detail?: string
  resumable?: boolean
  message?: string
}

/** A remote error in plain language, with the technical detail appended when there is one. */
export function remoteErrorText(t: Translate, event: RemoteErrorEvent): string {
  if (!event.code) return event.message ?? ''
  const key = `remote.err.${event.code}` as Key
  let text = t(key)
  if (event.detail) text += t('remote.detail', { detail: event.detail })
  if (event.resumable) text += t('remote.resumeHint')
  return text
}
