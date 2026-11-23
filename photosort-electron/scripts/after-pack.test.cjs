// Teste le script de lancement Linux avec de faux executables (stat, unshare) et
// un faux /proc/sys, pour pouvoir le verifier hors de Linux.
const assert = require('node:assert/strict')
const { spawnSync } = require('node:child_process')
const { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } = require('node:fs')
const { tmpdir } = require('node:os')
const { join } = require('node:path')
const { after, describe, it } = require('node:test')
const { wrapperScript } = require('./after-pack.cjs')

const root = mkdtempSync(join(tmpdir(), 'photosort-wrapper-'))
const sysctl = join(root, 'sys')
const stubs = join(root, 'stubs')
const appDir = join(root, 'app')
mkdirSync(join(sysctl, 'kernel'), { recursive: true })
mkdirSync(join(sysctl, 'user'), { recursive: true })
mkdirSync(stubs)
mkdirSync(appDir)

const write = (file, text, mode = 0o755) => writeFileSync(file, text, { mode })
write(join(appDir, 'photosort'), wrapperScript('photosort', sysctl))
write(join(appDir, 'photosort.bin'), '#!/bin/sh\nprintf "RAN"; for a in "$@"; do printf "|%s" "$a"; done; echo\n')
write(join(appDir, 'chrome-sandbox'), '')
write(join(stubs, 'stat'), '#!/bin/sh\necho "${FAKE_SANDBOX_OWNER:-1000}"\n')
write(join(stubs, 'unshare'), '#!/bin/sh\n[ "$FAKE_USERNS" = "1" ]\n')

function launch({ setuidRoot = false, userns = false, sysctls = {}, env = {} } = {}) {
  chmodSync(join(appDir, 'chrome-sandbox'), setuidRoot ? 0o4755 : 0o755)
  for (const [file, value] of Object.entries({ 'kernel/unprivileged_userns_clone': '', 'user/max_user_namespaces': '', 'kernel/apparmor_restrict_unprivileged_userns': '', ...sysctls })) {
    write(join(sysctl, file), value === '' ? '' : `${value}\n`, 0o644)
  }
  return spawnSync(join(appDir, 'photosort'), ['--flag', 'a b'], {
    encoding: 'utf8',
    env: { PATH: `${stubs}:/usr/bin:/bin`, FAKE_SANDBOX_OWNER: setuidRoot ? '0' : '1000', FAKE_USERNS: userns ? '1' : '0', ...env }
  })
}

describe('wrapper de lancement Linux', { skip: process.platform === 'win32' }, () => {
  it('garde le sandbox setuid sans --no-sandbox', () => {
    const run = launch({ setuidRoot: true })
    assert.equal(run.status, 0)
    assert.equal(run.stdout.trim(), 'RAN|--flag|a b')
    assert.equal(run.stderr, '')
  })

  it('garde le sandbox par espaces de noms quand ils sont utilisables', () => {
    const run = launch({ userns: true })
    assert.equal(run.stdout.trim(), 'RAN|--flag|a b')
    assert.equal(run.stderr, '')
  })

  it('retombe sur --no-sandbox avec un avertissement quand aucun sandbox n est utilisable', () => {
    const run = launch({ userns: false })
    assert.equal(run.stdout.trim(), 'RAN|--no-sandbox|--flag|a b')
    assert.match(run.stderr, /AVERTISSEMENT/)
  })

  it('traite comme indisponibles les espaces de noms desactives ou restreints par AppArmor', () => {
    for (const sysctls of [{ 'kernel/unprivileged_userns_clone': '0' }, { 'user/max_user_namespaces': '0' }, { 'kernel/apparmor_restrict_unprivileged_userns': '1' }]) {
      const run = launch({ userns: true, sysctls })
      assert.equal(run.stdout.trim(), 'RAN|--no-sandbox|--flag|a b', JSON.stringify(sysctls))
    }
  })

  it('refuse de se lancer sans sandbox avec PHOTOSORT_REQUIRE_SANDBOX=1', () => {
    const run = launch({ userns: false, env: { PHOTOSORT_REQUIRE_SANDBOX: '1' } })
    assert.notEqual(run.status, 0)
    assert.equal(run.stdout, '')
    assert.match(run.stderr, /PHOTOSORT_REQUIRE_SANDBOX/)
  })

  it('se lance normalement avec PHOTOSORT_REQUIRE_SANDBOX=1 quand un sandbox est disponible', () => {
    const run = launch({ userns: true, env: { PHOTOSORT_REQUIRE_SANDBOX: '1' } })
    assert.equal(run.status, 0)
    assert.equal(run.stdout.trim(), 'RAN|--flag|a b')
  })

  it('ne considere pas setuid un chrome-sandbox qui n appartient pas a root', () => {
    chmodSync(join(appDir, 'chrome-sandbox'), 0o4755)
    const run = launch({ userns: false, env: { FAKE_SANDBOX_OWNER: '1000' } })
    assert.match(run.stdout, /--no-sandbox/)
  })
})

after(() => rmSync(root, { recursive: true, force: true }))
