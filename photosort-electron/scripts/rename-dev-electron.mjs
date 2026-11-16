// On macOS the menu bar title comes from the bundle's Info.plist. In dev the
// bundle is node_modules' Electron.app, so give it the app name (no admin needed).
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const name = 'Photosort'
const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const plist = join(root, 'node_modules/electron/dist/Electron.app/Contents/Info.plist')

if (process.platform === 'darwin' && existsSync(plist)) {
  const before = readFileSync(plist, 'utf8')
  const after = before.replace(/(<key>(?:CFBundleName|CFBundleDisplayName)<\/key>\s*<string>)[^<]*(<\/string>)/g, `$1${name}$2`)
  if (after !== before) writeFileSync(plist, after)
}
