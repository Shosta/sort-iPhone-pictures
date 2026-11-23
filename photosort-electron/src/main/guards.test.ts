import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { after, describe, it } from 'node:test'
import {
  MAX_STRING,
  PathApprovals,
  checkOpenablePath,
  isAllowedExternalUrl,
  isSecretAccount,
  isTrustedPage,
  validateSidecarRequest
} from './guards.ts'

describe('validateSidecarRequest', () => {
  it('accepts the requests the renderer really sends', () => {
    const plan = validateSidecarRequest({
      id: '7',
      op: 'plan',
      sources: ['/photos/a', '/photos/b'],
      destination: '/out',
      remote: undefined,
      byLocation: true,
      dedupe: false
    })
    assert.equal(plan.ok, true)
    if (plan.ok) assert.deepEqual(JSON.parse(plan.line), { id: '7', op: 'plan', sources: ['/photos/a', '/photos/b'], destination: '/out', byLocation: true, dedupe: false })

    for (const request of [
      { id: '1', op: 'thumbnail', path: '/p/a.jpg', size: 256 },
      { id: '2', op: 'cancel', target: '1' },
      { id: '3', op: 'copy', planId: 'p1', remote: { protocol: 'ssh', host: 'nas', port: 22, user: 'me', module: '', path: '/photos', password: 'x', keyFile: '', checksum: true }, dryRun: true },
      { id: '4', op: 'travelLayout', steps: [{ lat: 1.5, lon: 2.5, label: 'Paris', nested: { a: [1, 2] } }], page: 'a4', provider: 'x', lang: 'fr' }
    ]) {
      assert.equal(validateSidecarRequest(request).ok, true, JSON.stringify(request))
    }
  })

  it('refuses unknown operations and malformed envelopes', () => {
    for (const request of [null, 'plan', [], { op: 'plan' }, { id: 'x', op: 'plan' }, { id: '1', op: 'exec' }, { id: '1', op: '__proto__' }, { id: '1' }, { id: '1', op: 42 }]) {
      assert.equal(validateSidecarRequest(request).ok, false, JSON.stringify(request))
    }
  })

  it('refuses fields the renderer must not control', () => {
    assert.equal(validateSidecarRequest({ id: '1', op: 'tag', aiDir: '/tmp/evil' }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'plan', anything: 1 }).ok, false)
    assert.equal(validateSidecarRequest(JSON.parse('{"id":"1","op":"plan","__proto__":{"x":1}}')).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'plan', remote: { host: 'h', command: 'rm -rf /' } }).ok, false)
  })

  it('enforces types and bounds', () => {
    assert.equal(validateSidecarRequest({ id: '1', op: 'plan', sources: '/a' }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'plan', sources: [1] }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'plan', byLocation: 'yes' }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'thumbnail', size: Number.NaN }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'thumbnail', size: 1e12 }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'thumbnail', path: 'a\0b' }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'thumbnail', path: 'a'.repeat(MAX_STRING + 1) }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'travelLayout', steps: [{ f() {} }] }).ok, false)
    assert.equal(validateSidecarRequest({ id: '1', op: 'travelLayout', steps: ['x'] }).ok, false)
  })
})

describe('isAllowedExternalUrl', () => {
  it('only lets plain https URLs through', () => {
    assert.equal(isAllowedExternalUrl('https://github.com/remi/photosort'), true)
    for (const url of ['file:///Applications/Calculator.app', 'http://example.com', 'javascript:alert(1)', 'smb://nas/share', 'ms-msdt:/id', 'vscode://x', 'https://user:pw@example.com', 'not a url', '']) {
      assert.equal(isAllowedExternalUrl(url), false, url)
    }
  })
})

describe('isTrustedPage', () => {
  const file = join(tmpdir(), 'app', 'out', 'renderer', 'index.html')
  it('accepts only the packaged renderer page', () => {
    assert.equal(isTrustedPage(pathToFileURL(file).href, { file }), true)
    assert.equal(isTrustedPage(pathToFileURL(file).href + '#/x', { file }), true)
    assert.equal(isTrustedPage('https://evil.example/', { file }), false)
    assert.equal(isTrustedPage('file:///etc/passwd', { file }), false)
    assert.equal(isTrustedPage('file://evil-host/share/index.html', { file }), false)
    assert.equal(isTrustedPage('', { file }), false)
    assert.equal(isTrustedPage(undefined, { file }), false)
  })
  it('uses the dev server origin only when one is configured', () => {
    const page = { file, devUrl: 'http://localhost:5173/' }
    assert.equal(isTrustedPage('http://localhost:5173/index.html', page), true)
    assert.equal(isTrustedPage('http://localhost:6000/', page), false)
    assert.equal(isTrustedPage(pathToFileURL(file).href, page), false)
  })
})

describe('checkOpenablePath', () => {
  const root = mkdtempSync(join(tmpdir(), 'photosort-guards-'))
  const folder = join(root, 'Sorted')
  const bundle = join(root, 'Evil.app')
  const pdf = join(root, 'planner.pdf')
  const script = join(root, 'run.command')
  const link = join(root, 'link-to-app')
  mkdirSync(folder)
  mkdirSync(join(bundle, 'Contents'), { recursive: true })
  writeFileSync(pdf, '%PDF-1.4')
  writeFileSync(script, '#!/bin/sh\n', { mode: 0o755 })
  symlinkSync(bundle, link)
  const approvals = new PathApprovals()
  for (const path of [folder, bundle, pdf, script, link]) approvals.remember(path)

  it('opens an announced folder and an announced PDF', () => {
    assert.equal(checkOpenablePath(folder, approvals).ok, true)
    assert.equal(checkOpenablePath(pdf, approvals).ok, true)
  })

  it('refuses paths the engine never announced', () => {
    assert.equal(checkOpenablePath(root, approvals).ok, false)
    assert.equal(checkOpenablePath('/System/Applications/Calculator.app', approvals).ok, false)
  })

  it('refuses applications, bundles, scripts and symlinks to them', () => {
    assert.equal(checkOpenablePath(bundle, approvals).ok, false)
    assert.equal(checkOpenablePath(link, approvals).ok, false)
    assert.equal(checkOpenablePath(script, approvals).ok, false)
  })

  it('accepts an alias of an announced folder but not a symlink leading elsewhere', () => {
    const alias = join(root, 'alias-to-folder')
    symlinkSync(folder, alias)
    assert.equal(checkOpenablePath(alias, approvals).ok, true)
    const escape = join(folder, 'escape')
    symlinkSync(tmpdir(), escape)
    assert.equal(checkOpenablePath(escape, approvals).ok, false)
  })

  it('refuses malformed values', () => {
    for (const value of [undefined, 42, '', 'relative/path', `${folder}\0.pdf`, join(root, 'missing')]) {
      assert.equal(checkOpenablePath(value, approvals).ok, false, String(value))
    }
  })

  after(() => rmSync(root, { recursive: true, force: true }))
})

describe('isSecretAccount', () => {
  it('accepts user@host:module and rejects dangerous keys', () => {
    assert.equal(isSecretAccount('remi@nas.local:photos'), true)
    for (const value of ['', '__proto__', 'constructor', 'a\nb', 'x'.repeat(257), 42, null]) assert.equal(isSecretAccount(value), false, String(value))
  })
})
