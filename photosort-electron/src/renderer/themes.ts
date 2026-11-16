export type ThemeId = 'aurora' | 'monochrome' | 'analogous' | 'complementary' | 'triadic' | 'stonewash' | 'brass' | 'linen'

export interface Theme {
  id: ThemeId
  // Palette swatches shown in the picker.
  swatches: string[]
  vars: {
    bg: string
    accent: string
    accent2: string
    aurora: [string, string, string]
    surface: string
    surfaceRaised: string
  }
}

// Dark, tinted bases keep text readable; the palette colours drive accents and the animated background.
export const THEMES: Theme[] = [
  {
    id: 'aurora',
    swatches: ['#4f46e5', '#7c8cff', '#ff7ce5', '#0891b2'],
    vars: { bg: '#0b0d14', accent: '#7c8cff', accent2: '#ff7ce5', aurora: ['#4f46e5', '#db2777', '#0891b2'], surface: '#151826', surfaceRaised: '#1c2033' }
  },
  {
    id: 'monochrome',
    swatches: ['#8fae7e', '#a5be94', '#6f9560', '#4a7a3c'],
    vars: { bg: '#0c150e', accent: '#8fb87a', accent2: '#5f9a4c', aurora: ['#4a7a3c', '#6f9560', '#8fae7e'], surface: '#132017', surfaceRaised: '#1a2b1e' }
  },
  {
    id: 'analogous',
    swatches: ['#d9733f', '#e8a46a', '#f1c9a0', '#c8582e'],
    vars: { bg: '#170f0b', accent: '#e8935a', accent2: '#f1c9a0', aurora: ['#c8582e', '#d9733f', '#e8a46a'], surface: '#231611', surfaceRaised: '#2e1d16' }
  },
  {
    id: 'complementary',
    swatches: ['#1f4e6b', '#e0a82e', '#c8552b', '#d9892b'],
    vars: { bg: '#08141c', accent: '#e0a82e', accent2: '#c8552b', aurora: ['#1f4e6b', '#c8552b', '#e0a82e'], surface: '#0f202b', surfaceRaised: '#152a38' }
  },
  {
    id: 'triadic',
    swatches: ['#6d9558', '#8fb4c8', '#e8c84a', '#5b7fa8'],
    vars: { bg: '#0d171d', accent: '#8fb4c8', accent2: '#e8c84a', aurora: ['#5b7fa8', '#6d9558', '#e8c84a'], surface: '#14222a', surfaceRaised: '#1b2d37' }
  },
  {
    id: 'stonewash',
    swatches: ['#f7f8f4', '#cfe7e5', '#9fb7b3', '#5b7c7a', '#2f3a3a'],
    vars: { bg: '#2f3a3a', accent: '#9fb7b3', accent2: '#cfe7e5', aurora: ['#5b7c7a', '#9fb7b3', '#cfe7e5'], surface: '#364343', surfaceRaised: '#3f4f4e' }
  },
  {
    id: 'brass',
    swatches: ['#0f172a', '#1f2937', '#c6a15b', '#e5e7eb', '#7c3aed'],
    vars: { bg: '#0f172a', accent: '#c6a15b', accent2: '#7c3aed', aurora: ['#7c3aed', '#c6a15b', '#1f2937'], surface: '#1f2937', surfaceRaised: '#2a3544' }
  },
  {
    id: 'linen',
    swatches: ['#f4efe6', '#d9c7b8', '#bfa58a', '#8c6b5a', '#2e2a2a'],
    vars: { bg: '#2e2a2a', accent: '#bfa58a', accent2: '#d9c7b8', aurora: ['#8c6b5a', '#bfa58a', '#d9c7b8'], surface: '#383332', surfaceRaised: '#443d3b' }
  }
]

export const themeById = (id: ThemeId): Theme => THEMES.find((theme) => theme.id === id) ?? THEMES[0]

const rgb = (hex: string): string => {
  const value = parseInt(hex.slice(1), 16)
  return `${(value >> 16) & 255}, ${(value >> 8) & 255}, ${value & 255}`
}

export function applyTheme(id: ThemeId): void {
  const { vars } = themeById(id)
  const style = document.documentElement.style
  style.setProperty('--bg', vars.bg)
  style.setProperty('--accent', vars.accent)
  style.setProperty('--accent2', vars.accent2)
  style.setProperty('--accent-rgb', rgb(vars.accent))
  style.setProperty('--aurora-1', vars.aurora[0])
  style.setProperty('--aurora-2', vars.aurora[1])
  style.setProperty('--aurora-3', vars.aurora[2])
  style.setProperty('--surface', vars.surface)
  style.setProperty('--surface-raised', vars.surfaceRaised)
}
