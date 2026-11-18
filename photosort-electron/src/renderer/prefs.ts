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
  language: Language
  theme: ThemeId
}

export type ScrapFrame = 'none' | 'polaroid' | 'stamp' | 'tape'
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
    language: detectLanguage(),
    theme: 'aurora'
  }
  try {
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as Partial<Prefs>
    return { ...defaults, ...stored }
  } catch {
    return defaults
  }
}

export function usePrefs(): [Prefs, (patch: Partial<Prefs>) => void] {
  const [prefs, setPrefs] = useState<Prefs>(load)
  useEffect(() => localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs)), [prefs])
  return [prefs, (patch) => setPrefs((previous) => ({ ...previous, ...patch }))]
}
