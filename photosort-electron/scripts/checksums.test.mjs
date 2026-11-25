import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { after, describe, it } from 'node:test'
import { createHash } from 'node:crypto'
import { MANIFEST, listArtifacts, verifyManifest, writeManifest } from './checksums.mjs'

const dir = mkdtempSync(join(tmpdir(), 'photosort-sums-'))
const script = new URL('./checksums.mjs', import.meta.url).pathname
const files = { 'Photosort-0.1.0-mac-arm64.dmg': 'dmg-bytes', 'Photosort-0.1.0-win-x64.exe': 'exe-bytes', 'Photosort-0.1.0-linux-x86_64.AppImage': 'appimage-bytes' }
for (const [name, body] of Object.entries(files)) writeFileSync(join(dir, name), body)
writeFileSync(join(dir, 'builder-debug.yml'), 'ignored')
writeFileSync(join(dir, 'Photosort.dmg.blockmap'), 'ignored')

describe('checksums', () => {
  it('lists only installers, sorted', () => {
    assert.deepEqual(listArtifacts(dir), Object.keys(files).sort())
  })

  it('writes sha256sum-compatible lines', async () => {
    const lines = await writeManifest(dir)
    assert.equal(lines.length, 3)
    const expected = createHash('sha256').update(files['Photosort-0.1.0-mac-arm64.dmg']).digest('hex')
    assert.ok(readFileSync(join(dir, MANIFEST), 'utf8').includes(`${expected}  Photosort-0.1.0-mac-arm64.dmg\n`))
    assert.deepEqual(await verifyManifest(dir), [])
  })

  it('is accepted by the system shasum/sha256sum', { skip: process.platform === 'win32' }, () => {
    const candidates = [['shasum', ['-a', '256', '-c', MANIFEST]], ['sha256sum', ['-c', MANIFEST]]]
    const tool = candidates.map(([cmd, args]) => spawnSync(cmd, args, { cwd: dir, encoding: 'utf8' })).find((run) => !run.error)
    if (!tool) return // ni shasum ni sha256sum sur cette machine
    assert.equal(tool.status, 0, tool.stderr + tool.stdout)
  })

  it('detects a tampered, missing, unlisted or malformed entry', async () => {
    writeFileSync(join(dir, 'Photosort-0.1.0-win-x64.exe'), 'tampered')
    assert.match((await verifyManifest(dir)).join('\n'), /win-x64\.exe : empreinte differente/)
    rmSync(join(dir, 'Photosort-0.1.0-linux-x86_64.AppImage'))
    assert.match((await verifyManifest(dir)).join('\n'), /AppImage : fichier manquant/)
    writeFileSync(join(dir, 'extra.deb'), 'x')
    assert.match((await verifyManifest(dir)).join('\n'), /extra\.deb : absent de SHA256SUMS/)
    writeFileSync(join(dir, MANIFEST), `${'a'.repeat(64)}  ../../etc/passwd\n`)
    assert.match((await verifyManifest(dir)).join('\n'), /ligne invalide/)
  })

  it('CLI exits non-zero on mismatch and when there is nothing to hash', () => {
    assert.notEqual(spawnSync(process.execPath, [script, dir, '--check'], { encoding: 'utf8' }).status, 0)
    const empty = mkdtempSync(join(tmpdir(), 'photosort-sums-empty-'))
    assert.notEqual(spawnSync(process.execPath, [script, empty], { encoding: 'utf8' }).status, 0)
    rmSync(empty, { recursive: true, force: true })
  })
})

after(() => rmSync(dir, { recursive: true, force: true }))
