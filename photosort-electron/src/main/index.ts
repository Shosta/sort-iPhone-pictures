import { app, BrowserWindow, dialog, ipcMain, Menu, MenuItemConstructorOptions, shell } from 'electron'
import { ChildProcessWithoutNullStreams, spawn } from 'node:child_process'
import { join } from 'node:path'
import { createInterface } from 'node:readline'

app.setName('Photosort')

let sidecar: ChildProcessWithoutNullStreams | null = null
let quitting = false

function sidecarPath(): string {
  const name = process.platform === 'win32' ? 'photosort-sidecar.exe' : 'photosort-sidecar'
  return app.isPackaged
    ? join(process.resourcesPath, 'bin', name)
    : join(app.getAppPath(), 'resources', 'bin', name)
}

function broadcast(channel: string, payload: unknown): void {
  for (const win of BrowserWindow.getAllWindows()) win.webContents.send(channel, payload)
}

function startSidecar(): void {
  sidecar = spawn(sidecarPath(), [], { stdio: ['pipe', 'pipe', 'pipe'] })
  createInterface({ input: sidecar.stdout }).on('line', (line) => {
    try {
      broadcast('sidecar:event', JSON.parse(line))
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
      sandbox: false
    }
  })
  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })
  if (process.env.ELECTRON_RENDERER_URL) win.loadURL(process.env.ELECTRON_RENDERER_URL)
  else win.loadFile(join(__dirname, '../renderer/index.html'))
}

ipcMain.on('sidecar:send', (_event, request: unknown) => {
  sidecar?.stdin.write(JSON.stringify(request) + '\n')
})

ipcMain.handle('dialog:pickFolder', async (_event, title: string) => {
  const result = await dialog.showOpenDialog({
    title,
    properties: ['openDirectory', 'multiSelections', 'createDirectory']
  })
  return result.canceled ? [] : result.filePaths
})

ipcMain.handle('shell:reveal', (_event, path: string) => shell.openPath(path))

function buildMenu(label: string): void {
  const mac = process.platform === 'darwin'
  const preferences: MenuItemConstructorOptions = {
    label,
    accelerator: 'CmdOrCtrl+,',
    click: () => BrowserWindow.getAllWindows()[0]?.webContents.send('menu:preferences')
  }
  const template: MenuItemConstructorOptions[] = [
    ...(mac
      ? [{ role: 'appMenu', submenu: [{ role: 'about' }, { type: 'separator' }, preferences, { type: 'separator' }, { role: 'services' }, { type: 'separator' }, { role: 'hide' }, { role: 'hideOthers' }, { role: 'unhide' }, { type: 'separator' }, { role: 'quit' }] } as MenuItemConstructorOptions]
      : [{ role: 'fileMenu', submenu: [preferences, { type: 'separator' }, { role: 'quit' }] } as MenuItemConstructorOptions]),
    { role: 'editMenu' },
    { role: 'viewMenu' },
    { role: 'windowMenu' }
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

ipcMain.on('menu:label', (_event, label: string) => buildMenu(label))

app.whenReady().then(() => {
  buildMenu('Preferences…')
  startSidecar()
  createWindow()
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
