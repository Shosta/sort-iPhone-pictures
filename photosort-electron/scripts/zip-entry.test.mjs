import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { readZipEntry } from './zip-entry.mjs'

test('lit une entree stockee ou compressee et renvoie null si absente', () => {
  const dir = mkdtempSync(join(tmpdir(), 'zip-'))
  try {
    mkdirSync(join(dir, 'a/b'), { recursive: true })
    const big = Buffer.alloc(100000, 'photosort')
    writeFileSync(join(dir, 'a/b/big.bin'), big)
    writeFileSync(join(dir, 'a/small.txt'), 'x')
    execFileSync('zip', ['-q', '-r', join(dir, 't.zip'), 'a'], { cwd: dir })
    const zip = readFileSync(join(dir, 't.zip'))
    assert.deepEqual(readZipEntry(zip, 'a/b/big.bin'), big)
    assert.equal(readZipEntry(zip, 'a/small.txt').toString(), 'x')
    assert.equal(readZipEntry(zip, 'a/none'), null)
    assert.throws(() => readZipEntry(Buffer.from('pas un zip, pas du tout un zip'), 'a'), /invalide/)
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})
