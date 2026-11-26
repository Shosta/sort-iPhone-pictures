// Logique pure du script de release (testée dans release.test.mjs) : SemVer, choix du tag précédent, CHANGELOG.
const IDENT = '(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
const SEMVER = new RegExp(`^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(?:-(${IDENT}(?:\\.${IDENT})*))?$`)

// « 1.0.0-rc.1 » -> { major, minor, patch, pre: ['rc', 1] } ; null si invalide (pas de préfixe v, pas de métadonnée +build).
export function parseSemver(text) {
  const m = SEMVER.exec(String(text))
  if (!m) return null
  const pre = m[4] === undefined ? [] : m[4].split('.').map((id) => (/^[0-9]+$/.test(id) ? Number(id) : id))
  return { major: Number(m[1]), minor: Number(m[2]), patch: Number(m[3]), pre }
}

export function isPrerelease(version) {
  return (parseSemver(version)?.pre.length ?? 0) > 0
}

// Ordre SemVer 2.0.0 : une préversion précède la version stable correspondante.
export function compareSemver(a, b) {
  const x = parseSemver(a)
  const y = parseSemver(b)
  if (!x || !y) throw new Error(`Version SemVer invalide : ${!x ? a : b}`)
  for (const key of ['major', 'minor', 'patch']) if (x[key] !== y[key]) return x[key] < y[key] ? -1 : 1
  if (!x.pre.length || !y.pre.length) return x.pre.length === y.pre.length ? 0 : x.pre.length ? -1 : 1
  for (let i = 0; i < Math.max(x.pre.length, y.pre.length); i++) {
    const p = x.pre[i]
    const q = y.pre[i]
    if (p === undefined) return -1
    if (q === undefined) return 1
    if (p === q) continue
    if (typeof p === 'number' && typeof q === 'number') return p < q ? -1 : 1
    if (typeof p === 'number') return -1
    if (typeof q === 'number') return 1
    return p < q ? -1 : 1
  }
  return 0
}

// Tag de départ du changelog : pour une préversion, le dernier tag inférieur (rc compris) ;
// pour une version stable, la dernière version stable inférieure (les rc sont regroupés dans la release finale).
export function previousTag(tags, version) {
  const stable = !isPrerelease(version)
  const candidates = tags
    .filter((tag) => tag.startsWith('v') && parseSemver(tag.slice(1)))
    .map((tag) => tag.slice(1))
    .filter((v) => compareSemver(v, version) < 0 && (!stable || !isPrerelease(v)))
    .sort(compareSemver)
  const last = candidates.at(-1)
  return last === undefined ? null : `v${last}`
}

const CONVENTIONAL = /^(feat|fix)(?:\(([^)]*)\))?(!)?:\s*(.+)$/

// Ne garde que les commits `feat:` / `fix:` (portée et `!` acceptés).
export function parseCommits(subjects) {
  const entries = []
  for (const subject of subjects) {
    const m = CONVENTIONAL.exec(subject.trim())
    if (m) entries.push({ type: m[1], scope: m[2] || '', breaking: Boolean(m[3]), text: m[4].trim() })
  }
  return entries
}

const line = (entry) => `- ${entry.scope ? `**${entry.scope}** : ` : ''}${entry.text}${entry.breaking ? ' (changement incompatible)' : ''}`

export function renderSection(version, date, entries) {
  const groups = [
    ['Nouveautés', entries.filter((e) => e.type === 'feat')],
    ['Corrections', entries.filter((e) => e.type === 'fix')]
  ]
  const body = groups.filter(([, items]) => items.length).map(([title, items]) => `### ${title}\n\n${items.map(line).join('\n')}`)
  return `## ${version} - ${date}\n\n${body.length ? body.join('\n\n') : '_Aucun changement `feat:` ou `fix:`._'}\n`
}

const HEADER = '# Changelog\n\nLes versions suivent [SemVer](https://semver.org/lang/fr/). Section générée par `npm run release`.\n'

// Insère la section en tête (sous le titre). Refuse d'écrire deux fois la même version.
export function updateChangelog(existing, version, date, entries) {
  const text = existing?.trim() ? existing : HEADER
  if (new RegExp(`^## ${version.replace(/[.+]/g, '\\$&')} - `, 'm').test(text)) {
    throw new Error(`CHANGELOG.md contient déjà la version ${version}`)
  }
  const section = renderSection(version, date, entries)
  const first = text.search(/^## /m)
  if (first === -1) return `${text.trimEnd()}\n\n${section}`
  return `${text.slice(0, first)}${section}\n${text.slice(first)}`
}

// Met à jour la version de package.json / package-lock.json en conservant la mise en forme (indentation de 2).
export function setPackageVersion(jsonText, version) {
  const data = JSON.parse(jsonText)
  data.version = version
  if (data.packages?.['']) data.packages[''].version = version
  return `${JSON.stringify(data, null, 2)}\n`
}
