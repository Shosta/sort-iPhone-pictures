import { useEffect, useState } from 'react'
import { Language, detectLanguage } from './i18n'
import type { ThemeId } from './themes'

export interface Prefs {
  destination: string
  byLocation: boolean
  landmarks: boolean
  skipDuplicates: boolean
  groupSimilar: boolean
  tagScenes: boolean
  tagMoment: boolean
  tagPeople: boolean
  language: Language
  theme: ThemeId
}

const STORAGE_KEY = 'prefs'

function load(): Prefs {
  const defaults: Prefs = {
    // Keys written by earlier versions are still honoured.
    destination: localStorage.getItem('destination') ?? '',
    byLocation: localStorage.getItem('byLocation') !== '0',
    landmarks: localStorage.getItem('landmarks') !== '0',
    skipDuplicates: true,
    groupSimilar: true,
    tagScenes: false,
    tagMoment: false,
    tagPeople: false,
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
