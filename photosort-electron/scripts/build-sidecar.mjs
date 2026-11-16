// Compile le moteur Go (sidecar) pour la plateforme cible.
// Variables optionnelles : GOOS, GOARCH (compilation croisee, sans CGO).
import { execFileSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..')
const goos = process.env.GOOS ?? { darwin: 'darwin', win32: 'windows', linux: 'linux' }[process.platform]
const out = resolve(root, 'photosort-electron', 'resources', 'bin')
mkdirSync(out, { recursive: true })
const file = resolve(out, goos === 'windows' ? 'photosort-sidecar.exe' : 'photosort-sidecar')
execFileSync('go', ['build', '-trimpath', '-ldflags', '-s -w', '-o', file, './cmd/photosort-sidecar'], {
  cwd: root,
  stdio: 'inherit',
  env: { ...process.env, CGO_ENABLED: '0', GOOS: goos }
})
console.log('sidecar ->', file)
