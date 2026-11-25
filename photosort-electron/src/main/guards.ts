// Validation des entrées du processus principal : tout ce qui vient du
// renderer (IPC) est traité comme non fiable. Module pur (sans import
// d'Electron) pour pouvoir être testé avec `node --test`.
import { realpathSync, statSync } from 'node:fs'
import { extname, isAbsolute, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export type Verdict<T> = ({ ok: true } & T) | { ok: false; reason: string }

// --- Requêtes vers le moteur Go (sidecar) ------------------------------------

// Opérations connues du moteur (cmd/photosort-sidecar/main.go, run + cancel).
export const SIDECAR_OPS: ReadonlySet<string> = new Set([
  'plan',
  'copy',
  'remoteTest',
  'remoteList',
  'thumbnail',
  'tag',
  'weekDays',
  'weekPhotos',
  'scrapbookLayout',
  'scrapbook',
  'yearMapYears',
  'yearMapScan',
  'yearMapLayout',
  'yearMapPdf',
  'travelTrips',
  'travelScan',
  'travelLayout',
  'travelPdf',
  'importDetect',
  'importScan',
  'importRun',
  'importControl',
  'importForget',
  'cancel'
])

type FieldKind = 'string' | 'strings' | 'number' | 'boolean' | 'records' | 'remote'

// Champs autorisés. `aiDir` en est volontairement absent : le dossier des
// modèles est fixé par le processus principal (PHOTOSORT_AI_DIR), jamais par le renderer.
const FIELDS: Readonly<Record<string, FieldKind>> = {
  sources: 'strings',
  destination: 'string',
  byLocation: 'boolean',
  landmarks: 'boolean',
  dedupe: 'boolean',
  similar: 'boolean',
  dateFallback: 'boolean',
  screenshots: 'boolean',
  blurry: 'boolean',
  tagScenes: 'boolean',
  tagMoment: 'boolean',
  tagPeople: 'boolean',
  tagMeta: 'boolean',
  folder: 'string',
  files: 'strings',
  planId: 'string',
  path: 'string',
  size: 'number',
  target: 'string',
  root: 'string',
  from: 'string',
  to: 'string',
  page: 'string',
  longSide: 'number',
  gutter: 'number',
  bleed: 'number',
  frame: 'string',
  layout: 'string',
  background: 'string',
  outDir: 'string',
  fallbackDir: 'string',
  name: 'string',
  year: 'number',
  provider: 'string',
  title: 'string',
  subtitle: 'string',
  places: 'records',
  steps: 'records',
  lang: 'string',
  remote: 'remote',
  remoteDir: 'string',
  remoteNoDedupe: 'boolean',
  remoteRefresh: 'boolean',
  dryRun: 'boolean',
  deviceId: 'string',
  devicePath: 'string',
  mode: 'string',
  action: 'string'
}

const REMOTE_FIELDS: Readonly<Record<string, 'string' | 'number' | 'boolean'>> = {
  protocol: 'string',
  host: 'string',
  port: 'number',
  user: 'string',
  module: 'string',
  path: 'string',
  password: 'string',
  keyFile: 'string',
  checksum: 'boolean'
}

export const MAX_STRING = 32768
export const MAX_LIST = 500_000
export const MAX_RECORDS = 20_000
export const MAX_REQUEST_BYTES = 32 << 20

const hasOwn = (target: object, key: string): boolean => Object.prototype.hasOwnProperty.call(target, key)
const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value) && [Object.prototype, null].includes(Object.getPrototypeOf(value))
const isText = (value: unknown): value is string => typeof value === 'string' && value.length <= MAX_STRING && !value.includes('\0')
const isNumber = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value) && Math.abs(value) <= 1e9

// JSON « plat » : objets/tableaux/chaînes/nombres/booléens à profondeur bornée.
function isPlainJson(value: unknown, depth = 0): boolean {
  if (depth > 6) return false
  if (value === null || typeof value === 'boolean') return true
  if (typeof value === 'string') return isText(value)
  if (typeof value === 'number') return isNumber(value)
  if (Array.isArray(value)) return value.length <= MAX_RECORDS && value.every((item) => isPlainJson(item, depth + 1))
  if (isRecord(value)) return Object.entries(value).every(([key, item]) => key !== '__proto__' && isPlainJson(item, depth + 1))
  return false
}

function checkRemote(value: unknown): boolean {
  if (!isRecord(value)) return false
  return Object.entries(value).every(([key, item]) => {
    if (item === undefined) return true
    const kind = hasOwn(REMOTE_FIELDS, key) ? REMOTE_FIELDS[key] : undefined
    if (kind === 'string') return isText(item)
    if (kind === 'number') return isNumber(item)
    if (kind === 'boolean') return typeof item === 'boolean'
    return false
  })
}

function checkField(kind: FieldKind, value: unknown): boolean {
  switch (kind) {
    case 'string':
      return isText(value)
    case 'number':
      return isNumber(value)
    case 'boolean':
      return typeof value === 'boolean'
    case 'strings':
      return Array.isArray(value) && value.length <= MAX_LIST && value.every(isText)
    case 'records':
      return Array.isArray(value) && value.length <= MAX_RECORDS && value.every((item) => isRecord(item) && isPlainJson(item))
    case 'remote':
      return checkRemote(value)
  }
}

/**
 * Valide une requête du renderer avant de l'écrire sur l'entrée du moteur :
 * opération connue, champs connus et typés, tailles bornées. Retourne la ligne
 * JSON à envoyer (réencodée à partir des seuls champs validés).
 */
export function validateSidecarRequest(raw: unknown): Verdict<{ line: string; id: string }> {
  if (!isRecord(raw)) return { ok: false, reason: 'requete invalide' }
  const { id, op } = raw
  if (typeof id !== 'string' || !/^\d{1,15}$/.test(id)) return { ok: false, reason: 'identifiant invalide' }
  if (typeof op !== 'string' || !SIDECAR_OPS.has(op)) return { ok: false, reason: 'operation inconnue' }
  const clean: Record<string, unknown> = { id, op }
  for (const [key, value] of Object.entries(raw)) {
    if (key === 'id' || key === 'op' || value === undefined || value === null) continue
    const kind = hasOwn(FIELDS, key) ? FIELDS[key] : undefined
    if (!kind) return { ok: false, reason: `champ non autorise: ${key.slice(0, 40)}` }
    if (!checkField(kind, value)) return { ok: false, reason: `valeur invalide: ${key}` }
    clean[key] = value
  }
  const line = JSON.stringify(clean)
  if (line.length > MAX_REQUEST_BYTES) return { ok: false, reason: 'requete trop volumineuse' }
  return { ok: true, line, id }
}

/** Identifiant utilisable pour répondre à une requête refusée ('' si absent ou invalide). */
export function requestId(raw: unknown): string {
  return isRecord(raw) && typeof raw.id === 'string' && /^\d{1,15}$/.test(raw.id) ? raw.id : ''
}

// --- URL externes et navigation ---------------------------------------------

/** Seules les URL https:// (sans identifiants) peuvent être ouvertes dans le navigateur. */
export function isAllowedExternalUrl(raw: string): boolean {
  if (typeof raw !== 'string' || raw.length > 2048) return false
  try {
    const url = new URL(raw)
    return url.protocol === 'https:' && url.hostname !== '' && url.username === '' && url.password === ''
  } catch {
    return false
  }
}

export interface TrustedPage {
  /** Origine du serveur de développement (jamais utilisée dans une app empaquetée). */
  devUrl?: string
  /** Fichier index.html du renderer empaqueté. */
  file: string
}

const sameFile = (a: string, b: string): boolean =>
  process.platform === 'win32' || process.platform === 'darwin' ? a.toLowerCase() === b.toLowerCase() : a === b

/** Vrai si `raw` est la page du renderer de l'application (et rien d'autre). */
export function isTrustedPage(raw: string | undefined | null, page: TrustedPage): boolean {
  if (!raw) return false
  try {
    const url = new URL(raw)
    if (page.devUrl) return url.origin === new URL(page.devUrl).origin
    return url.protocol === 'file:' && url.host === '' && sameFile(resolve(fileURLToPath(url)), resolve(page.file))
  } catch {
    return false
  }
}

// --- Ouverture de chemins (shell.openPath) -----------------------------------

// Dossiers que macOS traite comme des applications/paquets : les ouvrir les lance.
const BUNDLE_EXTENSIONS: ReadonlySet<string> = new Set([
  '.app', '.bundle', '.framework', '.plugin', '.kext', '.prefpane', '.saver', '.workflow', '.action',
  '.pkg', '.mpkg', '.xpc', '.appex', '.component', '.qlgenerator', '.mdimporter', '.wdgt', '.scptd', '.dext'
])

/** Chemins que le moteur a lui-même annoncés (dossier de destination, PDF générés). */
export class PathApprovals {
  private readonly paths = new Set<string>()
  private readonly limit = 1000

  remember(path: unknown): void {
    if (typeof path !== 'string' || path === '' || !isAbsolute(path) || path.includes('\0')) return
    this.add(resolve(path))
    const real = tryRealpath(path)
    if (real) this.add(real)
  }

  // Le moteur annonce souvent le chemin réel (/private/tmp) alors que l'interface
  // peut afficher l'alias (/tmp) : on accepte les deux, mais un lien symbolique
  // créé dans le dossier approuvé et pointant ailleurs reste refusé.
  has(path: string): boolean {
    if (this.paths.has(resolve(path))) return true
    const real = tryRealpath(path)
    return real !== null && this.paths.has(real)
  }

  private add(key: string): void {
    this.paths.delete(key)
    this.paths.add(key)
    if (this.paths.size > this.limit) this.paths.delete(this.paths.values().next().value as string)
  }
}

function tryRealpath(path: string): string | null {
  try {
    return realpathSync(path)
  } catch {
    return null
  }
}

/**
 * Autorise l'ouverture d'un chemin demandé par le renderer : il doit avoir été
 * annoncé par le moteur, exister, et être soit un dossier ordinaire, soit un PDF.
 * Les applications, paquets, scripts et exécutables sont refusés.
 */
export function checkOpenablePath(path: unknown, approvals: PathApprovals): Verdict<{ path: string }> {
  if (typeof path !== 'string' || path === '' || path.length > MAX_STRING || path.includes('\0')) return { ok: false, reason: 'chemin invalide' }
  if (!isAbsolute(path)) return { ok: false, reason: 'chemin non absolu' }
  if (!approvals.has(path)) return { ok: false, reason: 'chemin non produit par le moteur' }
  let real: string
  let stat: ReturnType<typeof statSync>
  try {
    real = realpathSync(path)
    stat = statSync(real)
  } catch {
    return { ok: false, reason: 'chemin introuvable' }
  }
  const extensions = [extname(path).toLowerCase(), extname(real).toLowerCase()]
  if (stat.isDirectory()) {
    if (extensions.some((ext) => BUNDLE_EXTENSIONS.has(ext))) return { ok: false, reason: 'dossier de type application refuse' }
    return { ok: true, path: real }
  }
  if (stat.isFile() && extensions.every((ext) => ext === '.pdf')) return { ok: true, path: real }
  return { ok: false, reason: 'type de fichier refuse' }
}

// --- Secrets et divers -------------------------------------------------------

const RESERVED_KEYS: ReadonlySet<string> = new Set(['__proto__', 'constructor', 'prototype'])

/** Identifiant d'un secret : `utilisateur@hote:module`, texte court sans caractère de contrôle. */
export function isSecretAccount(account: unknown): account is string {
  return typeof account === 'string' && account.length > 0 && account.length <= 256 && !/[\u0000-\u001f\u007f]/.test(account) && !RESERVED_KEYS.has(account)
}

export function isSecretValue(secret: unknown): secret is string {
  return typeof secret === 'string' && secret.length <= 4096 && !secret.includes('\0')
}

/** Titre de boîte de dialogue : texte court, sinon vide. */
export function dialogTitle(title: unknown): string {
  return typeof title === 'string' && !title.includes('\0') ? title.slice(0, 200) : ''
}
