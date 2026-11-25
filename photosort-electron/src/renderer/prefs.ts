import { useEffect, useState } from 'react'
import { Language, detectLanguage } from './i18n'
import type { ThemeId } from './themes'

export interface Prefs {
  destination: string
  byLocation: boolean
  landmarks: boolean
  skipDuplicates: boolean
  groupSimilar: boolean
  dateFallback: boolean
  screenshotFolder: boolean
  groupBlurry: boolean
  tagScenes: boolean
  tagMoment: boolean
  tagPeople: boolean
  tagMeta: boolean
  // Weekly planner sheet: PDF folder ('' = Downloads), photo size and space in mm.
  scrapOutputDir: string
  scrapPage: ScrapPage
  scrapLongSide: number
  scrapGutter: number
  scrapBleed: boolean
  scrapFrame: ScrapFrame
  scrapLayout: ScrapLayoutStyle
  scrapBackground: ScrapBackground
  yearMapPage: ScrapPage
  travelMapPage: ScrapPage
  travelMapProvider: string
  travelMapOutputDir: string
  yearMapProvider: string
  yearMapOutputDir: string
  // Remote destination (NAS, rsync). The password is never kept here.
  remoteEnabled: boolean
  remoteProtocol: RemoteProtocol
  remoteHost: string
  remotePort: string
  remoteUser: string
  remoteModule: string
  remotePath: string
  remoteKeyFile: string
  remoteChecksum: boolean
  remoteDedupe: boolean
  // Destinations already sent to; the first send to a new one proposes a simulation.
  remoteUsed: string[]
  // true: password kept in the system keychain; false: asked again each session.
  remoteRemember: boolean
  language: Language
  theme: ThemeId
}

export type RemoteProtocol = 'daemon' | 'ssh'

export type ScrapFrame = 'none' | 'polaroid' | 'stamp' | 'tape'
export type ScrapLayoutStyle = 'grid' | 'collage'
export type ScrapBackground = 'none' | 'cream' | 'rose' | 'mint' | 'sky' | 'lavender' | 'kraft'
export type ScrapPage = 'a4-landscape' | 'a4-portrait' | 'letter-landscape' | 'letter-portrait'


const STORAGE_KEY = 'prefs'

function load(): Prefs {
  const defaults: Prefs = {
    // Keys written by earlier versions are still honoured.
    destination: localStorage.getItem('destination') ?? '',
    byLocation: localStorage.getItem('byLocation') !== '0',
    landmarks: localStorage.getItem('landmarks') !== '0',
    skipDuplicates: true,
    groupSimilar: true,
    dateFallback: true,
    screenshotFolder: true,
    groupBlurry: false,
    // Tagging is an explicit action from its own panel, so every kind starts on.
    tagScenes: true,
    tagMoment: true,
    tagPeople: true,
    tagMeta: true,
    scrapOutputDir: '',
    scrapPage: 'a4-landscape',
    scrapLongSide: 65,
    scrapGutter: 5,
    scrapBleed: false,
    scrapFrame: 'none',
    scrapLayout: 'grid',
    scrapBackground: 'none',
    yearMapPage: 'a4-landscape',
    travelMapPage: 'a4-portrait',
    travelMapProvider: 'osm',
    travelMapOutputDir: '',
    yearMapProvider: 'osm',
    yearMapOutputDir: '',
    remoteEnabled: false,
    remoteProtocol: 'daemon',
    remoteHost: '',
    remotePort: '',
    remoteUser: '',
    remoteModule: '',
    remotePath: '',
    remoteKeyFile: '',
    remoteChecksum: false,
    remoteDedupe: true,
    remoteUsed: [],
    remoteRemember: false,
    language: detectLanguage(),
    theme: 'aurora'
  }
  try {
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as Partial<Prefs>
    const merged = { ...defaults, ...stored }
    // Backgrounds that were dropped fall back to the default.
    const known = ['osm', 'esritopo', 'offline']
    if (!known.includes(merged.yearMapProvider)) merged.yearMapProvider = defaults.yearMapProvider
    if (!known.includes(merged.travelMapProvider)) merged.travelMapProvider = defaults.travelMapProvider
    return merged
  } catch {
    return defaults
  }
}

export function usePrefs(): [Prefs, (patch: Partial<Prefs>) => void] {
  const [prefs, setPrefs] = useState<Prefs>(load)
  useEffect(() => localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs)), [prefs])
  return [prefs, (patch) => setPrefs((previous) => ({ ...previous, ...patch }))]
}

/** The remote destination as the engine expects it (password included, in memory only). */
export function remoteConfig(prefs: Prefs, password: string): Record<string, unknown> {
  const daemon = prefs.remoteProtocol === 'daemon'
  return {
    protocol: prefs.remoteProtocol,
    host: prefs.remoteHost.trim(),
    port: Number.parseInt(prefs.remotePort, 10) || 0,
    user: prefs.remoteUser.trim(),
    module: daemon ? prefs.remoteModule.trim() : '',
    path: prefs.remotePath.trim(),
    password: daemon ? password : '',
    keyFile: daemon ? '' : prefs.remoteKeyFile.trim(),
    checksum: prefs.remoteChecksum
  }
}

export function remoteReady(prefs: Prefs): boolean {
  if (!prefs.remoteHost.trim()) return false
  return prefs.remoteProtocol === 'daemon' ? !!prefs.remoteModule.trim() : !!prefs.remotePath.trim()
}

/** Short, readable destination folder: "module/folder" (daemon) or the remote path (SSH). Empty when none chosen. */
export function remoteFolder(prefs: Prefs): string {
  const path = prefs.remotePath.trim().replace(/^\/+|\/+$/g, '')
  if (prefs.remoteProtocol === 'daemon') return [prefs.remoteModule.trim(), path].filter(Boolean).join('/')
  return path
}

export function remoteLabel(prefs: Prefs): string {
  const user = prefs.remoteUser.trim() ? `${prefs.remoteUser.trim()}@` : ''
  const port = prefs.remotePort.trim() ? `:${prefs.remotePort.trim()}` : ''
  const path = prefs.remotePath.trim().replace(/^\/+/, '')
  return prefs.remoteProtocol === 'daemon'
    ? `rsync://${user}${prefs.remoteHost.trim()}${port}/${prefs.remoteModule.trim()}${path ? `/${path}` : ''}`
    : `ssh://${user}${prefs.remoteHost.trim()}${port}/${path}`
}
