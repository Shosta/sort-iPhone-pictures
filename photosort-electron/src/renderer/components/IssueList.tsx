import { useEffect, useMemo, useRef, useState } from 'react'
import { useT } from '../i18n'
import type { Duplicate, Issue } from '../types'
import { Thumb } from './Thumb'

const ROW = 72
const THUMB = 56
const OVERSCAN = 4

type Row =
  | { kind: 'header'; key: string; title: string }
  | { kind: 'failure'; key: string; issue: Issue }
  | { kind: 'duplicate'; key: string; duplicate: Duplicate }

const baseName = (path: string): string => path.split(/[\\/]/).pop() ?? path
const extension = (path: string): string => {
  const name = baseName(path)
  const dot = name.lastIndexOf('.')
  return dot > 0 ? name.slice(dot + 1).toUpperCase() : '?'
}

interface Props {
  failures: Issue[]
  duplicates: Duplicate[]
}

/** Failures and duplicates as fixed-height rows; only the visible ones mount, so only they decode a thumbnail. */
export function IssueList({ failures, duplicates }: Props): JSX.Element {
  const t = useT()
  const ref = useRef<HTMLUListElement>(null)
  const [view, setView] = useState({ top: 0, height: 400 })

  const rows = useMemo<Row[]>(() => {
    const list: Row[] = []
    if (failures.length > 0) {
      list.push({ kind: 'header', key: 'h-failed', title: `${t('issues.failed')} (${failures.length})` })
      for (const issue of failures) list.push({ kind: 'failure', key: `f:${issue.path}`, issue })
    }
    if (duplicates.length > 0) {
      list.push({ kind: 'header', key: 'h-dup', title: `${t('issues.duplicates')} (${duplicates.length})` })
      for (const duplicate of duplicates) list.push({ kind: 'duplicate', key: `d:${duplicate.path}`, duplicate })
    }
    return list
  }, [failures, duplicates, t])

  useEffect(() => {
    const element = ref.current
    if (!element) return
    let frame = 0
    const measure = (): void => {
      frame = 0
      setView((previous) =>
        previous.top === element.scrollTop && previous.height === element.clientHeight ? previous : { top: element.scrollTop, height: element.clientHeight }
      )
    }
    const onScroll = (): void => {
      if (!frame) frame = requestAnimationFrame(measure)
    }
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    element.addEventListener('scroll', onScroll, { passive: true })
    measure()
    return () => {
      observer.disconnect()
      element.removeEventListener('scroll', onScroll)
      cancelAnimationFrame(frame)
    }
  }, [])

  const first = Math.max(0, Math.floor(view.top / ROW) - OVERSCAN)
  const last = Math.min(rows.length, Math.ceil((view.top + view.height) / ROW) + OVERSCAN)

  return (
    <ul ref={ref} className="issue-list">
      <li className="issue-space" style={{ height: rows.length * ROW }}>
        {rows.slice(first, last).map((row, offset) => (
          <div key={row.key} className={`issue-row ${row.kind}`} style={{ height: ROW, transform: `translateY(${(first + offset) * ROW}px)` }}>
            {row.kind === 'header' && <strong>{row.title}</strong>}
            {row.kind === 'failure' && (
              <>
                <Thumb path={row.issue.path} size={THUMB} fallback={extension(row.issue.path)} />
                <div className="issue-text">
                  <span className="issue-name" title={row.issue.path}>{baseName(row.issue.path)}</span>
                  <span className="issue-cause">{row.issue.reason}</span>
                </div>
              </>
            )}
            {row.kind === 'duplicate' && (
              <>
                <Thumb path={row.duplicate.path} size={THUMB} fallback={extension(row.duplicate.path)} />
                <span className="issue-eq">=</span>
                <Thumb path={row.duplicate.of} size={THUMB} fallback={extension(row.duplicate.of)} />
                <div className="issue-text">
                  <span className="issue-name" title={row.duplicate.path}>{baseName(row.duplicate.path)}</span>
                  <span className="issue-cause" title={row.duplicate.of}>
                    {row.duplicate.inDestination ? t('duplicateInDest') : t('duplicateOf')} {baseName(row.duplicate.of)}
                  </span>
                </div>
              </>
            )}
          </div>
        ))}
      </li>
    </ul>
  )
}
