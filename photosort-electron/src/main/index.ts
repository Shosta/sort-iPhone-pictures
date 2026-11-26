import { app, BrowserWindow, dialog, ipcMain, IpcMainEvent, IpcMainInvokeEvent, Menu, MenuItemConstructorOptions, safeStorage, session, shell } from 'electron'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { ChildProcessWithoutNullStreams, spawn } from 'node:child_process'
import { join } from 'node:path'
import { createInterface } from 'node:readline'
import { formatVersion } from './version'
import { type AssetsState, assetsComplete, ensureAssets } from './assets'
import {
  PathApprovals,
  type TrustedPage,
  checkOpenablePath,
  dialogTitle,
  isAllowedExternalUrl,
  isSecretAccount,
  isSecretValue,
  isTrustedPage,
  requestId,
  validateSidecarRequest
} from './guards'

app.setName('Photosort')

let sidecar: ChildProcessWithoutNullStreams | null = null
let quitting = false

// Chemins annonces par le moteur (dossier de destination, PDF generes) : seuls
// ceux-la peuvent etre ouverts a la demande du renderer (voir shell:reveal).
const approvals = new PathApprovals()

// Page du renderer : la seule autorisee a parler au processus principal.
// L'URL du serveur de developpement n'est jamais prise en compte dans une app empaquetee.
function trustedPage(): TrustedPage {
  return {
    file: join(__dirname, '../renderer/index.html'),
    devUrl: app.isPackaged ? undefined : process.env.ELECTRON_RENDERER_URL
  }
}

function fromApp(event: IpcMainEvent | IpcMainInvokeEvent): boolean {
  const frame = event.senderFrame
  const trusted = !!frame && frame === event.sender.mainFrame && isTrustedPage(frame.url, trustedPage())
  if (!trusted) console.warn('[security] message IPC refuse (expediteur non fiable):', frame?.url)
  return trusted
}

function onApp<A extends unknown[]>(channel: string, listener: (event: IpcMainEvent, ...args: A) => void): void {
  ipcMain.on(channel, (event, ...args) => {
    if (fromApp(event)) listener(event, ...(args as A))
  })
}

function handleApp<A extends unknown[], R>(channel: string, listener: (event: IpcMainInvokeEvent, ...args: A) => R | Promise<R>): void {
  ipcMain.handle(channel, (event, ...args) => {
    if (!fromApp(event)) throw new Error('Expediteur IPC non autorise')
    return listener(event, ...(args as A))
  })
}

// Seules les URL https:// partent vers le navigateur ; tout le reste est ignore.
function openExternalSafely(url: string): void {
  if (isAllowedExternalUrl(url)) void shell.openExternal(url)
  else console.warn('[security] ouverture externe refusee:', String(url).slice(0, 200))
}

function sidecarPath(): string {
  const name = process.platform === 'win32' ? 'photosort-sidecar.exe' : 'photosort-sidecar'
  return app.isPackaged
    ? join(process.resourcesPath, 'bin', name)
    : join(app.getAppPath(), 'resources', 'bin', name)
}

function bundledAiDir(): string {
  return app.isPackaged ? join(process.resourcesPath, 'ai') : join(app.getAppPath(), 'ai-assets')
}

// Models bundled with the app win; otherwise they live in (and are downloaded to) userData.
function aiDir(): string {
  return assetsComplete(bundledAiDir()) ? bundledAiDir() : join(app.getPath('userData'), 'ai')
}

let assets: AssetsState = { state: 'checking' }
let provisioning = false

function setAssets(next: AssetsState): void {
  assets = next
  broadcast('assets:state', assets)
}

async function provisionAssets(): Promise<void> {
  if (provisioning) return
  if (assetsComplete(bundledAiDir())) return setAssets({ state: 'ready' })
  provisioning = true
  try {
    await ensureAssets(join(app.getPath('userData'), 'ai'), setAssets)
  } finally {
    provisioning = false
  }
}

function broadcast(channel: string, payload: unknown): void {
  for (const win of BrowserWindow.getAllWindows()) win.webContents.send(channel, payload)
}

function startSidecar(): void {
  sidecar = spawn(sidecarPath(), [], {
    stdio: ['pipe', 'pipe', 'pipe'],
    windowsHide: true,
    env: { ...process.env, PHOTOSORT_AI_DIR: aiDir() }
  })
  createInterface({ input: sidecar.stdout }).on('line', (line) => {
    try {
      const event = JSON.parse(line)
      if (event?.type === 'plan') approvals.remember(event.plan?.destination)
      else if (event?.type === 'done') approvals.remember(event.path)
      broadcast('sidecar:event', event)
    } catch {
      /* ligne non JSON ignoree */
    }
  })
  sidecar.stderr.on('data', (chunk) => console.error('[sidecar]', String(chunk)))
  sidecar.stdin.on('error', () => undefined)
  sidecar.on('exit', (code) => {
    sidecar = null
    if (!quitting) broadcast('sidecar:event', { id: '', type: 'error', message: `Moteur arrete (code ${code})` })
  })
}

function createWindow(): void {
  const win = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 960,
    minHeight: 640,
    backgroundColor: '#0b0d14',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webviewTag: false
    }
  })
  win.webContents.setWindowOpenHandler(({ url }) => {
    openExternalSafely(url)
    return { action: 'deny' }
  })
  const page = trustedPage()
  if (page.devUrl) win.loadURL(page.devUrl)
  else win.loadFile(page.file)
}

// Durcissement de tout contenu web : pas de navigation hors de l'application,
// pas de <webview>, aucune permission Chromium (camera, geolocalisation, ...).
app.on('web-contents-created', (_event, contents) => {
  const leave = (event: { preventDefault: () => void }, url: string): void => {
    if (isTrustedPage(url, trustedPage())) return
    event.preventDefault()
    openExternalSafely(url)
  }
  contents.on('will-navigate', leave)
  contents.on('will-redirect', leave)
  contents.on('will-attach-webview', (event) => event.preventDefault())
})

onApp('sidecar:send', (event, request: unknown) => {
  const verdict = validateSidecarRequest(request)
  if (!verdict.ok) {
    console.warn('[security] requete moteur refusee:', verdict.reason)
    event.sender.send('sidecar:event', { id: requestId(request), type: 'error', code: 'internal', message: 'internal error' })
    return
  }
  sidecar?.stdin.write(verdict.line + '\n')
})

handleApp('dialog:pickFolder', async (_event, title: unknown) => {
  const result = await dialog.showOpenDialog({
    title: dialogTitle(title),
    properties: ['openDirectory', 'multiSelections', 'createDirectory']
  })
  return result.canceled ? [] : result.filePaths
})

handleApp('dialog:pickFile', async (_event, title: unknown) => {
  const result = await dialog.showOpenDialog({ title: dialogTitle(title), properties: ['openFile', 'showHiddenFiles'] })
  return result.canceled ? '' : (result.filePaths[0] ?? '')
})

// Remote (NAS) passwords are stored encrypted with the OS keychain
// (Keychain, DPAPI, libsecret) through safeStorage, never in clear text.
const secretsFile = (): string => join(app.getPath('userData'), 'remote-secrets.json')

function readSecrets(): Record<string, string> {
  try {
    return existsSync(secretsFile()) ? JSON.parse(readFileSync(secretsFile(), 'utf8')) : {}
  } catch {
    return {}
  }
}

function writeSecrets(secrets: Record<string, string>): void {
  mkdirSync(app.getPath('userData'), { recursive: true })
  writeFileSync(secretsFile(), JSON.stringify(secrets), { mode: 0o600 })
}

function secretsAvailable(): boolean {
  if (!safeStorage.isEncryptionAvailable()) return false
  // Without a keyring Linux falls back to a hard-coded key: not a safe store.
  const backend = (safeStorage as unknown as { getSelectedStorageBackend?: () => string }).getSelectedStorageBackend
  return process.platform !== 'linux' || !backend || backend.call(safeStorage) !== 'basic_text'
}

handleApp('secret:available', () => secretsAvailable())

handleApp('secret:store', (_event, account: unknown, secret: unknown) => {
  if (!isSecretAccount(account) || !isSecretValue(secret) || !secretsAvailable()) return false
  writeSecrets({ ...readSecrets(), [account]: safeStorage.encryptString(secret).toString('base64') })
  return true
})

handleApp('secret:load', (_event, account: unknown) => {
  if (!isSecretAccount(account)) return ''
  const stored = readSecrets()[account]
  if (!stored || !secretsAvailable()) return ''
  try {
    return safeStorage.decryptString(Buffer.from(stored, 'base64'))
  } catch {
    return ''
  }
})

handleApp('secret:forget', (_event, account: unknown) => {
  if (!isSecretAccount(account)) return
  const secrets = readSecrets()
  delete secrets[account]
  writeSecrets(secrets)
})

// Ouvre le dossier de destination ou un PDF genere, et rien d'autre : jamais un
// chemin arbitraire, une application, un script ou un executable.
handleApp('shell:reveal', async (_event, path: unknown) => {
  const verdict = checkOpenablePath(path, approvals)
  if (!verdict.ok) {
    console.warn('[security] ouverture de chemin refusee:', verdict.reason)
    return `Refus: ${verdict.reason}`
  }
  return shell.openPath(verdict.path)
})

handleApp('assets:get', () => assets)
handleApp('assets:retry', () => provisionAssets())

const appCommit = typeof __PHOTOSORT_COMMIT__ === 'string' ? __PHOTOSORT_COMMIT__ : ''
handleApp('app:version', () => ({ version: app.getVersion(), commit: appCommit, display: formatVersion(app.getVersion(), appCommit) }))
handleApp('app:downloadsDir', () => app.getPath('downloads'))

interface MenuLabels {
  preferences: string
  tags: string
  scrapbook: string
}

function buildMenu(labels: MenuLabels): void {
  const mac = process.platform === 'darwin'
  const preferences: MenuItemConstructorOptions = {
    label: labels.preferences,
    accelerator: 'CmdOrCtrl+,',
    click: () => BrowserWindow.getAllWindows()[0]?.webContents.send('menu:preferences')
  }
  const tags: MenuItemConstructorOptions = {
    label: labels.tags,
    accelerator: 'CmdOrCtrl+T',
    click: () => BrowserWindow.getAllWindows()[0]?.webContents.send('menu:tags')
  }
  const scrapbook: MenuItemConstructorOptions = {
    label: labels.scrapbook,
    accelerator: 'CmdOrCtrl+P',
    click: () => BrowserWindow.getAllWindows()[0]?.webContents.send('menu:scrapbook')
  }
  const template: MenuItemConstructorOptions[] = [
    ...(mac
      ? [{ role: 'appMenu', submenu: [{ role: 'about' }, { type: 'separator' }, preferences, tags, scrapbook, { type: 'separator' }, { role: 'services' }, { type: 'separator' }, { role: 'hide' }, { role: 'hideOthers' }, { role: 'unhide' }, { type: 'separator' }, { role: 'quit' }] } as MenuItemConstructorOptions]
      : [{ role: 'fileMenu', submenu: [preferences, tags, scrapbook, { type: 'separator' }, { role: 'quit' }] } as MenuItemConstructorOptions]),
    { role: 'editMenu' },
    { role: 'viewMenu' },
    { role: 'windowMenu' }
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

onApp('menu:labels', (_event, labels: unknown) => {
  const record = (typeof labels === 'object' && labels !== null ? labels : {}) as Record<string, unknown>
  const label = (value: unknown, fallback: string): string => (typeof value === 'string' && value.length > 0 && value.length <= 100 ? value : fallback)
  buildMenu({ preferences: label(record.preferences, 'Preferences'), tags: label(record.tags, 'AI tags'), scrapbook: label(record.scrapbook, 'Planner') })
})

app.whenReady().then(async () => {
  app.setAboutPanelOptions({ applicationVersion: app.getVersion(), version: appCommit })
  session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
  session.defaultSession.setPermissionCheckHandler(() => false)
  buildMenu({ preferences: 'Preferences…', tags: 'AI tags…', scrapbook: 'Weekly planner sheet…' })
  createWindow()
  void provisionAssets()
  startSidecar()
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('before-quit', () => {
  quitting = true
  sidecar?.stdin.end()
  sidecar?.kill()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
