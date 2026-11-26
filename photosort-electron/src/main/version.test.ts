import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { formatVersion, isPrerelease } from './version.ts'

describe('formatVersion', () => {
  it('appends the short commit when known', () => {
    assert.equal(formatVersion('1.0.0', 'b89b82e'), '1.0.0 (b89b82e)')
    assert.equal(formatVersion('1.0.0-rc.1', 'b89b82e'), '1.0.0-rc.1 (b89b82e)')
  })
  it('falls back to the bare version without commit', () => {
    assert.equal(formatVersion('1.0.0', ''), '1.0.0')
  })
})

describe('isPrerelease', () => {
  it('detects a SemVer pre-release suffix', () => {
    assert.equal(isPrerelease('1.0.0-rc.1'), true)
    assert.equal(isPrerelease('1.0.0'), false)
  })
})
