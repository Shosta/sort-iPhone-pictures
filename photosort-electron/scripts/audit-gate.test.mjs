import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { evaluate } from './audit-gate.mjs'

const report = (vulns) => ({ vulnerabilities: Object.fromEntries(Object.entries(vulns).map(([name, severity]) => [name, { severity }])) })
const clean = report({})

describe('audit gate', () => {
  it('ignores build-tooling advisories but lists them as warnings', () => {
    const result = evaluate({ production: clean, full: report({ 'electron-builder': 'high', vite: 'critical' }) })
    assert.deepEqual(result.blocking, [])
    assert.deepEqual(result.warnings.map((v) => v.name).sort(), ['electron-builder', 'vite'])
  })

  it('blocks on a production vulnerability at the threshold', () => {
    const result = evaluate({ production: report({ react: 'moderate' }), full: report({ react: 'moderate' }) })
    assert.deepEqual(result.blocking.map((v) => `${v.name}:${v.scope}`), ['react:production'])
  })

  it('blocks on an Electron advisory even though it is a devDependency', () => {
    const result = evaluate({ production: clean, full: report({ electron: 'high', 'electron-builder': 'high' }) })
    assert.deepEqual(result.blocking.map((v) => v.name), ['electron'])
    assert.deepEqual(result.warnings.map((v) => v.name), ['electron-builder'])
  })

  it('respects the threshold level', () => {
    const input = { production: report({ react: 'low' }), full: report({ react: 'low' }) }
    assert.equal(evaluate(input, 'moderate').blocking.length, 0)
    assert.equal(evaluate(input, 'low').blocking.length, 1)
    assert.throws(() => evaluate(input, 'nope'), /niveau inconnu/)
  })

  it('fails closed on an unusable report', () => {
    assert.throws(() => evaluate({ production: { error: { code: 'ENOTFOUND' } }, full: clean }), /inutilisable : ENOTFOUND/)
    assert.throws(() => evaluate({ production: clean, full: null }), /inutilisable/)
  })
})
