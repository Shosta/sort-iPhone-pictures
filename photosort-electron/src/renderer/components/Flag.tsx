import type { Language } from '../i18n'

// Inline SVG: emoji flags are not rendered on Windows.
export function Flag({ code }: { code: Language }): JSX.Element {
  return (
    <svg className="flag" viewBox="0 0 30 20" width="24" height="16" aria-hidden>
      {code === 'fr' && (
        <>
          <rect width="10" height="20" fill="#0055a4" />
          <rect x="10" width="10" height="20" fill="#fff" />
          <rect x="20" width="10" height="20" fill="#ef4135" />
        </>
      )}
      {code === 'da' && (
        <>
          <rect width="30" height="20" fill="#c8102e" />
          <rect x="9" width="3.5" height="20" fill="#fff" />
          <rect y="8.25" width="30" height="3.5" fill="#fff" />
        </>
      )}
      {code === 'en-GB' && (
        <>
          <rect width="30" height="20" fill="#012169" />
          <path d="M0 0 30 20M30 0 0 20" stroke="#fff" strokeWidth="4" />
          <path d="M0 0 30 20M30 0 0 20" stroke="#c8102e" strokeWidth="1.5" />
          <path d="M15 0v20M0 10h30" stroke="#fff" strokeWidth="6" />
          <path d="M15 0v20M0 10h30" stroke="#c8102e" strokeWidth="3.5" />
        </>
      )}
    </svg>
  )
}
