// Garde-fou `npm audit` pour la CI.
//
//   node scripts/audit-gate.mjs [--level=moderate]
//
// `npm audit --omit=dev` ne voit pas Electron : electron-builder impose qu'il soit une devDependency.
// Ce script bloque donc sur (1) toute vulnerabilite des dependances de production et (2) toute
// vulnerabilite du paquet `electron` lui-meme (le runtime embarque), au niveau demande ou au-dessus.
// Les autres avis (electron-builder, vite, electron-vite... : outillage de build qui n'est pas livre)
// sont listes en avertissement sans bloquer.
//
// Echec ferme : si `npm audit` ne produit pas de rapport exploitable (registre injoignable...),
// le script echoue plutot que de laisser passer silencieusement.
import { spawnSync } from 'node:child_process'
import { pathToFileURL } from 'node:url'

export const LEVELS = ['info', 'low', 'moderate', 'high', 'critical']
export const DEFAULT_LEVEL = 'moderate'
const SHIPPED_RUNTIME = ['electron']

const rank = (severity) => LEVELS.indexOf(severity)

function vulnerabilitiesOf(report, label) {
  if (!report || typeof report !== 'object' || report.error || typeof report.vulnerabilities !== 'object') {
    const detail = report?.error?.summary ?? report?.error?.code ?? 'rapport invalide'
    throw new Error(`npm audit (${label}) inutilisable : ${detail}`)
  }
  return Object.entries(report.vulnerabilities).map(([name, v]) => ({ name, severity: v.severity }))
}

// Pure : retourne { blocking, warnings } a partir des deux rapports `npm audit --json`.
export function evaluate({ production, full }, level = DEFAULT_LEVEL) {
  if (!LEVELS.includes(level)) throw new Error(`niveau inconnu : ${level} (attendu : ${LEVELS.join(', ')})`)
  const threshold = rank(level)
  const prod = new Map(vulnerabilitiesOf(production, 'production').map((v) => [v.name, v]))
  const everything = vulnerabilitiesOf(full, 'complet')
  for (const v of prod.values()) if (!everything.some((e) => e.name === v.name)) everything.push(v)

  const blocking = []
  const warnings = []
  for (const v of everything) {
    const scope = prod.has(v.name) ? 'production' : SHIPPED_RUNTIME.includes(v.name) ? 'runtime embarque' : null
    if (scope && rank(v.severity) >= threshold) blocking.push({ ...v, scope })
    else warnings.push(v)
  }
  return { blocking, warnings }
}

function npmAudit(args) {
  const run = spawnSync('npm', ['audit', '--json', ...args], { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024, shell: process.platform === 'win32' })
  if (run.error) throw run.error
  try {
    return JSON.parse(run.stdout)
  } catch {
    throw new Error(`npm audit ${args.join(' ')} : sortie non JSON (${(run.stderr || run.stdout).slice(0, 200)})`)
  }
}

function main() {
  const flag = process.argv.slice(2).find((a) => a.startsWith('--level='))
  const level = flag ? flag.slice('--level='.length) : (process.env.AUDIT_LEVEL ?? DEFAULT_LEVEL)
  const { blocking, warnings } = evaluate({ production: npmAudit(['--omit=dev']), full: npmAudit([]) }, level)

  if (warnings.length > 0) {
    const names = warnings.map((v) => `${v.name} (${v.severity})`).join(', ')
    console.log(`::warning title=Avis npm audit hors production::${warnings.length} avis d'outillage de build (non livre, non bloquant) : ${names}`)
  }
  if (blocking.length > 0) {
    for (const v of blocking) console.error(`::error title=npm audit::${v.name} : ${v.severity} (${v.scope})`)
    console.error(`ECHEC : ${blocking.length} vulnerabilite(s) >= ${level} dans ce qui est livre. Lancez \`npm audit\` pour le detail.`)
    process.exit(1)
  }
  console.log(`OK : aucune vulnerabilite >= ${level} dans les dependances de production ni dans Electron.`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    main()
  } catch (error) {
    console.error(`ECHEC : ${error.message}`)
    process.exit(1)
  }
}
