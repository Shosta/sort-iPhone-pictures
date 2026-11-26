import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { compareSemver, isPrerelease, parseCommits, parseSemver, previousTag, renderSection, setPackageVersion, updateChangelog } from './release-lib.mjs'

describe('parseSemver', () => {
  it('accepts stable and pre-release versions', () => {
    assert.deepEqual(parseSemver('1.0.0'), { major: 1, minor: 0, patch: 0, pre: [] })
    assert.deepEqual(parseSemver('1.0.0-rc.1').pre, ['rc', 1])
  })
  it('rejects non-SemVer input', () => {
    for (const bad of ['v1.0.0', '1.0', '01.0.0', '1.0.0-', '1.0.0+abc', '1.0.0-rc.01', '1.0.0 (b89b82e)', '']) assert.equal(parseSemver(bad), null, bad)
  })
})

describe('compareSemver', () => {
  it('orders per SemVer 2.0.0', () => {
    const ordered = ['0.1.0', '1.0.0-alpha', '1.0.0-rc.1', '1.0.0-rc.2', '1.0.0-rc.10', '1.0.0', '1.0.1', '1.1.0', '2.0.0']
    for (let i = 1; i < ordered.length; i++) assert.equal(compareSemver(ordered[i - 1], ordered[i]), -1, ordered[i])
    assert.equal(compareSemver('1.0.0-rc.1', '1.0.0-rc.1'), 0)
  })
  it('throws on invalid versions', () => assert.throws(() => compareSemver('x', '1.0.0')))
  it('flags pre-releases', () => {
    assert.equal(isPrerelease('1.0.0-rc.1'), true)
    assert.equal(isPrerelease('1.0.0'), false)
  })
})

describe('previousTag', () => {
  const tags = ['v0.9.0', 'v1.0.0-rc.1', 'v1.0.0-rc.2', 'not-a-tag', 'v1.0.0']
  it('uses the last lower tag for a pre-release', () => {
    assert.equal(previousTag(tags, '1.0.0-rc.3'), 'v1.0.0-rc.2')
    assert.equal(previousTag(tags, '1.0.0-rc.1'), 'v0.9.0')
  })
  it('skips pre-releases for a stable version', () => {
    assert.equal(previousTag(['v0.9.0', 'v1.0.0-rc.1', 'v1.0.0-rc.2'], '1.0.0'), 'v0.9.0')
  })
  it('returns null without earlier tag', () => assert.equal(previousTag([], '1.0.0-rc.1'), null))
})

describe('parseCommits / changelog', () => {
  const subjects = ['feat: tri NAS', 'fix(sidecar): date manquante', 'feat!: nouveau format', 'website: texte', 'docs: readme', 'Fix: majuscule ignorée']
  it('keeps only feat and fix', () => {
    assert.deepEqual(parseCommits(subjects).map((e) => [e.type, e.scope, e.breaking]), [['feat', '', false], ['fix', 'sidecar', false], ['feat', '', true]])
  })
  it('renders sections', () => {
    const out = renderSection('1.0.0-rc.1', '2026-11-26', parseCommits(subjects))
    assert.match(out, /^## 1\.0\.0-rc\.1 - 2026-11-26\n\n### Nouveautés\n\n- tri NAS\n- nouveau format \(changement incompatible\)\n\n### Corrections\n\n- \*\*sidecar\*\* : date manquante\n$/)
    assert.match(renderSection('1.0.0', '2026-01-01', []), /Aucun changement/)
  })
  it('prepends a section and refuses duplicates', () => {
    const first = updateChangelog('', '1.0.0-rc.1', '2026-11-26', parseCommits(['feat: a']))
    assert.match(first, /^# Changelog/)
    const second = updateChangelog(first, '1.0.0-rc.2', '2026-11-27', parseCommits(['fix: b']))
    assert.ok(second.indexOf('rc.2 -') < second.indexOf('rc.1 -'))
    assert.throws(() => updateChangelog(second, '1.0.0-rc.1', '2026-11-28', []), /déjà/)
  })
})

describe('setPackageVersion', () => {
  it('updates package.json and lockfile root entries only', () => {
    const lock = JSON.stringify({ name: 'x', version: '0.1.0', packages: { '': { name: 'x', version: '0.1.0' }, 'node_modules/y': { version: '0.1.0' } } })
    const data = JSON.parse(setPackageVersion(lock, '1.0.0-rc.1'))
    assert.equal(data.version, '1.0.0-rc.1')
    assert.equal(data.packages[''].version, '1.0.0-rc.1')
    assert.equal(data.packages['node_modules/y'].version, '0.1.0')
    assert.ok(setPackageVersion('{"version":"1.0.0"}', '1.0.1').endsWith('}\n'))
  })
})
